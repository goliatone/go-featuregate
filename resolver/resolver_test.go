package resolver

import (
	"context"
	"errors"
	"testing"

	"github.com/goliatone/go-featuregate/gate"
	"github.com/goliatone/go-featuregate/store"
)

type staticDefaults map[string]DefaultResult

func (d staticDefaults) Default(_ context.Context, key string) (DefaultResult, error) {
	if value, ok := d[key]; ok {
		return value, nil
	}
	return DefaultResult{}, nil
}

type stubStore struct {
	overrides  map[string]store.Override
	getErr     error
	setErr     error
	unsetErr   error
	getCalls   []string
	setCalls   []string
	unsetCalls []string
}

func (s *stubStore) Get(_ context.Context, key string, _ gate.ScopeSet) (store.Override, error) {
	s.getCalls = append(s.getCalls, key)
	if s.getErr != nil {
		return store.MissingOverride(), s.getErr
	}
	if override, ok := s.overrides[key]; ok {
		return override, nil
	}
	return store.MissingOverride(), nil
}

func (s *stubStore) Set(_ context.Context, key string, _ gate.ScopeSet, _ bool, _ gate.ActorRef) error {
	s.setCalls = append(s.setCalls, key)
	if s.setErr != nil {
		return s.setErr
	}
	return nil
}

func (s *stubStore) Unset(_ context.Context, key string, _ gate.ScopeSet, _ gate.ActorRef) error {
	s.unsetCalls = append(s.unsetCalls, key)
	if s.unsetErr != nil {
		return s.unsetErr
	}
	return nil
}

func TestGateResolvesOverrideBeforeDefault(t *testing.T) {
	ctx := context.Background()
	defaults := staticDefaults{
		"users.signup": {Set: true, Value: true},
	}
	storeStub := &stubStore{
		overrides: map[string]store.Override{
			"users.signup": store.DisabledOverride(),
		},
	}
	g := New(
		WithDefaults(defaults),
		WithOverrideStore(storeStub),
	)

	value, err := g.Enabled(ctx, "users.signup")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value {
		t.Fatalf("expected override to disable feature")
	}
}

func TestGateFallsBackToDefaultsOnStoreError(t *testing.T) {
	ctx := context.Background()
	defaults := staticDefaults{
		"users.signup": {Set: true, Value: true},
	}
	storeStub := &stubStore{getErr: errors.New("store down")}
	g := New(
		WithDefaults(defaults),
		WithOverrideStore(storeStub),
	)

	value, err := g.Enabled(ctx, "users.signup")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !value {
		t.Fatalf("expected default to be used on store error")
	}
}

func TestGateStrictStoreReturnsError(t *testing.T) {
	ctx := context.Background()
	defaults := staticDefaults{
		"users.signup": {Set: true, Value: true},
	}
	storeStub := &stubStore{getErr: errors.New("store down")}
	g := New(
		WithDefaults(defaults),
		WithOverrideStore(storeStub),
		WithStrictStore(true),
	)

	if _, err := g.Enabled(ctx, "users.signup"); err == nil {
		t.Fatalf("expected strict store error")
	}
}

func TestGateResolvesAliasOverride(t *testing.T) {
	ctx := context.Background()
	storeStub := &stubStore{
		overrides: map[string]store.Override{
			"users.self_registration": store.EnabledOverride(),
		},
	}
	g := New(WithOverrideStore(storeStub))

	value, err := g.Enabled(ctx, "users.signup")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !value {
		t.Fatalf("expected alias override to enable feature")
	}
	if len(storeStub.getCalls) != 2 {
		t.Fatalf("expected 2 get calls, got %d", len(storeStub.getCalls))
	}
	if storeStub.getCalls[0] != "users.signup" || storeStub.getCalls[1] != "users.self_registration" {
		t.Fatalf("unexpected get call order: %v", storeStub.getCalls)
	}
}

func TestGateUnsetClearsAliases(t *testing.T) {
	ctx := context.Background()
	storeStub := &stubStore{}
	g := New(WithOverrideWriter(storeStub))

	if err := g.Unset(ctx, "users.signup", gate.ScopeSet{TenantID: "tenant-1"}, gate.ActorRef{ID: "actor"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(storeStub.unsetCalls) != 2 {
		t.Fatalf("expected 2 unset calls, got %d", len(storeStub.unsetCalls))
	}
	if storeStub.unsetCalls[0] != "users.signup" || storeStub.unsetCalls[1] != "users.self_registration" {
		t.Fatalf("unexpected unset call order: %v", storeStub.unsetCalls)
	}
}
