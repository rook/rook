package flume

import (
	"context"
)

// DefaultLogger is returned by FromContext if no other logger has been
// injected into the context.
var DefaultLogger = New("")

type ctxKey struct{}

var loggerKey = &ctxKey{}

// WithLogger returns a new context with the specified logger injected into it.
func WithLogger(ctx context.Context, l Logger) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

// FromContext returns a logger from the context.  If the context
// doesn't contain a logger, the DefaultLogger will be returned.
func FromContext(ctx context.Context) Logger {
	if l, ok := ctx.Value(loggerKey).(Logger); ok {
		return l
	}
	return DefaultLogger
}
