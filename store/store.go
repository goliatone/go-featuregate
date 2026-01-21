package store

import (
	"context"

	"github.com/goliatone/go-featuregate/gate"
)

// Override captures the runtime override state.
type Override struct {
	State gate.OverrideState
	Value bool
}

// MissingOverride builds a placeholder override for absent values.
func MissingOverride() Override {
	return Override{State: gate.OverrideStateMissing}
}

// UnsetOverride builds a placeholder override for explicit unsets.
func UnsetOverride() Override {
	return Override{State: gate.OverrideStateUnset}
}

// EnabledOverride marks an enabled override.
func EnabledOverride() Override {
	return Override{State: gate.OverrideStateEnabled, Value: true}
}

// DisabledOverride marks a disabled override.
func DisabledOverride() Override {
	return Override{State: gate.OverrideStateDisabled, Value: false}
}

// HasValue reports whether the override contains a concrete value.
func (o Override) HasValue() bool {
	return o.State == gate.OverrideStateEnabled || o.State == gate.OverrideStateDisabled
}

// Reader resolves runtime overrides.
type Reader interface {
	Get(ctx context.Context, key string, scope gate.ScopeSet) (Override, error)
}

// Writer stores runtime overrides.
type Writer interface {
	Set(ctx context.Context, key string, scope gate.ScopeSet, enabled bool, actor gate.ActorRef) error
	Unset(ctx context.Context, key string, scope gate.ScopeSet, actor gate.ActorRef) error
}

// ReadWriter is a combined reader/writer.
type ReadWriter interface {
	Reader
	Writer
}
