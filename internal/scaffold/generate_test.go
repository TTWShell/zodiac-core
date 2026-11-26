package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

func readModule(t *testing.T, dir string) *modfile.File {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	mod, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		t.Fatal(err)
	}
	return mod
}

func TestGenerateStandaloneProject(t *testing.T) {
	for _, destination := range []string{"new", "empty"} {
		t.Run(destination, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "service")
			if destination == "empty" {
				if err := os.Mkdir(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			got, err := Generate(Options{Name: "demo-service", Module: "example.com/team/demo/v2", Output: dir, CoreVersion: "v0.1.0"})
			if err != nil || got != dir {
				t.Fatalf("output = %q, error = %v; want %q", got, err, dir)
			}
			for path, content := range map[string]string{
				"cmd/demo-service/main.go":      "logging.New",
				"cmd/demo-service/root.go":      "",
				"cmd/demo-service/serve.go":     `"example.com/team/demo/v2/internal/app"`,
				"internal/app/app.go":           "user.Module",
				"internal/httpserver/module.go": "fx.Private",
				"internal/httpserver/server.go": "",
				"internal/httpserver/router.go": "",
				"internal/health/health.go":     "",
				"internal/health/module.go":     "fx.Invoke(Register)",
				"internal/user/user.go":         "User retrieved",
				"internal/user/http.go":         "/users/{id}",
				"internal/user/module.go":       "fx.Provide(NewService)",
				"internal/config/config.go":     "DEMO_SERVICE_HTTP_ADDR",
				"config/app.yaml":               "",
				"config/app.develop.yaml":       "",
				"config/app.testing.yaml":       "",
				"config/app.production.yaml":    "",
				".gitignore":                    "",
				"README.md":                     "",
			} {
				data, err := os.ReadFile(filepath.Join(dir, path))
				if err != nil || !strings.Contains(string(data), content) {
					t.Errorf("%s: missing file or content %q (read error: %v)", path, content, err)
				}
			}
			mod := readModule(t, dir)
			if mod.Module.Mod.Path != "example.com/team/demo/v2" || len(mod.Replace) != 0 {
				t.Fatal("release project must use the requested module without local replacements")
			}
			foundCore := false
			for _, requirement := range mod.Require {
				if requirement.Mod.Path == CoreModule && requirement.Mod.Version == "v0.1.0" {
					foundCore = true
				}
			}
			if !foundCore {
				t.Fatal("missing selected core version")
			}
		})
	}
}

func TestLocalCoreDependencyIsExplicitAndQuoted(t *testing.T) {
	base := t.TempDir()
	core := filepath.Join(base, "core checkout")
	if err := os.Mkdir(core, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(core, "go.mod"), []byte("module "+CoreModule+"\n\ngo 1.26.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir, err := Generate(Options{Name: "demo", Module: "example.com/demo", Output: filepath.Join(base, "demo"), CorePath: core})
	if err != nil {
		t.Fatal(err)
	}
	mod := readModule(t, dir)
	if len(mod.Replace) != 1 || mod.Replace[0].Old.Path != CoreModule || mod.Replace[0].New.Path != core {
		t.Fatalf("incorrect local replacement: %v", mod.Replace)
	}
}

func TestGenerateRejectsInvalidInputsBeforeWriting(t *testing.T) {
	for _, tc := range []struct {
		name, project, module, version, want string
	}{
		{"unsafe name", "../escape", "example.com/demo", "v0.1.0", "invalid project name"},
		{"invalid name", "demo--service", "example.com/demo", "v0.1.0", "invalid project name"},
		{"missing module", "demo", "", "v0.1.0", "invalid module path"},
		{"invalid module", "demo", "example.com/demo\nrequire malicious v1.0.0", "v0.1.0", "invalid module path"},
		{"core as business module", "demo", CoreModule, "v0.1.0", "business module must differ"},
		{"missing version", "demo", "example.com/demo", "", "no published core version"},
		{"invalid version", "demo", "example.com/demo", "latest", "canonical Go module version"},
		{"wrong major", "demo", "example.com/demo", "v2.0.0", "invalid core version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "service")
			_, err := Generate(Options{Name: tc.project, Module: tc.module, Output: dir, CoreVersion: tc.version})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
			if _, err := os.Lstat(dir); !os.IsNotExist(err) {
				t.Fatalf("invalid input created an output directory: %v", err)
			}
		})
	}
}

func TestGeneratePreservesExistingFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".existing")
	if err := os.WriteFile(path, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Generate(Options{Name: "demo", Module: "example.com/demo", Output: dir, CoreVersion: "v0.1.0"})
	if err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("expected nonempty directory error, got %v", err)
	}
	contents, err := os.ReadDir(dir)
	if err != nil || len(contents) != 1 {
		t.Fatalf("existing directory changed: %v, %v", contents, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "keep me" {
		t.Fatalf("existing file changed: %q, %v", data, err)
	}
}

func TestGenerateRejectsSymlinkDestination(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Generate(Options{Name: "demo", Module: "example.com/demo", Output: link, CoreVersion: "v0.1.0"}); err == nil {
		t.Fatal("expected symlink destination to fail")
	}
	contents, err := os.ReadDir(target)
	if err != nil || len(contents) != 0 {
		t.Fatalf("symlink target was modified: %v, %v", contents, err)
	}
}

func TestWriteFailureRemovesOnlyNewFiles(t *testing.T) {
	dir := t.TempDir()
	err := writeProject(dir, []entry{
		{path: "cmd", isDir: true},
		{path: "cmd/root.go", data: []byte("first")},
		{path: "cmd/root.go", data: []byte("conflict")},
	})
	if err == nil {
		t.Fatal("expected exclusive-create conflict")
	}
	contents, err := os.ReadDir(dir)
	if err != nil || len(contents) != 0 {
		t.Fatalf("partial output was not cleaned up: %v, %v", contents, err)
	}
}
