package easygin

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/zboyco/easygin/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// middleLogger 创建一个 Gin 中间件，用于记录请求日志并集成 OpenTelemetry 追踪
// 参数:
//   - serviceName: 服务名称，用于日志标识
//   - tracer: OpenTelemetry 追踪器实例
//
// 返回:
//   - gin.HandlerFunc: Gin 中间件函数
func middleLogger() gin.HandlerFunc {
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

			// 处理日志级别，可通过 x-log-level 请求头自定义
			var level zerolog.Level
			_ = level.UnmarshalText([]byte(c.Request.Header.Get("x-log-level")))
			// 如果未指定日志级别，默认使用 TraceLevel（最详细级别）
			if level == zerolog.NoLevel {
				level = zerolog.TraceLevel
			}

			// 计算请求处理总耗时
			duration := time.Since(startAt)

			// 构建日志字段，包含请求的关键信息
			keyAndValues := []interface{}{
				"tag", "access", // 标记为访问日志
				"remote_ip", c.ClientIP(), // 客户端 IP
				"cost", duration, // 请求耗时
				"method", c.Request.Method, // HTTP 方法
				"request_uri", c.Request.URL.RequestURI(), // 请求 URI
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
					if level <= zerolog.ErrorLevel {
						log.WithValues(keyAndValues...).Error(err)
					}
				} else {
					// 4xx 等其他错误记录为 WARN 级别
					if level <= zerolog.WarnLevel {
						log.WithValues(keyAndValues...).Warn(err)
					}
				}
			} else {
				// 正常请求记录为 INFO 级别
				if level <= zerolog.InfoLevel {
					log.WithValues(keyAndValues...).Info("")
				}
			}
		}()

		// 调用下一个中间件或处理函数
		c.Next()
	}
}

// NewContextAndLogger 创建一个新的上下文和日志记录器
func NewContextAndLogger(ctx context.Context, serviceName, spanName string) (context.Context, logr.Logger) {
	ctx, span := otel.Tracer(serviceName).Start(ctx, spanName, trace.WithTimestamp(time.Now()))
	log := SpanLogger(serviceName, span)
	return logr.WithLogger(ctx, log), log
}
