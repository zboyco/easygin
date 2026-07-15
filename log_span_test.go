package easygin

import (
	"context"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestFilteredLogStillEndsSpan 验证日志被级别过滤后仍会结束并导出 Span。
func TestFilteredLogStillEndsSpan(t *testing.T) {
	previousLevel := globalLogLevel
	globalLogLevel = zerolog.InfoLevel
	t.Cleanup(func() {
		globalLogLevel = previousLevel
	})

	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown tracer provider: %v", err)
		}
	})

	_, span := provider.Tracer("test").Start(context.Background(), "filtered-log")
	logger := SpanLogger("test", span)
	logger.Debug("filtered")
	logger.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported span count = %d, want 1", len(spans))
	}
}

// TestSpanLoggerWithValuesIsImmutable 验证派生 Logger 不修改父 Logger，并可安全并发派生。
func TestSpanLoggerWithValuesIsImmutable(t *testing.T) {
	provider := sdktrace.NewTracerProvider()
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown tracer provider: %v", err)
		}
	})

	_, span := provider.Tracer("test").Start(context.Background(), "immutable-values")
	base := SpanLogger("test", span).(*spanLogger)
	first := base.WithValues("request", "first").(*spanLogger)
	second := base.WithValues("request", "second").(*spanLogger)

	if len(base.attributes) != 0 {
		t.Fatalf("base logger attributes = %d, want 0", len(base.attributes))
	}
	if len(first.attributes) != 1 || first.attributes[0].Value.AsString() != "first" {
		t.Fatalf("unexpected first logger attributes: %#v", first.attributes)
	}
	if len(second.attributes) != 1 || second.attributes[0].Value.AsString() != "second" {
		t.Fatalf("unexpected second logger attributes: %#v", second.attributes)
	}

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(value int) {
			defer wg.Done()
			_ = base.WithValues("worker", value)
		}(i)
	}
	wg.Wait()
	base.End()
}
