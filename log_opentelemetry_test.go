package easygin

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
)

// panicOnNilExporter 用于验证 typed-nil Exporter 不会进入后台导出流程。
type panicOnNilExporter struct{}

// ExportSpans 在 nil 接收者被错误调用时触发 panic。
func (e *panicOnNilExporter) ExportSpans(context.Context, []sdktrace.ReadOnlySpan) error {
	if e == nil {
		panic("typed-nil exporter ExportSpans called")
	}
	return nil
}

// Shutdown 在 nil 接收者被错误调用时触发 panic。
func (e *panicOnNilExporter) Shutdown(context.Context) error {
	if e == nil {
		panic("typed-nil exporter Shutdown called")
	}
	return nil
}

// TestOpenTelemetryRequiresExplicitInitialization 验证包加载不会自动安装 SDK Provider。
func TestOpenTelemetryRequiresExplicitInitialization(t *testing.T) {
	if provider := otel.GetTracerProvider(); isSDKTracerProvider(provider) {
		t.Fatalf("global tracer provider = %T, want non-SDK provider before explicit initialization", provider)
	}
}

// TestInitGlobalTracerProviderUsesExplicitExporter 验证初始化返回 Provider，且只导出到显式传入的 Exporter。
func TestInitGlobalTracerProviderUsesExplicitExporter(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := InitGlobalTracerProvider("test-service", exporter)
	t.Cleanup(func() {
		otel.SetTracerProvider(noop.NewTracerProvider())
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown tracer provider: %v", err)
		}
	})

	if global := otel.GetTracerProvider(); global != provider {
		t.Fatalf("global tracer provider = %T, want returned provider %p", global, provider)
	}

	_, span := provider.Tracer("test").Start(context.Background(), "explicit-exporter")
	span.End()
	if err := provider.ForceFlush(context.Background()); err != nil {
		t.Fatalf("flush tracer provider: %v", err)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Name != "explicit-exporter" {
		t.Fatalf("exported spans = %#v, want one span sent to the explicit exporter", spans)
	}
}

// TestInitGlobalTracerProviderIgnoresNilExporters 验证 nil 和 typed-nil Exporter 不会进入处理器。
func TestInitGlobalTracerProviderIgnoresNilExporters(t *testing.T) {
	var typedNilExporter *panicOnNilExporter
	exporter := tracetest.NewInMemoryExporter()
	provider := InitGlobalTracerProvider("test-service", nil, typedNilExporter, exporter)
	t.Cleanup(func() {
		otel.SetTracerProvider(noop.NewTracerProvider())
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown tracer provider: %v", err)
		}
	})

	_, span := provider.Tracer("test").Start(context.Background(), "valid-exporter-only")
	span.End()
	if err := provider.ForceFlush(context.Background()); err != nil {
		t.Fatalf("flush tracer provider: %v", err)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Name != "valid-exporter-only" {
		t.Fatalf("exported spans = %#v, want only the valid exporter to receive one span", spans)
	}
}

// isSDKTracerProvider 判断全局 Provider 是否由 OpenTelemetry SDK 实现。
func isSDKTracerProvider(provider any) bool {
	_, ok := provider.(*sdktrace.TracerProvider)
	return ok
}
