package easygin

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"github.com/zboyco/easygin/logr"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// 全局日志等级
var globalLogLevel = zerolog.InfoLevel

// SetLogLevel 设置全局日志等级
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
	ignore      bool
}

func (t *spanLogger) Start(ctx context.Context, name string, keyAndValues ...interface{}) (context.Context, logr.Logger) {
	childCtx, childSpan := t.span.TracerProvider().Tracer(t.serviceName).Start(
		ctx, name,
		trace.WithAttributes(attrsFromKeyAndValues(keyAndValues...)...),
		trace.WithTimestamp(time.Now()),
	)
	return childCtx, &spanLogger{span: childSpan}
}

func (t *spanLogger) End() {
	if t.ignore {
		return
	}

	t.span.End(trace.WithTimestamp(time.Now()))
}

func (t *spanLogger) WithValues(keyAndValues ...interface{}) logr.Logger {
	t.attributes = append(t.attributes, attrsFromKeyAndValues(keyAndValues...)...)
	return t
}

func (t *spanLogger) info(level zerolog.Level, msg fmt.Stringer) {
	if level < globalLogLevel {
		t.ignore = true
		return
	}

	t.span.SetStatus(codes.Ok, "")

	t.span.AddEvent(
		"@"+level.String(),
		trace.WithTimestamp(time.Now()),
		trace.WithAttributes(t.attributes...),
		trace.WithAttributes(
			attribute.Stringer("message", msg),
		),
	)
}

func (t *spanLogger) error(level zerolog.Level, err error) {
	if level < globalLogLevel {
		t.ignore = true
		return
	}

	if t.span == nil || err == nil || !t.span.IsRecording() {
		return
	}

	attributes := append(t.attributes, attribute.String("message", err.Error()))

	if level >= zerolog.ErrorLevel {
		attributes = append(attributes, attribute.String("stack", fmt.Sprintf("%+v", err)))
	}

	t.span.SetStatus(codes.Error, "")

	t.span.RecordError(err)
	t.span.AddEvent(
		"@"+level.String(),
		trace.WithTimestamp(time.Now()),
		trace.WithAttributes(attributes...),
	)
}

func (t *spanLogger) Debug(msgOrFormat string, args ...interface{}) {
	t.info(zerolog.DebugLevel, Sprintf(msgOrFormat, args...))
}

func (t *spanLogger) Info(msgOrFormat string, args ...interface{}) {
	t.info(zerolog.InfoLevel, Sprintf(msgOrFormat, args...))
}

func (t *spanLogger) Warn(err error) {
	t.error(zerolog.WarnLevel, err)
}

func (t *spanLogger) Error(err error) {
	t.error(zerolog.ErrorLevel, err)
}

func attrsFromKeyAndValues(keysAndValues ...interface{}) []attribute.KeyValue {
	n := len(keysAndValues)
	if n > 0 && n%2 == 0 {
		fields := make([]attribute.KeyValue, len(keysAndValues)/2)
		for i := range fields {
			k, v := keysAndValues[2*i], keysAndValues[2*i+1]

			if s, ok := k.(string); ok {
				fields[i] = attribute.String(s, fmt.Sprint(v))
			}
		}
		return fields
	}
	return nil
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

type spanMapExporter struct {
	mappers []SpanMapper
	sdktrace.SpanExporter
}

func (e *spanMapExporter) ExportSpans(ctx context.Context, spanData []sdktrace.ReadOnlySpan) error {
	finalSpanSnapshot := make([]sdktrace.ReadOnlySpan, 0)

	mappers := e.mappers

	for i := range spanData {
		data := spanData[i]

		for _, m := range mappers {
			data = m(data)
		}

		if data != nil {
			finalSpanSnapshot = append(finalSpanSnapshot, data)
		}
	}

	if len(finalSpanSnapshot) == 0 {
		return nil
	}

	return e.SpanExporter.ExportSpans(ctx, finalSpanSnapshot)
}
