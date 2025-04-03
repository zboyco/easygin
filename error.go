package easygin

import "fmt"

type ErrorHttp interface {
	StatusCode() int
	Error() string
	Desc() string
}

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
			fmt.Fprintf(f, "Code: %d\n", e.C)
			fmt.Fprintf(f, "Message: %s\n", e.M)
			fmt.Fprintf(f, "Description: %s\n", e.D)
			if e.err != nil {
				fmt.Fprintf(f, "\nWrapped error:\n")
				if formatter, ok := e.err.(fmt.Formatter); ok {
					formatter.Format(f, c)
				} else {
					fmt.Fprintf(f, "%v", e.err)
				}
			}
			return
		}

		// %v 格式：提供基本信息
		fallthrough
	case 's':
		// %s 格式：简单字符串
		if e.D != "" {
			fmt.Fprintf(f, "%s: %s", e.M, e.D)
		} else {
			fmt.Fprintf(f, "%s", e.M)
		}
	case 'q':
		// %q 格式：带引号的字符串
		if e.D != "" {
			fmt.Fprintf(f, "%q: %q", e.M, e.D)
		} else {
			fmt.Fprintf(f, "%q", e.M)
		}
	}
}

func (e *Error) Unwrap() error {
	return e.err
}

func IsErrorHttp(err error) bool {
	_, ok := err.(ErrorHttp)
	return ok
}
