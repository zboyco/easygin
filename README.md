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

## 高级特性

### 参数标签说明

- `in`: 参数来源，支持 "path", "query", "header", "body"
- `name`: 参数名称，支持添加 ",omitempty" 后缀表示可选参数
- `default`: 参数默认值，当参数为空且设置了"omitempty"时使用
- `desc`: 参数描述，用于生成OpenAPI文档
- `mime`: 用于 body 参数，指定 MIME 类型，支持 "multipart" 表示表单上传

### Multipart 表单内存限制

easygin 支持设置 Multipart 表单的内存限制，用于控制文件上传时的内存使用量：

```go
// 设置 Multipart 表单的内存限制（字节）
easygin.SetMultipartMemoryLimit(50 * 1024 * 1024) // 设置为 50MB

// 获取当前的 Multipart 表单内存限制
limit := easygin.GetMultipartMemoryLimit()
```

默认情况下，Multipart 表单的内存限制为 100MB。可以通过 `SetMultipartMemoryLimit` 函数进行调整，参数为字节大小。该函数是并发安全的，可以在程序运行时动态调整。

> 注意：该配置作用于运行时反射绑定。当前生成的静态 multipart 绑定代码仍使用固定的 `1 << 30` 作为 `ParseMultipartForm` 内存限制。

### JSON 参数标签处理

easygin 支持控制是否处理 JSON 请求体中的 `omitempty` 和 `default` 标签：

```go
// 设置是否处理 JSON 请求体中的 omitempty 和 default 标签
easygin.SetHandleBodyJsonOmitEmptyAndDefault(true)

// 获取当前的设置状态
enabled := easygin.HandleBodyJsonOmitEmptyAndDefault()
```

默认情况下，此功能是关闭的（`false`），因为启用后会使用反射处理 JSON 标签，会对性能产生一定影响。启用后，系统会：

- 处理 `omitempty` 标签：标记字段为可选，不校验是否为空值。如果没有 `omitempty` 标签，则该字段为必填，如果为空会报错
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

这将在当前目录下生成 `openapi.json` 文件。   

![OpenAPI文档生成演示](https://raw.githubusercontent.com/zboyco/easygin/main/example/openapi.gif)   

> 内部实际调用了`easygin.GenerateOpenAPI`方法，该方法使用反射实现，有一定的耗时，可以根据需要在程序运行前手动生成，也可以在运行时自动生成文档，建议提前生成。

OpenAPI Schema 会把实现 `encoding.TextUnmarshaler` 的类型映射为 `string`，`time.Time` 及其别名映射为 `string` + `date-time`，`multipart.FileHeader` 及其别名映射为 `string` + `binary`。结构体组件会复用 `components/schemas`，循环引用会使用 `$ref` 避免无限展开。

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
