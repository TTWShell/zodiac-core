// Package scaffold generates standalone business projects from embedded templates.
package scaffold

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

//go:embed templates
var templates embed.FS

var projectName = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

// Options describes a standalone business project.
type Options struct {
	Name        string
	Module      string
	Output      string
	CoreVersion string
	CorePath    string
}

// CoreModule is the module path used by generated projects to import ZodiacCore.
const CoreModule = "github.com/TTWShell/zodiac-core"

type entry struct {
	path  string
	isDir bool
	data  []byte
}

// Generate creates a project without downloading dependencies or invoking Git.
// Existing files and symbolic-link destinations are never overwritten.
func Generate(opts Options) (string, error) {
	if !projectName.MatchString(opts.Name) {
		return "", fmt.Errorf("invalid project name %q: use lowercase letters, digits, and single hyphens; start with a letter", opts.Name)
	}
	if err := module.CheckPath(opts.Module); err != nil {
		return "", fmt.Errorf("invalid module path: %w", err)
	}
	if opts.Module == CoreModule {
		return "", fmt.Errorf("business module must differ from %s", CoreModule)
	}
	var err error
	opts, err = resolveCore(opts)
	if err != nil {
		return "", err
	}
	if opts.Output == "" {
		opts.Output = opts.Name
	}
	dir, err := filepath.Abs(opts.Output)
	if err != nil {
		return "", fmt.Errorf("resolve output directory: %w", err)
	}
	files, err := render(opts)
	if err != nil {
		return "", err
	}
	if err := writeProject(dir, files); err != nil {
		return "", err
	}
	return dir, nil
}

func resolveCore(opts Options) (Options, error) {
	if opts.CorePath != "" {
		path, err := filepath.Abs(opts.CorePath)
		if err != nil {
			return opts, fmt.Errorf("resolve core path: %w", err)
		}
		data, err := os.ReadFile(filepath.Join(path, "go.mod"))
		if err != nil {
			return opts, fmt.Errorf("read local core module: %w", err)
		}
		file, err := modfile.Parse("go.mod", data, nil)
		if err != nil {
			return opts, fmt.Errorf("parse local core module: %w", err)
		}
		if file.Module == nil || file.Module.Mod.Path != CoreModule {
			return opts, fmt.Errorf("core path must contain module %s", CoreModule)
		}
		opts.CorePath = path
	}
	if opts.CoreVersion == "" {
		if opts.CorePath == "" {
			return opts, errors.New("this development build has no published core version; use --core-path for a local checkout or --core-version for a published release")
		}
		opts.CoreVersion = "v0.0.0"
	}
	if !semver.IsValid(opts.CoreVersion) || semver.Canonical(opts.CoreVersion) != opts.CoreVersion {
		return opts, fmt.Errorf("core version must be a canonical Go module version, got %q", opts.CoreVersion)
	}
	if err := module.Check(CoreModule, opts.CoreVersion); err != nil {
		return opts, fmt.Errorf("invalid core version: %w", err)
	}
	return opts, nil
}

func render(opts Options) ([]entry, error) {
	var entries []entry
	err := fs.WalkDir(templates, "templates", func(path string, item fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "templates" {
			return nil
		}
		rel := outputPath(strings.TrimPrefix(path, "templates/"), opts.Name)
		if item.IsDir() {
			entries = append(entries, entry{path: rel, isDir: true})
			return nil
		}
		source, err := templates.ReadFile(path)
		if err != nil {
			return err
		}
		tmpl, err := template.New(path).Funcs(template.FuncMap{
			"envPrefix": func(name string) string { return strings.ToUpper(strings.ReplaceAll(name, "-", "_")) },
		}).Option("missingkey=error").Parse(string(source))
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, opts); err != nil {
			return err
		}
		rel = strings.TrimSuffix(rel, ".tmpl")
		if rel == "gitignore" {
			rel = ".gitignore"
		}
		data := buf.Bytes()
		if strings.HasSuffix(rel, ".go") {
			data, err = format.Source(data)
			if err != nil {
				return fmt.Errorf("format %s: %w", rel, err)
			}
		}
		entries = append(entries, entry{path: rel, data: data})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("render project: %w", err)
	}
	return entries, nil
}

// cmd/app holds the command templates. The generated binary is package main at cmd/<name>.
func outputPath(rel, name string) string {
	const commandDir = "cmd/app"
	if rel == commandDir {
		return "cmd/" + name
	}
	if strings.HasPrefix(rel, commandDir+"/") {
		return "cmd/" + name + strings.TrimPrefix(rel, commandDir)
	}
	return rel
}

func writeProject(dir string, entries []entry) (err error) {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return fmt.Errorf("create output parent: %w", err)
	}
	var created []string
	complete := false
	defer func() {
		if !complete {
			// Remove only entries created by this call, never unrelated files.
			for i := len(created) - 1; i >= 0; i-- {
				if removeErr := os.Remove(created[i]); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
					err = errors.Join(err, fmt.Errorf("clean up generated entry: %w", removeErr))
				}
			}
		}
	}()
	if err := os.Mkdir(dir, 0o755); err == nil {
		created = append(created, dir)
	} else if errors.Is(err, fs.ErrExist) {
		info, err := os.Lstat(dir)
		if err != nil {
			return fmt.Errorf("inspect output: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("output %q must be a directory, not a file or symbolic link", dir)
		}
		contents, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("read output: %w", err)
		}
		if len(contents) != 0 {
			return fmt.Errorf("output directory %q is not empty", dir)
		}
	} else {
		return fmt.Errorf("create output: %w", err)
	}
	for _, item := range entries {
		path := filepath.Join(dir, filepath.FromSlash(item.path))
		if item.isDir {
			if err := os.Mkdir(path, 0o755); err != nil {
				return fmt.Errorf("create directory %s: %w", item.path, err)
			}
			created = append(created, path)
			continue
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return fmt.Errorf("create file %s: %w", item.path, err)
		}
		created = append(created, path)
		_, writeErr := file.Write(item.data)
		if err := errors.Join(writeErr, file.Close()); err != nil {
			return fmt.Errorf("write file %s: %w", item.path, err)
		}
	}
	complete = true
	return nil
}
