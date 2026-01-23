package resolver

import (
	"context"
	"errors"
	"testing"

	goerrors "github.com/goliatone/go-errors"

	"github.com/goliatone/go-featuregate/ferrors"
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
	overrides    map[string]store.Override
	getErr       error
	setErr       error
	unsetErr     error
	getCalls     []string
	setCalls     []string
	unsetCalls   []string
	lastChainLen int
}

func (s *stubStore) GetAll(_ context.Context, key string, chain gate.ScopeChain) ([]store.OverrideMatch, error) {
	s.getCalls = append(s.getCalls, key)
	s.lastChainLen = len(chain)
	if s.getErr != nil {
		return nil, s.getErr
	}
	if override, ok := s.overrides[key]; ok {
		ref := gate.ScopeRef{Kind: gate.ScopeSystem}
		if len(chain) > 0 {
			ref = chain[0]
		}
		return []store.OverrideMatch{{Scope: ref, Override: override}}, nil
	}
	return nil, nil
}

func (s *stubStore) Set(_ context.Context, key string, _ gate.ScopeRef, _ bool, _ gate.ActorRef) error {
	s.setCalls = append(s.setCalls, key)
	if s.setErr != nil {
		return s.setErr
	}
	return nil
}

func (s *stubStore) Unset(_ context.Context, key string, _ gate.ScopeRef, _ gate.ActorRef) error {
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

	chain := gate.ScopeChain{
		{Kind: gate.ScopeTenant, ID: "tenant-1", TenantID: "tenant-1"},
		{Kind: gate.ScopeSystem},
	}
	_, err := g.Enabled(ctx, "users.signup", gate.WithScopeChain(chain))
	if err == nil {
		t.Fatalf("expected strict store error")
	}
	var rich *goerrors.Error
	if !goerrors.As(err, &rich) {
		t.Fatalf("expected rich error")
	}
	if rich.Category != goerrors.CategoryExternal {
		t.Fatalf("unexpected category: %s", rich.Category)
	}
	if rich.TextCode != ferrors.TextCodeStoreReadFailed {
		t.Fatalf("unexpected text code: %s", rich.TextCode)
	}
	if rich.Metadata == nil || rich.Metadata[ferrors.MetaStrict] != true {
		t.Fatalf("expected strict metadata to be set")
	}
	if rich.Metadata[ferrors.MetaFeatureKeyNormalized] != "users.signup" {
		t.Fatalf("expected feature key metadata to be set")
	}
}

func TestGateDoesNotResolveLegacyAliasOverride(t *testing.T) {
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
	if value {
		t.Fatalf("expected legacy alias override to be ignored")
	}
	if len(storeStub.getCalls) != 1 {
		t.Fatalf("expected 1 get call, got %d", len(storeStub.getCalls))
	}
	if storeStub.getCalls[0] != "users.signup" {
		t.Fatalf("unexpected get call order: %v", storeStub.getCalls)
	}
}

func TestGateUnsetDoesNotClearLegacyAliases(t *testing.T) {
	ctx := context.Background()
	storeStub := &stubStore{}
	g := New(WithOverrideWriter(storeStub))

	if err := g.Unset(ctx, "users.signup", gate.ScopeRef{Kind: gate.ScopeTenant, ID: "tenant-1", TenantID: "tenant-1"}, gate.ActorRef{ID: "actor"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(storeStub.unsetCalls) != 1 {
		t.Fatalf("expected 1 unset call, got %d", len(storeStub.unsetCalls))
	}
	if storeStub.unsetCalls[0] != "users.signup" {
		t.Fatalf("unexpected unset call order: %v", storeStub.unsetCalls)
	}
}
