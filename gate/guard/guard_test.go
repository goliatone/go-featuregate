package guard

import (
	"context"
	"errors"
	"testing"

	"github.com/goliatone/go-featuregate/gate"
)

type stubGate struct {
	enabled map[string]bool
	err     error
	calls   []string
}

func (s *stubGate) Enabled(ctx context.Context, key string, opts ...gate.ResolveOption) (bool, error) {
	s.calls = append(s.calls, key)
	if s.err != nil {
		return false, s.err
	}
	if s.enabled == nil {
		return true, nil
	}
	enabled, ok := s.enabled[key]
	if !ok {
		return true, nil
	}
	return enabled, nil
}

func TestRequireAllowsNilGate(t *testing.T) {
	if err := Require(context.Background(), nil, "users.signup"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRequireReturnsDisabledError(t *testing.T) {
	disabledErr := errors.New("disabled")
	stub := &stubGate{
		enabled: map[string]bool{
			"users.signup": false,
		},
	}

	err := Require(context.Background(), stub, "users.signup", WithDisabledError(disabledErr))
	if err != disabledErr {
		t.Fatalf("expected disabled error, got %v", err)
	}
	if len(stub.calls) != 1 || stub.calls[0] != "users.signup" {
		t.Fatalf("unexpected calls: %v", stub.calls)
	}
}

func TestRequireHonorsOverride(t *testing.T) {
	stub := &stubGate{
		enabled: map[string]bool{
			"users.password_reset":          false,
			"users.password_reset.finalize": true,
		},
	}

	err := Require(context.Background(), stub, "users.password_reset", WithOverrides("users.password_reset.finalize"))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(stub.calls) != 2 {
		t.Fatalf("expected two calls, got %v", stub.calls)
	}
}

func TestRequireMapsErrors(t *testing.T) {
	rawErr := errors.New("gate failed")
	mappedErr := errors.New("mapped")
	stub := &stubGate{err: rawErr}

	err := Require(context.Background(), stub, "users.signup", WithErrorMapper(func(err error) error {
		if err == rawErr {
			return mappedErr
		}
		return err
	}))
	if err != mappedErr {
		t.Fatalf("expected mapped error, got %v", err)
	}
}

func TestRequireDefaultDisabledError(t *testing.T) {
	stub := &stubGate{
		enabled: map[string]bool{
			"users.signup": false,
		},
	}

	err := Require(context.Background(), stub, "users.signup")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, ErrFeatureDisabled) {
		t.Fatalf("expected ErrFeatureDisabled, got %v", err)
	}
}
