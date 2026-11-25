// Package httptransport provides Zodiac middleware for net/http handlers.
package httptransport

import (
	"bufio"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"slices"
	"time"

	"github.com/felixge/httpsnoop"
	"github.com/google/uuid"

	"github.com/TTWShell/zodiac-core/requestctx"
	"github.com/TTWShell/zodiac-core/response"
)

// Options configures request metadata and access logging. Zero values use
// X-Request-ID, UUID v4 generation, and the context logger with no path exclusions.
type Options struct {
	ServiceName  string
	ExcludePaths []string
	HeaderName   string
	GenerateID   func() string
	Logger       *slog.Logger
}

// Middleware supplies request metadata, access logging and panic recovery.
// Incoming IDs follow Python's length-36 rule. Exclusions match paths exactly.
// Response bodies are not buffered and optional writer interfaces are preserved.
func Middleware(opts Options) func(http.Handler) http.Handler {
	if opts.HeaderName == "" {
		opts.HeaderName = "X-Request-ID"
	}
	if opts.GenerateID == nil {
		opts.GenerateID = uuid.NewString
	}
	opts.ExcludePaths = slices.Clone(opts.ExcludePaths)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := requestctx.RequestID(r.Context())
			if id == "" {
				id = r.Header.Get(opts.HeaderName)
				if len(id) != 36 {
					id = opts.GenerateID()
				}
			}
			ctx := requestctx.WithRequestID(r.Context(), id)
			if opts.ServiceName != "" {
				ctx = requestctx.WithServiceName(ctx, opts.ServiceName)
			}
			logger := opts.Logger
			if logger == nil {
				logger = requestctx.Logger(ctx)
			}
			r = r.WithContext(requestctx.WithLogger(ctx, logger))
			logger = requestctx.Logger(r.Context())
			w.Header().Set(opts.HeaderName, id)

			start := time.Now()
			status, hijacked := 0, false
			commit := func(code int) {
				if status == 0 && (code >= 200 || code == http.StatusSwitchingProtocols) {
					status = code
				}
			}
			writer := httpsnoop.Wrap(w, httpsnoop.Hooks{
				WriteHeader: func(next httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
					return func(code int) { next(code); commit(code) }
				},
				Write: func(next httpsnoop.WriteFunc) httpsnoop.WriteFunc {
					return func(p []byte) (int, error) { commit(http.StatusOK); return next(p) }
				},
				ReadFrom: func(next httpsnoop.ReadFromFunc) httpsnoop.ReadFromFunc {
					return func(src io.Reader) (int64, error) { commit(http.StatusOK); return next(src) }
				},
				Flush: func(next httpsnoop.FlushFunc) httpsnoop.FlushFunc {
					return func() { commit(http.StatusOK); next() }
				},
				FlushError: func(next httpsnoop.FlushErrorFunc) httpsnoop.FlushErrorFunc {
					return func() error { commit(http.StatusOK); return next() }
				},
				Hijack: func(next httpsnoop.HijackFunc) httpsnoop.HijackFunc {
					return func() (net.Conn, *bufio.ReadWriter, error) {
						conn, rw, err := next()
						hijacked = err == nil
						return conn, rw, err
					}
				},
			})
			defer func() {
				if !slices.Contains(opts.ExcludePaths, r.URL.Path) {
					if status == 0 {
						status = http.StatusOK
					}
					logger.InfoContext(r.Context(), "HTTP request", "method", r.Method,
						"path", r.URL.Path, "status", status,
						"latency_ms", float64(time.Since(start))/float64(time.Millisecond))
				}
			}()
			defer func() {
				if value := recover(); value != nil {
					if value == http.ErrAbortHandler {
						panic(value)
					}
					logger.ErrorContext(r.Context(), "Unhandled panic", "error", value, "stack", string(debug.Stack()))
					if status != 0 || hijacked {
						// A committed or upgraded response cannot be replaced with JSON.
						panic(http.ErrAbortHandler)
					}
					writer.Header().Del("Content-Length")
					writer.Header().Del("Content-Encoding")
					if err := Write(writer, http.StatusInternalServerError, response.NewError(http.StatusInternalServerError, "")); err != nil {
						logger.ErrorContext(r.Context(), "Write error response", "error", err)
					}
				}
			}()
			next.ServeHTTP(writer, r)
		})
	}
}

// Write serializes a JSON response before committing its HTTP status.
func Write(w http.ResponseWriter, status int, body any) error {
	if status < 200 || status == http.StatusNoContent || status == http.StatusNotModified {
		w.WriteHeader(status)
		return nil
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, err = w.Write(data)
	return err
}

// ErrorHandler serves a standard error, for example a router's 404 or 405.
func ErrorHandler(status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := Write(w, status, response.NewError(status, "")); err != nil {
			requestctx.Logger(r.Context()).ErrorContext(r.Context(), "Write error response", "error", err)
		}
	}
}
