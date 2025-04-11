package easygin

import (
	"context"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

var serviceName = "easygin"

// Server 封装gin.Engine，提供端口和调试模式配置
// 负责管理HTTP服务器的生命周期和路由注册
type Server struct {
	engine          *gin.Engine                               // Gin引擎实例
	contextInjector func(ctx context.Context) context.Context // 上下文注入函数

	serviceName string // 服务名称，用于标识追踪器
	addr        string // 监听地址，如":8080"
	debug       bool   // 调试模式标志，影响日志级别和pprof启用
}

// NewServer 创建一个新的Server实例
//
//	serviceName: 服务名称，用于标识追踪器
//	addr: 监听地址，默认为":80"
//	debug: 调试模式，true启用调试功能，false为生产模式
func NewServer(serviceName, addr string, debug bool) *Server {
	s := &Server{
		serviceName: serviceName,
		addr:        addr,
		debug:       debug,
	}

	// 设置默认监听地址
	if s.addr == "" {
		s.addr = ":80"
	}

	// 创建默认的Gin引擎，包含Logger和Recovery中间件
	gin.SetMode(gin.ReleaseMode)
	s.engine = gin.New()

	return s
}

// pprofRegister 注册pprof性能分析路由
// 参数e为要注册pprof路由的RouterGroup
// 注册后可通过/debug/pprof/访问性能分析工具
func pprofRegister(e *gin.RouterGroup) {
	// 注册pprof路由
	debug := e.Group("/debug/pprof")
	debug.GET("/", gin.WrapF(pprof.Index))                               // pprof首页
	debug.GET("/cmdline", gin.WrapF(pprof.Cmdline))                      // 显示程序的命令行参数
	debug.GET("/profile", gin.WrapF(pprof.Profile))                      // CPU分析
	debug.GET("/symbol", gin.WrapF(pprof.Symbol))                        // 查找程序计数器对应的函数
	debug.GET("/trace", gin.WrapF(pprof.Trace))                          // 程序执行跟踪
	debug.GET("/allocs", gin.WrapH(pprof.Handler("allocs")))             // 内存分配情况
	debug.GET("/block", gin.WrapH(pprof.Handler("block")))               // 阻塞分析
	debug.GET("/goroutine", gin.WrapH(pprof.Handler("goroutine")))       // goroutine分析
	debug.GET("/heap", gin.WrapH(pprof.Handler("heap")))                 // 堆内存分析
	debug.GET("/mutex", gin.WrapH(pprof.Handler("mutex")))               // 互斥锁分析
	debug.GET("/threadcreate", gin.WrapH(pprof.Handler("threadcreate"))) // 线程创建分析
	fmt.Println("[EasyGin] GET /debug/pprof")
	fmt.Println("[EasyGin]     pprof")
}

// Run 启动HTTP服务器并注册路由组
// 参数groups为要注册的路由组列表
// 如果命令行参数包含"gen"，则生成参数绑定函数后退出
// 如果命令行参数包含"openapi"，则生成OpenAPI文档后退出
func (s *Server) Run(groups ...*RouterGroup) error {
	args := os.Args
	// 处理生成参数绑定函数的命令
	if len(args) > 1 && args[1] == "gen" {
		GenerateParametersBindFunction(groups...)
		return nil
	}

	// 处理生成OpenAPI文档的命令
	if len(args) > 1 && args[1] == "openapi" {
		GenerateOpenAPI(groups...)
		return nil
	}

	// 创建根路由组
	rootGroup := s.engine.Group("/")

	// 调试模式下注册pprof路由
	if s.debug {
		// 添加pprof接口
		pprofRegister(rootGroup)
	}

	// 添加OpenTelemetry中间件
	rootGroup.Use(otelgin.Middleware(s.serviceName))

	// 添加自定义的日志中间件
	rootGroup.Use(middleLogger())

	// 添加Gin的Recovery中间件
	rootGroup.Use(gin.CustomRecovery(func(c *gin.Context, err any) {
		var e error
		// 记录错误日志
		switch v := err.(type) {
		case error:
			e = v
		default:
			e = fmt.Errorf("%v", err)
		}

		_ = c.Error(e)

		resp := &gin.H{
			"code": http.StatusInternalServerError,
			"msg":  http.StatusText(http.StatusInternalServerError),
			"desc": e.Error(),
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, resp)
	}))

	// 注册上下文注入中间件
	if s.contextInjector != nil {
		rootGroup.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(s.contextInjector(c.Request.Context()))
		})
	}

	// 注册所有路由组
	for _, group := range groups {
		handleGroup(rootGroup, group)
	}

	// 打印JSON请求体验证和默认值设置的状态提示
	println()
	if !HandleBodyJsonOmitEmptyAndDefault() {
		println("[EasyGin] Tips: HandleBodyJsonOmitEmptyAndDefault is false.")
		println("[EasyGin] Tips: The JSON in the request body will not be validated for empty values, and default values will not be set.")
		println("[EasyGin] Tips: If you want to use the validation and default value features, please use easygin.SetHandleBodyJsonOmitEmptyAndDefault to set.")
		println()
	} else {
		println("[EasyGin] Tips: HandleBodyJsonOmitEmptyAndDefault is true.")
		println("[EasyGin] Tips: The JSON in the request body will be validated for empty values and default values will be set.")
		println("[EasyGin] Tips: This feature uses runtime reflection, which may lead to some performance degradation.")
		println()
	}

	// 设置Gin模式为调试模式
	if s.debug {
		gin.SetMode(gin.DebugMode)
	}

	if !s.debug {
		// 打印服务器启动信息
		fmt.Printf("[EasyGin] Listening and serving HTTP on %s\n", s.addr)
	}

	// 启动HTTP服务器
	return s.engine.Run(s.addr)
}

// handleGroup 递归处理路由组，注册中间件和API
// 参数:
//   - e: 父路由组
//   - group: 要处理的路由组
//   - parentMiddlewareNames: 父路由组的中间件名称列表
func handleGroup(e *gin.RouterGroup, group *RouterGroup, parentMiddlewareNames ...string) {
	// 创建当前路由组
	g := e.Group(group.path)
	basePath := g.BasePath()

	middlewareNames := make([]string, 0, len(parentMiddlewareNames)+len(group.middlewares))

	// 添加父路由组的中间件名称
	middlewareNames = append(middlewareNames, parentMiddlewareNames...)

	// 注册中间件
	for _, handler := range group.middlewares {
		// 获取处理器名称
		operatorName := getHandlerName(handler)
		middlewareNames = append(middlewareNames, operatorName)

		if ginHandler, ok := handler.(GinHandler); ok {
			// 处理实现了GinHandler接口的中间件
			g.Use(renderGinHandler(ginHandler, operatorName))
			continue
		}

		// 处理实现了RouterHandler接口的中间件
		g.Use(renderMiddleware(handler, operatorName))
	}

	// 注册API并收集路由信息
	for _, handler := range group.apis {
		// 获取处理器名称
		operatorName := getHandlerName(handler)

		// 获取路由路径，处理可能的双斜杠问题
		path := handler.Path()
		routePath := basePath
		if path != "" {
			if strings.HasPrefix(path, "/") && strings.HasSuffix(basePath, "/") {
				routePath += path[1:] // 如果basePath以/结尾且path以/开头，则去掉path的前导斜杠
			} else if !strings.HasPrefix(path, "/") && !strings.HasSuffix(basePath, "/") {
				routePath += "/" + path // 如果basePath不以/结尾且path不以/开头，则添加/
			} else {
				routePath += path
			}
		}

		// 获取HTTP方法
		method := handler.Method()
		// 获取API描述
		description := getHandlerDescription(handler)

		// 打印路由信息
		if description != "" {
			fmt.Printf("[EasyGin] %s %s %s\n", getShortMethod(method), routePath, description)
		} else {
			fmt.Printf("[EasyGin] %s %s\n", getShortMethod(method), routePath)
		}

		// 打印中间件和处理器
		if len(middlewareNames) > 0 {
			fmt.Printf("[EasyGin]     %s %s\n", strings.Join(middlewareNames, " "), operatorName)
		} else {
			fmt.Printf("[EasyGin]     %s\n", operatorName)
		}

		// 注册路由
		if ginHandler, ok := handler.(GinHandler); ok {
			// 处理实现了GinHandler接口的API
			if handler.Method() == "ANY" {
				// 注册处理所有HTTP方法的路由
				g.Any(handler.Path(), renderGinHandler(ginHandler, operatorName))
			} else {
				// 注册处理特定HTTP方法的路由
				g.Handle(handler.Method(), handler.Path(), renderGinHandler(ginHandler, operatorName))
			}
			continue
		}

		// 处理实现了RouterHandler接口的API
		g.Handle(handler.Method(), handler.Path(), renderAPI(handler, operatorName))
	}

	// 递归处理子路由组，传递当前路由组的中间件名称
	for _, sub := range group.children {
		handleGroup(g, sub, middlewareNames...)
	}
}

// getShortMethod 获取HTTP方法的简短表示
func getShortMethod(method string) string {
	return strings.ToUpper(method)[:3]
}

// getHandlerName 获取处理器的名称
func getHandlerName(handler RouterHandler) string {
	// 使用反射获取处理器的类型名称
	t := reflect.TypeOf(handler)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// 返回包名.结构体名
	return fmt.Sprintf("%s.%s", t.PkgPath()[strings.LastIndex(t.PkgPath(), "/")+1:], t.Name())
}

// getHandlerDescription 获取处理器的描述信息
func getHandlerDescription(handler RouterHandler) string {
	// 尝试从结构体标签中获取summary
	t := reflect.TypeOf(handler)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() == reflect.Struct {
		// 遍历结构体字段
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			fieldType := field.Type

			// 检查字段类型是否实现了Method() string接口
			methodType, ok := fieldType.MethodByName("Method")
			if !ok {
				continue
			}

			// 验证Method方法的签名是否为 Method() string
			if methodType.Type.NumIn() != 1 || methodType.Type.NumOut() != 1 || methodType.Type.Out(0).Kind() != reflect.String {
				continue
			}

			// 检查字段是否有summary标签
			if summary, ok := field.Tag.Lookup("summary"); ok {
				return summary
			}
		}
	}

	return ""
}

// WithGinHandlers 添加全局Gin中间件
// 参数handlers为要添加的Gin中间件列表
func (s *Server) WithGinHandlers(handlers ...gin.HandlerFunc) {
	_ = s.engine.Use(handlers...)
}

// WithContextInjector 设置上下文注入函数
// 参数withContext为上下文注入函数，用于在请求处理前修改上下文
// 返回修改后的Server实例，支持链式调用
func (s Server) WithContextInjector(withContext WithContext) *Server {
	s.contextInjector = withContext
	return &s
}

// WithContext 定义了上下文注入函数类型
// 接收一个上下文并返回修改后的上下文
type WithContext = func(ctx context.Context) context.Context

// WithContextCompose 组合多个上下文注入函数
// 参数withContexts为要组合的上下文注入函数列表
// 返回一个新的上下文注入函数，该函数按顺序应用所有输入函数
func WithContextCompose(withContexts ...WithContext) WithContext {
	return func(ctx context.Context) context.Context {
		for i := range withContexts {
			ctx = withContexts[i](ctx)
		}
		return ctx
	}
}
