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

    // 初始化全局跟踪器，指定服务名称，用于链路追踪和日志记录
    easygin.InitGlobalTracerProvider(serviceName)

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
> easygin 内部使用了 OpenTelemetry 进行链路追踪和日志记录，默认会初始化服务名为"easygin"的全局跟踪器。  
> 如果需要自定义服务名称，可以使用 `easygin.InitGlobalTracerProvider` 方法进行初始化。  
> `easygin.StdoutSpanExporter()` 方法用于创建一个标准输出的SpanExporter，用于将追踪信息输出到控制台。
> 如果不使用`easygin.InitGlobalTracerProvider`，可以自定义全局跟踪器的配置，例如指定Trace标准、采样率、采样策略等。

### 服务器安全配置与生命周期

需要在生产环境收紧请求体、错误响应和访问日志时，可以使用 `NewServerWithOptions`：

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
        // 限制 JSON 和 Multipart 请求体；超限请求返回 HTTP 413。
        easygin.WithMaxBodyBytes(10*1024*1024),
        // 普通错误和 5xx easygin.Error 隐藏详情；5xx 状态码及非 5xx 业务响应保持不变。
        easygin.WithExposeErrorDetails(false),
        // 无参数时默认移除 token、access_token、authorization、api_key、password。
        // 也可以传入额外的参数名，例如 WithRedactQueryParameters("signature")。
        easygin.WithRedactQueryParameters(),
        // 不允许客户端使用 x-log-level 请求头改变访问日志级别。
        easygin.WithAllowClientLogLevel(false),
        // 可选：为 Gin OpenTelemetry 中间件注入指定 Provider。
        // easygin.WithTracerProvider(provider),
        easygin.WithHTTPTimeouts(5*time.Second, 15*time.Second, 30*time.Second, 60*time.Second),
    )
}
```

`WithMaxBodyBytes` 同时限制 JSON 和 Multipart 请求。请求在绑定阶段超过限制时返回 `413 Request Entity Too Large`，不会继续执行 API；Multipart 解析产生的临时文件会在请求处理链结束后自动清理。`WithExposeErrorDetails(false)` 会隐藏普通错误、所有 5xx `easygin.Error` 和 panic 文本；有效的 5xx 状态码会保留，响应内容改为对应的标准 HTTP 文本。非 5xx `easygin.Error` 仍按业务状态码、消息和描述输出。

`WithRedactQueryParameters` 只影响访问日志，不会修改 `http.Request.URL`。调用时不传参数会启用常见敏感字段默认列表；传入参数时会在该列表基础上加入自定义字段。日志级别覆盖默认允许 `x-log-level` 请求头，生产环境建议通过 `WithAllowClientLogLevel(false)` 关闭。

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

`NewServer` 保留旧版本兼容默认值：不限制请求体大小、允许返回普通错误和 5xx `ErrorHttp` 详情、允许客户端日志级别覆盖、不主动注入追踪 Provider，HTTP 超时保持零值。Recovery 始终返回 HTTP 500；兼容模式下 `msg` 保持 `Internal Server Error`，panic 文本位于 `desc`。已有项目无需修改即可继续使用；新项目建议按部署环境显式选择上述安全选项。

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
