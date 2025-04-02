package user

import (
	"context"
	"fmt"

	"github.com/zboyco/easygin"
	"github.com/zboyco/easygin/logr"
)

func init() {
	RouterRoot.RegisterAPI(&ListUser{})
}

type ListUser struct {
	easygin.MethodGet `summary:"Get user list" `
	Name              string `in:"query" name:"name,omitempty" desc:"User Name"`
	AgeMin            int    `in:"query" name:"ageMin,omitempty" default:"18" desc:"User Min Age"`
}

func (ListUser) Path() string {
	return ""
}

func (req *ListUser) Output(ctx context.Context) (any, error) {
	fmt.Println(req.Name)
	fmt.Println(req.AgeMin)

	logr.FromContext(ctx).Info("test log")

	return []RespGetUser{{
		ID:   1,
		Name: "someone",
	}, {
		ID:   2,
		Name: "someone2",
	}}, nil
}

func (ListUser) Responses() easygin.R {
	return easygin.R{
		200: []RespGetUser{},
	}
}
