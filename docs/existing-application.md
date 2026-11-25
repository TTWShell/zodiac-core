# Use ZodiacCore in an existing application

You can adopt configuration, logging, or HTTP helpers separately.

From your application's Go module:

```sh
go get github.com/TTWShell/zodiac-core@latest
```

Requires Go 1.26.0 or newer.

## Add configuration or logging

For configuration, define your application settings and load them using
`config.Load`. The [standalone configuration example](configuration.md#use-configuration-without-the-generator)
includes validation and an environment override.

For logging, create a logger at startup and pass it to your application's
constructors:

```go
logger := logging.New("myapp", slog.LevelInfo)
```

Import `log/slog` and `github.com/TTWShell/zodiac-core/logging`.
See [logging](logging.md) to set the process default or use a different output.
Neither package requires Fx or an HTTP server.

## Add an HTTP API with chi and Huma

Create `internal/httpapi/http.go` to provide a handler for your existing server:

```go
package httpapi

import (
    "context"
    "log/slog"
    "net/http"

    "github.com/danielgtaylor/huma/v2"
    "github.com/danielgtaylor/huma/v2/adapters/humachi"
    "github.com/go-chi/chi/v5"

    "github.com/TTWShell/zodiac-core/response"
    httptransport "github.com/TTWShell/zodiac-core/transport/http"
    humatransport "github.com/TTWShell/zodiac-core/transport/huma"
)

type Ticket struct {
    ID    int    `json:"id"`
    Title string `json:"title"`
}

type GetTicketInput struct {
    ID int `path:"id" minimum:"1"`
}

func NewHandler(logger *slog.Logger) http.Handler {
    router := chi.NewRouter()
    router.Use(httptransport.Middleware(httptransport.Options{
        Logger: logger, ServiceName: "myapp",
    }))
    router.NotFound(httptransport.ErrorHandler(http.StatusNotFound))
    api := humachi.New(router, humatransport.DefaultConfig("My App", "1.0.0"))
    humatransport.Register(api, huma.Operation{
        OperationID: "get-ticket",
        Method:      http.MethodGet,
        Path:        "/tickets/{id}",
        Errors:      []int{http.StatusNotFound},
    }, func(ctx context.Context, input *GetTicketInput) (*Ticket, error) {
        if input.ID != 1 {
            return nil, response.NewError(http.StatusNotFound, "Ticket not found")
        }
        logger.InfoContext(ctx, "Ticket retrieved", "ticket_id", input.ID)
        return &Ticket{ID: 1, Title: "Cannot sign in"}, nil
    })
    return router
}
```

Run `go mod tidy` after adding the file. In your server's constructor, use
`httpapi.NewHandler(logger)` as its `Handler`. Import `internal/httpapi` using
your application's module path. Keep your server's existing listener, timeouts,
and shutdown handling.

Once that server is running, request `/tickets/1` for the sample ticket and
`/tickets/2` for a public 404. Interactive API documentation is served at `/docs`.
The [ticket tutorial](http.md) shows how to inject a business service when using Fx.

## Keep an existing router

For an existing chi router, install `httptransport.Middleware` before registering
routes, create the Huma API with `humachi.New`, and register your new operations
on that API.

For another `net/http` router, the middleware can wrap its handler directly:

```go
handler := httptransport.Middleware(httptransport.Options{
    Logger: logger, ServiceName: "myapp",
})(existingHandler)
```

This adds request metadata, access logging, and panic recovery. It does not
change the successful response bodies produced by your existing handlers.
To use typed responses and OpenAPI, construct a Huma API with an appropriate
router adapter and register operations through `humatransport.Register`.
