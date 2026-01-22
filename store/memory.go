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

type scopeKind string

const (
	scopeSystem scopeKind = "system"
	scopeTenant scopeKind = "tenant"
	scopeOrg    scopeKind = "org"
	scopeUser   scopeKind = "user"
)

type scopeKey struct {
	kind scopeKind
	id   string
}

// NewMemoryStore constructs an in-memory override store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{entries: map[string]map[scopeKey]Override{}}
}

// Get implements Reader.
func (m *MemoryStore) Get(_ context.Context, key string, scopeSet gate.ScopeSet) (Override, error) {
	if m == nil {
		return MissingOverride(), storeRequiredError(key, scopeSet, "get")
	}
	normalized, err := normalizeKey(key)
	if err != nil {
		return MissingOverride(), err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	entries := m.entries[normalized]
	if len(entries) == 0 {
		return MissingOverride(), nil
	}
	for _, scope := range readScopes(scopeSet) {
		if override, ok := entries[scope]; ok {
			if override.State == "" {
				override.State = gate.OverrideStateMissing
			}
			return override, nil
		}
	}
	return MissingOverride(), nil
}

// Set implements Writer.
func (m *MemoryStore) Set(_ context.Context, key string, scopeSet gate.ScopeSet, enabled bool, _ gate.ActorRef) error {
	if m == nil {
		return storeRequiredError(key, scopeSet, "set")
	}
	normalized, err := normalizeKey(key)
	if err != nil {
		return err
	}
	override := DisabledOverride()
	if enabled {
		override = EnabledOverride()
	}
	scope := writeScope(scopeSet)
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
func (m *MemoryStore) Unset(_ context.Context, key string, scopeSet gate.ScopeSet, _ gate.ActorRef) error {
	if m == nil {
		return storeRequiredError(key, scopeSet, "unset")
	}
	normalized, err := normalizeKey(key)
	if err != nil {
		return err
	}
	scope := writeScope(scopeSet)
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
func (m *MemoryStore) Delete(key string, scopeSet gate.ScopeSet) bool {
	if m == nil {
		return false
	}
	normalized, err := normalizeKey(key)
	if err != nil {
		return false
	}
	scope := writeScope(scopeSet)
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

func readScopes(scopeSet gate.ScopeSet) []scopeKey {
	if scopeSet.System {
		return []scopeKey{{kind: scopeSystem}}
	}
	scopes := make([]scopeKey, 0, 4)
	if scopeSet.UserID != "" {
		scopes = append(scopes, scopeKey{kind: scopeUser, id: scopeSet.UserID})
	}
	if scopeSet.OrgID != "" {
		scopes = append(scopes, scopeKey{kind: scopeOrg, id: scopeSet.OrgID})
	}
	if scopeSet.TenantID != "" {
		scopes = append(scopes, scopeKey{kind: scopeTenant, id: scopeSet.TenantID})
	}
	scopes = append(scopes, scopeKey{kind: scopeSystem})
	return scopes
}

func writeScope(scopeSet gate.ScopeSet) scopeKey {
	switch {
	case scopeSet.System:
		return scopeKey{kind: scopeSystem}
	case scopeSet.UserID != "":
		return scopeKey{kind: scopeUser, id: scopeSet.UserID}
	case scopeSet.OrgID != "":
		return scopeKey{kind: scopeOrg, id: scopeSet.OrgID}
	case scopeSet.TenantID != "":
		return scopeKey{kind: scopeTenant, id: scopeSet.TenantID}
	default:
		return scopeKey{kind: scopeSystem}
	}
}

var _ ReadWriter = (*MemoryStore)(nil)

func storeRequiredError(key string, scopeSet gate.ScopeSet, operation string) error {
	trimmed := strings.TrimSpace(key)
	normalized := gate.NormalizeKey(trimmed)
	return ferrors.WrapSentinel(ferrors.ErrStoreRequired, "store: memory store is required", map[string]any{
		ferrors.MetaFeatureKey:           trimmed,
		ferrors.MetaFeatureKeyNormalized: normalized,
		ferrors.MetaScope:                scopeSet,
		ferrors.MetaStore:                "memory",
		ferrors.MetaOperation:            operation,
	})
}
