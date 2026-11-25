// Package logging provides application logging defaults.
package logging

import (
	"log/slog"
	"os"

	"github.com/TTWShell/zodiac-core/requestctx"
)

// New returns a logger that writes JSON to stderr at the given minimum level,
// including service and request metadata. A nil level defaults to INFO.
// It does not change the process default logger.
func New(service string, level slog.Leveler) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	return slog.New(requestctx.NewLogHandler(handler, service))
}
