# ZodiacCore Go

A public Go library with a Cobra-based developer CLI. `zodiac new` generates a
business service that consumes the library.

The project follows the design of [ZodiacCore-Py](https://github.com/TTWShell/ZodiacCore-Py),
adapted to Go and its ecosystem.

## Generate a project locally

Requires Go 1.26.0 or newer. From this repository:

```sh
go run ./cmd/zodiac new demo-service \
  --module example.com/demo-service \
  --core-path . \
  --output /tmp/demo-service
```

Then run the generated application:

```sh
cd /tmp/demo-service
go mod tidy
APPLICATION_ENVIRONMENT=develop go run . serve --config-dir ./config
```

In another terminal:

```sh
curl http://127.0.0.1:8080/health
```

The response is `{"code":0,"data":{"status":"ok"},"message":"Success"}`.
API documentation is at `/docs`, with OpenAPI at `/openapi.json`.
Ctrl+C gracefully stops the application.

## Build the developer CLI

```sh
go build -o bin/zodiac ./cmd/zodiac
./bin/zodiac new demo-service --module example.com/demo-service --core-path .
```

`--module` is required. `--output` defaults to `./<name>`. The destination must
be absent or empty; existing files and symbolic-link destinations are rejected.
Project names use lowercase letters, digits, and single hyphens, starting with
a letter. The CLI does not download generated-project dependencies or run Git.

`--core-path` explicitly adds an absolute local `replace` directive for development
against this unpublished checkout. A CLI installed from a release uses its own
module version by default; `--core-version` selects a specific published version.
Development builds require one of these options instead of generating an
unresolvable library dependency. A local replacement must be removed or updated
before sharing a generated project with another machine.

## Generated project

```text
demo-service/
├── main.go
├── go.mod
├── .gitignore
├── README.md
├── cmd/
│   ├── root.go
│   └── serve.go
├── config/
│   ├── app.yaml
│   ├── app.develop.yaml
│   ├── app.testing.yaml
│   └── app.production.yaml
└── internal/
    ├── app/
    │   ├── app.go
    │   ├── router.go
    │   └── server.go
    ├── config/
    │   └── config.go
    └── health/
        └── health.go
```

## HTTP APIs

Create a Huma API with `humatransport.DefaultConfig` and register operations with
`humatransport.Register`. Both are in `github.com/TTWShell/zodiac-core/transport/huma`.
The configuration works with Huma router adapters; the generated application uses
`humachi.New`.

```go
type Greeting struct {
    Name string `json:"name"`
}

humatransport.Register(api, huma.Operation{
    OperationID: "get-greeting",
    Method:      http.MethodGet,
    Path:        "/greeting",
}, func(ctx context.Context, input *struct{}) (*Greeting, error) {
    return &Greeting{Name: "world"}, nil
})
```

`Register` takes a handler returning `(T, error)`, where `T` is the business data.
The response is `{"code":0,"data":...,"message":"Success"}`.
Nil data produces `data: null`. Set the operation's `DefaultStatus` to 201 for HTTP 201 with business
code 201 and message `Created`.

Use `RegisterOutput` when the handler needs Huma's complete output contract:
response headers, explicit envelopes, raw bytes, streaming or no-body responses.
It preserves the returned output without automatic wrapping. For an explicit
Zodiac envelope, use a value `humatransport.Body[T]` as the output's `Body`:

```go
type GreetingOutput struct {
    ETag string `header:"ETag"`
    Body humatransport.Body[Greeting]
}
```

Populate that body with a struct literal or convert `response.OK(data)` to
`humatransport.Body[Greeting]`. Use native `Body []byte` for raw responses,
`Body func(huma.Context)` for streams, and an output without `Body` for 204.
Register routes before serving requests; declare additional response statuses
and custom media types using Huma's operation fields.

Both registration functions preserve public errors and hide unknown failures.
Use `response.NewError(status, message)` for business errors; its `Code` and
`Data` fields are independent of the HTTP status. Explicit Huma errors retain
their messages and headers, and validation errors retain Huma's native 422
format. Unknown errors are logged and return a generic Zodiac 500 response.

`httptransport.Middleware` from `transport/http` adds request IDs, optional service
names, access logs and panic recovery to `net/http` handlers. Its options support
an exact-path log exclusion list, a custom ID header/generator, and a `*slog.Logger`.
Use `requestctx.RequestID(ctx)`, `requestctx.ServiceName(ctx)` and
`requestctx.Logger(ctx)` in business code. Use context methods such as
`InfoContext(ctx, ...)` to include the current request ID and service.

## Configuration

The application defines its own types and validation. The shared library handles
file discovery, layered merging, explicit environment bindings, and strict decoding.

The reserved `APPLICATION_ENVIRONMENT` variable selects the environment and
defaults to `production` when unset or empty. Built-in names are `develop`,
`testing`, `staging`, and `production`; custom names are also supported. Business
`EnvBindings` cannot use this reserved variable. Configuration values cannot
change the selected environment.

Use `Options.SearchPaths` to choose search directories in library code. The
generated CLI exposes `--config-dir`, which defaults to `config`.

All base YAML files load before matching environment files. For the generated
service, `app.develop.yaml` overrides `app.yaml`. HTTP settings are in these files;
there is no `--addr` flag. The template explicitly binds
`DEMO_SERVICE_HTTP_ADDR` for a project named `demo-service`; unbound environment
variables do not override settings.

Maps merge recursively across files; other values, including lists and null,
replace earlier values. The final result is decoded into the application's types.
Missing fields retain application defaults; explicit null resets a field to its
Go zero value before application validation runs.

## Check

```sh
go test ./...
go vet ./...
```
