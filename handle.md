# handle.go 代码索引

## 文件概述
- 包名: `easygin`
- 主要功能: 处理 HTTP 请求、参数绑定、上下文注入和响应生成
- 核心机制: 优先使用生成的 `EasyGinBindParameters` 静态绑定方法；未生成时回退到基于反射的参数绑定和类型转换

## 核心数据结构

### 缓存结构
- `structFields`: 缓存结构体字段信息，类型为`sync.Map`

### 字段信息结构
- `fieldInfo`: 存储字段的元信息
  - `index`: 字段索引路径，用于访问嵌套字段
  - `field`: 字段的反射信息
  - `tagType`: 标签类型 (path, query, header, body)
  - `tagName`: 标签名称
  - `tagNames`: 标签名称列表，包含选项如omitempty
- 嵌入指针字段会记录完整索引路径，绑定时通过 `getOrInitFieldByIndex` 自动初始化中间层指针

### 响应类型
- `WithStatusCode`: 自定义状态码的响应
  - `StatusCode`: HTTP状态码
  - `Output`: 响应内容
- `AttachmentFromFile`: 文件附件响应
  - `Disposition`: 内联或附件
  - `ContentType`: 内容类型
  - `Filename`: 文件名
  - `Content`: 文件内容
- `AttachmentFromReader`: 流式文件附件响应
  - `Disposition`: 内联或附件
  - `ContentType`: 内容类型
  - `Filename`: 文件名
  - `Reader`: 内容读取器
  - `ContentLength`: 内容长度；正数表示精确长度，`0` 表示空文件，`-1` 表示未知长度并使用分块传输

## 主要函数索引

### 1. handleError
- 签名: `func handleError(c *gin.Context, err error) `
- 功能: 统一处理错误响应
- 处理内容:
  1. 将错误添加到 gin.Context 的 Errors 中
  2. 检查是否为自定义HTTP错误
  3. 生成标准格式的错误响应
  4. 返回适当的HTTP状态码

### 2. parseTime
- 签名: `func parseTime(val string, tagName string) (time.Time, error)`
- 功能: 统一处理时间格式解析
- 处理内容:
  1. 尝试解析RFC3339格式的时间字符串
  2. 返回解析结果或错误信息

### 3. setFieldValue
- 签名: `func setFieldValue(fieldValue reflect.Value, val string, tagName string, fieldType reflect.Type) error`
- 功能: 统一处理字段值的类型转换和赋值
- 支持类型:
  1. 指针类型
  2. 时间类型
  3. 字符串类型
  4. 整数类型 (有符号和无符号)
  5. 浮点数类型
  6. 布尔类型

### 4. getStructFields
- 签名: `func getStructFields(t reflect.Type) []fieldInfo`
- 功能: 获取结构体的字段信息，优先从缓存中获取
- 处理内容:
  1. 构建缓存键
  2. 尝试从缓存获取字段信息
  3. 缓存未命中时解析字段信息
  4. 存储解析结果到缓存

### 5. collectFields
- 签名: `func collectFields(t reflect.Type, indexPrefix []int, fields *[]fieldInfo)`
- 功能: 递归收集字段信息，包括嵌入字段
- 处理内容:
  1. 遍历结构体的所有字段
  2. 处理嵌入字段
  3. 解析字段标签
  4. 收集字段的元信息

### 6. bindParams
- 签名: `func bindParams(c *gin.Context, h RouterHandler) (RouterHandler, error)`
- 功能: 统一处理参数绑定逻辑
- 处理内容:
  1. 创建新的结构体实例
  2. 检查是否实现了WithBindParameters接口
     - 如果实现，直接调用生成的 `EasyGinBindParameters`
  3. 获取缓存的字段信息
  4. 使用 `getOrInitFieldByIndex` 获取嵌套字段，并自动初始化嵌入指针结构体
  5. 处理不同类型的参数 (body, query, path, header)
  6. 处理multipart表单和JSON请求体
  7. 按参数是否出现处理默认值和必填参数验证；显式空字符串不等同于缺失

### 7. getOrInitFieldByIndex
- 签名: `func getOrInitFieldByIndex(v reflect.Value, index []int) (reflect.Value, error)`
- 功能: 根据字段索引路径获取字段值，遇到中间层嵌入指针为 `nil` 时自动初始化
- 处理内容:
  1. 逐层展开指针和结构体字段
  2. 校验索引范围和字段类型
  3. 遇到可设置的 nil 指针结构体时创建新实例
  4. 无法初始化或索引非法时返回错误，避免反射 panic

### 8. renderAPI
- 签名: `func renderAPI(h RouterHandler, handlerName string) gin.HandlerFunc`
- 功能: 处理API请求并生成响应
- 处理内容:
  1. 将handlerName存入context
  2. 调用handleRouter处理请求
  3. 处理自定义状态码
  4. 根据返回值类型生成不同的响应
  5. 支持多种响应类型 (JSON, 字符串, 重定向, 文件)
  6. 文件响应通过 `contentDisposition` 设置符合 MIME 参数转义规则的 `Content-Disposition`

### 9. contentDisposition
- 签名: `func contentDisposition(disposition Disposition, filename string) string`
- 功能: 生成符合 MIME 参数转义规则的 `Content-Disposition` 响应头
- 处理内容:
  1. 使用 `mime.FormatMediaType` 组合 disposition 和 filename
  2. 文件名含空格、引号或非 ASCII 字符时生成兼容的转义参数，例如 `filename*`

### 10. renderMiddleware
- 签名: `func renderMiddleware(h RouterHandler, handlerName string) gin.HandlerFunc`
- 功能: 处理中间件逻辑
- 处理内容:
  1. 将handlerName存入context
  2. 调用handleRouter处理请求
  3. 将结果存入请求上下文
  4. 继续处理后续中间件和路由

### 11. renderGinHandler
- 签名: `func renderGinHandler(h GinHandler, handlerName string) gin.HandlerFunc`
- 功能: 处理原生Gin处理器
- 处理内容:
  1. 将handlerName存入context
  2. 调用GinHandler的GinHandle方法获取原生gin.HandlerFunc
  3. 执行原生处理器函数

### 12. handleRouter
- 签名: `func handleRouter(c *gin.Context, h RouterHandler) (any, error)`
- 功能: 处理通用的RouterHandler逻辑
- 处理内容:
  1. 绑定参数
  2. 将gin.Context添加到context中
  3. 调用Handler的Output方法


## 参数绑定流程

### 1. 参数来源
- 路径参数: 从URL路径中提取，如`/users/:id`
- 查询参数: 从URL查询字符串中提取，如`?name=value`
- 头部参数: 从HTTP请求头中提取
- 请求体: 从HTTP请求体中提取，支持JSON和multipart表单

### 2. 绑定过程
1. 创建新的结构体实例
2. 检查是否实现了WithBindParameters接口
   - 如果实现了，调用EasyGinBindParameters方法
   - 否则，使用反射进行参数绑定
3. 获取缓存的字段信息
4. 自动初始化嵌入指针字段链路，避免 `*Embedded` 字段为 nil 时访问子字段 panic
5. 遍历字段，根据标签类型进行不同处理
6. 按参数是否出现处理必填参数和默认值
7. 进行类型转换和赋值

### 3. 特殊处理
- multipart表单: 解析表单数据，处理文件上传
- JSON请求体: 解析JSON数据，支持嵌套结构；连续多个 JSON 值会被 `decodeJSON` 拒绝
- 时间类型: 特殊处理时间格式
- 指针类型: 缺失的可选参数保持 nil；显式空字符串绑定到 `*string` 时会得到非 nil 的空字符串指针
- presence 语义: query 使用 `GetQuery` 区分缺失和空值，header 使用请求头 key 是否存在区分缺失和空值；default 只在参数缺失时生效

## 响应生成流程

### 1. 响应类型
- JSON响应: 默认响应类型
- 字符串响应: 返回纯文本
- 重定向: 返回URL或*URL类型
- 文件下载: 返回AttachmentFromFile或AttachmentFromReader类型
- 自定义状态码: 使用WithStatusCode包装其他响应类型

### 2. 生成过程
1. 调用RouterHandler的Output方法获取返回值
2. 检查是否为WithStatusCode类型，提取状态码和实际输出
3. 根据输出类型生成不同的HTTP响应
4. 处理nil返回值，返回204 No Content或自定义状态码

### 3. 特殊处理
- 文件响应: 设置 `Content-Disposition` 头，文件名按 MIME 参数规则转义，支持包含引号或非 ASCII 字符的文件名
- 流式响应: 使用DataFromReader处理大文件或流式数据
- 资源管理: 自动关闭实现了io.Closer接口的文件资源
- 错误处理: 统一格式的错误响应

## 缓存机制

### 字段缓存
- 使用`sync.Map`实现线程安全的缓存
- 缓存键: 结构体的包路径和类型名
- 缓存值: 字段信息数组
- 目的: 减少重复解析结构体的开销


## 标签处理

### in标签
- 格式: `in:"path|query|header|body"`
- 处理: 指定参数来源
- 用途: 确定从哪里获取参数值

### name标签
- 格式: `name:"paramName,omitempty"`
- 处理: 指定参数名称和是否可选
- 用途: 确定参数的名称和是否允许缺失；未标记 `omitempty` 时参数必须出现

### default标签
- 格式: `default:"value"`
- 处理: 指定参数的默认值
- 用途: 当参数未提供时使用默认值；显式传入空字符串不会触发默认值

### mime标签
- 格式: `mime:"multipart"`
- 处理: 指定请求体的MIME类型
- 用途: 区分JSON和multipart表单

## 性能优化

### 缓存策略
- 使用`sync.Map`缓存结构体字段信息
- 减少重复解析结构体的开销
- 缓存键包含包路径和类型名，避免冲突


### 反射优化
- 缓存反射结果
- 使用字段索引直接访问嵌套字段
- 预分配切片容量

## 错误处理

### 错误类型
- `ErrorHttp`: 自定义HTTP错误接口
  - `Code()`: 返回HTTP状态码
  - `Error()`: 返回错误消息
  - `Desc()`: 返回错误描述

### 错误响应格式
- `code`: HTTP状态码
- `msg`: 错误消息
- `desc`: 错误描述

### 常见错误
- 参数缺失错误
- 参数类型转换错误
- 请求体解析错误
- 内部服务器错误

## 上下文处理

### contextKey
- 类型: `type contextKey int`
- 用途: 作为内部 context key，分别存储 handlerName、`*gin.Context` 和当前 `RouterAPI`

### ContextKey接口
- 方法: `ContextKey() interface{}`
- 用途: 中间件返回值的上下文键

### 上下文传递
- 将gin.Context添加到context.Context
- 在非中间件函数中访问gin.Context
- 中间件返回值传递给后续处理器
- `RouteFromContext` 会使用安全类型断言，值不存在或类型不匹配时返回 `nil`
- `Server` 注入当前路由时会优先按真实 HTTP 方法匹配，未命中时回退匹配 `ANY` 路由，保证 `MethodAny` 路由也能通过 `RouteFromContext` 获取
