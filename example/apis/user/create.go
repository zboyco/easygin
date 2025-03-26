package user

import (
	"context"

	"github.com/zboyco/easygin"
)

func init() {
	RouterRoot.RegisterAPI(&CreateUser{})
}

type CreateUser struct {
	easygin.MethodPost `summary:"Create user"`
	Body               ReqCreateUser `in:"body"`
}

type ReqCreateUser struct {
	Name string `json:"name" desc:"User Name"`
	Age  int    `json:"age" desc:"User Age"`
}

func (CreateUser) Path() string {
	return ""
}

func (req *CreateUser) Output(ctx context.Context) (any, error) {
	if req.Body.Name == "" {
		return nil, easygin.NewError(400, "name is empty", "name is empty")
	}
	return nil, nil
}

func (CreateUser) Responses() easygin.R {
	return easygin.R{
		204: nil,
		400: easygin.Error{},
	}
}
