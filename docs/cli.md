# CLI installation and options

Use `zodiac new` to generate an application.

## Install and generate

```sh
go install github.com/TTWShell/zodiac-core/cmd/zodiac@latest
zodiac new myapp --module example.com/myapp
```

For a versioned installation, replace `@latest` with an available version from
[Releases](https://github.com/TTWShell/zodiac-core/releases).
Go installs the executable into `GOBIN`, or `GOPATH/bin` when `GOBIN` is unset;
include that directory in your `PATH`.

A version-installed CLI records its own ZodiacCore version in the generated
project's `go.mod`. Without release tags, Go may resolve `@latest` to a
pseudo-version.

## Choose a directory or dependency version

```text
zodiac new <name> --module <module-path> [flags]
```

| Flag | When to use it |
|---|---|
| `--module` | Set the new application's Go module path; required |
| `--output`, `-o` | Write somewhere other than `./<name>` |
| `--core-version` | Select a particular available ZodiacCore version for the application |
| `--core-path` | Set a local ZodiacCore module path using a `replace` directive |

For example, generate into a separate application directory:

```sh
zodiac new myapp --module example.com/myapp --output ../services/myapp
```

Names must start with a lowercase letter and contain lowercase letters, digits,
and single hyphens. Output directories must be absent or empty. The generator
rejects existing files and symbolic-link destinations.

Run `go mod tidy` inside the generated project to download its dependencies.

## Run the generated application's commands

From the application's directory:

```sh
APPLICATION_ENVIRONMENT=develop go run . serve --config-dir ./config
```

`--config-dir` defaults to `config`, relative to the current working directory.
For the `myapp` project, `MYAPP_HTTP_ADDR` overrides the listener address.
Other project names use an uppercase prefix with hyphens replaced by underscores.

Add new application commands in `cmd/`, following the existing `serve.go` pattern.
Use `zodiac --help` for generator commands and `go run . --help` for application
commands.
