package logr

import "context"

func Discard() Logger {
	return &discardLogger{}
}

type discardLogger struct{}

func (d *discardLogger) WithValues(keyAndValues ...any) Logger {
	return d
}

func (d *discardLogger) Start(ctx context.Context, name string, keyAndValues ...any) (context.Context, Logger) {
	return ctx, d
}

func (discardLogger) End() {
}

func (discardLogger) Debug(format string, args ...any) {
}

func (discardLogger) Info(format string, args ...any) {
}

func (discardLogger) Warn(err error) {
}

func (discardLogger) Error(err error) {
}
