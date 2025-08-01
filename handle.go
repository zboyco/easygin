package easygin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// 预分配错误响应对象池
var errorResponsePool = sync.Pool{
	New: func() any {
		return &gin.H{}
	},
}

// fieldInfo 存储字段的元信息
type fieldInfo struct {
	index    []int // 索引路径
	field    reflect.StructField
	tagType  string
	tagName  string
	tagNames []string
}

// structFields 缓存结构体的字段信息
var structFields sync.Map

// handleError 统一处理错误响应
func handleError(c *gin.Context, err error) {
	// 将错误添加到 gin.Context 的 Errors 中
	_ = c.Error(err)

	// 从对象池获取响应对象
	resp := errorResponsePool.Get().(*gin.H)
	defer errorResponsePool.Put(resp)

	if errorHttp, ok := err.(ErrorHttp); ok {
		*resp = gin.H{
			"code": errorHttp.StatusCode(),
			"msg":  errorHttp.Error(),
			"desc": errorHttp.Desc(),
		}
		c.AbortWithStatusJSON(errorHttp.StatusCode(), *resp)
		return
	}

	*resp = gin.H{
		"code": 500,
		"msg":  err.Error(),
		"desc": "Internal Server Error",
	}
	c.AbortWithStatusJSON(500, *resp)
}

// parseTime 统一处理时间格式解析
func parseTime(val string, tagName string) (time.Time, error) {
	var (
		t   time.Time
		err error
	)
	t, err = time.Parse(time.RFC3339, val)
	if err == nil {
		return t, nil
	}
	return t, fmt.Errorf("invalid time format for parameter '%s': %v", tagName, err)
}

// setFieldValue 统一处理字段值的类型转换和赋值
func setFieldValue(fieldValue reflect.Value, val string, tagName string, fieldType reflect.Type) error {
	// 处理指针类型
	if fieldType.Kind() == reflect.Ptr {
		// 如果值为空，则设置为nil
		if val == "" {
			fieldValue.Set(reflect.Zero(fieldType))
			return nil
		}

		// 创建新的指针并设置值
		ptrValue := reflect.New(fieldType.Elem())
		if err := setFieldValue(ptrValue.Elem(), val, tagName, fieldType.Elem()); err != nil {
			return err
		}
		fieldValue.Set(ptrValue)
		return nil
	}

	// 处理time.Time类型
	if fieldType.String() == "time.Time" {
		t, err := parseTime(val, tagName)
		if err != nil {
			return err
		}
		fieldValue.Set(reflect.ValueOf(t))
		return nil
	}

	switch fieldType.Kind() {
	case reflect.String:
		fieldValue.SetString(val)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		intVal, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid parameter '%s': %w", tagName, err)
		}
		fieldValue.SetInt(intVal)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		uintVal, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid parameter '%s': %w", tagName, err)
		}
		fieldValue.SetUint(uintVal)
	case reflect.Float32, reflect.Float64:
		floatVal, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return fmt.Errorf("invalid parameter '%s': %w", tagName, err)
		}
		fieldValue.SetFloat(floatVal)
	case reflect.Bool:
		boolVal, err := strconv.ParseBool(val)
		if err != nil {
			return fmt.Errorf("invalid parameter '%s': %w", tagName, err)
		}
		fieldValue.SetBool(boolVal)
	default:
		return fmt.Errorf("unsupported parameter type: %v, field name: %s", fieldType.Kind(), tagName)
	}
	return nil
}

// getStructFields 获取结构体的字段信息，优先从缓存中获取
func getStructFields(t reflect.Type) []fieldInfo {
	// 构建缓存键
	structPath := t.PkgPath() + "." + t.Name()

	// 尝试从缓存中获取
	if cached, ok := structFields.Load(structPath); ok {
		return cached.([]fieldInfo)
	}

	// 缓存未命中，解析字段信息
	fields := make([]fieldInfo, 0, t.NumField())

	// 使用辅助函数递归处理字段，包括嵌入字段
	collectFields(t, nil, &fields)

	// 存入缓存
	structFields.Store(structPath, fields)
	return fields
}

// collectFields 递归收集字段信息，包括嵌入字段
func collectFields(t reflect.Type, indexPrefix []int, fields *[]fieldInfo) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// 构建当前字段的完整索引路径
		index := append(append([]int{}, indexPrefix...), i)

		tag := field.Tag.Get("in")

		// 处理嵌入字段
		if tag == "" && field.Anonymous && field.Type.Kind() == reflect.Struct {
			collectFields(field.Type, index, fields)
			continue
		}

		if tag != "" {
			// 对于body类型的字段，不需要name标签
			if tag == "body" {
				*fields = append(*fields, fieldInfo{
					index:   index,
					field:   field,
					tagType: tag,
				})
				continue
			}

			// 其他类型的字段需要name标签
			tagName := field.Tag.Get("name")
			if tagName != "" {
				tagNames := strings.Split(tagName, ",")
				*fields = append(*fields, fieldInfo{
					index:    index,
					field:    field,
					tagType:  tag,
					tagName:  tagNames[0],
					tagNames: tagNames,
				})
			}
		}
	}
}

// bindParams 统一处理参数绑定逻辑
func bindParams(c *gin.Context, h RouterHandler) (RouterHandler, error) {
	// 获取接口的真实类型
	handlerType := reflect.TypeOf(h)

	// 创建新的结构体实例
	newHandler := reflect.New(handlerType.Elem()).Interface().(RouterHandler)
	if bindParameters, ok := newHandler.(WithBindParameters); ok {
		if err := bindParameters.EasyGinBindParameters(c); err != nil {
			return nil, err
		}
		return newHandler, nil
	}

	handlerValue := reflect.ValueOf(newHandler).Elem()

	// 获取缓存的字段信息
	fields := getStructFields(handlerType.Elem())

	// 遍历字段信息
	for _, info := range fields {
		// 使用FieldByIndex获取嵌套字段
		fieldValue := handlerValue.FieldByIndex(info.index)

		// 处理body参数
		if info.tagType == "body" {
			mime := info.field.Tag.Get("mime")
			if mime == "multipart" {
				// 将 multipart 表单内存限制为 1GB，实际上相当于不做限制
				if err := c.Request.ParseMultipartForm(1 << 30); err != nil {
					return nil, fmt.Errorf("parse multipart form failed: %v", err)
				}

				// 创建新的结构体实例
				newStruct := reflect.New(fieldValue.Type()).Interface()

				// 使用decodeMultipartForm解析表单数据
				if err := decodeMultipartForm(c.Request.MultipartForm, newStruct); err != nil {
					return nil, err
				}

				// 将解析后的结构体赋值给原字段
				fieldValue.Set(reflect.ValueOf(newStruct).Elem())
				continue
			}
			if err := decodeJSON(c.Request.Body, fieldValue.Addr().Interface()); err != nil {
				return nil, fmt.Errorf("invalid body parameter: %v", err)
			}
			continue
		}

		// 处理query、path和header参数
		if info.tagType == "query" || info.tagType == "path" || info.tagType == "header" {
			// 跳过未命名的参数
			if info.tagName == "" || info.tagName == "-" {
				continue
			}

			var val string
			switch info.tagType {
			case "query":
				val = c.Query(info.tagName)
			case "path":
				val = c.Param(info.tagName)
			case "header":
				val = c.GetHeader(info.tagName)
			}

			if val == "" {
				if !slices.Contains(info.tagNames, "omitempty") {
					return nil, fmt.Errorf("missing required parameter '%s' in %s", info.tagName, info.tagType)
				}

				defaultValue := info.field.Tag.Get("default")
				if defaultValue == "" {
					continue
				}
				val = defaultValue
			}

			if err := setFieldValue(fieldValue, val, info.tagName, info.field.Type); err != nil {
				return nil, err
			}
			continue
		}
	}

	return newHandler, nil
}

// renderAPI 处理API
func renderAPI(h RouterHandler, handlerName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 将handlerName存入context
		c.Request = c.Request.WithContext(ContextWithHandlerName(c.Request.Context(), handlerName))

		output, err := handleRouter(c, h)
		if err != nil {
			handleError(c, err)
			return
		}

		var (
			code           = http.StatusOK
			withStatusCode = false
		)

		switch v := output.(type) {
		case WithStatusCode:
			code = v.StatusCode
			output = v.Output
			withStatusCode = true
		case *WithStatusCode:
			code = v.StatusCode
			output = v.Output
			withStatusCode = true
		default:
		}

		if output == nil {
			if withStatusCode {
				c.Status(code)
			} else {
				c.Status(http.StatusNoContent)
			}
			return
		}

		switch v := output.(type) {
		case url.URL:
			if withStatusCode {
				c.Redirect(code, v.String())
			} else {
				c.Redirect(http.StatusFound, v.String())
			}
		case *url.URL:
			if withStatusCode {
				c.Redirect(code, v.String())
			} else {
				c.Redirect(http.StatusFound, v.String())
			}
		case string:
			c.String(code, v)
		case AttachmentFromFile:
			c.Header("Content-Disposition", fmt.Sprintf("%s; filename=%s", v.Disposition, v.Filename))
			c.Data(code, v.ContentType, v.Content)
		case *AttachmentFromFile:
			c.Header("Content-Disposition", fmt.Sprintf("%s; filename=%s", v.Disposition, v.Filename))
			c.Data(code, v.ContentType, v.Content)
		case AttachmentFromReader:
			c.Header("Content-Disposition", fmt.Sprintf("%s; filename=%s", v.Disposition, v.Filename))
			c.DataFromReader(code, v.ContentLength, v.ContentType, v.Reader, nil)
			// if v.Reader implements io.Closer, close it
			if closer, ok := v.Reader.(io.Closer); ok {
				_ = closer.Close()
			}
		case *AttachmentFromReader:
			c.Header("Content-Disposition", fmt.Sprintf("%s; filename=%s", v.Disposition, v.Filename))
			c.DataFromReader(code, v.ContentLength, v.ContentType, v.Reader, nil)
			// if v.Reader implements io.Closer, close it
			if closer, ok := v.Reader.(io.Closer); ok {
				_ = closer.Close()
			}
		default:
			c.JSON(code, output)
		}
	}
}

// renderMiddleware 处理中间件
func renderMiddleware(h RouterHandler, handlerName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 将handlerName存入context
		c.Request = c.Request.WithContext(ContextWithHandlerName(c.Request.Context(), handlerName))

		output, err := handleRouter(c, h)
		if err != nil {
			handleError(c, err)
			return
		}

		if key, ok := h.(ContextKey); ok {
			c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), key.ContextKey(), output))
		}

		c.Next()
	}
}

func renderGinHandler(h GinHandler, handlerName string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// 将handlerName存入context
		ctx.Request = ctx.Request.WithContext(ContextWithHandlerName(ctx.Request.Context(), handlerName))
		h.GinHandle()(ctx)
	}
}

// handleRouter 处理通用的RouterHandler逻辑，包括参数绑定和调用Handle方法
func handleRouter(c *gin.Context, h RouterHandler) (any, error) {
	// 绑定参数
	newHandler, err := bindParams(c, h)
	if err != nil {
		return nil, NewError(http.StatusBadRequest, err.Error(), "invalid parameters")
	}

	// 将gin.Context添加到context中
	// 调用Handle方法
	return newHandler.Output(ContextWithGinContext(c.Request.Context(), c))
}

type Disposition string

const (
	DispositionAttachment Disposition = "attachment"
	DispositionInline     Disposition = "inline"
)

type AttachmentFromFile struct {
	// exp. "attachment","inline"
	Disposition Disposition
	// exp. "application/octet-stream","image/png","text/plain"
	ContentType string
	// exp. "file.png"
	Filename string
	// exp. os.Open("file.png")
	Content []byte
}

type AttachmentFromReader struct {
	// exp. "attachment","inline"
	Disposition Disposition
	// exp. "application/octet-stream","image/png","text/plain"
	ContentType string
	// exp. "file.png"
	Filename string
	// exp. os.Open("file.png")
	Reader io.Reader
	// File content length (in bytes):
	// A positive number indicates the exact size
	// 0 indicates an empty file
	// -1 indicates unknown length (chunked transfer will be used)
	ContentLength int64
}

// WithStatusCode 用于返回指定状态码和响应体的结构体
type WithStatusCode struct {
	StatusCode int
	Output     any
}
