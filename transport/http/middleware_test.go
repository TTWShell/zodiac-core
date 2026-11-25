package httptransport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/TTWShell/zodiac-core/requestctx"
	httptransport "github.com/TTWShell/zodiac-core/transport/http"
)

func TestRequestMetadataAndAccessLogs(t *testing.T) {
	var logs bytes.Buffer
	const generated = "12345678-1234-4234-8234-123456789abc"
	const incoming = "abcdefgh-ijkl-mnop-qrst-uvwxyz123456"
	opts := httptransport.Options{
		ServiceName: "example", HeaderName: "X-Trace-ID", GenerateID: func() string { return generated },
		ExcludePaths: []string{"/skip"}, Logger: slog.New(slog.NewJSONHandler(&logs, nil)),
	}
	handler := httptransport.Middleware(opts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := incoming
		if r.URL.Path == "/skip" {
			want = generated
		}
		if requestctx.RequestID(r.Context()) != want || requestctx.ServiceName(r.Context()) != "example" {
			t.Error("request metadata missing or shared between requests")
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	var group sync.WaitGroup
	for _, path := range []string{"/normal", "/skip"} {
		group.Go(func() {
			r := httptest.NewRequest("GET", path, nil)
			id := incoming
			if path == "/skip" {
				id = "invalid"
			}
			r.Header.Set("X-Trace-ID", id)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			want := incoming
			if path == "/skip" {
				want = generated
			}
			if w.Code != 202 || w.Header().Get("X-Trace-ID") != want {
				t.Errorf("wrong response: %d %v", w.Code, w.Header())
			}
			if requestctx.RequestID(r.Context()) != "" {
				t.Error("original request context mutated")
			}
		})
	}
	group.Wait()
	var entry map[string]any
	decoder := json.NewDecoder(&logs)
	if err := decoder.Decode(&entry); err != nil {
		t.Fatal(err)
	}
	if entry["request_id"] != incoming || entry["service"] != "example" || entry["path"] != "/normal" || entry["status"] != float64(202) || entry["latency_ms"] == nil {
		t.Fatalf("wrong access log: %v", entry)
	}
	if err := decoder.Decode(&entry); err != io.EOF {
		t.Fatalf("excluded path logged: %v", err)
	}
	if requestctx.ServiceName(context.Background()) != "" {
		t.Fatal("service name leaked")
	}
}

func TestRecoveryPreservesCommittedStreams(t *testing.T) {
	for _, flushed := range []bool{false, true} {
		var logs bytes.Buffer
		handler := httptransport.Middleware(httptransport.Options{Logger: slog.New(slog.NewJSONHandler(&logs, nil))})(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if flushed {
					_, _ = io.WriteString(w, "event\n")
					if err := http.NewResponseController(w).Flush(); err != nil {
						t.Error(err)
					}
				}
				panic("private failure")
			}))
		w := httptest.NewRecorder()
		var recovered any
		func() {
			defer func() { recovered = recover() }()
			handler.ServeHTTP(w, httptest.NewRequest("GET", "/events", nil))
		}()
		if len(w.Header().Get("X-Request-ID")) != 36 {
			t.Fatal("missing generated request ID")
		}
		if flushed {
			if recovered != http.ErrAbortHandler || w.Body.String() != "event\n" || !w.Flushed {
				t.Fatalf("stream corrupted: %v %s", recovered, w.Body)
			}
		} else if recovered != nil || w.Code != 500 || w.Body.String() != `{"code":500,"data":null,"message":"Internal Server Error"}` {
			t.Fatalf("panic response: %v %d %s", recovered, w.Code, w.Body)
		}
		if !strings.Contains(logs.String(), "private failure") || !strings.Contains(logs.String(), "HTTP request") {
			t.Fatalf("missing error or access log: %s", &logs)
		}
	}
}

func TestNestedRequestMetadata(t *testing.T) {
	for _, existing := range []string{"", "trusted-context-id"} {
		t.Run(existing, func(t *testing.T) {
			var logs bytes.Buffer
			const generated = "11111111-1111-4111-8111-111111111111"
			want := existing
			if want == "" {
				want = generated
			}
			calls := 0
			child := httptransport.Middleware(httptransport.Options{
				ServiceName: "child",
				GenerateID:  func() string { t.Fatal("child regenerated ID"); return "" },
			})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if requestctx.RequestID(r.Context()) != want || requestctx.ServiceName(r.Context()) != "child" {
					t.Fatal("child lost request metadata")
				}
				requestctx.Logger(r.Context()).With("component", "handler").InfoContext(r.Context(), "business")
			}))
			parent := httptransport.Middleware(httptransport.Options{
				ServiceName: "parent", Logger: slog.New(slog.NewJSONHandler(&logs, nil)),
				GenerateID: func() string { calls++; return generated },
			})(child)
			r := httptest.NewRequest("GET", "/nested", nil)
			if existing != "" {
				r = r.WithContext(requestctx.WithRequestID(r.Context(), existing))
				r.Header.Set("X-Request-ID", generated)
			}
			w := httptest.NewRecorder()
			parent.ServeHTTP(w, r)
			if w.Header().Get("X-Request-ID") != want || (existing == "" && calls != 1) || (existing != "" && calls != 0) {
				t.Fatal("request ID was replaced or generated more than once")
			}
			lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
			if len(lines) != 3 {
				t.Fatalf("expected business and two access logs: %s", &logs)
			}
			for i, line := range lines {
				// Decoding into a map alone would silently discard duplicate keys.
				if strings.Count(line, `"request_id":`) != 1 || strings.Count(line, `"service":`) != 1 {
					t.Fatalf("duplicate metadata: %s", line)
				}
				var entry map[string]any
				if err := json.Unmarshal([]byte(line), &entry); err != nil {
					t.Fatal(err)
				}
				service := "child"
				if i == 2 {
					service = "parent"
				}
				if entry["request_id"] != want || entry["service"] != service {
					t.Fatalf("wrong scope: %s", line)
				}
				if i == 0 && entry["component"] != "handler" {
					t.Fatal("logger attributes lost")
				}
			}
		})
	}
}
