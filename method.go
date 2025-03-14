package easygin

import "net/http"

type MethodGet struct{}

func (MethodGet) Method() string {
	return http.MethodGet
}

type MethodPost struct{}

func (MethodPost) Method() string {
	return http.MethodPost
}

type MethodPut struct{}

func (MethodPut) Method() string {
	return http.MethodPut
}

type MethodDelete struct{}

func (MethodDelete) Method() string {
	return http.MethodDelete
}

type MethodPatch struct{}

func (MethodPatch) Method() string {
	return http.MethodPatch
}

type MethodHead struct{}

func (MethodHead) Method() string {
	return http.MethodHead
}

type MethodOptions struct{}

func (MethodOptions) Method() string {
	return http.MethodOptions
}

type MethodConnect struct{}

func (MethodConnect) Method() string {
	return http.MethodConnect
}

type MethodTrace struct{}

func (MethodTrace) Method() string {
	return http.MethodTrace
}

type MethodAny struct{}

func (MethodAny) Method() string {
	return "ANY"
}
