package logging

import (
	"context"
	"io"
	"log/slog"
)

type loggerKey struct{}

func WithLogger(ctx context.Context, log *slog.Logger) context.Context {
	if log == nil {
		return ctx
	}
	return context.WithValue(ctx, loggerKey{}, log)
}

func FromContext(ctx context.Context) *slog.Logger {
	if ctx != nil {
		if log, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok && log != nil {
			return log
		}
	}
	return slog.Default()
}

func LogClose(ctx context.Context, name string, c io.Closer) {
	if c == nil {
		return
	}
	if err := c.Close(); err != nil {
		FromContext(ctx).WarnContext(ctx, "close error", slog.String("op", name), slog.String("err", err.Error()))
	}
}
