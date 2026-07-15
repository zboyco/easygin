# server.go / server_options.go 代码索引

## 文件概述

- 包名: `easygin`
- `server.go`: 管理 Gin 引擎、路由注册、HTTP Server 生命周期、pprof 和上下文注入
- `server_options.go`: 定义可选安全、日志、追踪和 HTTP 超时配置
- 便捷入口: `NewServer` 使用 v0.2.0 严格默认值
- 可配置入口: `NewServerWithOptions` 在严格默认值上应用 `ServerOption`

## 核心数据结构

### Server

- `engine`: 当前已提交的 `*gin.Engine`
- `handlerMap`: HTTP 方法和完整路径到 `RouterAPI` 的映射
- `customMiddleware`: Setup 前配置的全局 Gin 中间件
- `contextInjector`: 请求上下文注入函数
- `serviceName`: 日志和 OpenTelemetry 服务名
- `addr`: HTTP 监听地址，空值默认使用 `:80`
- `debug`: 控制 pprof 注册和 easygin 自身调试输出，不修改 Gin 全局模式
- `options`: 服务器可选配置快照
- `httpServer`: Setup 成功后创建的 `*http.Server`
- `prepared` / `settingUp`: 受 `mu` 保护的生命周期状态

### ServerOption

- 签名: `type ServerOption func(*serverOptions) error`
- 所有选项在 `NewServerWithOptions` 构造阶段执行
- nil 选项、负数限制、负数超时和 nil/typed-nil TracerProvider 会返回错误
- 选项应用失败时返回 nil Server，不创建可运行实例

## 构造函数

### NewServer

- 签名: `func NewServer(serviceName, addr string, debug bool) *Server`
- 使用 v0.2.0 严格默认值:
  - 请求体上限为 10 MiB
  - 普通内部错误、5xx `ErrorHttp` 和 Recovery panic 不向客户端暴露详情
  - 默认脱敏常见凭据类 Query 参数
  - 忽略客户端 `x-log-level`，访问日志仅由服务端全局日志级别控制
  - 不自动安装 OpenTelemetry SDK TracerProvider
  - HTTP Server 超时为 `5s / 60s / 60s / 120s`（ReadHeader / Read / Write / Idle）

### NewServerWithOptions

- 签名: `func NewServerWithOptions(serviceName, addr string, debug bool, options ...ServerOption) (*Server, error)`
- 从 v0.2.0 严格默认值开始，按传入顺序应用选项
- 后传入的同类选项覆盖前一个值；查询参数脱敏名称按集合累加

## ServerOption 索引

### WithMaxBodyBytes

- 签名: `func WithMaxBodyBytes(limit int64) ServerOption`
- `limit > 0`: 使用 `http.MaxBytesReader` 限制请求体读取大小
- `limit == 0`: 不限制
- `limit < 0`: 构造失败
- JSON 和 Multipart 绑定超限时返回 HTTP 413

### WithExposeErrorDetails

- 签名: `func WithExposeErrorDetails(enabled bool) ServerOption`
- false 时普通内部错误和所有 Recovery panic 返回通用 500；5xx `ErrorHttp` 保留有效状态码，但消息和描述替换为对应标准 HTTP 文本
- 4xx 业务 `ErrorHttp` 始终保留业务状态码、消息和描述
- Recovery 始终返回 HTTP 500，不把 panic 中的 `ErrorHttp` 转换为业务状态码
- true 时 Recovery 沿用旧版字段结构，通用消息位于 `msg`，panic 文本位于 `desc`

### WithRedactQueryParameters

- 签名: `func WithRedactQueryParameters(names ...string) ServerOption`
- 默认配置已经脱敏以下名称，调用该选项会继续追加:
  - `token`
  - `access_token`
  - `refresh_token`
  - `id_token`
  - `authorization`
  - `api_key`
  - `password`
  - `client_secret`
  - `secret`
  - `signature`
  - `sig`
  - `session`
  - `session_id`
- 调用方提供的名称会去除首尾空格、转换为小写并加入集合
- 该选项只影响访问日志，不修改原始 `http.Request.URL`

### 客户端日志级别请求头

- v0.2.0 删除旧客户端日志级别覆盖选项
- `x-log-level` 始终被忽略，客户端不能降低或禁用访问日志
- 访问日志最终过滤只由服务端 `SetLogLevel` 配置控制

### WithTracerProvider

- 签名: `func WithTracerProvider(provider trace.TracerProvider) ServerOption`
- 将 Provider 注入 `otelgin.Middleware`
- nil interface 和 typed-nil 指针都会在构造阶段被拒绝
- 未配置时使用 OpenTelemetry 全局 Provider；easygin 不再自动安装 SDK Provider
- 未显式配置 Provider 时日志仍正常写入标准错误，只是不包含有效 Trace/Span ID

### WithHTTPTimeouts

- 签名: `func WithHTTPTimeouts(readHeader, read, write, idle time.Duration) ServerOption`
- 依次映射到 `http.Server`:
  - `ReadHeaderTimeout`
  - `ReadTimeout`
  - `WriteTimeout`
  - `IdleTimeout`
- 任一参数为负数时构造失败；零值沿用 Go HTTP Server 的无限制语义
- 校验使用固定顺序，错误结果稳定
- 默认值分别为 `5s`、`60s`、`60s`、`120s`
- SSE、长轮询和大文件流式响应可能需要提高 `WriteTimeout`，或将 write 参数显式设为 `0`

## 生命周期函数

### Run

- 签名: `func (s *Server) Run(groups ...*RouterGroup) error`
- 普通启动流程等价于 `Setup(groups...)` 后调用 `Serve(nil)`
- 命令行第一个参数为 `gen` 时只生成静态参数绑定代码
- 命令行第一个参数为 `openapi` 时只生成 OpenAPI 文档
- 生成错误会原样返回，不启动 HTTP 服务

### Setup

- 签名: `func (s *Server) Setup(groups ...*RouterGroup) error`
- 只注册引擎、中间件和路由，不开始监听
- 不输出“Listening and serving”启动消息
- 并发调用时仅一个调用进入初始化；其他调用返回明确错误
- 初始化使用本地 `gin.Engine` 和本地 `handlerMap`
- 所有步骤成功后一次性提交到 Server，避免失败留下半成品状态
- 路由注册失败或 Gin 初始化 panic 后恢复 `settingUp`，允许修正配置后重试
- 自定义中间件和上下文注入器在 Setup 开始时复制快照
- Setup 成功后再次调用会返回 `server is already prepared`

### Serve

- 签名: `func (s *Server) Serve(listener net.Listener) error`
- 必须在 Setup 成功后调用
- listener 非 nil 时调用 `http.Server.Serve`
- listener 为 nil 时调用 `http.Server.ListenAndServe`
- 启动日志在此阶段输出；外部 Listener 使用 `listener.Addr()` 的真实地址
- 正常 Shutdown 后通常返回 `http.ErrServerClosed`

### Shutdown

- 签名: `func (s *Server) Shutdown(ctx context.Context) error`
- 调用底层 `http.Server.Shutdown` 实现优雅关闭
- Setup 前调用会返回 `server is not prepared`
- 上下文超时或取消错误由标准库返回

## Setup 中间件顺序

1. OpenTelemetry Gin 中间件
2. easygin 访问日志中间件
3. 当前主路由上下文注入
4. 自定义 Recovery
5. 请求体最大字节数限制（启用时）
6. Multipart 临时文件清理
7. 调用方配置的全局 Gin 中间件
8. 调用方配置的 context 注入器
9. 路由组中间件和 API 处理器

该顺序保证日志和追踪覆盖完整请求链，Recovery 能捕获后续处理器 panic，Multipart 清理在 panic 时仍可执行。

## 路由注册

### handleGroup / handleGroupWithOptions

- 递归注册路由组、中间件、API 和子路由组
- 注册前校验 nil、typed-nil、匿名结构体和非结构体处理器
- 重复的 HTTP 方法和完整路径返回错误
- `ANY` 路由使用 `gin.RouterGroup.Any` 注册
- 错误详情策略传递给 API 和 RouterHandler 中间件

### routeFromHandlerMap

- 先按真实 HTTP 方法和完整路径查找
- 未命中时回退到 `ANY` 路由
- 命中结果写入请求 context，供 `RouteFromContext` 获取

## Gin 模式所有权

- Gin 模式是进程级全局状态，不属于单个 Server
- easygin 在包初始化阶段设置兼容的 ReleaseMode 默认值
- `NewServer`、`Setup`、`Serve` 和请求处理过程不会调用 `gin.SetMode`
- 应用可在自身初始化阶段显式选择 Gin 模式，创建 Server 不会覆盖
- 应用不应在服务运行期间并发调用 `gin.SetMode`

## 全局配置函数

### WithGinMiddleware

- 签名: `func (s *Server) WithGinMiddleware(middleware ...gin.HandlerFunc)`
- 将全局 Gin 中间件加入配置列表
- 写入受 Server 互斥锁保护
- 应在 Setup 前调用；Setup 开始后新增配置不会进入当前引擎快照

### WithContextInjector

- 签名: `func (s *Server) WithContextInjector(withContext WithContext) *Server`
- 设置请求 context 注入函数并返回 Server，支持链式调用
- 写入受 Server 互斥锁保护

### WithContextCompose

- 签名: `func WithContextCompose(withContexts ...WithContext) WithContext`
- 按传入顺序组合多个 context 注入函数

## 调试模式

- `debug == true` 时注册 `/debug/pprof` 路由
- `debug` 不修改 Gin 全局模式
- 生产环境应控制 pprof 暴露范围，避免将调试端点直接开放到公网
