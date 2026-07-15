package easygin

import (
	"errors"
	"fmt"
	"reflect"
)

type ErrorHttp interface {
	StatusCode() int
	Error() string
	Desc() string
}

const (
	minErrorHTTPStatus = 400
	maxErrorHTTPStatus = 599
)

type Error struct {
	C   int    `json:"code" desc:"状态码"`
	M   string `json:"msg" desc:"错误信息"`
	D   string `json:"desc" desc:"错误描述"`
	err error  `json:"-"`
}

func NewError(code int, message, desc string) *Error {
	return &Error{
		C: code,
		M: message,
		D: desc,
	}
}

func WrapError(err error, code int, message string) *Error {
	if err == nil {
		return nil
	}

	return &Error{
		C:   code,
		M:   message,
		D:   err.Error(),
		err: err,
	}
}

func (e *Error) WithCode(code int) *Error {
	return &Error{
		C:   code,
		M:   e.M,
		D:   e.D,
		err: e.err,
	}
}

func (e *Error) WithMsg(msg string) *Error {
	return &Error{
		C:   e.C,
		M:   msg,
		D:   e.D,
		err: e.err,
	}
}

func (e *Error) WithDesc(desc string) *Error {
	return &Error{
		C:   e.C,
		M:   e.M,
		D:   desc,
		err: e.err,
	}
}

func (e *Error) WithError(err error) *Error {
	return &Error{
		C:   e.C,
		M:   e.M,
		D:   e.D,
		err: err,
	}
}

func (e *Error) StatusCode() int {
	return e.C
}

func (e *Error) Error() string {
	return e.M
}

func (e *Error) Desc() string {
	return e.D
}

func (e *Error) Format(f fmt.State, c rune) {
	// 根据不同的格式化符号提供不同级别的详细信息
	switch c {
	case 'v':
		if f.Flag('+') {
			_, _ = fmt.Fprintf(f, "Code: %d\n", e.C)
			_, _ = fmt.Fprintf(f, "Message: %s\n", e.M)
			_, _ = fmt.Fprintf(f, "Description: %s\n", e.D)
			if e.err != nil {
				_, _ = fmt.Fprintf(f, "\nWrapped error:\n")
				if formatter, ok := e.err.(fmt.Formatter); ok {
					formatter.Format(f, c)
				} else {
					_, _ = fmt.Fprintf(f, "%v", e.err)
				}
			}
			return
		}

		// %v 格式：提供基本信息
		fallthrough
	case 's':
		// %s 格式：简单字符串
		if e.D != "" {
			_, _ = fmt.Fprintf(f, "%s: %s", e.M, e.D)
		} else {
			_, _ = fmt.Fprintf(f, "%s", e.M)
		}
	case 'q':
		// %q 格式：带引号的字符串
		if e.D != "" {
			_, _ = fmt.Fprintf(f, "%q: %q", e.M, e.D)
		} else {
			_, _ = fmt.Fprintf(f, "%q", e.M)
		}
	}
}

func (e *Error) Unwrap() error {
	return e.err
}

// asErrorHTTP 从错误链中提取非 nil 的 ErrorHttp。
func asErrorHTTP(err error) (ErrorHttp, bool) {
	if err == nil {
		return nil, false
	}
	var target ErrorHttp
	if !errors.As(err, &target) || isNilInterfaceValue(target) {
		return nil, false
	}
	return target, true
}

// isNilInterfaceValue 判断接口是否为 nil 或包含 typed-nil 值。
func isNilInterfaceValue(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// isValidErrorHTTPStatus 判断状态码是否属于严格错误响应范围。
func isValidErrorHTTPStatus(statusCode int) bool {
	return statusCode >= minErrorHTTPStatus && statusCode <= maxErrorHTTPStatus
}

// IsErrorHttp 判断错误链中是否包含有效的非 nil ErrorHttp。
func IsErrorHttp(err error) bool {
	errorHTTP, ok := asErrorHTTP(err)
	return ok && isValidErrorHTTPStatus(errorHTTP.StatusCode())
}
