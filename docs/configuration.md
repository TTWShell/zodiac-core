# Configure your application

Use configuration files for shared settings and explicit environment bindings
for values that vary between deployments. This guide uses the `myapp` project
from [Create a service](getting-started.md).

## Select an environment

```sh
APPLICATION_ENVIRONMENT=develop go run ./cmd/myapp serve --config-dir ./config
```

This loads shared files such as `config/app.yaml`, then matching overrides such
as `config/app.develop.yaml`. When `APPLICATION_ENVIRONMENT` is unset or empty,
the environment is `production`.

`develop`, `testing`, `staging`, and `production` are standard names. Custom names
are also allowed: start with a letter and use letters, digits, underscores, or
hyphens. Names are normalized to lowercase.

The configuration directory is relative to the current working directory. For
an installed service, you can use an absolute path:

```sh
APPLICATION_ENVIRONMENT=production ./bin/myapp serve --config-dir /etc/myapp
```

## Add a business setting

The ticket example needs a support team name. In `internal/config/config.go`,
add `SupportConfig` and replace the existing `Config` declaration with:

```go
type SupportConfig struct {
    Team string `mapstructure:"team"`
}

type Config struct {
    HTTP    HTTPConfig         `mapstructure:"http"`
    Support SupportConfig      `mapstructure:"support"`
    Env     zconfig.Environment `mapstructure:"-"`
}
```

Keep the file's existing imports, `HTTPConfig`, and loading function.
Append this setting to `config/app.yaml`, keeping the existing `http` settings:

```yaml
support:
  team: "general"
```

## Allow an environment override

In the same file's `Load` function, update `EnvBindings` to include the new key:

```go
EnvBindings: map[string]string{
    "http.addr":    "MYAPP_HTTP_ADDR",
    "support.team": "MYAPP_SUPPORT_TEAM",
},
```

Restart with an override:

```sh
APPLICATION_ENVIRONMENT=develop MYAPP_SUPPORT_TEAM=priority go run ./cmd/myapp serve
```

The effective team is now `priority`. An unbound environment variable does not
change configuration; an empty binding value falls back to the file value.

`APPLICATION_ENVIRONMENT` is reserved for selecting which files to load. Do not
use it as a business setting's environment binding.

## Reject invalid settings at startup

Add this check inside the existing `(*Config).Validate` method, before its final
`return nil`:

```go
if c.Support.Team == "" {
    return errors.New("support.team is required")
}
```

Validation runs after file and environment values are decoded. An invalid
configuration prevents the generated server from starting. Keep the existing
HTTP checks.

## Understand overrides

The effective value is determined in this order, with later values taking precedence:

1. Base YAML files, such as `app.yaml`.
2. Matching environment files, such as `app.production.yaml`.
3. Nonempty values from explicitly bound environment variables.

Within each file group, directories follow `SearchPaths` order and filenames
follow lexical order. Files in subdirectories are not loaded. An environment
file is optional, but at least one matching configuration file must exist.

Maps merge by key. Lists and scalar values replace earlier values. Explicit
`null` resets the destination field to its Go zero value. Unknown configuration
fields and values that cannot decode into the declared Go types are errors.

## Use configuration without the generator

In an existing application, define your own struct and call the public loader:

```go
package settings

import (
    "errors"

    "github.com/TTWShell/zodiac-core/config"
)

type Config struct {
    Support struct {
        Team string `mapstructure:"team"`
    } `mapstructure:"support"`
}

func (c *Config) Validate() error {
    if c.Support.Team == "" {
        return errors.New("support.team is required")
    }
    return nil
}

func Load(directory string) (*Config, error) {
    return config.Load[Config](config.Options{
        SearchPaths: []string{directory},
        EnvBindings: map[string]string{
            "support.team": "MYAPP_SUPPORT_TEAM",
        },
    })
}
```

For this standalone example, create a YAML file containing only the `support`
setting shown above. When adapting it to your application, declare all the
settings your files contain.

Use `config.Read` followed by `source.Decode(&cfg)` when you need to populate
an existing struct with defaults. Missing fields retain those defaults. Settings
are read at startup; restart the application to apply file or environment changes.
See the [configuration reference](https://pkg.go.dev/github.com/TTWShell/zodiac-core/config)
for section decoding and file discovery.
