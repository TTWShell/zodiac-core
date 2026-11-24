// Package config discovers layered YAML files and decodes startup configuration.
//
// File discovery orders all base files before environment-specific files.
// APPLICATION_ENVIRONMENT selects the environment and defaults to production
// when unset or empty. It is reserved and cannot be used in business bindings.
// Applications own their configuration types, validation, and dependency
// injection. This package does not initialize global Viper state, expand file
// values, watch files, or start application resources.
package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Environment identifies an application environment. Custom names are allowed.
type Environment string

// Standard environment names. Applications may also use custom names.
const (
	Develop    Environment = "develop"
	Testing    Environment = "testing"
	Staging    Environment = "staging"
	Production Environment = "production"
)

// ErrNoFiles means no base or matching environment YAML files were discovered.
var ErrNoFiles = errors.New("config: no matching configuration files")

// Options controls file discovery and explicit environment bindings.
type Options struct {
	// SearchPaths are searched in order. Missing directories are skipped.
	// At least one nonempty path is required. Paths are relative to the caller's
	// working directory, and returned file paths are absolute.
	SearchPaths []string
	// EnvBindings maps configuration keys to explicit environment variable names.
	// Keys are case-insensitive; duplicate keys differing only in case are errors.
	// Nonempty values override files; empty values fall back to file values.
	// Unbound environment variables do not participate in configuration.
	// APPLICATION_ENVIRONMENT is reserved for environment selection.
	EnvBindings map[string]string
}

// Validator can be implemented by application configuration types to reject
// missing required fields or invalid combinations after decoding.
type Validator interface {
	Validate() error
}

var environmentName = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

const environmentVariable = "APPLICATION_ENVIRONMENT"

func resolveEnvironment() (Environment, error) {
	env := os.Getenv(environmentVariable)
	if env == "" {
		env = string(Production)
	}
	env = strings.ToLower(env)
	if !environmentName.MatchString(env) {
		return "", fmt.Errorf("config: invalid environment name %q", env)
	}
	return Environment(env), nil
}

// Load reads a configuration snapshot and strictly decodes it into a struct T.
// If *T implements Validator, validation runs before the result is returned.
// Go zero values are not implicitly required fields; enforce those in Validate.
func Load[T any](opts Options) (*T, error) {
	source, err := Read(opts)
	if err != nil {
		return nil, err
	}
	var result T
	if err := source.Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}
