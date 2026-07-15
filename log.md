# log.go 代码索引

## 文件概述

- 包名: `easygin`
- 主要功能: 记录访问日志、关联 OpenTelemetry Span、控制请求日志级别和脱敏查询参数
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

### 访问日志字段

- `tag`: 固定为 `access`
- `remote_ip`: `gin.Context.ClientIP()` 的结果
- `cost`: 完整请求处理耗时
- `method`: HTTP 方法
- `request_uri`: 经过查询参数脱敏的 Request URI
- `status`: HTTP 响应状态码
- `handler`: 写入 Span attribute，不直接加入字段数组

## 客户端日志级别

### requestLogLevel

- 签名: `func requestLogLevel(header http.Header, allowClientOverride bool) zerolog.Level`
- `allowClientOverride == false` 时忽略 `x-log-level`，返回 TraceLevel 以保留服务器正常日志判断
- 允许覆盖时解析 `x-log-level`
- 请求头缺失或值无效时回退到 TraceLevel
- 生产环境可通过 `WithAllowClientLogLevel(false)` 禁止客户端压低访问日志输出

## 查询参数脱敏

### redactRequestURI

- 签名: `func redactRequestURI(rawURL *url.URL, sensitiveNames map[string]struct{}) string`
- 返回用于日志记录的 URI，不修改原始 URL
- URL 为 nil 时返回空字符串
- 未配置敏感参数或请求没有 Query 时直接返回原始 Request URI
- 没有命中敏感参数时使用快速路径，不构造 `url.Values`
- 命中后使用标准 URL Query API 删除所有同名参数并重新编码

### containsSensitiveQueryParameter

- 签名: `func containsSensitiveQueryParameter(rawQuery string, sensitiveNames map[string]struct{}) bool`
- 扫描原始 Query 的参数名，命中前不创建 `url.Values`
- 支持无值参数、重复参数和普通 `&` 分隔形式
- 参数名包含 `%` 或 `+` 时使用 `url.QueryUnescape`
- 解码失败的参数名不会被误判为敏感名称

### isSensitiveQueryName

- 签名: `func isSensitiveQueryName(name string, sensitiveNames map[string]struct{}) bool`
- 先执行规范化名称的直接 map 查找
- 未命中时使用 `strings.EqualFold` 处理大小写变体
- 可以识别 `TOKEN`、编码后的 `access_token` 等变体

### 默认敏感参数

调用 `WithRedactQueryParameters` 后始终包含:

- `token`
- `access_token`
- `authorization`
- `api_key`
- `password`

调用方传入的名称会加入默认集合，不会替换默认集合。

## 性能特征

`BenchmarkRedactRequestURI` 覆盖三个路径:

- 无 Query: 约 0 allocs/op
- 有 Query 但无敏感参数: 约 1 alloc/op
- 命中敏感参数: 执行完整解析、删除和重新编码

快速路径优先保证常见非敏感请求不承担完整 URL Query 重建成本；命中路径优先保证正确性和原始 URL 不变。

## OpenTelemetry 关联

- `otelgin.Middleware` 必须位于 `middleLogger` 之前，以便访问日志获取请求 Span
- `SpanLogger` 写入 context 后，API 和中间件可以通过 `logr.FromContext` 获取关联 logger
- 请求处理产生的 `gin.Context.Errors` 会被访问日志读取，同时由 otelgin 记录到 Span

## NewContextAndLogger

- 签名: `func NewContextAndLogger(ctx context.Context, serviceName, spanName string) (context.Context, logr.Logger)`
- 使用 OpenTelemetry 全局 Provider 创建新 Span
- 创建关联的 `SpanLogger` 并写入返回 context
- 调用方负责按 logger/Span 生命周期约定结束派生操作

## 安全边界

- 查询脱敏只作用于 `request_uri` 日志字段，不修改业务请求
- 错误对象仍会写入服务端日志；错误内容的敏感信息治理由业务错误策略负责
- `WithExposeErrorDetails(false)` 控制客户端响应，不会删除服务端错误日志
- 应避免把认证凭据放入非 Query 路径片段或普通错误文本
