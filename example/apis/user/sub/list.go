package sub

import (
	"context"

	"github.com/zboyco/easygin"
)

func init() {
	RouterRoot.RegisterAPI(&ListSub{})
}

type ListSub struct {
	easygin.MethodGet `summary:"Get sub list" `
	Size              int `in:"query" name:"size,omitempty" default:"10" desc:"Sub Size"`
	Offset            int `in:"query" name:"offset,omitempty" default:"0" desc:"Sub Offset"`
}

func (ListSub) Path() string {
	return "/list"
}

func (req *ListSub) Output(ctx context.Context) (any, error) {
	return []string{"sub1", "sub2"}, nil
}
