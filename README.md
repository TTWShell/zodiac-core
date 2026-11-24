# ZodiacCore Go

A public Go library with a Cobra-based developer CLI. `zodiac new` generates a
business service that consumes the library.

The project follows the design of [ZodiacCore-Py](https://github.com/TTWShell/ZodiacCore-Py),
adapted to Go and its ecosystem. This is the design baseline for all modules;
implementation choices use Go conventions, Fx, Cobra, and Viper where appropriate.

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

The response is `{"status":"ok"}`. Ctrl+C gracefully stops the application.

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
    │   └── server.go
    └── config/
        └── config.go
```

The generated application owns its Cobra commands, Fx assembly, HTTP server,
and startup/shutdown. Each command has its own file, following the default
`cobra-cli` layout. The initial HTTP example uses `net/http` and a health endpoint;
Gin, chi, databases, and other runtime capabilities are not selected here.

The template depends on ZodiacCore's public configuration package, Cobra, and Fx.
Generated source belongs to the business project and runs without the Zodiac CLI.

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

## Repository structure

```text
cmd/zodiac/main.go             Developer CLI executable
config/                       Public configuration library
internal/cli/root.go           Root command assembly
internal/cli/new.go            New-project command
internal/scaffold/             Validation, rendering, and file generation
internal/scaffold/templates/   Embedded business-project template
```

Generator internals are not part of the public runtime API. Project files, comments, CLI
help, and documentation use English.

## Check

```sh
go test ./...
go vet ./...
```
