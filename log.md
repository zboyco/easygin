# log.go 代码索引

## 文件概述

- 包名: `easygin`
- 主要功能: 记录访问日志、关联 OpenTelemetry Span、执行服务端日志级别过滤和脱敏查询参数
- 核心入口: `middleLogger`
- 安全配置来源: `serverOptions`

## 访问日志中间件

### middleLogger

- 签名: `func middleLogger(serviceName string, options serverOptions) gin.HandlerFunc`
- 请求开始时:
  1. 记录开始时间
  2. 从请求 context 获取 otelgin 创建的 Span
  3. 创建 `SpanLogger` 并写入请求 context
- 请求结束时:
  1. 将 handler 名称写入 Span attribute
  2. 计算请求耗时
  3. 记录远端 IP、HTTP 方法、脱敏 URI、状态码和错误
  4. 5xx 错误使用 Error，其他错误使用 Warn，成功请求使用 Info
  5. 是否实际输出由服务端 `SetLogLevel` 配置的全局级别决定

### 访问日志字段

- `tag`: 固定为 `access`
- `remote_ip`: `gin.Context.ClientIP()` 的结果
- `cost`: 完整请求处理耗时
- `method`: HTTP 方法
- `request_uri`: 经过查询参数脱敏的 Request URI
- `status`: HTTP 响应状态码
- `handler`: 写入 Span attribute，不直接加入字段数组

## 日志级别边界

- v0.2.0 不再读取 `x-log-level`，并删除旧客户端日志级别覆盖选项
- 客户端不能通过请求头降低、禁用或提高访问日志级别
- 成功请求按 Info、4xx 按 Warn、5xx 按 Error 记录，最终只由服务端全局日志级别过滤

## 查询参数脱敏

### redactRequestURI

- 签名: `func redactRequestURI(rawURL *url.URL, sensitiveNames map[string]struct{}) string`
- 返回用于日志记录的 URI，不修改原始 URL
- URL 为 nil 时返回空字符串
- 未配置敏感参数或请求没有 Query 时直接返回原始 Request URI
- 没有命中敏感参数时使用快速路径，不构造 `url.Values`
- 命中后按原始 `&` 字段删除敏感参数，只解码字段名并保留其他原始字段
- 畸形字段不会触发原始 URI 回退；例如敏感值包含未转义分号时仍会删除整个字段

### containsSensitiveQueryParameter

- 签名: `func containsSensitiveQueryParameter(rawQuery string, sensitiveNames map[string]struct{}) bool`
- 扫描原始 Query 的参数名，命中前不创建 `url.Values`
- 支持无值参数、重复参数和普通 `&` 分隔形式
- 参数名包含 `%` 或 `+` 时使用 `url.QueryUnescape`
- 解码失败的参数名不会被误判为敏感名称
- 预扫描和最终删除复用同一字段名解析规则，避免判断与写出路径不一致

### isSensitiveQueryName

- 签名: `func isSensitiveQueryName(name string, sensitiveNames map[string]struct{}) bool`
- 先执行规范化名称的直接 map 查找
- 未命中时使用 `strings.EqualFold` 处理大小写变体
- 可以识别 `TOKEN`、编码后的 `access_token` 等变体

### 默认敏感参数

v0.2.0 默认始终包含:

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

调用方传入的名称会加入默认集合，不会替换默认集合。

## 性能特征

### 日志热路径

- Debug 和 Info 在服务端日志级别过滤通过后才格式化消息；被过滤的普通日志不会创建消息包装器
- `WithValues` 将合法键值对直接追加到预分配属性切片，非字符串 key 会被忽略
- 日志字段统一存储为字符串属性，zerolog 输出使用类型明确的 `Str`，避免 `Any` 的反射编码成本
- Error 堆栈只格式化一次，并复用于 zerolog 输出和 recording Span Event
- 本轮不缓存包级 zerolog Logger，继续在每次输出时读取当前 `os.Stderr`，避免增加全局输出状态

使用以下命令复测日志热路径：

```bash
go test -run '^$' -bench '^BenchmarkSpanLogger$' -benchmem ./
```

Apple M1 环境下的代表性内存结果：普通 Info 和已派生字段的 Info 均为 `1 alloc/op, 16 B/op`，六字段一次性访问日志为 `10 allocs/op, 512 B/op`，被级别过滤的 Debug 为 `0 allocs/op, 0 B/op`。具体耗时会随 Go 版本、CPU 和输出设备变化，应以目标部署环境的结果为准。

### Query 脱敏

`BenchmarkRedactRequestURI` 覆盖四个路径:

- 无 Query: 约 0 allocs/op
- 有 Query 但无敏感参数: 约 1 alloc/op
- 命中普通敏感参数: 按原始 Query 字段删除
- 命中包含未转义分号的敏感参数: 同样删除整个字段

快速路径优先保证常见非敏感请求不承担 Query 重建成本；命中路径优先保证 fail-closed 脱敏和原始 URL 不变。

## OpenTelemetry 关联

- `otelgin.Middleware` 必须位于 `middleLogger` 之前，以便访问日志获取请求 Span
- `SpanLogger` 写入 context 后，API 和中间件可以通过 `logr.FromContext` 获取关联 logger
- 请求处理产生的 `gin.Context.Errors` 会被访问日志读取，同时由 otelgin 记录到 Span
- 日志输出不依赖 SDK Provider：没有 Provider 或 Span 不 recording 时，日志仍以 zerolog JSON 写入标准错误
- 有效 Span 会为日志增加 `traceID` 和 `spanID`；recording Span 同时记录 `log.<level>` Event
- Warn 和 Error 会向 recording Span 记录错误，Error 还会将 Span 状态设置为 Error
- Debug 和 Info 只添加 Event，不设置 Span 为 Ok，避免覆盖业务已经记录的 Error 状态

## OpenTelemetry 初始化

- easygin 不再通过包级 `init()` 安装全局 SDK Provider，也不会自动创建标准输出 Exporter
- `InitGlobalTracerProvider(serviceName, exporters...)` 只注册显式传入的 Exporter，并返回新建的 `*sdktrace.TracerProvider`
- nil 和 typed-nil Exporter 会被忽略；没有有效 Exporter 时不会导出 Trace
- 该辅助函数不会关闭已有 Provider；调用方负责新 Provider 的 `ForceFlush` 和 `Shutdown`
- 已自行配置全局 SDK 的应用无需调用该函数，可用 `WithTracerProvider` 为单个 Server 注入 Provider

## NewContextAndLogger

- 签名: `func NewContextAndLogger(ctx context.Context, serviceName, spanName string) (context.Context, logr.Logger)`
- 使用 OpenTelemetry 全局 Provider 创建新 Span
- 创建关联的 `SpanLogger` 并写入返回 context
- 调用方负责按 logger/Span 生命周期约定结束派生操作

## 安全边界

- 查询脱敏只作用于 `request_uri` 日志字段，不修改业务请求
- 错误对象仍会写入服务端日志；错误内容的敏感信息治理由业务错误策略负责
- 默认错误脱敏以及 `WithExposeErrorDetails` 只控制客户端响应，不会删除服务端错误日志
- 应避免把认证凭据放入非 Query 路径片段或普通错误文本
