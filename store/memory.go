package store

import (
	"context"
	"strings"
	"sync"

	"github.com/goliatone/go-featuregate/ferrors"
	"github.com/goliatone/go-featuregate/gate"
)

// ErrMemoryStoreRequired signals a missing memory store.
var ErrMemoryStoreRequired = ferrors.ErrStoreRequired

// ErrInvalidKey signals a missing or invalid feature key.
var ErrInvalidKey = ferrors.ErrInvalidKey

// MemoryStore keeps overrides in memory for tests and examples.
type MemoryStore struct {
	mu      sync.RWMutex
	entries map[string]map[scopeKey]Override
}

type scopeKey struct {
	kind     gate.ScopeKind
	id       string
	tenantID string
	orgID    string
}

// NewMemoryStore constructs an in-memory override store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{entries: map[string]map[scopeKey]Override{}}
}

// GetAll implements Reader.
func (m *MemoryStore) GetAll(_ context.Context, key string, chain gate.ScopeChain) ([]OverrideMatch, error) {
	if m == nil {
		return nil, storeRequiredError(key, gate.ScopeRef{}, "get_all")
	}
	normalized, err := normalizeKey(key)
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	entries := m.entries[normalized]
	if len(entries) == 0 {
		return nil, nil
	}
	matches := make([]OverrideMatch, 0)
	for _, ref := range chain {
		scope := scopeKeyFromRef(ref)
		if override, ok := entries[scope]; ok {
			if override.State == "" {
				override.State = gate.OverrideStateMissing
			}
			matches = append(matches, OverrideMatch{
				Scope:    ref,
				Override: override,
			})
		}
	}
	return matches, nil
}

// Set implements Writer.
func (m *MemoryStore) Set(_ context.Context, key string, scopeRef gate.ScopeRef, enabled bool, _ gate.ActorRef) error {
	if m == nil {
		return storeRequiredError(key, scopeRef, "set")
	}
	normalized, err := normalizeKey(key)
	if err != nil {
		return err
	}
	override := DisabledOverride()
	if enabled {
		override = EnabledOverride()
	}
	scope := scopeKeyFromRef(scopeRef)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries == nil {
		m.entries = map[string]map[scopeKey]Override{}
	}
	if m.entries[normalized] == nil {
		m.entries[normalized] = map[scopeKey]Override{}
	}
	m.entries[normalized][scope] = override
	return nil
}

// Unset implements Writer.
func (m *MemoryStore) Unset(_ context.Context, key string, scopeRef gate.ScopeRef, _ gate.ActorRef) error {
	if m == nil {
		return storeRequiredError(key, scopeRef, "unset")
	}
	normalized, err := normalizeKey(key)
	if err != nil {
		return err
	}
	scope := scopeKeyFromRef(scopeRef)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries == nil {
		m.entries = map[string]map[scopeKey]Override{}
	}
	if m.entries[normalized] == nil {
		m.entries[normalized] = map[scopeKey]Override{}
	}
	m.entries[normalized][scope] = UnsetOverride()
	return nil
}

// Delete removes a stored override entirely.
func (m *MemoryStore) Delete(key string, scopeRef gate.ScopeRef) bool {
	if m == nil {
		return false
	}
	normalized, err := normalizeKey(key)
	if err != nil {
		return false
	}
	scope := scopeKeyFromRef(scopeRef)
	m.mu.Lock()
	defer m.mu.Unlock()
	entries := m.entries[normalized]
	if len(entries) == 0 {
		return false
	}
	if _, ok := entries[scope]; !ok {
		return false
	}
	delete(entries, scope)
	if len(entries) == 0 {
		delete(m.entries, normalized)
	}
	return true
}

// Clear removes all stored overrides.
func (m *MemoryStore) Clear() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = map[string]map[scopeKey]Override{}
}

func normalizeKey(key string) (string, error) {
	trimmed := strings.TrimSpace(key)
	normalized := gate.NormalizeKey(trimmed)
	if normalized == "" {
		return "", ferrors.WrapSentinel(ferrors.ErrInvalidKey, "store: feature key required", map[string]any{
			ferrors.MetaFeatureKey:           trimmed,
			ferrors.MetaFeatureKeyNormalized: normalized,
			ferrors.MetaStore:                "memory",
		})
	}
	return normalized, nil
}

func scopeKeyFromRef(ref gate.ScopeRef) scopeKey {
	id := ref.ID
	if id == "" {
		switch ref.Kind {
		case gate.ScopeTenant:
			id = ref.TenantID
		case gate.ScopeOrg:
			id = ref.OrgID
		}
	}
	return scopeKey{
		kind:     ref.Kind,
		id:       id,
		tenantID: ref.TenantID,
		orgID:    ref.OrgID,
	}
}

var _ ReadWriter = (*MemoryStore)(nil)

func storeRequiredError(key string, scopeRef gate.ScopeRef, operation string) error {
	trimmed := strings.TrimSpace(key)
	normalized := gate.NormalizeKey(trimmed)
	return ferrors.WrapSentinel(ferrors.ErrStoreRequired, "store: memory store is required", map[string]any{
		ferrors.MetaFeatureKey:           trimmed,
		ferrors.MetaFeatureKeyNormalized: normalized,
		ferrors.MetaScope:                scopeRef,
		ferrors.MetaStore:                "memory",
		ferrors.MetaOperation:            operation,
	})
}
