package echo

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	humatransport "github.com/TTWShell/zodiac-core/transport/huma"
)

// Pause after hijacking but before Accept returns and the connection is registered.
type pausedUpgrade struct {
	http.ResponseWriter
	entered, release chan struct{}
}

func (w pausedUpgrade) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := w.ResponseWriter.(http.Hijacker).Hijack()
	close(w.entered)
	<-w.release
	return conn, rw, err
}

func TestStopDuringUpgrade(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	handler := NewHandler()
	router := chi.NewRouter()
	handler.Register(humachi.New(router, humatransport.DefaultConfig("Test", "1")))
	entered, release := make(chan struct{}), make(chan struct{})
	resume := sync.OnceFunc(func() { close(release) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery == "pause" {
			w = pausedUpgrade{w, entered, release}
		}
		router.ServeHTTP(w, r)
	}))
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/echo"
	type dialResult struct {
		conn *websocket.Conn
		err  error
	}
	dialed := make(chan dialResult, 1)
	go func() {
		conn, _, err := websocket.Dial(ctx, url+"?pause", nil)
		dialed <- dialResult{conn, err}
	}()
	var conn *websocket.Conn
	received := false
	t.Cleanup(func() {
		cancel()
		resume()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if !received {
			select {
			case result := <-dialed:
				conn = result.conn
			case <-cleanupCtx.Done():
				t.Error("WebSocket dial did not exit")
			}
		}
		if conn != nil {
			_ = conn.CloseNow()
		}
		if err := handler.Stop(cleanupCtx); err != nil {
			t.Error(err)
		}
		server.Close()
	})
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("upgrade did not reach Hijack")
	}

	// Stop must wait for this upgrade, even though it is absent from the connection map.
	stopCtx, stopCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	err := handler.Stop(stopCtx)
	stopCancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop during a blocked upgrade: %v", err)
	}
	resume()
	select {
	case result := <-dialed:
		received, conn = true, result.conn
		if result.err != nil {
			t.Fatal(result.err)
		}
	case <-ctx.Done():
		t.Fatal("upgrade did not finish")
	}
	if _, _, err := conn.Read(ctx); websocket.CloseStatus(err) != websocket.StatusGoingAway {
		t.Fatalf("shutdown close: %v", err)
	}
	if err := handler.Stop(ctx); err != nil {
		t.Fatal(err)
	}

	unexpected, res, err := websocket.Dial(ctx, url, nil)
	if unexpected != nil {
		_ = unexpected.CloseNow()
	}
	if res != nil && res.Body != nil {
		defer res.Body.Close()
	}
	if err == nil || res == nil || res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("upgrade after shutdown: response=%v error=%v", res, err)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil || strings.TrimSpace(string(body)) != `{"code":503,"data":null,"message":"Service Unavailable"}` {
		t.Fatalf("shutdown response: %s %v", body, err)
	}
}
