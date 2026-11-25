// Package requestctx carries request metadata independently of a web framework.
package requestctx

import (
	"context"
	"log/slog"
)

type key int

const (
	requestIDKey key = iota
	serviceNameKey
	loggerKey
)

// WithRequestID derives a context with the given request ID.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestID returns the request ID, or an empty string when absent.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// WithServiceName derives a context with the given service name.
func WithServiceName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, serviceNameKey, name)
}

// ServiceName returns the service name, or an empty string when absent.
func ServiceName(ctx context.Context) string {
	name, _ := ctx.Value(serviceNameKey).(string)
	return name
}

// WithLogger derives a context with a logger that reads request metadata when
// logging. Use the logger's Context methods to supply the current context.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, contextualLogger(logger))
}

// Logger returns the request logger, or the process default outside a request.
func Logger(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return contextualLogger(slog.Default())
}

func contextualLogger(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}
	if _, ok := logger.Handler().(*logHandler); ok {
		return logger
	}
	return slog.New(&logHandler{logger.Handler()})
}

// logHandler is the single owner of request metadata in log records.
type logHandler struct{ slog.Handler }

func (h *logHandler) Handle(ctx context.Context, record slog.Record) error {
	record = record.Clone()
	if id := RequestID(ctx); id != "" {
		record.AddAttrs(slog.String("request_id", id))
	}
	if service := ServiceName(ctx); service != "" {
		record.AddAttrs(slog.String("service", service))
	}
	return h.Handler.Handle(ctx, record)
}

func (h *logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &logHandler{h.Handler.WithAttrs(attrs)}
}

func (h *logHandler) WithGroup(name string) slog.Handler {
	return &logHandler{h.Handler.WithGroup(name)}
}
