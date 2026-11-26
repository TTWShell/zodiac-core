# Add a business endpoint

Add `GET /tickets/{id}` to the `myapp` project from
[Create a service](getting-started.md). A client will receive a ticket, a 404 when
it does not exist, or a validation error when its ID is invalid.

First, follow [Add a business setting](configuration.md#add-a-business-setting)
to define `cfg.Support.Team`, add its YAML value, and bind `MYAPP_SUPPORT_TEAM`.

## Write the ticket service and route

Create `internal/tickets/tickets.go` with the following contents. The example
contains one read-only sample ticket so you can try the API without a database.

```go
package tickets

import (
    "context"
    "log/slog"
    "net/http"

    "github.com/danielgtaylor/huma/v2"

    "github.com/TTWShell/zodiac-core/response"
    humatransport "github.com/TTWShell/zodiac-core/transport/huma"
)

type Ticket struct {
    ID    int    `json:"id"`
    Title string `json:"title"`
    Team  string `json:"team"`
}

type Service struct {
    logger  *slog.Logger
    tickets map[int]Ticket
}

func NewService(team string, logger *slog.Logger) *Service {
    return &Service{
        logger: logger,
        tickets: map[int]Ticket{
            1: {ID: 1, Title: "Cannot sign in", Team: team},
        },
    }
}

func (s *Service) Get(ctx context.Context, id int) (*Ticket, error) {
    ticket, ok := s.tickets[id]
    if !ok {
        return nil, response.NewError(http.StatusNotFound, "Ticket not found")
    }
    s.logger.InfoContext(ctx, "Ticket retrieved", "ticket_id", id)
    return &ticket, nil
}

type GetInput struct {
    ID int `path:"id" minimum:"1"`
}

func Register(api huma.API, service *Service) {
    humatransport.Register(api, huma.Operation{
        OperationID: "get-ticket",
        Method:      http.MethodGet,
        Path:        "/tickets/{id}",
        Summary:     "Get a ticket",
        Errors:      []int{http.StatusNotFound},
    }, func(ctx context.Context, input *GetInput) (*Ticket, error) {
        return service.Get(ctx, input.ID)
    })
}
```

The handler returns business data. `Register` supplies the response envelope and
exposes the input and output types in OpenAPI.

## Connect the service to your application

Create `internal/tickets/module.go`:

```go
package tickets

import (
    "log/slog"

    "go.uber.org/fx"

    "example.com/myapp/internal/config"
)

func newService(cfg *config.Config, logger *slog.Logger) *Service {
    return NewService(cfg.Support.Team, logger)
}

var Module = fx.Module("tickets",
    fx.Provide(newService),
    fx.Invoke(Register),
)
```

In `internal/app/app.go`, add this import:

```go
"example.com/myapp/internal/tickets"
```

Add `tickets.Module` inside the existing `fx.New(...)` call, keeping its current
options:

```go
tickets.Module,
```

## Try the endpoint

Start the application from its project directory:

```sh
APPLICATION_ENVIRONMENT=develop MYAPP_SUPPORT_TEAM=priority go run ./cmd/myapp serve
```

Request the sample ticket:

```sh
curl -i http://127.0.0.1:8080/tickets/1
```

HTTP 200:

```json
{"code":0,"data":{"id":1,"title":"Cannot sign in","team":"priority"},"message":"Success"}
```

Request a ticket that does not exist:

```sh
curl -i http://127.0.0.1:8080/tickets/2
```

HTTP 404:

```json
{"code":404,"data":null,"message":"Ticket not found"}
```

Request `/tickets/0` to receive HTTP 422:

```json
{"code":422,"data":{"errors":[{"message":"expected number >= 1","location":"path.id","value":0}]},"message":"validation failed"}
```

Visit <http://127.0.0.1:8080/docs> to see and call the new operation.

The successful request also emits a `Ticket retrieved` log with `ticket_id` and
`request_id`. Compare the latter with the response's `X-Request-ID` header.
See [logging](logging.md) for recording additional business events.

## Return other results

| What the endpoint needs | What to use |
|---|---|
| Business data with the standard envelope | `humatransport.Register` |
| HTTP 201 after creating a resource | `Register` with `DefaultStatus: http.StatusCreated` |
| A public business failure | Return `response.NewError(status, message)` |
| Response headers or a custom body | `humatransport.RegisterOutput` |
| A stream or HTTP 204 with no body | `RegisterOutput` with Huma's corresponding output type |

A nil result from `Register` is returned as `data: null` within the envelope.
Creation responses use business code `201` and message `Created`. Other successful
`Register` responses use code `0` and message `Success`.

Business error codes default to the HTTP status. Set the returned error's `Code`
and `Data` fields if clients need an application-specific code or public details.
Huma errors use the same envelope and retain their HTTP status and explicit
message. Client error details appear in `data.errors`; without details, `data`
is null. For 5xx errors, causes are logged instead of included in the response.
Unexpected handler errors return a generic HTTP 500 response.

Declare error statuses in the operation's `Errors` field. Use `Responses` for
custom response documentation; an entry in `Errors` overrides `Responses` for
the same status.

To combine a header with a Zodiac envelope using `RegisterOutput`, declare:

```go
type TicketOutput struct {
    ETag string `header:"ETag"`
    Body humatransport.Body[Ticket]
}
```

Return `&TicketOutput{ETag: etag, Body: humatransport.Body[Ticket](response.OK(ticket))}`
from its handler, where `ticket` is a `Ticket` value. See
[Huma response outputs](https://huma.rocks/features/response-outputs/) for streaming
and empty responses, and the
[registration API reference](https://pkg.go.dev/github.com/TTWShell/zodiac-core/transport/huma)
for complete signatures.
