package configadapter

import (
	"context"
	"strings"

	"github.com/goliatone/go-config/config"
	"github.com/goliatone/go-featuregate/gate"
	"github.com/goliatone/go-featuregate/resolver"
)

type configOptions struct {
	delimiter string
}

// Option configures configadapter parsing.
type Option func(*configOptions)

// WithDelimiter sets the key delimiter used when flattening nested maps.
func WithDelimiter(delimiter string) Option {
	return func(cfg *configOptions) {
		if cfg == nil {
			return
		}
		cfg.delimiter = delimiter
	}
}

// Defaults provides resolver.Defaults backed by config maps.
type Defaults struct {
	values map[string]resolver.DefaultResult
}

// NewDefaults builds Defaults from a nested map containing OptionalBool or bool values.
func NewDefaults(data map[string]any, opts ...Option) *Defaults {
	cfg := configOptions{delimiter: "."}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.delimiter == "" {
		cfg.delimiter = "."
	}

	values := map[string]resolver.DefaultResult{}
	flattenDefaults("", data, cfg.delimiter, values)
	return &Defaults{values: values}
}

// NewDefaultsFromBools builds Defaults from a simple map of booleans.
func NewDefaultsFromBools(data map[string]bool, opts ...Option) *Defaults {
	if len(data) == 0 {
		return NewDefaults(nil, opts...)
	}
	raw := make(map[string]any, len(data))
	for key, value := range data {
		raw[key] = value
	}
	return NewDefaults(raw, opts...)
}

// Default implements resolver.Defaults.
func (d *Defaults) Default(_ context.Context, key string) (resolver.DefaultResult, error) {
	if d == nil || len(d.values) == 0 {
		return resolver.DefaultResult{}, nil
	}
	normalized := gate.NormalizeKey(strings.TrimSpace(key))
	if normalized == "" {
		return resolver.DefaultResult{}, nil
	}
	if value, ok := d.values[normalized]; ok {
		return value, nil
	}
	return resolver.DefaultResult{}, nil
}

type optionalBool interface {
	IsSet() bool
	Value() bool
}

func flattenDefaults(prefix string, data map[string]any, delim string, out map[string]resolver.DefaultResult) {
	if len(data) == 0 {
		return
	}
	for key, value := range data {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		path := trimmedKey
		if prefix != "" {
			path = prefix + delim + trimmedKey
		}

		switch typed := value.(type) {
		case map[string]any:
			flattenDefaults(path, typed, delim, out)
		case map[string]bool:
			flattenDefaults(path, boolMapToAny(typed), delim, out)
		default:
			if def, ok := defaultFromValue(value); ok {
				normalized := gate.NormalizeKey(path)
				if normalized == "" {
					continue
				}
				out[normalized] = def
			}
		}
	}
}

func defaultFromValue(value any) (resolver.DefaultResult, bool) {
	switch typed := value.(type) {
	case optionalBool:
		return resolver.DefaultResult{Set: typed.IsSet(), Value: typed.Value()}, true
	case config.OptionalBool:
		return resolver.DefaultResult{Set: typed.IsSet(), Value: typed.Value()}, true
	case *config.OptionalBool:
		if typed == nil {
			return resolver.DefaultResult{}, true
		}
		return resolver.DefaultResult{Set: typed.IsSet(), Value: typed.Value()}, true
	case bool:
		return resolver.DefaultResult{Set: true, Value: typed}, true
	case *bool:
		if typed == nil {
			return resolver.DefaultResult{}, true
		}
		return resolver.DefaultResult{Set: true, Value: *typed}, true
	default:
		return resolver.DefaultResult{}, false
	}
}

func boolMapToAny(data map[string]bool) map[string]any {
	if len(data) == 0 {
		return nil
	}
	out := make(map[string]any, len(data))
	for key, value := range data {
		out[key] = value
	}
	return out
}
