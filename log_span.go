package easygin

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/zboyco/easygin/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// 全局日志等级
var globalLogLevel = zerolog.InfoLevel

// SetLogLevel 设置全局日志等级，应在服务启动前调用。
func SetLogLevel(l Level) {
	globalLogLevel = zerolog.Level(l)
}

type Level int8

const (
	// DebugLevel defines debug log level.
	DebugLevel Level = iota
	// InfoLevel defines info log level.
	InfoLevel
	// WarnLevel defines warn log level.
	WarnLevel
	// ErrorLevel defines error log level.
	ErrorLevel
)

func (l Level) String() string {
	return zerolog.Level(l).String()
}

// ParseLevel converts a level string into a Level value.
// returns an error if the input string does not match known values.
func ParseLevel(levelStr string) (Level, error) {
	l, err := zerolog.ParseLevel(levelStr)
	if err != nil {
		return InfoLevel, err
	}
	return Level(l), nil
}

// UnmarshalText implements encoding.TextUnmarshaler to allow for easy reading from toml/yaml/json formats
func (l *Level) UnmarshalText(text []byte) error {
	var zl zerolog.Level
	err := zl.UnmarshalText(text)
	if err != nil {
		return err
	}

	*l = Level(zl)
	return nil
}

// MarshalText implements encoding.TextMarshaler to allow for easy writing into toml/yaml/json formats
func (l Level) MarshalText() ([]byte, error) {
	return zerolog.Level(l).MarshalText()
}

func SpanLogger(serviceName string, span trace.Span) logr.Logger {
	return &spanLogger{
		serviceName: serviceName,
		span:        span,
	}
}

type spanLogger struct {
	serviceName string
	span        trace.Span
	attributes  []attribute.KeyValue
}

func (t *spanLogger) Start(ctx context.Context, name string, keyAndValues ...interface{}) (context.Context, logr.Logger) {
	provider := otel.GetTracerProvider()
	if t != nil && t.span != nil {
		provider = t.span.TracerProvider()
	}
	childCtx, childSpan := provider.Tracer(t.serviceName).Start(
		ctx, name,
		trace.WithAttributes(attrsFromKeyAndValues(keyAndValues...)...),
		trace.WithTimestamp(time.Now()),
	)
	return childCtx, &spanLogger{
		serviceName: t.serviceName,
		span:        childSpan,
	}
}

func (t *spanLogger) End() {
	if t == nil || t.span == nil {
		return
	}

	t.span.End(trace.WithTimestamp(time.Now()))
}

func (t *spanLogger) WithValues(keyAndValues ...interface{}) logr.Logger {
	attributes := make([]attribute.KeyValue, len(t.attributes), len(t.attributes)+len(keyAndValues)/2)
	copy(attributes, t.attributes)
	attributes = appendAttrsFromKeyAndValues(attributes, keyAndValues...)
	return &spanLogger{
		serviceName: t.serviceName,
		span:        t.span,
		attributes:  attributes,
	}
}

// info 在服务端级别过滤通过后格式化并记录普通日志。
func (t *spanLogger) info(level zerolog.Level, msgOrFormat string, args ...interface{}) {
	if level < globalLogLevel {
		return
	}

	message := msgOrFormat
	if len(args) > 0 {
		message = fmt.Sprintf(msgOrFormat, args...)
	}
	t.logEvent(level).Str("msg", message).Send()
	if t.span != nil && t.span.IsRecording() {
		t.span.AddEvent(
			"log."+level.String(),
			trace.WithTimestamp(time.Now()),
			trace.WithAttributes(t.attributes...),
			trace.WithAttributes(attribute.String("message", message)),
		)
	}
}

// error 记录错误日志，并将同一份错误堆栈复用于 zerolog 和 recording Span。
func (t *spanLogger) error(level zerolog.Level, err error) {
	if level < globalLogLevel || err == nil {
		return
	}

	event := t.logEvent(level).Str("msg", err.Error())
	var stack string
	if level >= zerolog.ErrorLevel {
		stack = fmt.Sprintf("%+v", err)
		event = event.Str("stack", stack)
	}
	event.Send()

	if t.span == nil || !t.span.IsRecording() {
		return
	}

	attributes := make([]attribute.KeyValue, len(t.attributes), len(t.attributes)+2)
	copy(attributes, t.attributes)
	attributes = append(attributes, attribute.String("message", err.Error()))
	if level >= zerolog.ErrorLevel {
		attributes = append(attributes, attribute.String("stack", stack))
		t.span.SetStatus(codes.Error, "")
	}
	t.span.RecordError(err)
	t.span.AddEvent(
		"log."+level.String(),
		trace.WithTimestamp(time.Now()),
		trace.WithAttributes(attributes...),
	)
}

// logEvent 创建携带结构化字段和 Trace 关联信息的 zerolog 事件。
func (t *spanLogger) logEvent(level zerolog.Level) *zerolog.Event {
	logger := zerolog.New(os.Stderr)
	event := logger.WithLevel(level).Timestamp()
	for _, field := range t.attributes {
		event = event.Str(string(field.Key), field.Value.AsString())
	}
	if t.span == nil {
		return event
	}
	spanContext := t.span.SpanContext()
	if spanContext.HasTraceID() {
		event = event.Str("traceID", spanContext.TraceID().String())
	}
	if spanContext.HasSpanID() {
		event = event.Str("spanID", spanContext.SpanID().String())
	}
	return event
}

func (t *spanLogger) Debug(msgOrFormat string, args ...interface{}) {
	t.info(zerolog.DebugLevel, msgOrFormat, args...)
}

func (t *spanLogger) Info(msgOrFormat string, args ...interface{}) {
	t.info(zerolog.InfoLevel, msgOrFormat, args...)
}

func (t *spanLogger) Warn(err error) {
	t.error(zerolog.WarnLevel, err)
}

func (t *spanLogger) Error(err error) {
	t.error(zerolog.ErrorLevel, err)
}

// attrsFromKeyAndValues 将合法键值对规范化为字符串属性，并忽略非字符串 key。
func attrsFromKeyAndValues(keysAndValues ...interface{}) []attribute.KeyValue {
	return appendAttrsFromKeyAndValues(nil, keysAndValues...)
}

// appendAttrsFromKeyAndValues 将合法键值对直接追加到目标切片，避免 WithValues 创建临时属性切片。
func appendAttrsFromKeyAndValues(attributes []attribute.KeyValue, keysAndValues ...interface{}) []attribute.KeyValue {
	n := len(keysAndValues)
	if n > 0 && n%2 == 0 {
		for i := 0; i < n; i += 2 {
			key, ok := keysAndValues[i].(string)
			if !ok {
				continue
			}
			attributes = append(attributes, attribute.String(key, fmt.Sprint(keysAndValues[i+1])))
		}
	}
	return attributes
}

func Sprintf(format string, args ...interface{}) fmt.Stringer {
	return &printer{format: format, args: args}
}

type printer struct {
	format string
	args   []interface{}
}

func (p *printer) String() string {
	if len(p.args) == 0 {
		return p.format
	}
	return fmt.Sprintf(p.format, p.args...)
}

type SpanMapper = func(data sdktrace.ReadOnlySpan) sdktrace.ReadOnlySpan
