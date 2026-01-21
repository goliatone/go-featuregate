package configadapter

import (
	"context"
	"testing"

	"github.com/goliatone/go-config/config"
)

func TestDefaultsOptionalBoolValues(t *testing.T) {
	defaults := NewDefaults(map[string]any{
		"users": map[string]any{
			"signup": config.NewOptionalBool(true),
		},
	})

	result, err := defaults.Default(context.Background(), "users.signup")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Set || !result.Value {
		t.Fatalf("expected optional bool default to be set true, got %+v", result)
	}
}

func TestDefaultsOptionalBoolUnset(t *testing.T) {
	defaults := NewDefaults(map[string]any{
		"users": map[string]any{
			"signup": config.NewOptionalBoolUnset(),
		},
	})

	result, err := defaults.Default(context.Background(), "users.signup")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Set {
		t.Fatalf("expected optional bool default to be unset, got %+v", result)
	}
}

func TestDefaultsFromBools(t *testing.T) {
	defaults := NewDefaultsFromBools(map[string]bool{
		"users.signup": true,
	})

	result, err := defaults.Default(context.Background(), "users.signup")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Set || !result.Value {
		t.Fatalf("expected bool default to be set true, got %+v", result)
	}
}
