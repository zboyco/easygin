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
  1. 初始化已处理类型的映射 `processedTypes`
  2. 创建 OpenAPI 规范文档结构
  3. 遍历所有路由组，生成路径信息
  4. 将文档序列化为 JSON 并保存到文件

### generateGroupPaths
- 签名: `func generateGroupPaths(doc *openapi3.T, group *RouterGroup, parentPath string, parentMiddlewareParams ...*openapi3.ParameterRef) error`
- 功能: 递归处理路由组，生成 OpenAPI 路径信息
- 处理内容:
  1. 处理中间件参数，包括继承父路由组的中间件参数
  2. 遍历组中的 API，生成路径和操作
  3. 处理标签和响应
  4. 递归处理子组，传递当前组的中间件参数

### processStructFields
- 签名: `func processStructFields(doc *openapi3.T, t reflect.Type, op *openapi3.Operation, processedTypes map[reflect.Type]bool)`
- 功能: 处理结构体字段，提取 API 参数信息
- 处理内容:
  1. 处理嵌入字段
  2. 处理带有 `in` 标签的字段
  3. 支持 body、path、query、header 等参数类型
  4. 防止循环引用的处理

### generateSchema
- 签名: `func generateSchema(doc *openapi3.T, t reflect.Type, isMultipart bool) *openapi3.Schema`
- 功能: 生成 OpenAPI Schema 对象
- 特性:
  1. 处理指针类型
  2. 特殊处理 time.Time 和 multipart.FileHeader
  3. 将结构体类型添加到 components/schemas
  4. 支持引用已定义的组件
  5. 使用 `processedTypes` 映射跟踪已处理类型，避免无限递归
  6. 对于已处理过的类型，直接使用 `$ref` 扩展字段返回引用，不使用 `allOf` 包装

### generateSchemaValue
- 签名: `func generateSchemaValue(doc *openapi3.T, t reflect.Type, isMultipart bool) *openapi3.Schema`
- 功能: 根据 Go 类型生成 OpenAPI Schema 值
- 特性:
  1. 处理结构体 (对象)
  2. 基本类型 (字符串、整数、浮点数、布尔值)
  3. 数组和切片
  4. Map 类型
  5. 接口类型处理
  6. 特殊处理自嵌套类型，使用 `$ref` 扩展字段直接引用
  7. 支持根据 JSON 标签中的类型指定（如 `json:"field,string"`）生成相应的 OpenAPI 类型

## 辅助函数

### snakeToPascalCase
- 签名: `func snakeToPascalCase(s string) string`
- 功能: 将蛇形命名转换为帕斯卡命名
- 处理: 将路径分隔符和特殊字符转换为大写字母开头的单词

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

## 全局变量

### processedTypes
- 类型: `map[string]bool`
- 功能: 跟踪已处理的类型，避免重复处理和无限递归
- 用途: 解决自嵌套类型和循环引用问题，确保生成的 OpenAPI 文档正确

## 路由结构体

### OpenAPI
- 结构: `type OpenAPI struct`
- 功能: 提供 OpenAPI JSON 文件的访问路由
- 路径: `""`
- 实现: 读取并返回生成的 openapi.json 文件

### SwaggerUI
- 结构: `type SwaggerUI struct`
- 功能: 提供 Swagger UI 界面
- 路径: `"/swagger/*any"`
- 工厂函数: `NewSwaggerUIRouter(path string) *SwaggerUI`
- 实现: 集成 gin-swagger 提供 Swagger UI 界面

## 关键接口和类型

### RouterResponse
- 功能: 定义 API 响应规范
- 方法: `Responses() map[int]interface{}`
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
   - 处理请求参数和响应
   - 支持自定义响应码和内容

3. **参数处理**:
   - 支持路径参数、查询参数、请求体
   - 支持 JSON 和 multipart 表单
   - 处理嵌入字段和标签

4. **类型映射**:
   - Go 结构体 → OpenAPI 对象
   - Go 基本类型 → OpenAPI 基本类型
   - 特殊处理时间和文件类型

5. **组件复用**:
   - 将复杂类型添加到 components/schemas
   - 使用 `$ref` 扩展字段直接引用，不使用 `allOf` 包装

6. **循环引用处理**:
   - 使用 `processedTypes` 映射跟踪已处理类型
   - 对于自嵌套类型，使用 `$ref` 扩展字段直接引用
   - 为自嵌套引用添加 `title` 属性，帮助识别引用类型
   - 处理数组元素中的循环引用

## 文件生成

- 输出文件: `openapi.json`
- 格式: JSON 格式的 OpenAPI 3.0.3 规范文档
- 访问方式: 通过 OpenAPI 路由或直接访问文件
- 可视化: 通过 Swagger UI 界面查看和测试 API