# ZodiacCore Go

Build Go services with consistent configuration, API responses, and request logs.

## Choose your starting point

| Your situation | Start here |
|---|---|
| You are building a new service | [Create a project](getting-started.md), then [add a business endpoint](http.md) |
| You already have a Go application | [Use ZodiacCore in an existing application](existing-application.md) |
| You need to change a service setting | [Configure your application](configuration.md) |
| You need to find the logs for a request | [Write and correlate logs](logging.md) |

A generated project uses Cobra for commands, Fx for dependency injection, and
chi with Huma for HTTP APIs.

In an existing application, import the packages you need. Configuration and
logging can be used without a web server; the HTTP middleware accepts `net/http`
handlers. The typed API helpers integrate with Huma.

## What a client receives

A successful business endpoint returns data inside a standard response:

```json
{"code":0,"data":{"id":1,"title":"Cannot sign in"},"message":"Success"}
```

Endpoints can return public business errors with an HTTP status and message.
Request IDs connect a client's response to the service's logs. Huma generates
OpenAPI documentation from the endpoint's input and output types.

## API reference

See [pkg.go.dev](https://pkg.go.dev/github.com/TTWShell/zodiac-core) for types and
functions. Choose the
version used by your application's `go.mod` in both the API reference and this
site's version selector. `dev` describes the development version.
