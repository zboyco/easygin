package easygin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
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

// apiErrorResponse 定义 easygin 严格错误响应的固定 JSON 契约。
type apiErrorResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Desc string `json:"desc"`
}

// handleErrorWithPolicy 根据错误详情策略生成统一错误响应；关闭详情时会隐藏 5xx ErrorHttp 内容。
func handleErrorWithPolicy(c *gin.Context, err error, exposeDetails bool) {
	if err == nil {
		return
	}
	if isNilInterfaceValue(err) {
		err = fmt.Errorf("typed-nil error")
	}
	// 将错误添加到 gin.Context 的 Errors 中
	_ = c.Error(err)

	if errorHttp, ok := asErrorHTTP(err); ok {
		statusCode := errorHttp.StatusCode()
		if !isValidErrorHTTPStatus(statusCode) {
			_ = c.Error(fmt.Errorf("invalid ErrorHttp status code %d", statusCode))
			writeInternalServerError(c, http.StatusText(http.StatusInternalServerError))
			return
		}
		if statusCode < http.StatusInternalServerError || exposeDetails {
			writeAPIError(c, statusCode, errorHttp.Error(), errorHttp.Desc())
			return
		}

		message := http.StatusText(statusCode)
		if message == "" {
			message = http.StatusText(http.StatusInternalServerError)
		}
		writeAPIError(c, statusCode, message, message)
		return
	}

	message := http.StatusText(http.StatusInternalServerError)
	if exposeDetails {
		message = err.Error()
	}
	writeInternalServerError(c, message)
}

// handleRecoveryErrorWithPolicy 处理 panic；始终返回 500，并在允许时沿用旧版 desc 字段暴露 panic 文本。
func handleRecoveryErrorWithPolicy(c *gin.Context, err error, exposeDetails bool) {
	if err == nil {
		return
	}

	_ = c.Error(err)
	description := http.StatusText(http.StatusInternalServerError)
	if exposeDetails {
		description = err.Error()
	}
	writeAPIError(c, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError), description)
}

// writeInternalServerError 写入不包含内部错误详情的通用 500 响应。
func writeInternalServerError(c *gin.Context, message string) {
	writeAPIError(c, http.StatusInternalServerError, message, http.StatusText(http.StatusInternalServerError))
}

// writeAPIError 按固定契约写出已校验的 HTTP 错误响应。
func writeAPIError(c *gin.Context, statusCode int, message, description string) {
	if !isValidErrorHTTPStatus(statusCode) {
		statusCode = http.StatusInternalServerError
		message = http.StatusText(statusCode)
		description = message
	}
	c.AbortWithStatusJSON(statusCode, apiErrorResponse{
		Code: statusCode,
		Msg:  message,
		Desc: description,
	})
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
		intVal, err := strconv.ParseInt(val, 10, fieldType.Bits())
		if err != nil {
			return fmt.Errorf("invalid parameter '%s': %w", tagName, err)
		}
		if fieldValue.OverflowInt(intVal) {
			return fmt.Errorf("invalid parameter '%s': value %q overflows %s", tagName, val, fieldType)
		}
		fieldValue.SetInt(intVal)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		uintVal, err := strconv.ParseUint(val, 10, fieldType.Bits())
		if err != nil {
			return fmt.Errorf("invalid parameter '%s': %w", tagName, err)
		}
		if fieldValue.OverflowUint(uintVal) {
			return fmt.Errorf("invalid parameter '%s': value %q overflows %s", tagName, val, fieldType)
		}
		fieldValue.SetUint(uintVal)
	case reflect.Float32, reflect.Float64:
		floatVal, err := strconv.ParseFloat(val, fieldType.Bits())
		if err != nil {
			return fmt.Errorf("invalid parameter '%s': %w", tagName, err)
		}
		if fieldValue.OverflowFloat(floatVal) {
			return fmt.Errorf("invalid parameter '%s': value %q overflows %s", tagName, val, fieldType)
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

		// 处理嵌入字段（支持指针形式）
		if tag == "" && field.Anonymous {
			embeddedType := field.Type
			if embeddedType.Kind() == reflect.Ptr {
				embeddedType = embeddedType.Elem()
			}
			if embeddedType.Kind() == reflect.Struct {
				collectFields(embeddedType, index, fields)
				continue
			}
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

// getOrInitFieldByIndex 获取结构体字段，遇到中间的指针字段为nil时自动初始化，避免反射访问嵌入指针时panic
func getOrInitFieldByIndex(v reflect.Value, index []int) (reflect.Value, error) {
	current := v
	for depth, idx := range index {
		for current.Kind() == reflect.Ptr {
			if current.IsNil() {
				if !current.CanSet() {
					return reflect.Value{}, fmt.Errorf("cannot initialize pointer for embedded field at depth %d", depth)
				}
				current.Set(reflect.New(current.Type().Elem()))
			}
			current = current.Elem()
		}

		if current.Kind() != reflect.Struct {
			return reflect.Value{}, fmt.Errorf("invalid handler definition: non-struct value at depth %d", depth)
		}

		if idx >= current.NumField() {
			return reflect.Value{}, fmt.Errorf("invalid handler definition: index %d out of range for %s", idx, current.Type())
		}

		field := current.Field(idx)

		if depth < len(index)-1 && field.Kind() == reflect.Ptr {
			if field.IsNil() {
				if !field.CanSet() {
					return reflect.Value{}, fmt.Errorf("cannot initialize embedded pointer field %s", current.Type().Field(idx).Name)
				}
				elemType := field.Type().Elem()
				if elemType.Kind() != reflect.Struct {
					return reflect.Value{}, fmt.Errorf("invalid handler definition: embedded pointer %s does not reference a struct", current.Type().Field(idx).Name)
				}
				field.Set(reflect.New(elemType))
			}
		}

		current = field
	}
	return current, nil
}

// bindParams 统一处理参数绑定逻辑
func bindParams(c *gin.Context, h RouterHandler) (RouterHandler, error) {
	handlerType, err := routerHandlerStructType(h)
	if err != nil {
		return nil, err
	}

	// 创建新的结构体实例
	newHandler, ok := reflect.New(handlerType).Interface().(RouterHandler)
	if !ok {
		return nil, fmt.Errorf("router handler %s pointer does not implement RouterHandler", handlerType)
	}
	if bindParameters, ok := newHandler.(WithBindParameters); ok {
		if err := bindParameters.EasyGinBindParameters(c); err != nil {
			return nil, err
		}
		return newHandler, nil
	}

	handlerValue := reflect.ValueOf(newHandler).Elem()

	// 获取缓存的字段信息
	fields := getStructFields(handlerType)

	// 遍历字段信息
	for _, info := range fields {
		// 使用FieldByIndex获取嵌套字段
		fieldValue, err := getOrInitFieldByIndex(handlerValue, info.index)
		if err != nil {
			return nil, fmt.Errorf("prepare handler field failed: %w", err)
		}

		// 处理body参数
		if info.tagType == "body" {
			mime := info.field.Tag.Get("mime")
			if mime == "multipart" {
				// 使用全局设置的 multipart 表单内存限制
				if err := c.Request.ParseMultipartForm(GetMultipartMemoryLimit()); err != nil {
					return nil, fmt.Errorf("parse multipart form failed: %w", err)
				}

				targetValue := fieldValue
				if fieldValue.Kind() == reflect.Ptr {
					if fieldValue.IsNil() {
						fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
					}
					targetValue = fieldValue
				} else {
					targetValue = fieldValue.Addr()
				}

				// 使用decodeMultipartForm解析表单数据
				if err := decodeMultipartForm(c.Request.MultipartForm, targetValue.Interface()); err != nil {
					return nil, err
				}

				continue
			}
			target := fieldValue
			if fieldValue.Kind() == reflect.Ptr {
				if fieldValue.IsNil() {
					fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
				}
				target = fieldValue
			} else {
				target = fieldValue.Addr()
			}
			if err := decodeJSON(c.Request.Body, target.Interface()); err != nil {
				return nil, fmt.Errorf("invalid body parameter: %w", err)
			}
			continue
		}

		// 处理query、path和header参数
		if info.tagType == "query" || info.tagType == "path" || info.tagType == "header" {
			// 跳过未命名的参数
			if info.tagName == "" || info.tagName == "-" {
				continue
			}

			var (
				val     string
				present bool
			)
			switch info.tagType {
			case "query":
				val, present = c.GetQuery(info.tagName)
			case "path":
				val = c.Param(info.tagName)
				present = val != ""
			case "header":
				headerKey := http.CanonicalHeaderKey(info.tagName)
				headerVals, ok := c.Request.Header[headerKey]
				if !ok && headerKey != info.tagName {
					headerVals, ok = c.Request.Header[info.tagName]
				}
				present = ok
				if len(headerVals) > 0 {
					val = headerVals[0]
				}
			}

			if !present {
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

// multipartFormCleanup 在请求处理完成后删除 ParseMultipartForm 创建的临时文件。
// 清理通过 defer 执行，确保后续处理器发生 panic 时也不会遗留上传文件。
func multipartFormCleanup() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if c.Request == nil || c.Request.MultipartForm == nil {
				return
			}
			if err := c.Request.MultipartForm.RemoveAll(); err != nil {
				_ = c.Error(fmt.Errorf("remove multipart temporary files: %w", err))
			}
		}()

		c.Next()
	}
}

// maxBodyBytes 限制单个请求体的读取大小，超限时由 handleRouter 映射为 413。
func maxBodyBytes(limit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request != nil && c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		}
		c.Next()
	}
}

// renderAPI 处理API
func renderAPI(h RouterHandler, handlerName string) gin.HandlerFunc {
	return renderAPIWithPolicy(h, handlerName, true)
}

// renderAPIWithPolicy 使用指定错误详情策略处理 API 输出。
func renderAPIWithPolicy(h RouterHandler, handlerName string, exposeErrorDetails bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 将handlerName存入context
		c.Request = c.Request.WithContext(ContextWithHandlerName(c.Request.Context(), handlerName))

		output, err := handleRouter(c, h)
		if err != nil {
			handleErrorWithPolicy(c, err, exposeErrorDetails)
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
			if v == nil {
				handleErrorWithPolicy(c, fmt.Errorf("WithStatusCode is nil"), exposeErrorDetails)
				return
			}
			code = v.StatusCode
			output = v.Output
			withStatusCode = true
		default:
		}
		if withStatusCode && !isValidResponseStatusCode(code) {
			handleErrorWithPolicy(c, fmt.Errorf("invalid response status code %d", code), exposeErrorDetails)
			return
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
			c.Header("Content-Disposition", contentDisposition(v.Disposition, v.Filename))
			c.Data(code, v.ContentType, v.Content)
		case *AttachmentFromFile:
			c.Header("Content-Disposition", contentDisposition(v.Disposition, v.Filename))
			c.Data(code, v.ContentType, v.Content)
		case AttachmentFromReader:
			c.Header("Content-Disposition", contentDisposition(v.Disposition, v.Filename))
			c.DataFromReader(code, v.ContentLength, v.ContentType, v.Reader, nil)
			// if v.Reader implements io.Closer, close it
			if closer, ok := v.Reader.(io.Closer); ok {
				_ = closer.Close()
			}
		case *AttachmentFromReader:
			c.Header("Content-Disposition", contentDisposition(v.Disposition, v.Filename))
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

// isValidResponseStatusCode 判断状态码是否可用于 easygin 最终响应。
func isValidResponseStatusCode(statusCode int) bool {
	return statusCode >= http.StatusOK && statusCode <= maxErrorHTTPStatus
}

// contentDisposition 生成符合 MIME 参数转义规则的 Content-Disposition 响应头。
func contentDisposition(disposition Disposition, filename string) string {
	return mime.FormatMediaType(string(disposition), map[string]string{
		"filename": filename,
	})
}

// renderMiddlewareWithPolicy 使用指定错误详情策略处理中间件输出。
func renderMiddlewareWithPolicy(h RouterHandler, handlerName string, exposeErrorDetails bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 将handlerName存入context
		c.Request = c.Request.WithContext(ContextWithHandlerName(c.Request.Context(), handlerName))

		output, err := handleRouter(c, h)
		if err != nil {
			handleErrorWithPolicy(c, err, exposeErrorDetails)
			return
		}

		if key, ok := h.(ContextKey); ok {
			c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), key.ContextKey(), output))
		}

		c.Next()
	}
}

func renderGinHandler(h GinHandler, handlerName string) gin.HandlerFunc {
	ginHandler := h.GinHandle()
	return func(ctx *gin.Context) {
		// 将handlerName存入context
		ctx.Request = ctx.Request.WithContext(ContextWithHandlerName(ctx.Request.Context(), handlerName))
		ginHandler(ctx)
	}
}

// handleRouter 处理通用的RouterHandler逻辑，包括参数绑定和调用Handle方法
func handleRouter(c *gin.Context, h RouterHandler) (any, error) {
	// 绑定参数
	newHandler, err := bindParams(c, h)
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return nil, NewError(http.StatusRequestEntityTooLarge, http.StatusText(http.StatusRequestEntityTooLarge), "request body exceeds configured limit")
		}
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
