package easygin

import (
	"context"

	"github.com/gin-gonic/gin"
)

// R 定义了API响应状态码到响应内容的映射
// 用于在OpenAPI文档中描述不同状态码对应的响应结构
type R map[int]any

// GinHandler 定义了可以转换为gin.HandlerFunc的接口
// 实现此接口的类型可以直接用于Gin路由注册
type GinHandler interface {
	GinHandle() gin.HandlerFunc
}

// RouterHandler 定义了路由处理器的基本接口
// 所有的API和中间件都需要实现此接口
// Output方法接收上下文并返回处理结果或错误
type RouterHandler interface {
	Output(ctx context.Context) (any, error)
}

// RouterResponse 定义了可以提供API响应信息的接口
// 实现此接口的API可以在OpenAPI文档中生成详细的响应说明
type RouterResponse interface {
	Responses() R
}

// RouterAPI 定义了API路由的接口
// 包含HTTP方法、路径和处理逻辑
// 继承了RouterHandler接口
type RouterAPI interface {
	Method() string // 返回HTTP方法，如GET、POST等
	Path() string   // 返回API路径
	RouterHandler   // 嵌入RouterHandler接口
}

// ContextKey 定义了可以作为上下文键的接口
// 中间件可以实现此接口，将处理结果存储到上下文中
type ContextKey interface {
	ContextKey() any
}

// NoOpenAPI 标记接口，实现此接口的API将不会生成OpenAPI文档
// 用于内部API或不需要文档的特殊路由
type NoOpenAPI interface {
	IgnoreOpenAPI()
}

// NoGenParameter 标记接口，实现此接口的API将不会生成参数绑定代码
// 用于特殊路由，如Swagger UI或静态文件服务
type NoGenParameter interface {
	IgnoreGenParameter()
}

// WithBindParameters 定义了自定义参数绑定的接口
// 实现此接口的API可以自定义参数绑定逻辑，而不使用自动生成的绑定代码
type WithBindParameters interface {
	EasyGinBindParameters(c *gin.Context) error
}

// RouterGroup 定义了路由组结构
// 用于组织和管理相关的API和子路由组
type RouterGroup struct {
	path        string          // 路由组路径前缀
	children    []*RouterGroup  // 子路由组列表
	apis        []RouterAPI     // 当前组中的API列表
	middlewares []RouterHandler // 应用于当前组的中间件列表
}

// NewRouterGroup 创建一个新的路由组
// 参数:
//   - path: 路由组的路径前缀
//   - middlewares: 应用于该组的中间件列表
//
// 返回:
//   - 新创建的路由组指针
func NewRouterGroup(path string, middlewares ...RouterHandler) *RouterGroup {
	return &RouterGroup{
		path:        path,
		apis:        make([]RouterAPI, 0),
		middlewares: middlewares,
	}
}

// Path 返回路由组的路径前缀
func (g *RouterGroup) Path() string {
	return g.path
}

// RegisterAPI 向路由组注册一个API
// 参数:
//   - api: 实现了RouterAPI接口的API
func (g *RouterGroup) RegisterAPI(api RouterAPI) {
	g.apis = append(g.apis, api)
}

// RegisterGroup 向当前路由组注册一个子路由组
// 参数:
//   - group: 要注册的子路由组
func (g *RouterGroup) RegisterGroup(group *RouterGroup) {
	g.children = append(g.children, group)
}
