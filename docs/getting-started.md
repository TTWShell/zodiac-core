# Create a service

Requires Go 1.26.0 or newer.

## Install the project generator

```sh
go install github.com/TTWShell/zodiac-core/cmd/zodiac@latest
```

Make sure Go's binary installation directory is on your `PATH`: `GOBIN` if set,
otherwise the `bin` directory under `GOPATH` (usually `$HOME/go/bin`).

For a specific version, see [CLI installation and versions](cli.md).

## Create your project

```sh
zodiac new myapp --module example.com/myapp
cd myapp
go mod tidy
```

`myapp` is the project name. `example.com/myapp` is the Go module path used
throughout this tutorial; replace it with your repository's module path in your
own project, including in the examples' imports.

The generator creates a new `myapp` directory. It must be absent or empty.

## Start the service

```sh
APPLICATION_ENVIRONMENT=develop go run . serve
```

Check it from another terminal:

```sh
curl -i http://127.0.0.1:8080/health
```

You should receive HTTP 200, an `X-Request-ID` response header, and this body:

```json
{"code":0,"data":{"status":"ok"},"message":"Success"}
```

Visit <http://127.0.0.1:8080/docs> to try endpoints in your browser. The OpenAPI
document is available at `/openapi.json`. Press Ctrl+C to stop the service.

If port 8080 is occupied, choose another address:

```sh
APPLICATION_ENVIRONMENT=develop MYAPP_HTTP_ADDR=127.0.0.1:9090 go run . serve
```

## Add your first business endpoint

Follow the [ticket endpoint tutorial](http.md) to add a service and route.

## Build an executable

```sh
go build -o bin/myapp .
APPLICATION_ENVIRONMENT=production ./bin/myapp serve --config-dir ./config
```

Ship the configuration directory with the binary, or supply another directory
with `--config-dir`. The generated production configuration listens on `:8080`.
Use [configuration](configuration.md) to set environment-specific values and
bind environment variables for deployment.
