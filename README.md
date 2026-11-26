# ZodiacCore Go

Build Go services with consistent configuration, API responses, and request
logs. Use ZodiacCore in an existing application, or generate a new service with
Cobra commands, Fx dependency injection, and chi/Huma HTTP APIs.

The project follows the design of [ZodiacCore-Py](https://github.com/TTWShell/ZodiacCore-Py),
adapted to Go and its ecosystem.

[Documentation](https://ttwshell.github.io/zodiac-core/) ·
[API reference](https://pkg.go.dev/github.com/TTWShell/zodiac-core)

## Quick start

Requires Go 1.26.0 or newer. Install the generator and create your application:

```sh
go install github.com/TTWShell/zodiac-core/cmd/zodiac@latest
zodiac new myapp --module example.com/myapp
cd myapp
go mod tidy
APPLICATION_ENVIRONMENT=develop go run ./cmd/myapp serve --config-dir ./config
```

Check the service in another terminal:

```sh
curl http://127.0.0.1:8080/health
```

Expected response:

```json
{"code":0,"data":{"status":"ok"},"message":"Success"}
```

Open <http://127.0.0.1:8080/docs> for the service's API documentation.
Press Ctrl+C to stop.

Next, [add a ticket endpoint](docs/http.md) with configuration, business errors,
and request logs, or follow the
[existing application guide](docs/existing-application.md).

See [CLI options](docs/cli.md) for version selection and local development, or
[Contributing](CONTRIBUTING.md) to work on ZodiacCore itself.
