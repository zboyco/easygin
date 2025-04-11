package user

import (
	"github.com/zboyco/easygin"
	"github.com/zboyco/easygin/example/apis/middleware"
	"github.com/zboyco/easygin/example/apis/user/sub"
)

var RouterRoot = easygin.NewRouterGroup("/user", &middleware.MustAuth{})

func init() {
	RouterRoot.RegisterGroup(sub.RouterRoot)
}
