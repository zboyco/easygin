package file

import (
	"context"
	"net/url"

	"github.com/zboyco/easygin"
)

func init() {
	RouterRoot.RegisterAPI(&Redirect{})
}

type Redirect struct {
	easygin.MethodGet `summary:"Redirect"`
	Url               string `in:"query" name:"url"`
}

func (Redirect) Path() string {
	return "/redirect"
}

func (req *Redirect) Output(c context.Context) (any, error) {
	u, err := url.Parse(req.Url)
	if err != nil {
		return nil, easygin.NewError(400, "Invalid Parameters", err.Error())
	}

	return u, nil
}
