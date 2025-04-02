package logr

import "context"

type Logger interface {
	// Start start span for tracing
	//
	// 	ctx log = logr.Start(ctx, "SpanName")
	// 	defer log.End()
	//
	Start(ctx context.Context, name string, keyAndValues ...any) (context.Context, Logger)
	// End end span
	End()

	// WithValues key value pairs
	WithValues(keyAndValues ...any) Logger

	// Debug debug info
	Debug(msg string, args ...any)
	// Info info
	Info(msg string, args ...any)

	// Warn
	Warn(err error)

	// Error
	Error(err error)
}
