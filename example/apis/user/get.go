package user

import (
	"context"
	"fmt"

	"github.com/zboyco/easygin"
)

func init() {
	RouterRoot.RegisterAPI(&GetUser{})
}

type GetUser struct {
	easygin.MethodGet `summary:"Get user info" `
	Token             string   `in:"header" name:"Token" desc:"User token"`
	ID                int      `in:"path" name:"id" desc:"User ID"`
	Names             []string `in:"query" name:"names" desc:"User Names"`
	IDs               []uint64 `in:"query" name:"ids,omitempty" desc:"User IDs"`
	Bools             []bool   `in:"query" name:"bools" desc:"User bool"`
}

func (GetUser) Path() string {
	return "/:id"
}

func (req *GetUser) Output(ctx context.Context) (any, error) {
	fmt.Println(req.Token)
	if req.Token == "" {
		return nil, easygin.NewError(401, "token is empty", "token is empty")
	}
	if req.ID != 1 {
		return nil, easygin.NewError(404, "user doesn't exist", "request id not equal 1")
	}
	return &RespGetUser{
		ID:   req.ID,
		Name: "someone",
	}, nil
}

func (GetUser) Responses() easygin.R {
	return easygin.R{
		200: &RespGetUser{},
		401: &easygin.Error{},
		404: &easygin.Error{},
	}
}

type RespGetUser struct {
	ID           int    `json:"id" desc:"User ID"`
	IDString     int    `json:"idString,string" desc:"User ID"`
	Name         string `json:"name" desc:"User Name"`
	Active       bool   `json:"active" desc:"User Active"`
	ActiveString bool   `json:"activeString,string" desc:"User Active"`
}
