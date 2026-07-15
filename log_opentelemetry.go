package easygin

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
)

// InitGlobalTracerProvider 使用显式提供的 Exporter 初始化全局 OpenTelemetry Provider。
// 返回的 Provider 由调用方负责 Flush 和 Shutdown；nil Exporter 会被忽略，已有 Provider 不会被关闭。
func InitGlobalTracerProvider(serviceName string, exporters ...sdktrace.SpanExporter) *sdktrace.TracerProvider {
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
			semconv.SchemaURL,                                     // 语义约定的 Schema URL
			semconv.ServiceNameKey.String(serviceName),            // 服务名称
			semconv.TelemetrySDKLanguageGo,                        // 使用的编程语言
			semconv.TelemetrySDKVersionKey.String(otel.Version()), // SDK 版本
			semconv.TelemetrySDKNameKey.String("opentelemetry"),   // SDK 名称
		)),
		// 设置采样策略，决定哪些 span 会被记录。
		sdktrace.WithSampler(sdktrace.ParentBased(
			sdktrace.AlwaysSample(),
		)),
	}

	for _, exporter := range exporters {
		if isNilInterfaceValue(exporter) {
			continue
		}
		opts = append(opts, sdktrace.WithBatcher(exporter))
	}

	// 创建 TracerProvider 并设置为全局默认值
	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	return tp
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
