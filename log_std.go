package easygin

import (
	"context"
	"os"
	"strings"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type OutputType string

var (
	OutputAlways    OutputType = "Always"
	OutputOnFailure OutputType = "OnFailure"
	OutputNever     OutputType = "Never"
)

func OutputFilter(outputType OutputType) SpanMapper {
	return func(data sdktrace.ReadOnlySpan) sdktrace.ReadOnlySpan {
		if outputType == OutputNever {
			return nil
		}
		if outputType == OutputOnFailure {
			if data.Status().Code == codes.Ok {
				return nil
			}
		}
		return data
	}
}

type SpanMapper = func(data sdktrace.ReadOnlySpan) sdktrace.ReadOnlySpan

func WithSpanMapExporter(mappers ...SpanMapper) func(spanExporter sdktrace.SpanExporter) sdktrace.SpanExporter {
	return func(spanExporter sdktrace.SpanExporter) sdktrace.SpanExporter {
		return &spanMapExporter{
			mappers:      mappers,
			SpanExporter: spanExporter,
		}
	}
}

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

func StdoutSpanExporter() sdktrace.SpanExporter {
	return &stdoutSpanExporter{}
}

type stdoutSpanExporter struct{}

func (e *stdoutSpanExporter) Shutdown(ctx context.Context) error {
	return nil
}

// ExportSpan writes a SpanSnapshot in json format to stdout.
func (e *stdoutSpanExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	for i := range spans {
		data := spans[i]

		for _, event := range data.Events() {
			if event.Name == "" || event.Name[0] != '@' {
				continue
			}

			var lv zerolog.Level
			if err := lv.UnmarshalText([]byte(event.Name[1:])); err != nil {
				continue
			}

			// 使用zerolog，输出至stdout
			logger := zerolog.New(os.Stdout).With().Logger()
			logr := (&logger).Log().Time("time", event.Time).Str("level", strings.ToUpper(lv.String()))

			for _, kv := range event.Attributes {
				k := string(kv.Key)

				switch k {
				case "message":
					logr = logr.Str("msg", kv.Value.AsString())
				case "stack":
					logr = logr.Stack().Any("stack", kv.Value.AsInterface())
				default:
					logr = logr.Any(k, kv.Value.AsInterface())
				}
			}

			// 定义要过滤的字段列表，控制台不用输出这些字段
			fieldsToFilter := map[string]bool{
				"http.method":          true,
				"http.scheme":          true,
				"net.host.name":        true,
				"net.sock.peer.addr":   true,
				"net.sock.peer.port":   true,
				"user_agent.original":  true,
				"http.target":          true,
				"net.protocol.version": true,
				"http.route":           true,
				"http.status_code":     true,
				"net.host.port":        true,
			}

			// 过滤掉不需要的 span 属性
			for _, kv := range data.Attributes() {
				key := string(kv.Key)
				// 只添加不在过滤列表中的字段
				if !fieldsToFilter[key] {
					logr = logr.Any(key, kv.Value.AsInterface())
				}
			}

			logr = logr.Str("span", data.Name())

			if data.SpanContext().HasTraceID() {
				logr = logr.Any("traceID", data.SpanContext().TraceID())
			}

			if data.SpanContext().HasSpanID() {
				logr = logr.Any("spanID", data.SpanContext().SpanID())
			}

			if data.Parent().IsValid() {
				logr = logr.Any("parentSpanID", data.Parent().SpanID())
			}

			// 指定时间
			logr.Send()
		}
	}

	return nil
}
