package easygin

import (
	"context"
	"net/http/pprof"
	"os"

	"github.com/gin-gonic/gin"
)

// Server 封装gin.Engine，提供端口和调试模式配置
type Server struct {
	engine          *gin.Engine
	contextInjector func(ctx context.Context) context.Context

	addr  string // listen address
	debug bool   // debug mode
}

// NewServer creates a new Server
//
//	addr: listen address, default ":80"
//	debug: debug mode
func NewServer(addr string, debug bool) *Server {
	s := &Server{
		addr:  addr,
		debug: debug,
	}

	if !s.debug {
		gin.SetMode(gin.ReleaseMode)
	}

	if s.addr == "" {
		s.addr = ":80"
	}

	s.engine = gin.Default()

	return s
}

func pprofRegister(e *gin.RouterGroup) {
	// 注册pprof路由
	debug := e.Group("/debug/pprof")
	debug.GET("/", gin.WrapF(pprof.Index))
	debug.GET("/cmdline", gin.WrapF(pprof.Cmdline))
	debug.GET("/profile", gin.WrapF(pprof.Profile))
	debug.GET("/symbol", gin.WrapF(pprof.Symbol))
	debug.GET("/trace", gin.WrapF(pprof.Trace))
	debug.GET("/allocs", gin.WrapH(pprof.Handler("allocs")))
	debug.GET("/block", gin.WrapH(pprof.Handler("block")))
	debug.GET("/goroutine", gin.WrapH(pprof.Handler("goroutine")))
	debug.GET("/heap", gin.WrapH(pprof.Handler("heap")))
	debug.GET("/mutex", gin.WrapH(pprof.Handler("mutex")))
	debug.GET("/threadcreate", gin.WrapH(pprof.Handler("threadcreate")))
}

// Run 启动服务
func (s *Server) Run(groups ...*RouterGroup) error {
	args := os.Args
	if len(args) > 1 && args[1] == "gen" {
		GenerateParametersBindFunction(groups...)
		return nil
	}

	if len(args) > 1 && args[1] == "openapi" {
		GenerateOpenAPI(groups...)
		return nil
	}

	println()
	if !HandleBodyJsonOmitEmptyAndDefault() {
		println("[EasyGin Tips]: HandleBodyJsonOmitEmptyAndDefault is false.")
		println("[EasyGin Tips]: The JSON in the request body will not be validated for empty values, and default values will not be set.")
		println("[EasyGin Tips]: If you want to use the validation and default value features, please use easygin.SetHandleBodyJsonOmitEmptyAndDefault to set.")
		println()
	} else {
		println("[EasyGin Tips]: HandleBodyJsonOmitEmptyAndDefault is true.")
		println("[EasyGin Tips]: The JSON in the request body will be validated for empty values and default values will be set.")
		println("[EasyGin Tips]: This feature uses runtime reflection, which may lead to some performance degradation.")
		println()
	}

	rootGroup := s.engine.Group("/")

	if s.debug {
		// 添加pprof接口
		pprofRegister(rootGroup)
	}

	if s.contextInjector != nil {
		rootGroup.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(s.contextInjector(c.Request.Context()))
		})
	}

	// 添加路由组
	for _, group := range groups {
		handleGroup(rootGroup, group)
	}

	return s.engine.Run(s.addr)
}

func handleGroup(e *gin.RouterGroup, group *RouterGroup) {
	g := e.Group(group.path)
	for _, handler := range group.middlewares {
		if ginHandler, ok := handler.(GinHandler); ok {
			// 兼容gin.HandlerFunc
			g.Use(ginHandler.GinHandle())
			continue
		}

		g.Use(renderMiddleware(handler))
	}

	for _, handler := range group.apis {
		if ginHandler, ok := handler.(GinHandler); ok {
			// 兼容gin.HandlerFunc
			if handler.Method() == "ANY" {
				g.Any(handler.Path(), ginHandler.GinHandle())
			} else {
				g.Handle(handler.Method(), handler.Path(), ginHandler.GinHandle())
			}
			continue
		}

		g.Handle(handler.Method(), handler.Path(), renderAPI(handler))
	}

	for _, sub := range group.children {
		handleGroup(g, sub)
	}
}

func (s *Server) WithGinHandlers(handlers ...gin.HandlerFunc) {
	_ = s.engine.Use(handlers...)
}

func (s Server) WithContextInjector(withContext WithContext) *Server {
	s.contextInjector = withContext
	return &s
}

type WithContext = func(ctx context.Context) context.Context

func WithContextCompose(withContexts ...WithContext) WithContext {
	return func(ctx context.Context) context.Context {
		for i := range withContexts {
			ctx = withContexts[i](ctx)
		}
		return ctx
	}
}
