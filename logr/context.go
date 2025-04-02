package logr

import "context"

type contextKey struct{}

func WithLogger(ctx context.Context, logger Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, logger)
}

func FromContext(ctx context.Context) Logger {
	logger, ok := ctx.Value(contextKey{}).(Logger)
	if !ok {
		return Discard()
	}
	return logger
}

func Start(ctx context.Context, name string, keyAndValues ...any) (context.Context, Logger) {
	return FromContext(ctx).Start(ctx, name, keyAndValues...)
}
