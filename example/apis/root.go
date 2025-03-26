package apis

import (
	"github.com/zboyco/easygin"
	"github.com/zboyco/easygin/example/apis/file"
	"github.com/zboyco/easygin/example/apis/user"
)

var (
	RouterRoot   = easygin.NewRouterGroup("/")
	RouterServer = easygin.NewRouterGroup("/server")
)

func init() {
	RouterRoot.RegisterAPI(easygin.NewLivenessRouter("/liveness")) // 注册健康检查路由
	RouterRoot.RegisterGroup(RouterServer)                         // 注册路由组
	RouterServer.RegisterAPI(easygin.OpenAPIRouter)                // 注册OpenAPI路由
	RouterServer.RegisterAPI(easygin.NewSwaggerUIRouter(RouterServer.Path()))
	{
		RouterServer.RegisterGroup(file.RouterRoot)
		RouterServer.RegisterGroup(user.RouterRoot)
	}
}
