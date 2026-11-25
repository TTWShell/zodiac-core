package requestctx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"testing/slogtest"

	"github.com/TTWShell/zodiac-core/requestctx"
)

func TestLogHandlerContract(t *testing.T) {
	var buf bytes.Buffer
	handler := requestctx.NewLogHandler(slog.NewJSONHandler(&buf, nil), "")
	err := slogtest.TestHandler(handler, func() []map[string]any {
		var entries []map[string]any
		for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
			var entry map[string]any
			if err := json.Unmarshal(line, &entry); err != nil {
				t.Fatal(err)
			}
			entries = append(entries, entry)
		}
		return entries
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLogHandlerServiceFallback(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	logger := slog.New(requestctx.NewLogHandler(slog.NewJSONHandler(&buf, nil), "parent"))
	logger = logger.With("component", "worker")
	ctx := requestctx.WithServiceName(context.Background(), "child")
	ctx = requestctx.WithRequestID(ctx, "request-1")
	ctx = requestctx.WithLogger(ctx, logger.WithGroup("job"))
	requestctx.Logger(ctx).InfoContext(ctx, "Business event")
	slog.NewLogLogger(logger.Handler(), slog.LevelError).Print("Server error")
	if slog.Default() != previous {
		t.Fatal("library changed the process default logger")
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("unexpected log output: %s", &buf)
	}
	for i, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}
		if entry["component"] != "worker" || strings.Count(line, `"service":`) != 1 {
			t.Fatalf("lost attributes or duplicate service: %s", line)
		}
		if i == 0 {
			group, ok := entry["job"].(map[string]any)
			if !ok || group["service"] != "child" || group["request_id"] != "request-1" || strings.Count(line, `"request_id":`) != 1 {
				t.Fatalf("request metadata lost or duplicated: %s", line)
			}
		} else if entry["service"] != "parent" || entry["request_id"] != nil || entry["level"] != "ERROR" {
			t.Fatalf("incorrect background log: %s", line)
		}
	}
}
