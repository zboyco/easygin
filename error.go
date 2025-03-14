package easygin

type ErrorHttp interface {
	Code() int
	Error() string
	Desc() string
}

type Error struct {
	C int    `json:"code" desc:"错误码"`
	M string `json:"msg" desc:"错误信息"`
	D string `json:"desc" desc:"错误描述"`
}

func NewError(code int, msg, desc string) *Error {
	return &Error{
		C: code,
		M: msg,
		D: desc,
	}
}

func (e *Error) Code() int {
	return e.C
}

func (e *Error) Error() string {
	return e.M
}

func (e *Error) Desc() string {
	return e.D
}
