package apis

import (
	"github.com/zboyco/easygin"
	"github.com/zboyco/easygin/example/apis/user"
)

var (
	RouterRoot   = easygin.NewRouterGroup("/")
	RouterServer = easygin.NewRouterGroup("/server")
)

func init() {
	RouterRoot.RegisterGroup(RouterServer)
	RouterServer.RegisterAPI(easygin.OpenAPIRouter)
	RouterServer.RegisterAPI(easygin.NewSwaggerUIRouter(RouterServer.Path()))
	{
		RouterServer.RegisterGroup(user.RouterRoot)
	}
}
