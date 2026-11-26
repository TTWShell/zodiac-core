package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewCommandDefaultsToProjectName(t *testing.T) {
	t.Chdir(t.TempDir())
	var output bytes.Buffer
	cmd := newRootCommand()
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"new", "demo-service", "--module", "example.com/demo-service", "--core-version", "v0.1.0"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join("demo-service", "cmd", "demo-service", "serve.go")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "go mod tidy") {
		t.Fatalf("missing next steps: %s", output.String())
	}
}

func TestNewCommandRejectsInvalidArguments(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"new", "--module", "example.com/demo"}, "accepts 1 arg(s)"},
		{[]string{"new", "demo"}, `required flag(s) "module" not set`},
		{[]string{"new", "demo", "extra", "--module", "example.com/demo"}, "accepts 1 arg(s)"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			cmd := newRootCommand()
			cmd.SetArgs(append(tc.args, "--core-version", "v0.1.0"))
			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
			contents, err := os.ReadDir(dir)
			if err != nil || len(contents) != 0 {
				t.Fatalf("invalid command wrote files: %v, %v", contents, err)
			}
		})
	}
}
