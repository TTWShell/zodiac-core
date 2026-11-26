package humatransport_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"

	"github.com/TTWShell/zodiac-core/requestctx"
	humatransport "github.com/TTWShell/zodiac-core/transport/huma"
)

func TestWebSocket(t *testing.T) {
	for _, serverCloses := range []bool{false, true} {
		name := "client_close"
		if serverCloses {
			name = "server_close"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			var logs bytes.Buffer
			router, api := newAPI(t, &logs)
			var requestID string
			var handlerErr error
			humatransport.RegisterOutput(api, huma.Operation{
				OperationID: "echo-websocket", Method: http.MethodGet, Path: "/ws/echo",
				DefaultStatus: http.StatusSwitchingProtocols, Hidden: true,
			}, func(context.Context, *struct{}) (*huma.StreamResponse, error) {
				return &huma.StreamResponse{Body: func(hctx huma.Context) {
					r, w := humachi.Unwrap(hctx)
					conn, err := websocket.Accept(w, r, nil)
					if err != nil {
						handlerErr = err
						return
					}
					defer conn.CloseNow()
					requestID = requestctx.RequestID(r.Context())
					for range 2 {
						kind, data, err := conn.Read(ctx)
						if err != nil {
							handlerErr = err
							return
						}
						if err := conn.Write(ctx, kind, data); err != nil {
							handlerErr = err
							return
						}
					}
					if serverCloses {
						handlerErr = conn.Close(websocket.StatusGoingAway, "server shutting down")
					} else if _, _, err := conn.Read(ctx); websocket.CloseStatus(err) != websocket.StatusNormalClosure {
						handlerErr = fmt.Errorf("client close: %v", err)
					}
				}}, nil
			})
			// Wait for middleware logging too: an upgraded connection outlives Server.Close.
			finished := make(chan struct{})
			started := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				defer close(finished)
				router.ServeHTTP(w, r)
			}))
			conn, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws/echo", nil)
			t.Cleanup(func() {
				cancel()
				if conn != nil {
					_ = conn.CloseNow()
				}
				server.Close()
				select {
				case <-started:
					select {
					case <-finished:
					case <-time.After(5 * time.Second):
						t.Error("WebSocket handler did not exit during cleanup")
					}
				default:
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != http.StatusSwitchingProtocols || len(res.Header.Get("X-Request-ID")) != 36 {
				t.Fatalf("handshake: %d %v", res.StatusCode, res.Header)
			}
			for _, msg := range []struct {
				kind websocket.MessageType
				body []byte
			}{
				{websocket.MessageText, []byte("hello")},
				{websocket.MessageBinary, []byte{0, 1, 254, 255}},
			} {
				if err := conn.Write(ctx, msg.kind, msg.body); err != nil {
					t.Fatal(err)
				}
				kind, body, err := conn.Read(ctx)
				if err != nil || kind != msg.kind || !bytes.Equal(body, msg.body) {
					t.Fatalf("echo: %v %q %v", kind, body, err)
				}
			}
			if serverCloses {
				if _, _, err := conn.Read(ctx); websocket.CloseStatus(err) != websocket.StatusGoingAway {
					t.Fatalf("server close: %v", err)
				}
			} else if err := conn.Close(websocket.StatusNormalClosure, "done"); err != nil {
				t.Fatal(err)
			}
			select {
			case <-finished:
			case <-ctx.Done():
				t.Fatal("WebSocket handler did not finish")
			}
			if handlerErr != nil {
				t.Fatal(handlerErr)
			}
			if requestID != res.Header.Get("X-Request-ID") || !strings.Contains(logs.String(), "request_id="+requestID) {
				t.Fatalf("request ID mismatch: header=%q context=%q logs=%s", res.Header.Get("X-Request-ID"), requestID, &logs)
			}
			if !strings.Contains(logs.String(), "status=101") || strings.Count(logs.String(), "HTTP request") != 1 {
				t.Fatalf("access log: %s", &logs)
			}
			if api.OpenAPI().Paths["/ws/echo"] != nil {
				t.Fatal("hidden WebSocket operation exposed in OpenAPI")
			}
		})
	}
}
