package config

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func readSource(t *testing.T, opts Options) *Source {
	t.Helper()
	source, err := Read(opts)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func TestDiscoveryOrdersAllBaseFilesBeforeEnvironmentFiles(t *testing.T) {
	t.Setenv("APPLICATION_ENVIRONMENT", "develop")
	first := writeFiles(t, map[string]string{
		"z.yaml": "port: 1", "a.yml": "{}", "a.develop.yaml": "port: 3",
		"my.service.develop.yml": "{}", "app.testing.yaml": "{}",
		"app.dev.yaml": "{}", "ignored.txt": "{}",
	})
	second := writeFiles(t, map[string]string{"b.yaml": "port: 2", "b.develop.yaml": "{}"})
	opts := Options{SearchPaths: []string{filepath.Join(first, "missing"), first, second, first}}
	files, err := DiscoverFiles(opts)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(first, "a.yml"), filepath.Join(first, "z.yaml"), filepath.Join(second, "b.yaml"),
		filepath.Join(first, "a.develop.yaml"), filepath.Join(first, "my.service.develop.yml"), filepath.Join(second, "b.develop.yaml"),
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("files = %v, want %v", files, want)
	}
	cfg, err := Load[settings](opts)
	if err != nil || cfg.Port != 3 {
		t.Fatalf("later-directory base outranked environment override: %+v, %v", cfg, err)
	}
}

func TestEnvironmentSelection(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"app.yaml":            "port: 1",
		"app.testing.yaml":    "port: 2",
		"app.production.yaml": "port: 3",
		"app.qa.yaml":         "port: 4",
	})
	opts := Options{SearchPaths: []string{dir}}
	for _, tc := range []struct {
		name, value string
		want        Environment
		port        int
	}{
		{"unset", "", Production, 3},
		{"empty", "", Production, 3},
		{"selected", "TESTING", Testing, 2},
		{"custom", "QA", "qa", 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("APPLICATION_ENVIRONMENT", tc.value)
			if tc.name == "unset" {
				if err := os.Unsetenv("APPLICATION_ENVIRONMENT"); err != nil {
					t.Fatal(err)
				}
			}
			source := readSource(t, opts)
			var cfg settings
			if err := source.Decode(&cfg); err != nil || cfg.Port != tc.port || source.Environment() != tc.want {
				t.Fatalf("environment = %q, port = %d, error = %v; want %q, %d", source.Environment(), cfg.Port, err, tc.want, tc.port)
			}
		})
	}
	t.Setenv("APPLICATION_ENVIRONMENT", "../production")
	if _, err := Read(opts); err == nil {
		t.Fatal("unsafe environment name accepted by Read")
	}
	if _, err := DiscoverFiles(opts); err == nil {
		t.Fatal("unsafe environment name accepted by DiscoverFiles")
	}
}

func TestReadRejectsConflictingBindings(t *testing.T) {
	t.Setenv("APPLICATION_ENVIRONMENT", "")
	dir := writeFiles(t, map[string]string{"app.yaml": "{}"})
	for _, tc := range []struct {
		name     string
		bindings map[string]string
		want     string
	}{
		{"reserved", map[string]string{"port": "APPLICATION_ENVIRONMENT"}, "reserved"},
		{"reserved lowercase", map[string]string{"port": "application_environment"}, "reserved"},
		{"duplicate same variable", map[string]string{"port": "APP_PORT", "PORT": "APP_PORT"}, "duplicate"},
		{"duplicate different variables", map[string]string{"port": "APP_PORT", "PORT": "OTHER_PORT"}, "duplicate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Read(Options{SearchPaths: []string{dir}, EnvBindings: tc.bindings})
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "port") {
				t.Fatalf("expected %s binding error with key context, got %v", tc.want, err)
			}
		})
	}
}

type httpSettings struct {
	Addr    string        `mapstructure:"addr"`
	Timeout time.Duration `mapstructure:"timeout"`
}

type settings struct {
	HTTP    httpSettings `mapstructure:"http"`
	Level   slog.Level   `mapstructure:"level"`
	Port    int          `mapstructure:"port"`
	Enabled bool         `mapstructure:"enabled"`
	Secret  string       `mapstructure:"secret"`
	Tags    []string     `mapstructure:"tags"`
}

func TestMergeBindingsAndNativeConversions(t *testing.T) {
	t.Setenv("APPLICATION_ENVIRONMENT", "develop")
	dir := writeFiles(t, map[string]string{
		"app.yaml":         "http:\n  addr: localhost:8080\n  timeout: 5s\nlevel: info\nport: 8000\nenabled: true\nsecret: '${LITERAL}'\ntags: [base]\n",
		"app.develop.yaml": "http:\n  addr: localhost:8081\nlevel: debug\nport: 8001\ntags: [develop]\n",
	})
	t.Setenv("CONFIG_TEST_TIMEOUT", "10s")
	t.Setenv("CONFIG_TEST_PORT", "8002")
	t.Setenv("CONFIG_TEST_ENABLED", "false")
	t.Setenv("CONFIG_TEST_SECRET", "")
	t.Setenv("HTTP_ADDR", "unbound")
	cfg, err := Load[settings](Options{
		SearchPaths: []string{dir},
		EnvBindings: map[string]string{
			"HTTP.Timeout": "CONFIG_TEST_TIMEOUT",
			"port":         "CONFIG_TEST_PORT",
			"enabled":      "CONFIG_TEST_ENABLED",
			"secret":       "CONFIG_TEST_SECRET",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.Addr != "localhost:8081" || cfg.HTTP.Timeout != 10*time.Second || cfg.Level != slog.LevelDebug || cfg.Port != 8002 || cfg.Enabled {
		t.Fatalf("unexpected merged config: %+v", cfg)
	}
	if cfg.Secret != "${LITERAL}" || !reflect.DeepEqual(cfg.Tags, []string{"develop"}) {
		t.Fatal("file values were expanded or slices were merged instead of replaced")
	}
}

func TestFileOverridesReplaceIncompatibleValues(t *testing.T) {
	t.Setenv("APPLICATION_ENVIRONMENT", "testing")
	for _, tc := range []struct {
		name, override string
		want           any
		target         any
		invalidStruct  bool
	}{
		{"map", "HTTP: {ADDR: localhost:9090}", map[string]any{"addr": "localhost:9090", "timeout": "5s"}, &settings{}, false},
		{"scalar", "http: localhost:9090", "localhost:9090", &struct{ HTTP string }{}, true},
		{"list", "http: [1, 2]", []any{1, 2}, &struct{ HTTP []int }{}, true},
		{"null", "http: null", nil, &settings{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeFiles(t, map[string]string{
				"app.yaml":         "http: {addr: localhost:8080, timeout: 5s}",
				"app.testing.yaml": tc.override,
			})
			source := readSource(t, Options{SearchPaths: []string{dir}})
			var raw struct{ HTTP any }
			if err := source.Decode(&raw); err != nil || !reflect.DeepEqual(raw.HTTP, tc.want) {
				t.Fatalf("override = %#v, error = %v; want %#v", raw.HTTP, err, tc.want)
			}
			if err := source.Decode(tc.target); err != nil {
				t.Fatalf("compatible schema rejected: %v", err)
			}
			if tc.invalidStruct {
				if err := source.Decode(&settings{}); err == nil || !strings.Contains(err.Error(), "http") {
					t.Fatalf("incompatible struct should fail with field context: %v", err)
				}
			}
		})
	}
}

func TestReadCapturesIndependentStartupSnapshots(t *testing.T) {
	t.Setenv("APPLICATION_ENVIRONMENT", "testing")
	t.Setenv("CONFIG_TEST_PORT", "8081")
	dir := writeFiles(t, map[string]string{
		"app.yaml":            "application_environment: production",
		"app.testing.yaml":    "file: testing",
		"app.production.yaml": "file: production",
	})
	type config struct {
		Environment string `mapstructure:"application_environment"`
		File        string `mapstructure:"file"`
		Port        int    `mapstructure:"port"`
	}
	opts := Options{SearchPaths: []string{dir}, EnvBindings: map[string]string{"port": "CONFIG_TEST_PORT"}}
	source := readSource(t, opts)
	t.Setenv("APPLICATION_ENVIRONMENT", "production")
	t.Setenv("CONFIG_TEST_PORT", "8082")
	var old config
	if err := source.Decode(&old); err != nil || old.Port != 8081 || old.File != "testing" || source.Environment() != Testing {
		t.Fatalf("file settings or process changes affected environment selection: %+v, %v", old, err)
	}
	current, err := Load[config](opts)
	if err != nil || current.Port != 8082 || current.File != "production" {
		t.Fatalf("new load missed process changes: %+v, %v", current, err)
	}
	unbound, err := Load[config](Options{SearchPaths: []string{dir}})
	if err != nil || unbound.Port != 0 {
		t.Fatalf("environment binding leaked across loads: %+v, %v", unbound, err)
	}
	files := source.Files()
	files[0] = "changed"
	if source.Files()[0] == "changed" {
		t.Fatal("loaded paths are not isolated from callers")
	}
}

func TestDecodeIsolatesMutableData(t *testing.T) {
	t.Setenv("APPLICATION_ENVIRONMENT", "testing")
	dir := writeFiles(t, map[string]string{"app.yaml": "payload: {values: {items: [{name: original}]}}"})
	source := readSource(t, Options{SearchPaths: []string{dir}})
	type payload struct {
		Values map[string]any `mapstructure:"values"`
	}
	want := map[string]any{"items": []any{map[string]any{"name": "original"}}}
	for _, method := range []string{"Decode", "DecodeKey"} {
		t.Run(method, func(t *testing.T) {
			decode := func() payload {
				t.Helper()
				var cfg struct{ Payload payload }
				var err error
				if method == "Decode" {
					err = source.Decode(&cfg)
				} else {
					err = source.DecodeKey("payload", &cfg.Payload)
				}
				if err != nil {
					t.Fatal(err)
				}
				return cfg.Payload
			}
			first, second := decode(), decode()
			items := first.Values["items"].([]any)
			items[0].(map[string]any)["name"] = "changed"
			items[0] = "replaced"
			for _, result := range []payload{second, decode()} {
				if !reflect.DeepEqual(result.Values, want) {
					t.Fatalf("another result or the snapshot was mutated: %#v", result.Values)
				}
			}
		})
	}
}

func TestStrictDecodeAndPartialSections(t *testing.T) {
	t.Setenv("APPLICATION_ENVIRONMENT", "develop")
	dir := writeFiles(t, map[string]string{"app.yaml": "http:\n  addr: localhost:8080\n  timeout: 5s\nport: not-an-integer\n"})
	source := readSource(t, Options{SearchPaths: []string{dir}})
	var http httpSettings
	if err := source.DecodeKey("http", &http); err != nil || http.Timeout != 5*time.Second {
		t.Fatalf("unrelated settings broke section decode: %+v, %v", http, err)
	}
	var full settings
	if err := source.Decode(&full); err == nil || !strings.Contains(err.Error(), "port") {
		t.Fatalf("invalid type accepted: %v", err)
	}
	if err := source.DecodeKey("missing", &http); err == nil {
		t.Fatal("missing section accepted")
	}
	for _, target := range []any{nil, (*settings)(nil), settings{}, new(int)} {
		if err := source.Decode(target); err == nil {
			t.Fatalf("invalid target %T accepted", target)
		}
	}
}

func TestDecodeRejectsUnknownFields(t *testing.T) {
	t.Setenv("APPLICATION_ENVIRONMENT", "testing")
	for _, value := range []string{"5s", "{}", "null"} {
		t.Run(value, func(t *testing.T) {
			dir := writeFiles(t, map[string]string{"app.yaml": "http: {timout: " + value + "}"})
			source := readSource(t, Options{SearchPaths: []string{dir}})
			for _, err := range []error{source.Decode(&settings{}), source.DecodeKey("http", &httpSettings{})} {
				if err == nil || !strings.Contains(err.Error(), "timout") {
					t.Fatalf("unknown field accepted or missing error context: %v", err)
				}
			}
		})
	}
}

var errRequired = errors.New("required name missing")

type validatedSettings struct {
	Name     string            `mapstructure:"name"`
	HTTP     httpSettings      `mapstructure:"http"`
	Optional *httpSettings     `mapstructure:"optional"`
	Labels   map[string]string `mapstructure:"labels"`
	Tags     []string          `mapstructure:"tags"`
}

func (c *validatedSettings) Validate() error {
	if c.Name == "" {
		return errRequired
	}
	return nil
}

func TestDefaultsAndApplicationValidation(t *testing.T) {
	t.Setenv("APPLICATION_ENVIRONMENT", "develop")
	for _, tc := range []struct {
		name, yaml, section string
		cleared             bool
	}{
		{"missing", "{}", "", false},
		{"partial map", "labels: {added: value}", "", false},
		{"null fields", "name: null\nhttp: null\noptional: null\nlabels: null\ntags: null", "", true},
		{"null section", "section: null", "section", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeFiles(t, map[string]string{"app.yaml": tc.yaml})
			source := readSource(t, Options{SearchPaths: []string{dir}})
			target := validatedSettings{
				Name: "default", HTTP: httpSettings{Addr: "localhost:8080"},
				Optional: &httpSettings{Addr: "localhost:8081"},
				Labels:   map[string]string{"default": "kept"}, Tags: []string{"default"},
			}
			var err error
			if tc.section == "" {
				err = source.Decode(&target)
			} else {
				err = source.DecodeKey(tc.section, &target)
			}
			if tc.cleared {
				if !errors.Is(err, errRequired) || !reflect.DeepEqual(target, validatedSettings{}) {
					t.Fatalf("null must clear defaults and run validation: %+v, %v", target, err)
				}
			} else if err != nil || target.Name != "default" || target.HTTP.Addr == "" || target.Optional == nil || target.Labels["default"] != "kept" || len(target.Tags) != 1 {
				t.Fatalf("missing fields must retain defaults: %+v, %v", target, err)
			}
			if tc.name == "partial map" && target.Labels["added"] != "value" {
				t.Fatal("present map entry was not decoded")
			}
		})
	}
}

func TestDiscoveryAndReadFailures(t *testing.T) {
	t.Setenv("APPLICATION_ENVIRONMENT", "develop")
	empty := t.TempDir()
	opts := Options{SearchPaths: []string{filepath.Join(empty, "missing")}}
	if files, err := DiscoverFiles(opts); err != nil || len(files) != 0 {
		t.Fatalf("discovery must tolerate missing directories: %v, %v", files, err)
	}
	if _, err := Read(opts); !errors.Is(err, ErrNoFiles) {
		t.Fatalf("read should reject empty discovery: %v", err)
	}
	if _, err := DiscoverFiles(Options{}); err == nil {
		t.Fatal("missing search paths accepted")
	}
	dir := writeFiles(t, map[string]string{"app.yaml": "port: 8080"})
	if _, err := Load[settings](Options{SearchPaths: []string{dir}}); err != nil {
		t.Fatalf("environment file should be optional: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.develop.yaml"), []byte("http: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(Options{SearchPaths: []string{dir}}); err == nil || !strings.Contains(err.Error(), "app.develop.yaml") {
		t.Fatalf("malformed file context lost: %v", err)
	}
}
