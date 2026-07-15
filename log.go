package easygin

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zboyco/easygin/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// middleLogger 创建一个 Gin 中间件，用于记录请求日志并集成 OpenTelemetry 追踪
// 参数:
//   - serviceName: 服务名称，用于日志标识
//
// 返回:
//   - gin.HandlerFunc: Gin 中间件函数
func middleLogger(serviceName string, options serverOptions) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 记录请求开始时间，用于计算请求处理耗时
		startAt := time.Now()

		// 获取请求上下文
		ctx := c.Request.Context()

		// 从当前上下文中获取已存在的 span
		// otelgin.Middleware 已经创建了 span 并将其放入上下文中
		span := trace.SpanFromContext(ctx)

		// 创建与当前 span 关联的日志记录器
		log := SpanLogger(serviceName, span)

		// 将日志记录器添加到上下文中，以便后续处理函数使用
		ctx = logr.WithLogger(ctx, log)
		// 更新请求上下文
		c.Request = c.Request.WithContext(ctx)

		defer func() {
			handlerName := HandlerNameFromContext(c.Request.Context())
			if handlerName != "" {
				span.SetAttributes(attribute.String("handler", handlerName))
			}

			// 计算请求处理总耗时
			duration := time.Since(startAt)

			// 构建日志字段，包含请求的关键信息
			keyAndValues := []interface{}{
				"tag", "access", // 标记为访问日志
				"remote_ip", c.ClientIP(), // 客户端 IP
				"cost", duration, // 请求耗时
				"method", c.Request.Method, // HTTP 方法
				"request_uri", redactRequestURI(c.Request.URL, options.redactQueryParameters), // 脱敏后的请求 URI
				"status", c.Writer.Status(), // HTTP 状态码
			}

			// 获取请求处理过程中可能发生的错误
			var err error
			errs := c.Errors.ByType(gin.ErrorTypePrivate)
			if len(errs) > 0 {
				err = errs[0].Err
			}

			// 根据错误状态和日志级别记录不同级别的日志
			if err != nil {
				if c.Writer.Status() >= http.StatusInternalServerError {
					// 5xx 错误记录为 ERROR 级别
					log.WithValues(keyAndValues...).Error(err)
				} else {
					// 4xx 等其他错误记录为 WARN 级别
					log.WithValues(keyAndValues...).Warn(err)
				}
			} else {
				// 正常请求记录为 INFO 级别
				log.WithValues(keyAndValues...).Info("")
			}
		}()

		// 调用下一个中间件或处理函数
		c.Next()
	}
}

// redactRequestURI 返回脱敏后的请求 URI，不修改请求本身。
func redactRequestURI(rawURL *url.URL, sensitiveNames map[string]struct{}) string {
	if rawURL == nil {
		return ""
	}
	if len(sensitiveNames) == 0 || rawURL.RawQuery == "" {
		return rawURL.RequestURI()
	}
	redactedQuery, redacted := redactSensitiveQueryParameters(rawURL.RawQuery, sensitiveNames)
	if !redacted {
		return rawURL.RequestURI()
	}

	clone := *rawURL
	clone.RawQuery = redactedQuery
	return clone.RequestURI()
}

// redactSensitiveQueryParameters 从原始 Query 中删除敏感字段，避免标准解析器丢弃畸形字段后回退未脱敏内容。
func redactSensitiveQueryParameters(rawQuery string, sensitiveNames map[string]struct{}) (string, bool) {
	if !containsSensitiveQueryParameter(rawQuery, sensitiveNames) {
		return rawQuery, false
	}

	var redactedQuery strings.Builder
	redactedQuery.Grow(len(rawQuery))
	for rawQuery != "" {
		field := rawQuery
		if separator := strings.IndexByte(rawQuery, '&'); separator >= 0 {
			field = rawQuery[:separator]
			rawQuery = rawQuery[separator+1:]
		} else {
			rawQuery = ""
		}
		name, ok := queryParameterName(field)
		if ok && isSensitiveQueryName(name, sensitiveNames) {
			continue
		}
		if field == "" {
			continue
		}
		if redactedQuery.Len() > 0 {
			redactedQuery.WriteByte('&')
		}
		redactedQuery.WriteString(field)
	}
	return redactedQuery.String(), true
}

// containsSensitiveQueryParameter 扫描原始 Query 的参数名，命中前不创建 url.Values。
// 参数名包含转义字符时使用标准 QueryUnescape，保证与 URL 查询语义一致。
func containsSensitiveQueryParameter(rawQuery string, sensitiveNames map[string]struct{}) bool {
	for rawQuery != "" {
		field := rawQuery
		if separator := strings.IndexByte(rawQuery, '&'); separator >= 0 {
			field = rawQuery[:separator]
			rawQuery = rawQuery[separator+1:]
		} else {
			rawQuery = ""
		}
		name, ok := queryParameterName(field)
		if ok && isSensitiveQueryName(name, sensitiveNames) {
			return true
		}
	}
	return false
}

// queryParameterName 提取并解码原始 Query 字段名；字段名无效时返回 false。
func queryParameterName(field string) (string, bool) {
	if field == "" {
		return "", false
	}
	key := field
	if separator := strings.IndexByte(field, '='); separator >= 0 {
		key = field[:separator]
	}
	if key == "" {
		return "", false
	}
	if !strings.ContainsAny(key, "%+") {
		return key, true
	}
	decodedKey, err := url.QueryUnescape(key)
	if err != nil {
		return "", false
	}
	return decodedKey, true
}

// isSensitiveQueryName 以大小写不敏感方式匹配已规范化的敏感参数名称。
func isSensitiveQueryName(name string, sensitiveNames map[string]struct{}) bool {
	if _, ok := sensitiveNames[name]; ok {
		return true
	}
	for configuredName := range sensitiveNames {
		if strings.EqualFold(name, configuredName) {
			return true
		}
	}
	return false
}

// NewContextAndLogger 创建一个新的上下文和日志记录器
func NewContextAndLogger(ctx context.Context, serviceName, spanName string) (context.Context, logr.Logger) {
	ctx, span := otel.Tracer(serviceName).Start(ctx, spanName, trace.WithTimestamp(time.Now()))
	log := SpanLogger(serviceName, span)
	return logr.WithLogger(ctx, log), log
}
