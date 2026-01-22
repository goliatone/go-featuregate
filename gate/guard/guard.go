package guard

import (
	"context"
	"errors"
	"fmt"

	"github.com/goliatone/go-featuregate/gate"
)

// ErrFeatureDisabled is returned when a feature is disabled and no custom error is provided.
var ErrFeatureDisabled = errors.New("feature disabled")

// DisabledError includes the disabled feature key and unwraps to ErrFeatureDisabled.
type DisabledError struct {
	Key string
}

func (e DisabledError) Error() string {
	if e.Key == "" {
		return ErrFeatureDisabled.Error()
	}
	return fmt.Sprintf("%s: %s", ErrFeatureDisabled.Error(), e.Key)
}

func (e DisabledError) Unwrap() error {
	return ErrFeatureDisabled
}

// Option configures Require behavior.
type Option func(*config)

type config struct {
	disabledErr error
	errorMapper func(error) error
	overrides   []string
}

// WithDisabledError sets the error returned when the gate is disabled.
func WithDisabledError(err error) Option {
	return func(c *config) {
		if c == nil {
			return
		}
		c.disabledErr = err
	}
}

// WithErrorMapper transforms gate errors before returning them.
func WithErrorMapper(mapper func(error) error) Option {
	return func(c *config) {
		if c == nil {
			return
		}
		c.errorMapper = mapper
	}
}

// WithOverrides allows fallback to alternate keys when the primary key is disabled.
func WithOverrides(keys ...string) Option {
	return func(c *config) {
		if c == nil {
			return
		}
		c.overrides = append(c.overrides, keys...)
	}
}

// Require checks a feature gate and returns an error when access is denied.
// If a gate is nil, Require returns nil.
func Require(ctx context.Context, fg gate.FeatureGate, key string, opts ...Option) error {
	if fg == nil {
		return nil
	}

	cfg := &config{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	enabled, err := fg.Enabled(ctx, key)
	if err != nil {
		return mapErr(cfg, err)
	}
	if enabled {
		return nil
	}

	for _, override := range cfg.overrides {
		ok, err := fg.Enabled(ctx, override)
		if err != nil {
			return mapErr(cfg, err)
		}
		if ok {
			return nil
		}
	}

	if cfg.disabledErr != nil {
		return cfg.disabledErr
	}

	return DisabledError{Key: key}
}

func mapErr(cfg *config, err error) error {
	if err == nil {
		return nil
	}
	if cfg != nil && cfg.errorMapper != nil {
		return cfg.errorMapper(err)
	}
	return err
}
