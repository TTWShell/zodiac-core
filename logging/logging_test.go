package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/TTWShell/zodiac-core/logging"
	"github.com/TTWShell/zodiac-core/requestctx"
)

func TestNewDefaultsAndRequestMetadata(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	stderr, previous := os.Stderr, slog.Default()
	os.Stderr = file
	t.Cleanup(func() { os.Stderr = stderr })
	logger := logging.New("parent", nil)
	os.Stderr = stderr
	if slog.Default() != previous {
		t.Fatal("New changed the process default logger")
	}
	ctx := requestctx.WithServiceName(context.Background(), "child")
	ctx = requestctx.WithRequestID(ctx, "request-1")
	logger.DebugContext(ctx, "Hidden event")
	logger.InfoContext(ctx, "Request event")
	logger.Info("Background event")
	data, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("unexpected log output: %s", data)
	}
	for i, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatal(err)
		}
		if entry["level"] != "INFO" {
			t.Fatalf("unexpected log level: %s", line)
		}
		if i == 0 {
			if entry["service"] != "child" || entry["request_id"] != "request-1" {
				t.Fatalf("missing request metadata: %s", line)
			}
		} else if entry["service"] != "parent" || entry["request_id"] != nil {
			t.Fatalf("incorrect background metadata: %s", line)
		}
	}
}
