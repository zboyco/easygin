package easygin

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"reflect"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

// init 在应用进入并发运行前设置兼容默认模式；调用方可在初始化阶段显式覆盖。
func init() {
	gin.SetMode(gin.ReleaseMode)
}

// Server 封装gin.Engine，提供端口和调试模式配置
// 负责管理HTTP服务器的生命周期和路由注册
type Server struct {
	engine           *gin.Engine                               // Gin引擎实例
	handlerMap       map[string]RouterAPI                      // 路由处理器映射
	customMiddleware []gin.HandlerFunc                         // 自定义中间件列表
	contextInjector  func(ctx context.Context) context.Context // 上下文注入函数

	serviceName string // 服务名称，用于标识追踪器
	addr        string // 监听地址，如":8080"
	debug       bool   // 调试模式标志，影响日志级别和pprof启用

	options serverOptions

	mu         sync.Mutex
	httpServer *http.Server
	prepared   bool
	settingUp  bool
}

// NewServer 创建一个新的Server实例
//
//	serviceName: 服务名称，用于标识追踪器
//	addr: 监听地址，默认为":80"
//	debug: 调试模式，true启用调试功能，false为生产模式
func NewServer(serviceName, addr string, debug bool) *Server {
	return newServer(serviceName, addr, debug, defaultServerOptions())
}

// NewServerWithOptions 创建带可选安全、日志、追踪和生命周期配置的 Server。
// 配置应用失败时返回错误，且不会创建可运行的服务器实例。
func NewServerWithOptions(serviceName, addr string, debug bool, options ...ServerOption) (*Server, error) {
	serverOptions := defaultServerOptions()
	for _, option := range options {
		if option == nil {
			return nil, fmt.Errorf("server option is nil")
		}
		if err := option(&serverOptions); err != nil {
			return nil, err
		}
	}
	return newServer(serviceName, addr, debug, serverOptions), nil
}

// newServer 创建并初始化内部 Gin 引擎。
func newServer(serviceName, addr string, debug bool, options serverOptions) *Server {
	s := &Server{
		serviceName:      serviceName,
		addr:             addr,
		debug:            debug,
		customMiddleware: make([]gin.HandlerFunc, 0),
		options:          options,
	}

	// 设置默认监听地址
	if s.addr == "" {
		s.addr = ":80"
	}

	// 创建默认的Gin引擎，包含Logger和Recovery中间件。
	s.engine = gin.New()

	return s
}

// pprofRegister 注册pprof性能分析路由
// 参数e为要注册pprof路由的RouterGroup
// 注册后可通过/debug/pprof/访问性能分析工具
func pprofRegister(e *gin.Engine) {
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

// Run 注册路由组并启动 HTTP 服务器。
// 命令行参数为 gen 或 openapi 时只执行对应生成任务，不启动监听器。
func (s *Server) Run(groups ...*RouterGroup) error {
	args := os.Args
	if len(args) > 1 && args[1] == "gen" {
		if err := GenerateParametersBindFunction(groups...); err != nil {
			fmt.Fprintf(os.Stderr, "[EasyGin] generate parameters bind failed: %v\n", err)
			return err
		}
		return nil
	}
	if len(args) > 1 && args[1] == "openapi" {
		if err := GenerateOpenAPI(groups...); err != nil {
			fmt.Fprintf(os.Stderr, "[EasyGin] generate OpenAPI failed: %v\n", err)
			return err
		}
		return nil
	}
	if err := s.Setup(groups...); err != nil {
		return err
	}
	return s.Serve(nil)
}

// Setup 注册路由和中间件，但不开始监听，适合自定义 Listener、测试和优雅关闭。
func (s *Server) Setup(groups ...*RouterGroup) error {
	s.mu.Lock()
	if s.prepared {
		s.mu.Unlock()
		return fmt.Errorf("server is already prepared")
	}
	if s.settingUp {
		s.mu.Unlock()
		return fmt.Errorf("server setup is already in progress")
	}
	s.settingUp = true
	handlerMap := make(map[string]RouterAPI)
	customMiddleware := append([]gin.HandlerFunc(nil), s.customMiddleware...)
	contextInjector := s.contextInjector
	s.mu.Unlock()

	// 在任何初始化异常下恢复 Setup 状态，避免后续调用永久停留在 settingUp。
	setupCommitted := false
	defer func() {
		if setupCommitted {
			return
		}
		s.mu.Lock()
		s.settingUp = false
		s.mu.Unlock()
	}()

	engine := gin.New()

	// 在本地引擎上完成全部注册，成功后再一次性提交到 Server，避免失败或并发 Setup 污染运行中的状态。
	// 调试模式下注册pprof路由
	if s.debug {
		// 添加pprof接口
		pprofRegister(engine)
	}

	// 添加OpenTelemetry中间件；未配置时继续使用全局 Provider。
	var otelOptions []otelgin.Option
	if s.options.tracerProvider != nil {
		otelOptions = append(otelOptions, otelgin.WithTracerProvider(s.options.tracerProvider))
	}
	engine.Use(otelgin.Middleware(s.serviceName, otelOptions...))

	// 添加日志中间件
	engine.Use(middleLogger(s.serviceName, s.options))

	// 添加404处理
	engine.NoRoute(func(c *gin.Context) {
		resp := &gin.H{
			"code": http.StatusNotFound,
			"msg":  "404 page not found",
			"desc": "404 page not found",
		}
		c.AbortWithStatusJSON(http.StatusNotFound, resp)
	})

	// 添加请求主路由到上下文中
	engine.Use(func(c *gin.Context) {
		// 定位到主处理函数
		mainHandler := routeFromHandlerMap(handlerMap, c.Request.Method, c.FullPath())
		if mainHandler != nil {
			// 将路由添加到上下文中，以便后续处理函数使用
			c.Request = c.Request.WithContext(ContextWithRoute(c.Request.Context(), mainHandler))
		}
	})

	// 添加Gin的Recovery中间件
	engine.Use(gin.CustomRecovery(func(c *gin.Context, err any) {
		var e error
		// 记录错误日志
		switch v := err.(type) {
		case error:
			e = v
		default:
			e = fmt.Errorf("%v", err)
		}

		handleRecoveryErrorWithPolicy(c, e, s.options.exposeErrorDetails)
	}))

	if s.options.maxBodyBytes > 0 {
		engine.Use(maxBodyBytes(s.options.maxBodyBytes))
	}

	// 在整条请求处理链结束后清理 multipart 临时文件。
	engine.Use(multipartFormCleanup())

	// 注册自定义的中间件
	if len(customMiddleware) > 0 {
		engine.Use(customMiddleware...)
	}

	// 注册上下文注入中间件
	if contextInjector != nil {
		engine.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(contextInjector(c.Request.Context()))
		})
	}

	// 注册所有路由组
	for _, group := range groups {
		if err := handleGroupWithOptions(handlerMap, &engine.RouterGroup, group, s.options); err != nil {
			return err
		}
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

	httpServer := &http.Server{
		Addr:              s.addr,
		Handler:           engine,
		ReadHeaderTimeout: s.options.readHeaderTimeout,
		ReadTimeout:       s.options.readTimeout,
		WriteTimeout:      s.options.writeTimeout,
		IdleTimeout:       s.options.idleTimeout,
	}

	s.mu.Lock()
	s.engine = engine
	s.handlerMap = handlerMap
	s.httpServer = httpServer
	s.prepared = true
	s.settingUp = false
	setupCommitted = true
	s.mu.Unlock()
	return nil
}

// Serve 使用已完成 Setup 的路由启动 HTTP 服务；listener 非 nil 时使用外部监听器。
func (s *Server) Serve(listener net.Listener) error {
	s.mu.Lock()
	server := s.httpServer
	prepared := s.prepared
	debug := s.debug
	addr := s.addr
	s.mu.Unlock()
	if !prepared || server == nil {
		return fmt.Errorf("server is not prepared; call Setup first")
	}
	if listener != nil {
		if !debug {
			fmt.Printf("[EasyGin] Listening and serving HTTP on %s\n", listener.Addr())
		}
		return server.Serve(listener)
	}
	if !debug {
		fmt.Printf("[EasyGin] Listening and serving HTTP on %s\n", addr)
	}
	return server.ListenAndServe()
}

// Shutdown 优雅关闭已准备或运行中的 HTTP 服务。
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	server := s.httpServer
	s.mu.Unlock()
	if server == nil {
		return fmt.Errorf("server is not prepared")
	}
	return server.Shutdown(ctx)
}

// handleGroup 递归处理路由组，注册中间件和API
// 参数:
//   - e: 父路由组
//   - group: 要处理的路由组
//   - parentMiddlewareNames: 父路由组的中间件名称列表
func handleGroup(handlerMap map[string]RouterAPI, e *gin.RouterGroup, group *RouterGroup, parentMiddlewareNames ...string) error {
	return handleGroupWithOptions(handlerMap, e, group, defaultServerOptions(), parentMiddlewareNames...)
}

// handleGroupWithOptions 递归注册路由，并将服务器错误策略传递给各处理器。
func handleGroupWithOptions(handlerMap map[string]RouterAPI, e *gin.RouterGroup, group *RouterGroup, options serverOptions, parentMiddlewareNames ...string) error {
	if group == nil {
		return fmt.Errorf("router group is nil")
	}

	// 创建当前路由组
	g := e.Group(group.path)
	basePath := g.BasePath()

	middlewareNames := make([]string, 0, len(parentMiddlewareNames)+len(group.middlewares))

	// 添加父路由组的中间件名称
	middlewareNames = append(middlewareNames, parentMiddlewareNames...)

	// 注册中间件
	for _, handler := range group.middlewares {
		// 获取处理器名称
		handlerName, err := getHandlerName(handler)
		if err != nil {
			return fmt.Errorf("invalid middleware in group %q: %w", basePath, err)
		}
		middlewareNames = append(middlewareNames, handlerName)

		if ginHandler, ok := handler.(GinHandler); ok {
			// 处理实现了GinHandler接口的中间件
			g.Use(renderGinHandler(ginHandler, handlerName))
			continue
		}

		// 处理实现了RouterHandler接口的中间件
		g.Use(renderMiddlewareWithPolicy(handler, handlerName, options.exposeErrorDetails))
	}

	// 注册API并收集路由信息
	for i := range group.apis {
		handler := group.apis[i]
		// 获取处理器名称
		handlerName, err := getHandlerName(handler)
		if err != nil {
			return fmt.Errorf("invalid API in group %q: %w", basePath, err)
		}

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
		description, err := getHandlerDescription(handler)
		if err != nil {
			return fmt.Errorf("read API description for %s: %w", handlerName, err)
		}

		// 打印路由信息
		if description != "" {
			fmt.Printf("[EasyGin] %s %s %s\n", getShortMethod(method), routePath, description)
		} else {
			fmt.Printf("[EasyGin] %s %s\n", getShortMethod(method), routePath)
		}

		// 收集路由信息
		key := fmt.Sprintf("%s %s", method, routePath)
		if _, ok := handlerMap[key]; ok {
			return fmt.Errorf("duplicate route %s %s", method, routePath)
		}
		handlerMap[key] = handler

		// 打印中间件和处理器
		if len(middlewareNames) > 0 {
			fmt.Printf("[EasyGin]     %s %s\n", strings.Join(middlewareNames, " "), handlerName)
		} else {
			fmt.Printf("[EasyGin]     %s\n", handlerName)
		}

		// 注册路由
		if ginHandler, ok := handler.(GinHandler); ok {
			// 处理实现了GinHandler接口的API
			if handler.Method() == "ANY" {
				// 注册处理所有HTTP方法的路由
				g.Any(handler.Path(), renderGinHandler(ginHandler, handlerName))
			} else {
				// 注册处理特定HTTP方法的路由
				g.Handle(handler.Method(), handler.Path(), renderGinHandler(ginHandler, handlerName))
			}
			continue
		}

		// 处理实现了RouterHandler接口的API
		if handler.Method() == "ANY" {
			g.Any(handler.Path(), renderAPIWithPolicy(handler, handlerName, options.exposeErrorDetails))
			continue
		}
		g.Handle(handler.Method(), handler.Path(), renderAPIWithPolicy(handler, handlerName, options.exposeErrorDetails))
	}

	// 递归处理子路由组，传递当前路由组的中间件名称
	for _, sub := range group.children {
		if err := handleGroupWithOptions(handlerMap, g, sub, options, middlewareNames...); err != nil {
			return err
		}
	}

	return nil
}

// routeFromHandlerMap 按请求方法查找路由，未命中时回退到 ANY 路由。
func routeFromHandlerMap(handlerMap map[string]RouterAPI, method, fullPath string) RouterAPI {
	mainHandler := handlerMap[fmt.Sprintf("%s %s", method, fullPath)]
	if mainHandler == nil {
		mainHandler = handlerMap[fmt.Sprintf("ANY %s", fullPath)]
	}
	return mainHandler
}

// getShortMethod 获取HTTP方法的简短表示
func getShortMethod(method string) string {
	method = strings.ToUpper(method)
	if len(method) <= 3 {
		return method
	}
	return method[:3]
}

// getHandlerName 获取处理器的名称
func getHandlerName(handler RouterHandler) (string, error) {
	t, err := routerHandlerStructType(handler)
	if err != nil {
		return "", err
	}

	// 返回包名.结构体名
	return fmt.Sprintf("%s.%s", t.PkgPath()[strings.LastIndex(t.PkgPath(), "/")+1:], t.Name()), nil
}

// getHandlerDescription 获取处理器的描述信息
func getHandlerDescription(handler RouterHandler) (string, error) {
	// 尝试从结构体标签中获取summary
	t, err := routerHandlerStructType(handler)
	if err != nil {
		return "", err
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
				return summary, nil
			}
		}
	}

	return "", nil
}

// WithGinMiddleware 添加全局Gin中间件
// 参数middlewares为要添加的Gin中间件列表
func (s *Server) WithGinMiddleware(middleware ...gin.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.customMiddleware = append(s.customMiddleware, middleware...)
}

// WithContextInjector 设置上下文注入函数
// 参数withContext为上下文注入函数，用于在请求处理前修改上下文
// 返回修改后的Server实例，支持链式调用
func (s *Server) WithContextInjector(withContext WithContext) *Server {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.contextInjector = withContext
	return s
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
