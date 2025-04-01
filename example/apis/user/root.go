package user

import (
	"github.com/zboyco/easygin"
	"github.com/zboyco/easygin/example/apis/middleware"
)

var RouterRoot = easygin.NewRouterGroup("/user", &middleware.MustAuth{})
