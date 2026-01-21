package optionsadapter

import (
	"context"
	"sync"
	"testing"

	opts "github.com/goliatone/go-options"
	"github.com/goliatone/go-options/pkg/state"

	"github.com/goliatone/go-featuregate/gate"
	"github.com/goliatone/go-featuregate/scope"
)

type memoryStateStore struct {
	mu          sync.RWMutex
	snapshots   map[string]map[string]any
	lastSaveRef state.Ref
}

func newMemoryStateStore() *memoryStateStore {
	return &memoryStateStore{
		snapshots: map[string]map[string]any{},
	}
}

func (m *memoryStateStore) Load(_ context.Context, ref state.Ref) (map[string]any, state.Meta, bool, error) {
	key, err := ref.Identifier()
	if err != nil {
		return nil, state.Meta{}, false, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	snapshot, ok := m.snapshots[key]
	if !ok {
		return nil, state.Meta{}, false, nil
	}
	return cloneSnapshot(snapshot), state.Meta{}, true, nil
}

func (m *memoryStateStore) Save(_ context.Context, ref state.Ref, snapshot map[string]any, _ state.Meta) (state.Meta, error) {
	key, err := ref.Identifier()
	if err != nil {
		return state.Meta{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastSaveRef = ref
	m.snapshots[key] = cloneSnapshot(snapshot)
	return state.Meta{}, nil
}

func (m *memoryStateStore) seed(ref state.Ref, snapshot map[string]any) error {
	key, err := ref.Identifier()
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.snapshots[key] = cloneSnapshot(snapshot)
	return nil
}

func cloneSnapshot(snapshot map[string]any) map[string]any {
	if snapshot == nil {
		return nil
	}
	out := make(map[string]any, len(snapshot))
	for key, value := range snapshot {
		out[key] = value
	}
	return out
}

func TestStoreSetWritesUserScopeMetadata(t *testing.T) {
	ctx := context.Background()
	stateStore := newMemoryStateStore()
	store := NewStore(stateStore)

	scopeSet := gate.ScopeSet{UserID: "user-1"}
	if err := store.Set(ctx, "users.signup", scopeSet, true, gate.ActorRef{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ref := stateStore.lastSaveRef
	if ref.Scope.Name != "user" {
		t.Fatalf("expected scope name user, got %q", ref.Scope.Name)
	}
	if ref.Scope.Metadata == nil || ref.Scope.Metadata[scope.MetadataUserID] != "user-1" {
		t.Fatalf("expected scope metadata user_id to be set")
	}
}

func TestStoreGetRespectsScopePrecedence(t *testing.T) {
	ctx := context.Background()
	stateStore := newMemoryStateStore()
	store := NewStore(stateStore)

	tenantScope := opts.NewScope("tenant", 20, opts.WithScopeMetadata(map[string]any{
		scope.MetadataTenantID: "tenant-1",
	}))
	userScope := opts.NewScope("user", 40, opts.WithScopeMetadata(map[string]any{
		scope.MetadataUserID: "user-1",
	}))

	if err := stateStore.seed(state.Ref{Domain: DefaultDomain, Scope: tenantScope}, map[string]any{
		"users.signup": true,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := stateStore.seed(state.Ref{Domain: DefaultDomain, Scope: userScope}, map[string]any{
		"users.signup": false,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	override, err := store.Get(ctx, "users.signup", gate.ScopeSet{
		TenantID: "tenant-1",
		UserID:   "user-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if override.State != gate.OverrideStateDisabled {
		t.Fatalf("expected user override to win, got %q", override.State)
	}
}
