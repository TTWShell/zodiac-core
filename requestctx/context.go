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
	return slog.New(NewLogHandler(logger.Handler(), ""))
}

// NewLogHandler adds request metadata to records before passing them to next.
// The context's service name overrides defaultService. Use the logger's Context
// methods to supply request metadata. This does not change the default logger.
// Let this handler own request_id and service; do not also bind those attributes.
func NewLogHandler(next slog.Handler, defaultService string) slog.Handler {
	return &logHandler{next: next, defaultService: defaultService}
}

// logHandler is the single owner of request metadata in log records.
type logHandler struct {
	next           slog.Handler
	defaultService string
}

func (h *logHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *logHandler) Handle(ctx context.Context, record slog.Record) error {
	record = record.Clone()
	if id := RequestID(ctx); id != "" {
		record.AddAttrs(slog.String("request_id", id))
	}
	service := ServiceName(ctx)
	if service == "" {
		service = h.defaultService
	}
	if service != "" {
		record.AddAttrs(slog.String("service", service))
	}
	return h.next.Handle(ctx, record)
}

func (h *logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &logHandler{next: h.next.WithAttrs(attrs), defaultService: h.defaultService}
}

func (h *logHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &logHandler{next: h.next.WithGroup(name), defaultService: h.defaultService}
}
