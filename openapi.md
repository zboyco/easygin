# OpenAPI 代码索引

## 包结构
- 包名: `easygin`
- 文件: `openapi.go`
- 主要功能: OpenAPI 规范生成和 Swagger UI 集成

## 核心函数

### GenerateOpenAPI
- 签名: `func GenerateOpenAPI(groups ...*RouterGroup) error`
- 功能: 为给定的路由组生成 OpenAPI 3.0.3 规范文档
- 流程:
  1. 创建 OpenAPI 规范文档结构
  2. 遍历所有路由组，生成路径与组件信息
  3. 执行 OpenAPI 3.0.3 完整规范校验
  4. 将文档序列化为 JSON，并通过同目录临时文件原子替换旧文件
- 输出文件固定为当前工作目录下的 `openapi.json`，可通过 `NewOpenAPIRouter` 或 `OpenAPIRouter` 暴露访问

### generateOperationID
- 签名: `func generateOperationID(apiType reflect.Type) string`
- 功能: 将 API 结构体名称转换为 operationId 格式（首字母小写的驼峰命名）
- 参数: 
  - `apiType`: API 结构体的反射类型
- 返回值: 转换后的 operationId 字符串
- 转换示例:
  - `CreateUser` → `createUser`
  - `GetUserInfo` → `getUserInfo`
  - `ListUsers` → `listUsers`
- 边界处理: 处理空名称和单字符名称的情况

### generateGroupPaths
- 签名: `func generateGroupPaths(doc *openapi3.T, group *RouterGroup, parentPath string, parentMiddlewareParams ...*openapi3.ParameterRef) error`
- 功能: 递归处理路由组，生成 OpenAPI 路径信息
- 处理内容:
  1. 处理中间件参数，包括继承父路由组的中间件参数；仅允许 `path/query/header`，遇到 `in:"body"` 会返回错误，支持 `desc` 描述
  2. 使用 `joinURLPath` 拼接路径并将 `:param` 转为 `{param}`
  3. 标签以 RouterGroup 完整路径命名，按需添加
  4. 为每个 API 生成 operationId；`ANY` 路由按具体 HTTP 方法生成唯一后缀
  5. 默认挂载 `default` 响应（携带 `Error` Schema），未实现 `RouterResponse` 时自动添加 200 响应
  6. `RouterResponse` 的响应体为 `nil` 时只生成响应描述，不生成 `content`
  7. 递归处理子组，传递当前组的中间件参数

### processStructFields
- 签名: `func processStructFields(doc *openapi3.T, t reflect.Type, op *openapi3.Operation, processedTypes map[reflect.Type]bool)`
- 功能: 处理结构体字段，提取 API 参数信息
- 处理内容:
  1. 处理嵌入字段：`in:"body"` 直接生成请求体，其余 `in` 标签按普通字段处理；无 `in` 标签的嵌入 struct 会递归展开
  2. 处理带有 `in` 标签的字段，支持 `desc` 描述和 `omitempty` 控制 OpenAPI required 标记；required 表示参数必须出现，不表示值必须非空或非零
  3. `mime:"multipart"` 会将请求体 content-type 设置为 `multipart/form-data`，并在 Schema 生成时以表单字段名解析
  4. 使用当前递归路径集合防止循环嵌入，并允许不同分支复用同一嵌入类型

### generateSchema
- 签名: `func generateSchema(doc *openapi3.T, t reflect.Type, isMultipart bool) *openapi3.SchemaRef`
- 功能: 生成 OpenAPI Schema 对象
- 特性:
  1. 处理指针类型
  2. 特殊处理 time.Time 及其别名、multipart.FileHeader 及其别名
  3. 实现 `encoding.TextUnmarshaler` 的命名类型统一映射为字符串；`time.Time` 本身不走该分支，仍按 `date-time` 处理
  4. 将结构体类型添加到 components/schemas，组件名使用 `snakeToPascalCase` 规整化
  5. 支持引用已定义的组件
  6. 先注册组件占位再生成内容，使用标准 `SchemaRef.Ref` 和已解析值打断循环引用

### generateSchemaValue
- 签名: `func generateSchemaValue(doc *openapi3.T, t reflect.Type, isMultipart bool) *openapi3.Schema`
- 功能: 根据 Go 类型生成 OpenAPI Schema 值
- 特性:
  1. 处理结构体 (对象)，值或指针形式的嵌入字段都会展开，属性与必填信息会合并到父级；支持 `desc` 描述
  2. 基本类型 (字符串、整数、浮点数、布尔值)
  3. 数组和切片（组件已注册时使用标准 `$ref` 避免循环）
  4. Map 类型（仅允许字符串键，否则 panic 提示）
  5. 接口类型处理
  6. 自嵌套类型使用标准 `$ref`；字段描述需要包裹引用时使用合法的 `allOf`
  7. 支持根据 JSON 标签中的类型指定（如 `json:"field,string"`）生成相应的 OpenAPI 类型
  8. `isMultipart` 为 true 时使用 `name` 标签解析字段名并识别 `omitempty`，用于 multipart/form-data 表单
  9. 实现 `TextUnmarshaler` 的字段和自定义时间/文件别名在 Schema 中按字符串/二进制格式输出
  10. query/path/header 参数如果是切片或数组，会生成数组 Schema，元素类型继续按上述规则映射

## 辅助函数

### snakeToPascalCase
- 签名: `func snakeToPascalCase(s string) string`
- 功能: 将蛇形命名转换为帕斯卡命名
- 处理: 将路径分隔符和特殊字符转换为大写字母开头的单词

### joinURLPath
- 签名: `func joinURLPath(parts ...string) string`
- 功能: 以 POSIX 形式拼接路径，避免因操作系统分隔符导致的 URL 反斜杠问题

### convertPathParams
- 签名: `func convertPathParams(path string) string`
- 功能: 将 `:param` 格式的路径参数转换为 `{param}` 格式
- 用途: 适配 OpenAPI 规范的路径参数格式

### isTimeTypeOrAlias
- 签名: `func isTimeTypeOrAlias(t reflect.Type) bool`
- 功能: 检查类型是否为 time.Time 或其别名
- 用途: 特殊处理时间类型，将其映射为 OpenAPI 的 date-time 格式

### Ptr
- 签名: `func Ptr[T any](v T) *T`
- 功能: 返回指向给定值的指针
- 用途: 简化创建指针的操作，特别是在 OpenAPI 规范中需要指针的场景

### isTextUnmarshalerType
- 功能: 判断命名类型（含指针接收者）是否实现 `encoding.TextUnmarshaler`，用于 Schema 中降级为字符串
- 约束: 匿名结构体不会按 `TextUnmarshaler` 处理；`time.Time` 本身保留 `date-time` 映射，自定义时间类型如果实现 `TextUnmarshaler` 则可映射为普通字符串

### isFileHeaderTypeOrAlias
- 功能: 判断类型是否可转换为 `multipart.FileHeader`，在 Schema 中映射为 `string` + `binary` 格式

## 生成状态

- OpenAPI 生成不使用包级可变状态；每次调用创建独立文档和组件表。
- 命名结构体会先在 `components/schemas` 注册占位，再递归生成字段，从而安全处理自嵌套和循环引用。

## 路由结构体

### OpenAPI
- 结构: `type OpenAPI struct`
- 功能: 提供 OpenAPI JSON 文件的访问路由
- 路径: 创建实例时由 `NewOpenAPIRouter(path string)` 决定
- 实现: 读取并返回生成的 `openapi.json` 文件，使用 `AttachmentFromReader` 以内联方式输出

### SwaggerUI
- 结构: `type SwaggerUI struct`
- 功能: 提供 Swagger UI 界面
- 路径: `"/swagger/*any"`
- 工厂函数: `NewSwaggerUIRouter(path string) *SwaggerUI`
- 实现: 集成 gin-swagger 提供 Swagger UI 界面，`GinHandle` 配置了 `docExpansion=list`、`Title=Swagger UI` 等展示参数

## 关键接口和类型

### RouterResponse
- 功能: 定义 API 响应规范
- 方法: `Responses() R`，其中 `R` 是 `map[int]any`
- 用途: 允许 API 定义不同状态码的响应内容和结构

### NoOpenAPI
- 功能: 标记不生成 OpenAPI 文档的路由
- 用途: 排除某些路由，不将其包含在 OpenAPI 文档中

### NoGenParameter
- 功能: 标记不生成参数绑定函数的路由
- 用途: 对于特殊路由，如 Swagger UI，不需要生成参数绑定代码

## 处理逻辑

1. **路由组处理**:
   - 递归遍历路由组树
   - 为每个组创建标签
   - 处理中间件参数
   - 子路由组继承父路由组的中间件参数

2. **API 处理**:
   - 提取 HTTP 方法和路径
   - 自动生成 operationId（基于结构体名称的首字母小写驼峰命名）
   - 处理请求参数和响应
   - 支持自定义响应码和内容

3. **参数处理**:
   - 支持路径参数、查询参数、请求体
   - 支持 JSON 和 multipart 表单
   - 处理嵌入字段和标签
   - 中间件参数会继承到子路由组和 API 操作；中间件不能声明 `in:"body"` 参数

4. **类型映射**:
   - Go 结构体 → OpenAPI 对象
   - Go 基本类型 → OpenAPI 基本类型
   - 特殊处理时间和文件类型
   - 实现 `encoding.TextUnmarshaler` 的命名类型 → `string`

5. **组件复用**:
   - 将复杂类型添加到 components/schemas
   - 使用标准 `SchemaRef.Ref` 引用组件；字段描述需要作为引用 sibling 时使用 `allOf` 包装

6. **循环引用处理**:
   - 通过组件占位和当前递归路径集合跟踪结构体、数组元素及匿名嵌入指针
   - 对于自嵌套类型，使用标准 `$ref` 引用已注册组件
   - 处理数组元素中的循环引用

## 文件生成

- 输出文件: `openapi.json`
- 格式: JSON 格式的 OpenAPI 3.0.3 规范文档
- 写入保障: 规范校验通过后使用同目录临时文件原子替换，失败时保留旧文件
- 访问方式: 通过 OpenAPI 路由或直接访问文件
- 可视化: 通过 Swagger UI 界面查看和测试 API
