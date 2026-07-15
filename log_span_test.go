package easygin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// captureTestStderr 捕获函数执行期间写入标准错误的内容，并在返回前恢复输出句柄。
func captureTestStderr(t *testing.T, fn func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	previous := os.Stderr
	os.Stderr = writer
	defer func() {
		os.Stderr = previous
		_ = writer.Close()
		_ = reader.Close()
	}()

	fn()
	os.Stderr = previous
	if err := writer.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read captured stderr: %v", err)
	}
	return string(output)
}

// decodeTestLogLine 解码单行 zerolog JSON，格式无效时终止当前测试。
func decodeTestLogLine(t *testing.T, output string) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 || lines[0] == "" {
		t.Fatalf("log line count = %d, want 1; output=%q", len(lines), output)
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &fields); err != nil {
		t.Fatalf("decode log line %q: %v", lines[0], err)
	}
	return fields
}

// discardBenchmarkStderr 将基准日志写入系统空设备，并在结束后恢复标准错误。
func discardBenchmarkStderr(b *testing.B) {
	b.Helper()
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		b.Fatalf("open null device: %v", err)
	}
	previous := os.Stderr
	os.Stderr = devNull
	b.Cleanup(func() {
		os.Stderr = previous
		if err := devNull.Close(); err != nil {
			b.Fatalf("close null device: %v", err)
		}
	})
}

// TestSpanLoggerPrintsWithoutProvider 验证 nil/no-op Span 不会吞掉普通日志。
func TestSpanLoggerPrintsWithoutProvider(t *testing.T) {
	previousLevel := globalLogLevel
	globalLogLevel = zerolog.InfoLevel
	t.Cleanup(func() { globalLogLevel = previousLevel })

	output := captureTestStderr(t, func() {
		SpanLogger("test", nil).WithValues("component", "worker").Info("ready")
	})
	fields := decodeTestLogLine(t, output)
	if fields["level"] != "info" || fields["msg"] != "ready" || fields["component"] != "worker" {
		t.Fatalf("unexpected log fields: %#v", fields)
	}
	if _, ok := fields["traceID"]; ok {
		t.Fatalf("log without Span contains traceID: %#v", fields)
	}
}

// TestSpanLoggerFormatsMessageAfterFiltering 验证通过级别过滤后仍保持格式化消息语义。
func TestSpanLoggerFormatsMessageAfterFiltering(t *testing.T) {
	previousLevel := globalLogLevel
	globalLogLevel = zerolog.InfoLevel
	t.Cleanup(func() { globalLogLevel = previousLevel })

	output := captureTestStderr(t, func() {
		SpanLogger("test", nil).Info("ready %d", 2)
	})
	fields := decodeTestLogLine(t, output)
	if fields["msg"] != "ready 2" {
		t.Fatalf("formatted message = %#v, want %q", fields["msg"], "ready 2")
	}
}

// TestSpanLoggerSkipsInvalidKeys 验证非字符串 key 被忽略，合法 value 继续按字符串记录。
func TestSpanLoggerSkipsInvalidKeys(t *testing.T) {
	previousLevel := globalLogLevel
	globalLogLevel = zerolog.InfoLevel
	t.Cleanup(func() { globalLogLevel = previousLevel })

	output := captureTestStderr(t, func() {
		SpanLogger("test", nil).WithValues(42, "ignored", "attempt", 2).Info("ready")
	})
	fields := decodeTestLogLine(t, output)
	if _, ok := fields[""]; ok {
		t.Fatalf("log contains an empty field from a non-string key: %#v", fields)
	}
	if fields["attempt"] != "2" {
		t.Fatalf("attempt = %#v, want string value %q", fields["attempt"], "2")
	}
}

// TestSpanLoggerCorrelatesRecordingSpan 验证日志只输出一次，并同步 Trace 关联和错误状态。
func TestSpanLoggerCorrelatesRecordingSpan(t *testing.T) {
	previousLevel := globalLogLevel
	globalLogLevel = zerolog.InfoLevel
	t.Cleanup(func() { globalLogLevel = previousLevel })

	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown tracer provider: %v", err)
		}
	})
	_, span := provider.Tracer("test").Start(context.Background(), "correlated-log")
	spanContext := span.SpanContext()
	output := captureTestStderr(t, func() {
		SpanLogger("test", span).WithValues("request", "one").Error(errors.New("boom"))
	})
	span.End()

	fields := decodeTestLogLine(t, output)
	if fields["traceID"] != spanContext.TraceID().String() || fields["spanID"] != spanContext.SpanID().String() || fields["msg"] != "boom" {
		t.Fatalf("unexpected correlated log fields: %#v", fields)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Status.Code != codes.Error {
		t.Fatalf("exported spans = %#v, want one error span", spans)
	}
	foundLogEvent := false
	for _, event := range spans[0].Events {
		if event.Name == "log.error" {
			foundLogEvent = true
			break
		}
	}
	if !foundLogEvent {
		t.Fatalf("span events = %#v, want log.error", spans[0].Events)
	}
}

// TestSpanLoggerInfoPreservesErrorStatus 验证普通日志不会把已失败的 Span 状态覆盖为 Ok。
func TestSpanLoggerInfoPreservesErrorStatus(t *testing.T) {
	previousLevel := globalLogLevel
	globalLogLevel = zerolog.InfoLevel
	t.Cleanup(func() { globalLogLevel = previousLevel })

	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown tracer provider: %v", err)
		}
	})

	_, span := provider.Tracer("test").Start(context.Background(), "error-then-info")
	logger := SpanLogger("test", span)
	output := captureTestStderr(t, func() {
		logger.Error(errors.New("handled failure"))
		logger.Info("request completed")
	})
	span.End()

	if lines := strings.Split(strings.TrimSpace(output), "\n"); len(lines) != 2 {
		t.Fatalf("log line count = %d, want 2; output=%q", len(lines), output)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Status.Code != codes.Error {
		t.Fatalf("exported spans = %#v, want one error span after trailing info log", spans)
	}
}

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
	output := captureTestStderr(t, func() {
		logger.Debug("filtered")
	})
	if output != "" {
		t.Fatalf("filtered debug log wrote output: %q", output)
	}
	logger.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported span count = %d, want 1", len(spans))
	}
	for _, event := range spans[0].Events {
		if event.Name == "log.debug" {
			t.Fatalf("filtered debug log created a span event: %#v", event)
		}
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

// BenchmarkSpanLogger 记录普通、结构化、访问、过滤和错误日志的热路径成本。
func BenchmarkSpanLogger(b *testing.B) {
	discardBenchmarkStderr(b)
	previousLevel := globalLogLevel
	b.Cleanup(func() { globalLogLevel = previousLevel })

	logger := SpanLogger("benchmark", nil)
	withValues := logger.WithValues("component", "worker", "attempt", 1)
	accessValues := []any{
		"tag", "access",
		"remote_ip", "192.0.2.1",
		"cost", time.Millisecond,
		"method", "GET",
		"request_uri", "/users?page=1",
		"status", http.StatusOK,
	}
	logError := errors.New("boom")

	b.Run("info", func(b *testing.B) {
		globalLogLevel = zerolog.DebugLevel
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info("ready")
		}
	})

	b.Run("with_values_info", func(b *testing.B) {
		globalLogLevel = zerolog.DebugLevel
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			withValues.Info("ready")
		}
	})

	b.Run("access_one_shot", func(b *testing.B) {
		globalLogLevel = zerolog.DebugLevel
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.WithValues(accessValues...).Info("")
		}
	})

	b.Run("filtered_debug", func(b *testing.B) {
		globalLogLevel = zerolog.ErrorLevel
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Debug("filtered")
		}
	})

	b.Run("error", func(b *testing.B) {
		globalLogLevel = zerolog.ErrorLevel
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Error(logError)
		}
	})
}
