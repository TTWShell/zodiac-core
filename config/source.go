package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/cast"
	"github.com/spf13/viper"
)

// Source is a startup snapshot of merged files and explicitly bound environment
// values. Later process-environment changes do not change this snapshot.
// Obtain a Source with Read. Decoding copies snapshot data so results do not
// share mutable data with the Source or other results. Applications should treat
// decoded structs as read-only after dependency assembly.
type Source struct {
	values *viper.Viper
	env    Environment
	files  []string
}

// Read discovers and merges files, then captures the effective environment values.
// Maps merge recursively; other values, including null, replace earlier values.
// Unlike DiscoverFiles, Read rejects an empty file set. An environment override
// file is optional. Each discovered file must be readable and valid; errors
// identify the file and preserve their cause.
func Read(opts Options) (*Source, error) {
	env, err := resolveEnvironment()
	if err != nil {
		return nil, err
	}
	files, err := discover(opts.SearchPaths, env)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, ErrNoFiles
	}
	v := viper.New()
	decoder, err := viper.NewCodecRegistry().Decoder("yaml")
	if err != nil {
		return nil, fmt.Errorf("config: YAML decoder: %w", err)
	}
	merged := make(map[string]any)
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("config: read %q: %w", path, err)
		}
		document := make(map[string]any)
		if err := decoder.Decode(data, document); err != nil {
			return nil, fmt.Errorf("config: parse %q: %w", path, err)
		}
		mergeValue(merged, document)
	}
	// Initialize Viper once with the merged result. Its map merge skips some
	// type-changing overrides, so it must not perform the inter-file merging.
	if err := v.MergeConfigMap(merged); err != nil {
		return nil, fmt.Errorf("config: initialize merged configuration: %w", err)
	}
	keys := make([]string, 0, len(opts.EnvBindings))
	for key := range opts.EnvBindings {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	seen := make(map[string]string, len(keys))
	for _, key := range keys {
		variable := opts.EnvBindings[key]
		if strings.EqualFold(variable, environmentVariable) {
			return nil, fmt.Errorf("config: %s is reserved for environment selection; cannot bind key %q", environmentVariable, key)
		}
		if strings.TrimSpace(key) == "" || strings.HasPrefix(key, ".") || strings.HasSuffix(key, ".") || strings.Contains(key, "..") || variable == "" || strings.ContainsAny(variable, "=\x00") {
			return nil, fmt.Errorf("config: invalid environment binding for key %q", key)
		}
		normalized := strings.ToLower(key)
		if previous, ok := seen[normalized]; ok {
			return nil, fmt.Errorf("config: duplicate environment binding keys %q and %q", previous, key)
		}
		seen[normalized] = key
		if err := v.BindEnv(key, variable); err != nil {
			return nil, fmt.Errorf("config: bind environment for %q: %w", key, err)
		}
	}
	// AllSettings flattens away empty maps and nulls. Keep root values intact so
	// unknown empty fields still reach strict decoding.
	values := make(map[string]any)
	for root := range merged {
		values[root] = v.Get(root)
	}
	bound := make(map[string]any)
	for _, key := range keys {
		if value := v.Get(key); value != nil {
			bound[key] = value
		}
	}
	for _, key := range keys {
		if value, ok := bound[key]; ok {
			setValue(values, strings.Split(strings.ToLower(key), "."), value)
		}
	}
	// UnmarshalKey preserves the complete map, unlike whole-Viper flattening.
	// This private wrapper has no environment bindings; subsequent decoding
	// therefore observes startup values rather than the current process state.
	snapshot := viper.New()
	snapshot.Set("values", values)
	return &Source{values: snapshot, env: env, files: files}, nil
}

// mergeValue normalizes keys and merges maps in place. Lists and scalars replace
// earlier values; maps inside replacement lists are normalized without merging.
func mergeValue(previous, next any) any {
	switch next := next.(type) {
	case map[any]any:
		return mergeValue(previous, cast.ToStringMap(next))
	case map[string]any:
		result, ok := previous.(map[string]any)
		if !ok {
			result = make(map[string]any, len(next))
		}
		for key, value := range next {
			key = strings.ToLower(key)
			result[key] = mergeValue(result[key], value)
		}
		return result
	case []any:
		for i, value := range next {
			next[i] = mergeValue(nil, value)
		}
		return next
	default:
		return next
	}
}

func setValue(values map[string]any, path []string, value any) {
	for _, key := range path[:len(path)-1] {
		nested, ok := values[key].(map[string]any)
		if !ok {
			nested = make(map[string]any)
			values[key] = nested
		}
		values = nested
	}
	values[path[len(path)-1]] = value
}

// cloneValue copies the maps and slices produced by YAML loading. Scalars and
// bound environment strings can be reused without sharing mutable data.
func cloneValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, item := range value {
			result[key] = cloneValue(item)
		}
		return result
	case []any:
		result := slices.Clone(value)
		for i, item := range value {
			result[i] = cloneValue(item)
		}
		return result
	default:
		return value
	}
}

// Mapstructure dereferences prefilled struct pointers before decoding fields.
// Clear explicit null pointers first so the decoder cannot retain that pointer.
func clearNullPointers(data map[string]any, target reflect.Value) {
	for i := 0; i < target.NumField(); i++ {
		field := target.Field(i)
		definition := target.Type().Field(i)
		tags := strings.Split(definition.Tag.Get("mapstructure"), ",")
		if tags[0] == "-" {
			continue
		}
		if slices.Contains(tags[1:], "squash") {
			nested := reflect.Indirect(field)
			if nested.Kind() == reflect.Struct {
				clearNullPointers(data, nested)
			}
			continue
		}
		name := tags[0]
		if name == "" {
			name = definition.Name
		}
		if value, exists := data[strings.ToLower(name)]; exists && value == nil && field.Kind() == reflect.Pointer && field.CanSet() {
			field.SetZero()
		}
	}
}

// Environment returns the resolved environment used during discovery.
func (s *Source) Environment() Environment { return s.env }

// Files returns an independent copy of the loaded paths in precedence order.
func (s *Source) Files() []string { return slices.Clone(s.files) }

// Decode strictly decodes the whole snapshot into a nonnil pointer to a struct.
// Unknown fields are errors. Existing target values act as application defaults.
// Missing fields retain defaults; explicit null resets a field to its zero value.
// A decode failure may partially populate target; discard it on error.
func (s *Source) Decode(target any) error {
	return s.decode("", target)
}

// DecodeKey strictly decodes one configuration section, ignoring other sections.
// Missing sections are errors. Only the selected target's Validator runs.
// An explicit null section resets target before validation.
func (s *Source) DecodeKey(key string, target any) error {
	if strings.TrimSpace(key) == "" {
		return errors.New("config: a section key is required")
	}
	return s.decode(key, target)
}

func (s *Source) decode(key string, target any) error {
	if s == nil || s.values == nil {
		return errors.New("config: source must be initialized with Read")
	}
	value := reflect.ValueOf(target)
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() || value.Elem().Kind() != reflect.Struct {
		return errors.New("config: decode target must be a nonnil pointer to a struct")
	}
	options := []viper.DecoderConfigOption{
		func(c *mapstructure.DecoderConfig) {
			c.ErrorUnused = true
			c.DecodeNil = true
			// Preserve Viper's duration/slice conversions and add TextUnmarshaler
			// support for application types such as slog.Level.
			convert := mapstructure.ComposeDecodeHookFunc(
				mapstructure.TextUnmarshallerHookFunc(), c.DecodeHook,
			)
			c.DecodeHook = func(from, to reflect.Value) (any, error) {
				if data, ok := from.Interface().(map[string]any); ok && to.Kind() == reflect.Struct {
					clearNullPointers(data, to)
				}
				// Mapstructure represents null as a typed zero: nil maps/slices
				// for containers, or the target's zero value for scalars.
				if from.Type() == to.Type() && from.IsZero() {
					to.SetZero()
					return nil, nil
				}
				switch from.Kind() {
				case reflect.Map, reflect.Slice, reflect.Pointer, reflect.Interface:
					if from.IsNil() {
						to.SetZero()
						return nil, nil
					}
				}
				return mapstructure.DecodeHookExec(convert, from, to)
			}
		},
	}
	path := "values"
	if key != "" {
		path += "." + key
		// IsSet treats null as absent; AllKeys retains explicit null entries.
		if !s.values.IsSet(path) && !slices.Contains(s.values.AllKeys(), strings.ToLower(path)) {
			return fmt.Errorf("config: section %q not found", key)
		}
	}
	// Decode a private copy while retaining Viper's conversion behavior and
	// preserving empty maps and nulls for strict decoding.
	input := viper.New()
	input.Set("values", cloneValue(s.values.Get(path)))
	if err := input.UnmarshalKey("values", target, options...); err != nil {
		return fmt.Errorf("config: decode section %q: %w", key, err)
	}
	if validator, ok := target.(Validator); ok {
		if err := validator.Validate(); err != nil {
			return fmt.Errorf("config: validate section %q: %w", key, err)
		}
	}
	return nil
}
