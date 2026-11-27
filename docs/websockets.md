# Add a WebSocket endpoint

Add an echo endpoint to the `myapp` project from [Create a service](getting-started.md).
This example uses [coder/websocket](https://pkg.go.dev/github.com/coder/websocket)
with the generated Chi adapter. Text and binary messages are echoed unchanged.

From the project directory, install the dependency:

```sh
go get github.com/coder/websocket@v1.8.15
```

## Write the handler

Create `internal/echo/echo.go`:

```go
package echo

import (
	"context"
	"net/http"
	"sync"
	"time"

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

	controller := http.NewResponseController(w)
	if err := controller.SetReadDeadline(time.Time{}); err != nil {
		httptransport.ErrorHandler(http.StatusInternalServerError)(w, r)
		return
	}
	if err := controller.SetWriteDeadline(time.Time{}); err != nil {
		httptransport.ErrorHandler(http.StatusInternalServerError)(w, r)
		return
	}
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
```

`RegisterOutput` passes control to the stream callback. `humachi.Unwrap` obtains
the HTTP request and writer needed by `websocket.Accept`. The handler then clears
the server's read and write deadlines. The
endpoint is hidden from OpenAPI because its messages are WebSocket frames, not
HTTP response bodies.

## Connect the module

Create `internal/echo/module.go`:

```go
package echo

import (
    "github.com/danielgtaylor/huma/v2"
    "go.uber.org/fx"
)

var Module = fx.Module("echo",
    fx.Provide(NewHandler),
    fx.Invoke(func(api huma.API, handler *Handler, lifecycle fx.Lifecycle) {
        handler.Register(api)
        lifecycle.Append(fx.Hook{OnStop: handler.Stop})
    }),
)
```

In `internal/app/app.go`, import `example.com/myapp/internal/echo` and add
`echo.Module` after `httpserver.Module` in the existing `fx.New(...)` call.

The stop hook sends close code `1001` (going away) to active clients and waits
for their close handshakes until the stop context expires. If it expires, the
hook returns the context error; unfinished handshakes remain subject to the
WebSocket library's timeouts. Upgrades already in progress are included in the
wait and close with `1001`; new upgrade requests receive HTTP 503. This hook is
necessary because
[`http.Server.Shutdown`](https://pkg.go.dev/net/http#Server.Shutdown) does not
close WebSocket connections.

## Try the endpoint

Start the application:

```sh
APPLICATION_ENVIRONMENT=develop go run ./cmd/myapp serve
```

Open <http://127.0.0.1:8080/docs> and run this in the browser's developer console:

```javascript
const socket = new WebSocket("ws://127.0.0.1:8080/ws/echo");
socket.onopen = () => socket.send("hello");
socket.onmessage = (event) => console.log(event.data);
socket.onclose = (event) => console.log("closed", event.code, event.reason);
```

The console prints `hello`. Stop the server with Ctrl+C while the socket is open;
the client receives close code `1001`. To close from the client instead, run
`socket.close(1000, "done")`.

## Messages and handshake errors

After the HTTP 101 upgrade, messages use your application's WebSocket protocol;
Zodiac does not add the JSON response envelope. Authentication or other business
checks can return an error from the registered handler before returning the
`StreamResponse`. Errors written by `websocket.Accept`, such as an invalid
handshake, use that library's HTTP response format.

The example keeps the library's same-origin check. If your browser client is
hosted elsewhere, configure `AcceptOptions.OriginPatterns` with its allowed
hosts. See [AcceptOptions](https://pkg.go.dev/github.com/coder/websocket#AcceptOptions).
