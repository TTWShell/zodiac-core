package echo

import (
	"context"
	"net/http"
	"sync"

	"github.com/coder/websocket"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"

	httptransport "github.com/TTWShell/zodiac-core/transport/http"
	humatransport "github.com/TTWShell/zodiac-core/transport/huma"
)

type Handler struct {
	mu       sync.Mutex
	handlers sync.WaitGroup
	stopping bool
	conns    map[*websocket.Conn]struct{}
}

func NewHandler() *Handler {
	return &Handler{conns: make(map[*websocket.Conn]struct{})}
}

func (h *Handler) Register(api huma.API) {
	humatransport.RegisterOutput(api, huma.Operation{
		OperationID:   "echo-websocket",
		Method:        http.MethodGet,
		Path:          "/ws/echo",
		DefaultStatus: http.StatusSwitchingProtocols,
		Hidden:        true,
	}, func(context.Context, *struct{}) (*huma.StreamResponse, error) {
		return &huma.StreamResponse{Body: h.serve}, nil
	})
}

func (h *Handler) serve(ctx huma.Context) {
	r, w := humachi.Unwrap(ctx)
	h.mu.Lock()
	if h.stopping {
		h.mu.Unlock()
		httptransport.ErrorHandler(http.StatusServiceUnavailable)(w, r)
		return
	}
	// Admission and shutdown share the lock, so no Add can race with Stop's Wait.
	h.handlers.Add(1)
	h.mu.Unlock()
	defer h.handlers.Done()

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()

	h.mu.Lock()
	if h.stopping {
		h.mu.Unlock()
		_ = conn.Close(websocket.StatusGoingAway, "server shutting down")
		return
	}
	h.conns[conn] = struct{}{}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.conns, conn)
		h.mu.Unlock()
	}()

	conn.SetReadLimit(64 * 1024)
	for {
		kind, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		if err := conn.Write(r.Context(), kind, data); err != nil {
			return
		}
	}
}

func (h *Handler) Stop(ctx context.Context) error {
	h.mu.Lock()
	h.stopping = true
	conns := make([]*websocket.Conn, 0, len(h.conns))
	for conn := range h.conns {
		conns = append(conns, conn)
	}
	h.mu.Unlock()

	for _, conn := range conns {
		go func() {
			_ = conn.Close(websocket.StatusGoingAway, "server shutting down")
		}()
	}
	done := make(chan struct{})
	go func() {
		h.handlers.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
