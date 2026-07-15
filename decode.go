package easygin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"reflect"
	"slices"
	"strings"
	"sync"
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
func decodeJSON(r io.Reader, v any) error {
	body, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("read json failed: %v", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(v); err != nil {
		return fmt.Errorf("parse json failed: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("parse json failed: multiple JSON values are not allowed")
		}
		return fmt.Errorf("parse json failed: %v", err)
	}

	if !HandleBodyJsonOmitEmptyAndDefault() {
		return nil
	}

	// 验证必填字段
	return ValidateJsonRequiredFieldsWithRaw(reflect.ValueOf(v), body)
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
	hasOmitempty := slices.Contains(tagParts[1:], "omitempty")

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

// isEmptyValue 判断字段值是否为空（使用更安全的实现）
func isEmptyValue(v reflect.Value) bool {
	return IsZeroValue(v)
}

// typeCachePath 返回适合标签解析缓存使用的类型标识。
//
// 命名类型使用包路径和类型名；匿名结构体使用结构描述，避免多个匿名结构体共享空 key。
func typeCachePath(t reflect.Type) string {
	if t.PkgPath() != "" || t.Name() != "" {
		return t.PkgPath() + "." + t.Name()
	}
	return t.String()
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
	anonymous    bool
}

// getValidateFields 获取并缓存结构体的验证字段信息
func getValidateFields(t reflect.Type) []validateFieldInfo {
	// 尝试从缓存获取
	if cached, ok := validateFieldsCache.Load(t); ok {
		return cached.([]validateFieldInfo)
	}

	structPath := typeCachePath(t)

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
			anonymous:    field.Anonymous,
		})
	}

	// 存入缓存
	validateFieldsCache.Store(t, fields)
	return fields
}

// ValidateJsonRequiredFields 递归验证结构体的必填字段。
//
// 该函数无法区分 JSON 字段缺失和显式零值，保留旧反射语义以兼容直接调用者。
// 新的 JSON body 绑定应优先使用 ValidateJsonRequiredFieldsWithRaw。
func ValidateJsonRequiredFields(v reflect.Value) error {
	return validateJsonRequiredFields(v, nil, false)
}

// ValidateJsonRequiredFieldsWithRaw 根据原始 JSON 字段出现情况验证必填字段并应用默认值。
//
// 参数:
//   - v: 已反序列化后的目标值，通常是结构体或结构体指针。
//   - raw: 与 v 对应的原始 JSON 对象字节。
//
// 返回值:
//   - error: 当必填字段缺失、默认值转换失败或原始 JSON 无法解析时返回错误。
func ValidateJsonRequiredFieldsWithRaw(v reflect.Value, raw []byte) error {
	return validateJsonRequiredFields(v, json.RawMessage(raw), true)
}

// validateJsonRequiredFields 按反射信息递归验证 JSON body 字段。
func validateJsonRequiredFields(v reflect.Value, raw json.RawMessage, usePresence bool) error {
	// 处理指针类型，添加安全检查
	for {
		if v.Kind() == reflect.Ptr {
			if v.IsNil() || !v.IsValid() {
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
	var rawFields map[string]json.RawMessage
	if usePresence {
		var err error
		rawFields, err = jsonObjectFields(raw)
		if err != nil {
			return err
		}
	}

	// 遍历字段信息
	for _, info := range fields {
		fieldValue := v.Field(info.index)
		if usePresence {
			if info.anonymous && info.jsonName == "" && info.isStruct {
				if err := validateJsonRequiredFields(fieldValue, raw, true); err != nil {
					return err
				}
				continue
			}
			if info.jsonName == "" || info.jsonName == "-" {
				continue
			}
		}

		// 处理结构体类型字段（包括指针类型的结构体）
		if info.isStruct {
			var nestedRaw json.RawMessage
			if usePresence {
				var ok bool
				nestedRaw, ok = rawFields[info.jsonName]
				if !ok || isJSONNull(nestedRaw) {
					if !info.canBeEmpty {
						return fmt.Errorf("missing required field '%s' in body", info.jsonName)
					}
					continue
				}
			}
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
				// 如果指针不为nil，安全地获取其指向的值
				if fieldValue.IsValid() && fieldValue.CanInterface() {
					fieldValue = fieldValue.Elem()
				} else {
					continue
				}
			}
			// 无论字段是否可以为空，只要不为nil就需要递归验证其内部字段
			if err := validateJsonRequiredFields(fieldValue, nestedRaw, usePresence); err != nil {
				return err
			}
			continue
		}

		if usePresence {
			fieldRaw, ok := rawFields[info.jsonName]
			if ok && !isJSONNull(fieldRaw) {
				continue
			}
			if !info.canBeEmpty {
				return fmt.Errorf("missing required field '%s' in body", info.jsonName)
			}
			if info.defaultValue != "" {
				if err := setFieldValue(fieldValue, info.defaultValue, info.jsonName, fieldValue.Type()); err != nil {
					return err
				}
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

// jsonObjectFields 将原始 JSON 对象解析为字段映射。
func jsonObjectFields(raw json.RawMessage) (map[string]json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return nil, nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("parse json fields failed: %v", err)
	}
	return fields, nil
}

// isJSONNull 判断原始 JSON 值是否为 null。
func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// handleFormFieldValue 统一处理表单字段值的 presence 校验和设置。
func handleFormFieldValue(field reflect.StructField, fieldVal reflect.Value, value, name, structPath string, present bool) error {
	canBeEmpty, defaultValue := handleEmptyValue(structPath, field, "name")
	if !present {
		if !canBeEmpty {
			return fmt.Errorf("missing required parameter '%s' in form", name)
		}
		// 参数缺失时才应用默认值；显式空值会继续按目标类型绑定。
		if defaultValue != "" {
			if err := setFieldValue(fieldVal, defaultValue, name, field.Type); err != nil {
				return err
			}
		}
		return nil
	}

	return setFieldValue(fieldVal, value, name, field.Type)
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
	// 尝试从缓存获取
	if cached, ok := formFieldCache.Load(t); ok {
		return cached.([]formFieldInfo)
	}

	structPath := typeCachePath(t)

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
	formFieldCache.Store(t, fields)
	return fields
}

// decodeMultipartForm 从请求中解析multipart表单数据
func decodeMultipartForm(form *multipart.Form, v any) error {
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
		vals, present := form.Value[info.name]
		if info.isSlice {
			if !present {
				canBeEmpty, _ := handleEmptyValue(info.structPath, structType.Field(info.fieldIndex), "name")
				if !canBeEmpty {
					return fmt.Errorf("missing required parameter '%s' in form", info.name)
				}
				continue
			}
			if err := handleSliceValue(fieldVal, vals, info.name); err != nil {
				return err
			}
		} else {
			var val string
			if len(vals) > 0 {
				val = vals[0]
			}
			if err := handleFormFieldValue(structType.Field(info.fieldIndex), fieldVal, val, info.name, info.structPath, present); err != nil {
				return err
			}
		}
	}
	return nil
}

// IsZeroValue 检查值是否为零值（改进版本，更安全）
func IsZeroValue(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}

	switch v.Kind() {
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Complex64, reflect.Complex128:
		return v.Complex() == 0
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Interface, reflect.Ptr:
		if v.IsNil() {
			return true
		}
		elem, err := SafeReflectElem(v)
		if err != nil {
			return true
		}
		return IsZeroValue(elem)
	case reflect.Struct:
		// 对于结构体，检查所有字段是否都为零值
		for i := 0; i < v.NumField(); i++ {
			if !IsZeroValue(v.Field(i)) {
				return false
			}
		}
		return true
	default:
		// 对于其他类型，使用reflect.Zero比较
		return v.Interface() == reflect.Zero(v.Type()).Interface()
	}
}

// SafeReflectElem 安全地获取反射值的元素，包含nil检查
func SafeReflectElem(v reflect.Value) (reflect.Value, error) {
	if !v.IsValid() {
		return reflect.Value{}, fmt.Errorf("invalid reflect value")
	}

	if v.Kind() != reflect.Ptr && v.Kind() != reflect.Interface {
		return v, nil
	}

	if v.IsNil() {
		return reflect.Value{}, fmt.Errorf("cannot get element of nil pointer")
	}

	return v.Elem(), nil
}
