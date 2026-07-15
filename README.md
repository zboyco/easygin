# easygin
基于Gin框架的增强库，提供自动化参数绑定、路由生成、OpenAPI 文档和项目脚手架功能

## 功能特性

- 📦 支持多种参数来源：
  - Path路径参数
  - Query查询参数
  - Header请求头
  - JSON请求体
  - Multipart表单
- 🔍 自动校验参数必填性
- ⚙️ 支持默认值设置
- 🧩 静态参数绑定支持查询参数切片和 `encoding.TextUnmarshaler` 自定义类型
- 🚀 可选生成静态参数绑定方法，避免使用运行时反射
- 📚 可选生成OpenAPI文档
- 🔗 路由组嵌套支持
- 📁 文件上传下载支持
- 🔄 重定向支持
- 🔒 中间件支持
- 🛠️ 提供 `easygin` CLI 初始化项目骨架

## 快速开始

### 安装
```bash
go get github.com/zboyco/easygin  
```

### 安装 CLI

```bash
go install github.com/zboyco/easygin/cmd/easygin@latest
```

初始化一个新项目：

```bash
easygin new srv-demo --module github.com/acme/srv-demo --service-name srv-demo --router-path /demo
```

`easygin new` 会创建项目目录、写入示例 API/模型/控制器模板，并执行 `go mod init`。可选参数说明：

- `--module`: 指定 Go module 名称；不指定时使用 `go mod init` 的默认推导结果
- `--service-name`: 指定服务名；不指定时使用项目名
- `--router-path`: 指定根路由路径；不指定时，`srv-xxx` 项目默认使用 `/xxx`，其他项目默认使用 `/<project>`

## 使用示例

### 项目结构示例

以下是一个使用easygin框架的项目结构示例：

```
project/
├── main.go                 # 主程序入口
└── apis/                   # API定义目录
    ├── root.go             # 根路由定义
    ├── file/               # 文件相关API
    │   ├── download.go     # 文件下载
    │   ├── image.go        # 图片显示
    │   ├── redirect.go     # URL重定向
    │   ├── root.go         # 文件模块路由组定义
    │   └── upload.go       # 文件上传
    └── user/               # 用户相关API
        ├── create.go       # 创建用户
        ├── get.go          # 获取用户详情
        ├── list.go         # 获取用户列表
        └── root.go         # 用户模块路由组定义
```

在这个结构中：
- `main.go` 是应用程序的入口点，负责创建和启动HTTP服务器
- `apis/root.go` 定义了根路由组和服务路由组，并注册了各个子模块的路由组
- 每个功能模块（如`file`和`user`）都有自己的目录，包含该模块的所有API定义
- 每个模块都有一个`root.go`文件，定义了该模块的路由组
- 每个API处理器都定义在单独的文件中，便于维护和扩展

### 路由组定义

在根模块中定义根路由和服务路由：

```go
package apis

var RouterRoot = easygin.NewRouterGroup("/")
var RouterServer = easygin.NewRouterGroup("/server")
```

在用户模块中定义用户路由组，并添加认证中间件：

```go
package user

var RouterRoot = easygin.NewRouterGroup("/user", &middleware.MustAuth{})
```

在文件模块中定义文件路由组：

```go
package file

var RouterRoot = easygin.NewRouterGroup("/file")
```

### 路由注册

在根模块中注册子路由组和OpenAPI文档路由：
```go
package apis

func init() {
    // 注册健康检查路由
    RouterRoot.RegisterAPI(easygin.NewLivenessRouter("/liveness"))
    // 注册子路由组
    RouterRoot.RegisterGroup(RouterServer)
    // 注册OpenAPI文档路由
    RouterServer.RegisterAPI(easygin.OpenAPIRouter)
    // 注册Swagger UI路由
    RouterServer.RegisterAPI(easygin.NewSwaggerUIRouter(RouterServer.Path()))
    // 注册其他模块路由组
    {
        RouterServer.RegisterGroup(file.RouterRoot)
        RouterServer.RegisterGroup(user.RouterRoot)
    }
}
```

### 参数绑定示例

#### Path/Query/Header 参数绑定

```go
type GetUser struct {
    easygin.MethodGet `summary:"获取用户详情"`
    ID    int    `in:"path" name:"id" desc:"用户ID"`
    Token string `in:"header" name:"token" desc:"认证Token"`
}

func (GetUser) Path() string {
    return "/:id"
}

func (req *GetUser) Output(ctx context.Context) (any, error) {
    // 使用req.ID和req.Token
    if req.Token == "" {
        return nil, easygin.NewError(401, "token is empty", "token is empty")
    }
    if req.ID != 1 {
        return nil, easygin.NewError(404, "user doesn't exist", "request id not equal 1")
    }
    return &RespGetUser{
        ID:   req.ID,
        Name: "someone",
    }, nil
}
```

#### 查询参数列表示例

```go
type ListUser struct {
    easygin.MethodGet `summary:"获取用户列表"`
    Name      string   `in:"query" name:"name,omitempty" desc:"用户名"`
    AgeMin    int      `in:"query" name:"age_min,omitempty" default:"18" desc:"最小年龄"`
    TagIDs    []int    `in:"query" name:"tag_id,omitempty" desc:"标签ID，可重复传入 tag_id=1&tag_id=2"`
    CreatedAt []MyTime `in:"query" name:"created_at,omitempty" desc:"实现 encoding.TextUnmarshaler 的时间切片"`
}

func (ListUser) Path() string {
    return ""
}

func (req *ListUser) Output(ctx context.Context) (any, error) {
    // 使用req.Name和req.AgeMin
    return []RespGetUser{{
        ID:   1,
        Name: "someone",
    }, {
        ID:   2,
        Name: "someone2",
    }}, nil
}
```

查询参数切片会使用同名参数数组解析，例如 `?tag_id=1&tag_id=2`。该能力依赖生成的静态参数绑定方法；未生成静态绑定时，运行时反射绑定按单值参数处理，不支持 query 切片。静态参数绑定支持 `[]T`、`*[]T`、`[]*T`、`*[]*T` 形态，元素类型支持 `string`、`bool`、各整数/浮点类型，以及实现 `encoding.TextUnmarshaler` 的自定义类型（含指针接收者的 `UnmarshalText`）。

API 和中间件处理器可以使用命名结构体值或命名结构体指针注册；easygin 会为每次请求创建新的处理器指针并绑定参数。`nil`、typed nil、匿名结构体和非结构体处理器会在注册或生成阶段返回明确错误。

#### JSON请求体绑定

```go
type CreateUser struct {
    easygin.MethodPost `summary:"创建用户"`
    Body               ReqCreateUser `in:"body"`
}

type ReqCreateUser struct {
    Name string `json:"name" desc:"用户名称"`
    Age  int    `json:"age" desc:"用户年龄"`
}

func (CreateUser) Path() string {
    return ""
}

func (req *CreateUser) Output(ctx context.Context) (any, error) {
    if req.Body.Name == "" {
        return nil, easygin.NewError(400, "name is empty", "name is empty")
    }
    return nil, nil
}
```

JSON 请求体只允许包含一个 JSON 值。无论使用运行时反射绑定还是生成的静态绑定，类似 `{"name":"a"}{"name":"b"}` 的连续 JSON 值都会被拒绝，避免只解析第一个对象而忽略后续内容。

#### 文件上传

```go
type UploadFile struct {
    easygin.MethodPost `summary:"上传文件"`
    Body               *ReqUploadFile `in:"body" mime:"multipart"`
}

type ReqUploadFile struct {
    File   *multipart.FileHeader   `name:"file" desc:"文件"`
    Images []*multipart.FileHeader `name:"images,omitempty" desc:"图片列表"`
}

func (UploadFile) Path() string {
    return "/upload"
}

func (req *UploadFile) Output(ctx context.Context) (any, error) {
    // 处理上传的文件
    fmt.Println(req.Body.File.Filename)
    fmt.Println(len(req.Body.Images))
    return nil, nil
}
```

超过 `SetMultipartMemoryLimit` 的文件内容会由 Go 写入临时目录。easygin 会在完整请求处理链结束后统一删除这些临时文件，因此应在 `Output` 或其同步调用链内完成文件读取，不要在请求结束后继续持有上传文件路径。

#### 文件下载

```go
type Download struct {
    easygin.MethodGet `summary:"下载文件"`
}

func (Download) Path() string {
    return "/download"
}

func (Download) Output(ctx context.Context) (any, error) {
    file, err := os.ReadFile("easygin.png")
    if err != nil {
        return nil, easygin.NewError(500, "open file failed", err.Error())
    }
    return &easygin.AttachmentFromFile{
        Disposition: easygin.DispositionAttachment,
        ContentType: "image/png",
        Filename:    "easygin.png",
        Content:     file,
    }, nil
}
```

#### 图片显示

```go
type Image struct {
    easygin.MethodGet `summary:"显示图片"`
}

func (Image) Path() string {
    return "/image"
}

func (Image) Output(ctx context.Context) (any, error) {
    file, err := os.Open("easygin.png")
    if err != nil {
        return nil, easygin.NewError(500, "open file error", err.Error())
    }

    return &easygin.AttachmentFromReader{
        Disposition:   easygin.DispositionInline,
        ContentType:   "image/png",
        Filename:      "easygin.png",
        ContentLength: -1,
        Reader:        file,
    }, nil
}
```

#### URL重定向

```go
type Redirect struct {
    easygin.MethodGet `summary:"重定向"`
    Url string `in:"query" name:"url"`
}

func (Redirect) Path() string {
    return "/redirect"
}

func (req *Redirect) Output(c context.Context) (any, error) {
    u, err := url.Parse(req.Url)
    if err != nil {
        return nil, easygin.NewError(400, "Invalid Parameters", err.Error())
    }
    return u, nil
}
```

### 响应定义

```go
func (GetUser) Responses() easygin.R {
    return easygin.R{
        200: &RespGetUser{},
        401: &easygin.Error{},
        404: &easygin.Error{},
    }
}
```

### 启动服务

```go
package main

import (
    "github.com/zboyco/easygin"
    "github.com/zboyco/easygin/example/apis"
)

func main() {
    serviceName := "srv-example"

    // 设置日志等级为DebugLevel
    easygin.SetLogLevel(easygin.DebugLevel)

    // 创建服务器，指定服务名称、端口和是否启用调试模式
    srv := easygin.NewServer(serviceName, ":8080", true)
    
    // 运行服务，注册根路由组
    if err := srv.Run(apis.RouterRoot); err != nil {
        panic(err)
    }
}
```

日志与 OpenTelemetry 配置相互独立。未显式安装 SDK Provider 时，访问日志和 `logr.FromContext` 获取的业务日志仍会以 zerolog JSON 写入标准错误，只是不包含 `traceID`、`spanID`，也不会导出 Trace。

需要 Trace 时，由应用显式创建 Exporter 并管理 Provider 生命周期：

```go
// exporter 由应用按实际后端创建，例如 OTLP Exporter。
provider := easygin.InitGlobalTracerProvider(serviceName, exporter)
defer func() {
    _ = provider.Shutdown(context.Background())
}()
```

`InitGlobalTracerProvider` 只安装显式传入的有效 Exporter，nil 和 typed-nil Exporter 会被忽略；它不再附加标准输出 Exporter，也不会关闭此前安装的 Provider。调用方负责在退出前调用 `ForceFlush` 或 `Shutdown`。已经自行配置 OpenTelemetry SDK 的应用无需调用该辅助函数，可以通过 `WithTracerProvider` 为单个 Server 指定 Provider。

### 服务器安全配置与生命周期

从 v0.2.0 开始，`NewServer` 和 `NewServerWithOptions` 使用相同的严格默认值：

| 配置 | 默认值 |
| --- | --- |
| 请求体上限 | 10 MiB |
| 普通错误、5xx 和 panic 详情 | 不返回给客户端 |
| 访问日志 Query 脱敏 | 默认启用 |
| 客户端访问日志级别覆盖 | 不支持，忽略 `x-log-level` |
| HTTP 超时 | `5s / 60s / 60s / 120s`（ReadHeader / Read / Write / Idle） |
| OpenTelemetry SDK Provider | 不自动安装 |

需要按部署场景调整默认值时，使用 `NewServerWithOptions`：

```go
package main

import (
    "time"

    "github.com/zboyco/easygin"
)

func newProductionServer(serviceName string) (*easygin.Server, error) {
    return easygin.NewServerWithOptions(
        serviceName,
        ":8080",
        false,
        // 将默认 10 MiB 请求体上限调整为 20 MiB。
        easygin.WithMaxBodyBytes(20*1024*1024),
        // 在默认敏感参数集合上追加业务字段。
        easygin.WithRedactQueryParameters("tenant_secret"),
        // 可选：为 Gin OpenTelemetry 中间件注入指定 Provider。
        // easygin.WithTracerProvider(provider),
        easygin.WithHTTPTimeouts(5*time.Second, 60*time.Second, 60*time.Second, 120*time.Second),
    )
}
```

`WithMaxBodyBytes` 同时限制 JSON 和 Multipart 请求。请求在绑定阶段超过限制时返回 `413 Request Entity Too Large`，不会继续执行 API；Multipart 解析产生的临时文件会在请求处理链结束后自动清理。传入 `WithMaxBodyBytes(0)` 可以显式关闭 Body 上限。

默认错误策略会隐藏普通错误、所有 5xx `ErrorHttp` 和 panic 文本；有效的 5xx 状态码会保留，响应内容改为对应的标准 HTTP 文本。4xx `ErrorHttp` 仍按业务状态码、消息和描述输出。只有明确接受泄露风险时才使用 `WithExposeErrorDetails(true)` 恢复详情。

`WithRedactQueryParameters` 只影响访问日志，不修改 `http.Request.URL`，并且只会向默认集合追加字段。默认集合包含 `token`、`access_token`、`refresh_token`、`id_token`、`authorization`、`api_key`、`password`、`client_secret`、`secret`、`signature`、`sig`、`session` 和 `session_id`。脱敏按原始 Query 字段执行，敏感值包含未转义分号等畸形内容时也不会回退输出原始 URI。访问日志是否输出只由服务端 `SetLogLevel` 控制，客户端传入 `x-log-level` 不会生效。

默认 `WriteTimeout` 为 60 秒。SSE、长轮询、大文件下载等可能持续写响应的接口，应按最长响应时间提高该值，或通过 `WithHTTPTimeouts` 将 `write` 参数设为 `0`；同时仍建议在业务上下文中设置取消和超时边界。

如果需要自行管理监听器、测试服务或实现优雅关闭，可以拆开 `Setup`、`Serve` 和 `Shutdown`：

```go
import (
    "context"
    "errors"
    "net"
    "net/http"
    "time"

    "github.com/zboyco/easygin/example/apis"
)

func runManagedServer() error {
    srv, err := newProductionServer("srv-example")
    if err != nil {
        return err
    }
    if err := srv.Setup(apis.RouterRoot); err != nil {
        return err
    }

    listener, err := net.Listen("tcp", ":8080")
    if err != nil {
        return err
    }

    serveErr := make(chan error, 1)
    go func() {
        serveErr <- srv.Serve(listener)
    }()

    // 收到退出信号后使用带超时的上下文优雅关闭。
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    if err := srv.Shutdown(shutdownCtx); err != nil {
        return err
    }

    err = <-serveErr
    if errors.Is(err, http.ErrServerClosed) {
        return nil
    }
    return err
}
```

`Setup` 只完成路由和 HTTP Server 准备，不会开始监听或输出“Listening and serving”消息。`Serve(nil)` 使用构造时配置的地址；传入外部 Listener 时，启动日志记录 `listener.Addr()` 返回的真实地址。

`NewServer` 是无需额外选项的便捷入口，使用上述 v0.2.0 严格默认值；`NewServerWithOptions` 在同一组默认值上应用显式覆盖。Recovery 始终返回 HTTP 500，默认不会把 panic 文本写入响应。

### 从 v0.1.x 迁移到 v0.2.0

v0.2.0 是安全默认值和 API 契约的断代版本，升级时需要检查以下变化：

| 变化 | v0.2.0 行为 | 必要的迁移动作 |
| --- | --- | --- |
| Body 限制 | 默认 10 MiB，超限返回 413 | 大请求提高 `WithMaxBodyBytes`；确需无限制时显式传 `0` |
| 错误详情 | 默认隐藏普通错误、5xx 和 panic 详情 | 客户端不要依赖内部错误文本；仅在受控环境使用 `WithExposeErrorDetails(true)` |
| 访问日志 Query | 默认脱敏常见凭据字段 | 自定义敏感字段通过 `WithRedactQueryParameters` 追加 |
| 日志级别请求头 | `x-log-level` 始终忽略，旧客户端覆盖选项已删除 | 改由服务端 `SetLogLevel` 或部署配置控制 |
| HTTP 超时 | 默认 `5s / 60s / 60s / 120s` | 检查上传、SSE、长轮询和大文件下载；必要时覆盖对应超时 |
| OpenTelemetry | 不再包级自动初始化，不再提供内置标准输出 Exporter | 应用显式安装 Exporter/Provider，并负责 Flush/Shutdown；无需 Trace 时不必配置 |
| `ErrorHttp` | 只接受 400-599，支持包装错误，typed nil 和非法状态降级为 500 | 修正使用 2xx、3xx 或 600 以上状态码的自定义错误 |
| `WithStatusCode` | 只接受 200-599，nil 指针和非法状态降级为 500 | 修正 1xx、600 以上和 nil 包装响应 |

所有框架错误响应固定使用 `{"code": number, "msg": string, "desc": string}`。404、参数绑定错误、Recovery 和非法状态码都遵守该结构；业务 4xx `ErrorHttp` 仍保留其消息和描述。

Gin 的运行模式是进程级全局状态，并不属于单个 `gin.Engine`。easygin 在包初始化阶段设置兼容的 ReleaseMode 默认值，之后 `NewServer`、`Setup` 和请求处理过程都不会再调用 `gin.SetMode`；应用可以在自身初始化阶段显式选择 DebugMode、ReleaseMode 或 TestMode，创建 Server 不会覆盖该选择。应用仍应避免在服务运行期间并发调用 `gin.SetMode`；`debug` 参数只控制 easygin 的 pprof 注册和自身调试输出。

## 高级特性

### 参数标签说明

- `in`: 参数来源，支持 "path", "query", "header", "body"
- `name`: 参数名称，支持添加 ",omitempty" 后缀表示可选参数
- `default`: 参数默认值，当参数缺失且设置了"omitempty"时使用；显式传入空字符串不会触发默认值
- `desc`: 参数描述，用于生成OpenAPI文档
- `mime`: 用于 body 参数，指定 MIME 类型，支持 "multipart" 表示表单上传

### 必填、空值与默认值语义

easygin 对参数必填性的判断基于 **presence**：只判断客户端是否提供了该字段或参数，不把显式零值当作缺失。

- 未添加 `omitempty` 的字段为必填；添加 `omitempty` 后允许缺失。
- `default` 只在字段或参数缺失时生效；显式传入空字符串、`0`、`false` 等值不会触发默认值。
- `?name=`、空 header、multipart/form 中的 `name=` 会被视为已提供；如果目标类型是 `string` 或 `*string`，会绑定为空字符串。
- `?age=` 这类空字符串绑定到 `int`、`bool`、`time.Time` 等非字符串类型时，会通过必填检查，但在类型转换阶段返回错误。
- JSON body 中显式提供的 `0`、`false`、`""` 会被视为已提供；JSON `null` 仍按缺失处理。

### Multipart 表单内存限制

easygin 支持设置 Multipart 表单的内存限制，用于控制文件上传时的内存使用量：

```go
// 设置 Multipart 表单的内存限制（字节）
easygin.SetMultipartMemoryLimit(50 * 1024 * 1024) // 设置为 50MB

// 获取当前的 Multipart 表单内存限制
limit := easygin.GetMultipartMemoryLimit()
```

默认情况下，Multipart 表单的内存限制为 100MB。可以通过 `SetMultipartMemoryLimit` 函数进行调整，参数为字节大小。该函数是并发安全的，可以在程序运行时动态调整。

> 注意：该配置同时作用于运行时反射绑定和当前生成的静态 multipart 绑定代码。

### JSON 参数标签处理

easygin 支持控制是否处理 JSON 请求体中的 `omitempty` 和 `default` 标签：

```go
// 设置是否处理 JSON 请求体中的 omitempty 和 default 标签
easygin.SetHandleBodyJsonOmitEmptyAndDefault(true)

// 获取当前的设置状态
enabled := easygin.HandleBodyJsonOmitEmptyAndDefault()
```

默认情况下，此功能是关闭的（`false`），因为启用后会使用反射处理 JSON 标签，会对性能产生一定影响。启用后，系统会：

- 处理 `omitempty` 标签：标记字段为可选，不校验是否缺失。如果没有 `omitempty` 标签，则该字段为必填；显式传入零值或空字符串会被视为已提供，JSON `null` 仍按缺失处理
- 处理 `default` 标签：当字段未提供时，使用标签中指定的默认值

该功能是并发安全的，可以在程序运行时动态调整。

### 错误处理

easygin 提供了统一的错误处理机制：

```go
return nil, easygin.NewError(404, "user not found", "detailed error message")
```

第一个参数是HTTP状态码，第二个参数是错误标题，第三个参数是详细错误信息。

### 中间件支持

easygin 支持在路由组级别添加中间件，中间件会应用到该路由组及其所有子路由：

```go
// 定义中间件
type MustAuth struct{}

func (MustAuth) Method() string {
    return "ANY"
}

func (MustAuth) Path() string {
    return ""
}

func (m *MustAuth) Output(ctx context.Context) (any, error) {
    // 从请求头获取token
    token := ctx.Value("token")
    if token == nil || token.(string) == "" {
        return nil, easygin.NewError(401, "Unauthorized", "token is required")
    }
    // 验证通过，继续处理请求
    return nil, nil
}

// 在路由组定义时添加中间件
var RouterUser = easygin.NewRouterGroup("/user", &middleware.MustAuth{})
```

中间件会按照注册顺序执行，可以注册多个中间件：

```go
var RouterUser = easygin.NewRouterGroup("/user", &middleware.MustAuth{}, &middleware.Logger{})
```   

### 上下文和路由信息

easygin 会在请求上下文中写入处理器名称、`gin.Context` 和当前主路由信息：

```go
func (req *GetUser) Output(ctx context.Context) (any, error) {
    ginCtx := easygin.GinContextFromContext(ctx)
    route := easygin.RouteFromContext(ctx)
    handlerName := easygin.HandlerNameFromContext(ctx)
    _ = ginCtx
    _ = route
    _ = handlerName
    return nil, nil
}
```

`RouteFromContext` 会在上下文值不存在或类型不匹配时返回 `nil`，不会触发 panic。对于 `MethodAny`/`ANY` 路由，请求使用真实 HTTP 方法访问时也会回退匹配到对应的 ANY 路由。

### 生成静态参数绑定方法

为了避免运行时反射带来的性能开销，easygin 提供了生成静态参数绑定方法的功能：

```go
// 在项目开发时调用
go run main.go gen
```

这将为每个包生成 `zz_easygin_generated.go` 文件，包含静态的参数绑定方法。   

匿名嵌入指针会在生成代码中逐层初始化，并使用完整字段路径绑定，行为与运行时反射绑定一致。生成内容会先经过 `gofmt` 语法格式化，再通过同目录临时文件原子替换旧文件；生成失败不会覆盖上一份有效产物。

生成期间如果遇到静态绑定暂不支持的参数类型，命令会直接返回错误并以非 0 状态退出，不会把 `unsupported` 分支写入生成文件。例如不支持的 query 切片类型会输出类似：

```text
[EasyGin] generate parameters bind failed: failed to generate file for package ...: ListUser: unsupported query slice parameter type "[]struct {}" for field r.Unsupported
```

![静态参数绑定方法生成演示](https://raw.githubusercontent.com/zboyco/easygin/main/example/gen.gif)   

> 注意：生成静态方法需要在项目开发时手动调用，即先运行 `go run . gen` 命令，然后再运行程序，因为生成的代码需要参与编译过程。   

> 内部实际调用了`easygin.GenerateParametersBindFunction`方法。  
   
> 如果不生成静态方法，在运行时会使用反射来解析参数。   

### 生成OpenAPI文档

easygin 提供了生成OpenAPI文档的功能：

```go
// 在项目开发时调用
go run main.go openapi
```

这将在当前目录下生成 `openapi.json` 文件。写盘前会执行 OpenAPI 3.0.3 规范校验，校验失败时保留上一份有效文档。

![OpenAPI文档生成演示](https://raw.githubusercontent.com/zboyco/easygin/main/example/openapi.gif)   

> 内部实际调用了`easygin.GenerateOpenAPI`方法，该方法使用反射实现，有一定的耗时，可以根据需要在程序运行前手动生成，也可以在运行时自动生成文档，建议提前生成。

OpenAPI Schema 会把实现 `encoding.TextUnmarshaler` 的类型映射为 `string`，`time.Time` 及其别名映射为 `string` + `date-time`，`multipart.FileHeader` 及其别名映射为 `string` + `binary`。结构体组件会复用 `components/schemas`，循环引用会使用标准 `$ref` 避免无限展开；匿名嵌入指针会展开其字段。请求体和路径参数按运行时语义标记为必填，`ANY` 路由展开后会为每种 HTTP 方法生成唯一 operationId。

### 文件处理

easygin 支持两种文件返回方式：

1. 从[]byte返回：

```go
return &easygin.AttachmentFromFile{
    Disposition: easygin.DispositionAttachment, // 或 DispositionInline
    ContentType: "image/png",
    Filename:    "easygin.png",
    Content:     fileBytes,
}, nil
```

2. 从io.Reader返回：

```go
return &easygin.AttachmentFromReader{
    Disposition:   easygin.DispositionInline,
    ContentType:   "image/png",
    Filename:      "easygin.png",
    ContentLength: -1, // -1表示不指定长度
    Reader:        fileReader,
}, nil
```

文件响应会自动设置 `Content-Disposition`。文件名会通过 MIME 参数规则进行转义，包含空格、引号或非 ASCII 字符时会生成兼容的 `filename*` 参数。
