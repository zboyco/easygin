package easygin

import (
	"context"

	"github.com/gin-gonic/gin"
)

type R map[int]any

type GinHandler interface {
	GinHandle() gin.HandlerFunc
}

type RouterHandler interface {
	Output(ctx context.Context) (any, error)
}

type RouterResponse interface {
	Responses() R
}

type RouterAPI interface {
	Method() string
	Path() string
	RouterHandler
}

type ContextKey interface {
	ContextKey() any
}

type NoOpenAPI interface {
	IgnoreOpenAPI()
}

type NoGenParameter interface {
	IgnoreGenParameter()
}

type WithBindParameters interface {
	EasyGinBindParameters(c *gin.Context) error
}

type RouterGroup struct {
	path        string         // Group path
	children    []*RouterGroup // Group children
	apis        []RouterAPI    // Group APIs
	middlewares []RouterHandler
}

func NewRouterGroup(path string, middlewares ...RouterHandler) *RouterGroup {
	return &RouterGroup{
		path:        path,
		apis:        make([]RouterAPI, 0),
		middlewares: middlewares,
	}
}

func (g *RouterGroup) Path() string {
	return g.path
}

func (g *RouterGroup) RegisterAPI(api RouterAPI) {
	g.apis = append(g.apis, api)
}

func (g *RouterGroup) RegisterGroup(group *RouterGroup) {
	g.children = append(g.children, group)
}
