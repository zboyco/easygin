package easygin

import (
	"context"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
)

// SetCustomTracing 设置自定义的追踪器
// 参数:
//   - exporters: 一个或多个自定义的 span 导出器，用于将追踪数据发送到不同的后端系统
func (s *Server) SetCustomTracing(exporters ...sdktrace.SpanExporter) {
	s.customExporters = exporters
}

// initTracerProvider 初始化OpenTelemetry追踪，使用W3C Trace Context标准
// 该方法配置全局的 TracerProvider，设置资源属性、采样策略和导出器
// 所有的追踪数据都将通过这里配置的导出器发送出去
func (s *Server) initTracerProvider() {
	// 设置全局传播器为W3C Trace Context标准
	// 这确保了追踪上下文可以在不同服务之间正确传递
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, // W3C Trace Context 标准，用于传递 traceparent 和 tracestate
		propagation.Baggage{},      // W3C Baggage 标准，用于传递自定义键值对
	))

	// 创建TracerProvider选项
	opts := []sdktrace.TracerProviderOption{
		// 设置资源属性，用于标识服务和遥测数据
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,                                    // 语义约定的 Schema URL
			semconv.ServiceNameKey.String(s.serviceName),         // 服务名称
			semconv.TelemetrySDKLanguageGo,                       // 使用的编程语言
			semconv.TelemetrySDKVersionKey.String(otel.Version()), // SDK 版本
			semconv.TelemetrySDKNameKey.String("opentelemetry"),  // SDK 名称
		)),
		// 设置采样策略，决定哪些 span 会被记录
		sdktrace.WithSampler(sdktrace.ParentBased(
			sdktrace.AlwaysSample(), // 如果没有父 span，则总是采样
		)),
		// 配置标准输出导出器，使用批处理模式
		sdktrace.WithBatcher(StdoutSpanExporter(),
			sdktrace.WithBatchTimeout(100*time.Millisecond), // 设置较短的批处理超时，提高实时性
		),
	}

	// 添加用户自定义的 Exporter 选项
	// 这允许将追踪数据同时发送到多个后端系统
	for _, exporter := range s.customExporters {
		opts = append(opts, sdktrace.WithBatcher(exporter))
	}

	// 创建 TracerProvider 并设置为全局默认值
	tp := sdktrace.NewTracerProvider(opts...)
	// 设置全局 TracerProvider，使其对整个应用程序可用
	otel.SetTracerProvider(tp)
}

// InjectTraceParent 注入 trace parent 到 header 中
// 这个函数用于在发起 HTTP 请求时，将当前的追踪上下文注入到请求头中
// 参数:
//   - ctx: 包含追踪信息的上下文
//   - header: 要注入追踪信息的 HTTP 头
func InjectTraceParent(ctx context.Context, header http.Header) {
	// 从当前上下文获取 trace 信息并注入到请求 header 中
	// 这样接收请求的服务可以继续当前的追踪链
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(header))
}
