package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DiscoverFiles discovers .yaml and .yml files without loading their contents.
// Files with one dot are base files (app.yaml); other names must have the selected
// environment immediately before their extension (app.develop.yaml).
// All base files precede all environment files. Within each group, search-path
// order and then lexical filename order determine precedence. Missing directories
// and unmatched names are skipped; an empty discovery result is not an error.
func DiscoverFiles(opts Options) ([]string, error) {
	env, err := resolveEnvironment()
	if err != nil {
		return nil, err
	}
	return discover(opts.SearchPaths, env)
}

func discover(paths []string, env Environment) ([]string, error) {
	var base, overrides []string
	seen := make(map[string]bool)
	for _, path := range paths {
		if path == "" {
			continue
		}
		dir, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("config: resolve directory %q: %w", path, err)
		}
		if seen[dir] {
			continue
		}
		seen[dir] = true
		entries, err := os.ReadDir(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("config: read directory %q: %w", dir, err)
		}
		for _, item := range entries {
			if item.IsDir() {
				continue
			}
			name := item.Name()
			ext := filepath.Ext(name)
			if ext != ".yaml" && ext != ".yml" {
				continue
			}
			parts := strings.Split(name, ".")
			file := filepath.Join(dir, name)
			if len(parts) == 2 && parts[0] != "" {
				base = append(base, file)
			} else if len(parts) >= 3 && strings.ToLower(parts[len(parts)-2]) == string(env) {
				overrides = append(overrides, file)
			}
		}
	}
	if len(seen) == 0 {
		return nil, errors.New("config: at least one search path is required")
	}
	return append(base, overrides...), nil
}
