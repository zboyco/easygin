package easygin

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"
)

// fieldCache 用于缓存结构体字段的标签信息
var fieldCache sync.Map

// bodyFieldInfo 存储字段的标签信息
type bodyFieldInfo struct {
	tagValue     string
	hasOmitempty bool
	defaultValue string
}

// decodeJSON 从请求中解析JSON数据并验证必填字段
func decodeJSON(r io.Reader, v interface{}) error {
	// 使用json.NewDecoder避免额外的内存分配
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(v); err != nil {
		return fmt.Errorf("parse json failed: %v", err)
	}

	if !HandleBodyJsonOmitEmptyAndDefault() {
		return nil
	}

	// 验证必填字段
	return ValidateJsonRequiredFields(reflect.ValueOf(v))
}

// handleEmptyValue 处理字段值为空的情况，检查omitempty和default标签
func handleEmptyValue(structPath string, field reflect.StructField, tagKey string) (bool, string) {
	// 尝试从缓存获取字段信息
	// 在缓存key中加入结构体类型信息，避免不同结构体中相同字段名导致的key重复
	cacheKey := structPath + "." + field.Name + ":tag:" + tagKey
	if cached, ok := fieldCache.Load(cacheKey); ok {
		info := cached.(bodyFieldInfo)
		return info.hasOmitempty || info.tagValue == "" || info.tagValue == "-", info.defaultValue
	}

	// 解析标签
	tag := field.Tag.Get(tagKey)
	tagParts := strings.Split(tag, ",")

	// 跳过未命名的参数
	if tagParts[0] == "" || tagParts[0] == "-" {
		// 缓存字段信息
		fieldCache.Store(cacheKey, bodyFieldInfo{tagValue: tagParts[0], hasOmitempty: true, defaultValue: ""})
		return true, ""
	}

	// 检查标签选项
	hasOmitempty := false
	if slices.Contains(tagParts[1:], "omitempty") {
		hasOmitempty = true
	}

	// 如果没有omitempty标签，则为必填字段
	if !hasOmitempty {
		// 缓存字段信息
		fieldCache.Store(cacheKey, bodyFieldInfo{tagValue: tagParts[0], hasOmitempty: false, defaultValue: ""})
		return false, ""
	}

	// 如果有omitempty标签，则为可选字段
	defaultValue := field.Tag.Get("default")
	// 缓存字段信息
	fieldCache.Store(cacheKey, bodyFieldInfo{tagValue: tagParts[0], hasOmitempty: true, defaultValue: defaultValue})

	return true, defaultValue
}

// isEmptyValue 判断字段值是否为空
func isEmptyValue(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}

	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		if v.IsNil() {
			return true
		}
		return isEmptyValue(v.Elem())
	case reflect.Struct:
		// 处理time.Time类型
		if v.Type().String() == "time.Time" {
			return v.Interface().(time.Time).IsZero()
		}
		// 检查结构体的所有字段是否为空
		for i := 0; i < v.NumField(); i++ {
			if !isEmptyValue(v.Field(i)) {
				return false
			}
		}
		return true
	}
	return false
}

// validateFieldsCache 用于缓存结构体字段的验证信息
var validateFieldsCache sync.Map

// validateFieldInfo 存储字段的验证信息
type validateFieldInfo struct {
	index        int
	canBeEmpty   bool
	defaultValue string
	jsonName     string
	isStruct     bool
}

// getValidateFields 获取并缓存结构体的验证字段信息
func getValidateFields(t reflect.Type) []validateFieldInfo {
	// 在缓存key中加入结构体类型信息，避免不同结构体中相同字段名导致的key重复
	structPath := t.PkgPath() + "." + t.Name()

	// 尝试从缓存获取
	if cached, ok := validateFieldsCache.Load(structPath); ok {
		return cached.([]validateFieldInfo)
	}

	// 缓存未命中，解析字段信息
	fields := make([]validateFieldInfo, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		canBeEmpty, defaultValue := handleEmptyValue(structPath, field, "json")
		jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
		fieldType := field.Type
		// 如果是指针类型，获取指针指向的类型
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}
		isStruct := fieldType.Kind() == reflect.Struct && fieldType.String() != "time.Time"

		fields = append(fields, validateFieldInfo{
			index:        i,
			canBeEmpty:   canBeEmpty,
			defaultValue: defaultValue,
			jsonName:     jsonName,
			isStruct:     isStruct,
		})
	}

	// 存入缓存
	validateFieldsCache.Store(structPath, fields)
	return fields
}

// validateRequiredFields 递归验证结构体的必填字段
func ValidateJsonRequiredFields(v reflect.Value) error {
	// 处理指针类型
	for {
		if v.Kind() == reflect.Ptr {
			if v.IsNil() {
				return nil
			}
			v = v.Elem()
		} else {
			break
		}
	}

	// 只处理结构体类型
	if v.Kind() != reflect.Struct {
		return nil
	}

	// 获取类型信息和缓存的字段信息
	t := v.Type()
	fields := getValidateFields(t)

	// 遍历字段信息
	for _, info := range fields {
		fieldValue := v.Field(info.index)

		// 处理结构体类型字段（包括指针类型的结构体）
		if info.isStruct {
			// 如果是指针类型
			if fieldValue.Kind() == reflect.Ptr {
				// 如果指针为nil且字段不能为空，返回错误
				if fieldValue.IsNil() {
					if !info.canBeEmpty {
						return fmt.Errorf("missing required field '%s' in body", info.jsonName)
					}
					// 如果字段可以为空，则跳过后续验证
					continue
				}
				// 如果指针不为nil，获取其指向的值
				fieldValue = fieldValue.Elem()
			}
			// 无论字段是否可以为空，只要不为nil就需要递归验证其内部字段
			if err := ValidateJsonRequiredFields(fieldValue); err != nil {
				return err
			}
			continue
		}

		// 如果字段值为空，检查omitempty和default标签
		if isEmptyValue(fieldValue) {
			if !info.canBeEmpty {
				return fmt.Errorf("missing required field '%s' in body", info.jsonName)
			}
			// 如果有默认值，设置默认值
			if info.defaultValue != "" {
				if err := setFieldValue(fieldValue, info.defaultValue, info.jsonName, fieldValue.Type()); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// handleFormFieldValue 统一处理表单字段值的验证和设置
func handleFormFieldValue(field reflect.StructField, fieldVal reflect.Value, value, name, structPath string) error {
	// 如果值不为空，先尝试设置字段值
	if value != "" {
		if err := setFieldValue(fieldVal, value, name, field.Type); err != nil {
			return err
		}
	}

	// 检查设置后的值是否为空
	if value == "" || isEmptyValue(fieldVal) {
		canBeEmpty, defaultValue := handleEmptyValue(structPath, field, "name")
		if !canBeEmpty {
			return fmt.Errorf("missing required parameter '%s' in form", name)
		}
		// 如果有默认值，设置默认值
		if defaultValue != "" {
			if err := setFieldValue(fieldVal, defaultValue, name, field.Type); err != nil {
				return err
			}
		}
	}
	return nil
}

// handleSliceValue 处理数组类型字段的值
func handleSliceValue(fieldVal reflect.Value, vals []string, name string) error {
	// 如果没有值且字段类型是字符串数组，直接返回空数组
	if len(vals) == 0 && fieldVal.Type().Elem().Kind() == reflect.String {
		fieldVal.Set(reflect.MakeSlice(fieldVal.Type(), 0, 0))
		return nil
	}

	// 创建新的切片
	slice := reflect.MakeSlice(fieldVal.Type(), len(vals), len(vals))

	// 遍历所有值
	for i, val := range vals {
		// 获取切片元素
		elem := slice.Index(i)

		// 设置元素值
		if err := setFieldValue(elem, val, name, elem.Type()); err != nil {
			return err
		}
	}

	// 设置字段值
	fieldVal.Set(slice)
	return nil
}

// formFieldCache 用于缓存结构体字段的表单信息
var formFieldCache sync.Map

// formFieldInfo 存储字段的表单信息
type formFieldInfo struct {
	name       string
	isFile     bool
	isSlice    bool
	fieldIndex int
	fieldType  reflect.Type
	structPath string
}

// getFormFields 获取并缓存结构体的表单字段信息
func getFormFields(t reflect.Type) []formFieldInfo {
	// 在缓存key中加入结构体类型信息，避免不同结构体中相同字段名导致的key重复
	structPath := t.PkgPath() + "." + t.Name()

	// 尝试从缓存获取
	if cached, ok := formFieldCache.Load(structPath); ok {
		return cached.([]formFieldInfo)
	}

	// 缓存未命中，解析字段信息
	fields := make([]formFieldInfo, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name := field.Tag.Get("name")
		if name == "" {
			continue
		}
		name = strings.Split(name, ",")[0]

		// 判断字段类型
		isFile := false
		isSlice := false
		fieldType := field.Type

		switch field.Type.Kind() {
		case reflect.Ptr:
			if field.Type.Elem().String() == "multipart.FileHeader" {
				isFile = true
			}
		case reflect.Slice:
			isSlice = true
			if field.Type.Elem().String() == "*multipart.FileHeader" {
				isFile = true
			}
		}

		fields = append(fields, formFieldInfo{
			name:       name,
			isFile:     isFile,
			isSlice:    isSlice,
			fieldIndex: i,
			fieldType:  fieldType,
			structPath: structPath,
		})
	}

	// 存入缓存
	formFieldCache.Store(structPath, fields)
	return fields
}

// decodeMultipartForm 从请求中解析multipart表单数据
func decodeMultipartForm(form *multipart.Form, v interface{}) error {
	// 获取结构体类型
	structValue := reflect.ValueOf(v)
	if structValue.Kind() == reflect.Ptr {
		structValue = structValue.Elem()
	}
	structType := structValue.Type()

	// 获取缓存的字段信息
	fields := getFormFields(structType)

	// 遍历字段信息
	for _, info := range fields {
		fieldVal := structValue.Field(info.fieldIndex)

		// 处理文件字段
		if info.isFile {
			files := form.File[info.name]
			if len(files) > 0 {
				if info.isSlice {
					fieldVal.Set(reflect.ValueOf(files))
				} else {
					fieldVal.Set(reflect.ValueOf(files[0]))
				}
			} else {
				// 检查文件字段是否必填
				canBeEmpty, _ := handleEmptyValue(info.structPath, structType.Field(info.fieldIndex), "name")
				if !canBeEmpty {
					return fmt.Errorf("missing required file '%s' in form", info.name)
				}
			}
			continue
		}

		// 处理普通字段
		vals := form.Value[info.name]
		if info.isSlice {
			if err := handleSliceValue(fieldVal, vals, info.name); err != nil {
				return err
			}
		} else {
			var val string
			if len(vals) > 0 {
				val = vals[0]
			}
			if err := handleFormFieldValue(structType.Field(info.fieldIndex), fieldVal, val, info.name, info.structPath); err != nil {
				return err
			}
		}
	}
	return nil
}
