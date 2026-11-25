# Write and correlate logs

Use a single application logger and pass each request's context when recording
business events.

## Use the logger in a generated service

The generated application provides a `*slog.Logger` through Fx. Receive it in
your constructor, as shown in the
[ticket service](http.md#write-the-ticket-service-and-route).

Inside a request, record what happened with `InfoContext`, `WarnContext`, or
`ErrorContext`:

```go
s.logger.InfoContext(ctx, "Ticket retrieved", "ticket_id", id)
```

Request logs include `request_id` and `service` automatically.

## Find the logs for a request

Make a request and inspect its response headers:

```sh
curl -i http://127.0.0.1:8080/tickets/1
```

Find `X-Request-ID` in the response and search your log destination for that value
in the `request_id` field. The ticket event and access log share the same ID.
Access logs include the method, path, HTTP status, and duration in milliseconds.

Logs are written as JSON to stderr.

## Change the log level

In a generated application's `main.go`, select the minimum severity when creating
the logger:

```go
logger := logging.New("myapp", slog.LevelDebug)
```

Import `log/slog` and `github.com/TTWShell/zodiac-core/logging` in an existing
application. Use `slog.LevelInfo`, `slog.LevelWarn`, or `slog.LevelError` for other
thresholds; `nil` defaults to INFO.

If your application needs to change the threshold while running, retain a
`*slog.LevelVar` and pass it to `logging.New`:

```go
var level slog.LevelVar
logger := logging.New("myapp", &level)
level.Set(slog.LevelDebug)
```

## Initialize logging in an existing application

Create the logger in your application's entry point:

```go
logger := logging.New("myapp", slog.LevelInfo)
slog.SetDefault(logger)
```

`SetDefault` is needed when your application uses package-level functions such
as `slog.InfoContext`. Pass the logger to components that receive their
dependencies explicitly. Install the
[HTTP middleware](existing-application.md#add-an-http-api-with-chi-and-huma)
to make request metadata available.

## Choose another output format or destination

Use a standard slog handler and wrap it to retain request metadata:

```go
handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
    Level: slog.LevelInfo,
})
logger := slog.New(requestctx.NewLogHandler(handler, "myapp"))
```

Import `os` and `github.com/TTWShell/zodiac-core/requestctx` for this example.
Pass another `io.Writer` in place of `os.Stderr` to change the destination.
See [slog](https://pkg.go.dev/log/slog) for handler options.
