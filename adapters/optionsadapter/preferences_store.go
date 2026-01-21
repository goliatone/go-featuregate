package optionsadapter

import (
	"context"
	"fmt"
	"strings"

	"github.com/goliatone/go-admin/admin"
	"github.com/goliatone/go-featuregate/scope"
	opts "github.com/goliatone/go-options"
	"github.com/goliatone/go-options/pkg/state"
)

// ErrPreferencesStoreRequired indicates a missing preferences store.
var ErrPreferencesStoreRequired = fmt.Errorf("optionsadapter: preferences store is required")

// PreferencesOption customizes the PreferencesStore adapter.
type PreferencesOption func(*PreferencesStoreAdapter)

// PreferencesStoreAdapter adapts go-admin PreferencesStore into a state.Store.
type PreferencesStoreAdapter struct {
	store     admin.PreferencesStore
	keyPrefix string
	keys      []string
}

// NewPreferencesStoreAdapter constructs a new adapter for PreferencesStore.
func NewPreferencesStoreAdapter(store admin.PreferencesStore, opts ...PreferencesOption) *PreferencesStoreAdapter {
	adapter := &PreferencesStoreAdapter{store: store}
	for _, opt := range opts {
		if opt != nil {
			opt(adapter)
		}
	}
	return adapter
}

// WithKeyPrefix overrides the key prefix used for domain names.
func WithKeyPrefix(prefix string) PreferencesOption {
	return func(adapter *PreferencesStoreAdapter) {
		if adapter == nil {
			return
		}
		adapter.keyPrefix = strings.TrimSpace(prefix)
	}
}

// WithKeys restricts loads to the provided feature keys (without prefix).
func WithKeys(keys ...string) PreferencesOption {
	return func(adapter *PreferencesStoreAdapter) {
		if adapter == nil {
			return
		}
		cleaned := make([]string, 0, len(keys))
		for _, key := range keys {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			cleaned = append(cleaned, key)
		}
		adapter.keys = cleaned
	}
}

// Load implements state.Store.
func (a *PreferencesStoreAdapter) Load(ctx context.Context, ref state.Ref) (map[string]any, state.Meta, bool, error) {
	if a == nil || a.store == nil {
		return nil, state.Meta{}, false, ErrPreferencesStoreRequired
	}
	level, prefScope, err := a.preferenceScope(ref.Scope)
	if err != nil {
		return nil, state.Meta{}, false, err
	}

	keys := a.prefixedKeys(ref.Domain)
	snapshot, err := a.store.Resolve(ctx, admin.PreferencesResolveInput{
		Scope:  prefScope,
		Levels: []admin.PreferenceLevel{level},
		Keys:   keys,
	})
	if err != nil {
		return nil, state.Meta{}, false, err
	}
	if len(snapshot.Effective) == 0 {
		return nil, state.Meta{}, false, nil
	}

	prefix := a.domainPrefix(ref.Domain)
	result := map[string]any{}
	for key, value := range snapshot.Effective {
		if prefix != "" {
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			key = strings.TrimPrefix(key, prefix)
		}
		if err := setPath(result, key, value); err != nil {
			return nil, state.Meta{}, false, err
		}
	}
	if len(result) == 0 {
		return nil, state.Meta{}, false, nil
	}
	return result, state.Meta{}, true, nil
}

// Save implements state.Store.
func (a *PreferencesStoreAdapter) Save(ctx context.Context, ref state.Ref, snapshot map[string]any, _ state.Meta) (state.Meta, error) {
	if a == nil || a.store == nil {
		return state.Meta{}, ErrPreferencesStoreRequired
	}
	level, prefScope, err := a.preferenceScope(ref.Scope)
	if err != nil {
		return state.Meta{}, err
	}

	prefix := a.domainPrefix(ref.Domain)
	flat := map[string]any{}
	flattenMap("", snapshot, flat)
	flat = a.withPrefix(flat, prefix)

	existing, _, ok, err := a.Load(ctx, ref)
	if err != nil {
		return state.Meta{}, err
	}

	var deleteKeys []string
	if ok {
		existingFlat := map[string]any{}
		flattenMap("", existing, existingFlat)
		existingFlat = a.withPrefix(existingFlat, prefix)
		for key := range existingFlat {
			if _, stillPresent := flat[key]; !stillPresent {
				deleteKeys = append(deleteKeys, key)
			}
		}
	}

	if len(flat) > 0 {
		if _, err := a.store.Upsert(ctx, admin.PreferencesUpsertInput{
			Scope:  prefScope,
			Level:  level,
			Values: flat,
		}); err != nil {
			return state.Meta{}, err
		}
	}

	if len(deleteKeys) > 0 {
		if err := a.store.Delete(ctx, admin.PreferencesDeleteInput{
			Scope: prefScope,
			Level: level,
			Keys:  deleteKeys,
		}); err != nil {
			return state.Meta{}, err
		}
	}

	return state.Meta{}, nil
}

func (a *PreferencesStoreAdapter) preferenceScope(scopeDef opts.Scope) (admin.PreferenceLevel, admin.PreferenceScope, error) {
	switch scopeDef.Name {
	case "system":
		return admin.PreferenceLevelSystem, admin.PreferenceScope{}, nil
	case "tenant":
		id, err := extractScopeID(scopeDef, scope.MetadataTenantID)
		if err != nil {
			return "", admin.PreferenceScope{}, err
		}
		return admin.PreferenceLevelTenant, admin.PreferenceScope{TenantID: id}, nil
	case "org":
		id, err := extractScopeID(scopeDef, scope.MetadataOrgID)
		if err != nil {
			return "", admin.PreferenceScope{}, err
		}
		return admin.PreferenceLevelOrg, admin.PreferenceScope{OrgID: id}, nil
	case "user":
		id, err := extractScopeID(scopeDef, scope.MetadataUserID)
		if err != nil {
			return "", admin.PreferenceScope{}, err
		}
		return admin.PreferenceLevelUser, admin.PreferenceScope{UserID: id}, nil
	default:
		return "", admin.PreferenceScope{}, fmt.Errorf("optionsadapter: unsupported scope %q", scopeDef.Name)
	}
}

func extractScopeID(scopeDef opts.Scope, key string) (string, error) {
	if scopeDef.Metadata == nil {
		return "", fmt.Errorf("optionsadapter: missing metadata for scope %q", scopeDef.Name)
	}
	raw, ok := scopeDef.Metadata[key]
	if !ok {
		return "", fmt.Errorf("optionsadapter: missing metadata key %q for scope %q", key, scopeDef.Name)
	}
	id, ok := raw.(string)
	if !ok || strings.TrimSpace(id) == "" {
		return "", fmt.Errorf("optionsadapter: invalid metadata key %q for scope %q", key, scopeDef.Name)
	}
	return strings.TrimSpace(id), nil
}

func (a *PreferencesStoreAdapter) domainPrefix(domain string) string {
	if a.keyPrefix != "" {
		return normalizePrefix(a.keyPrefix)
	}
	return normalizePrefix(domain)
}

func (a *PreferencesStoreAdapter) prefixedKeys(domain string) []string {
	if len(a.keys) == 0 {
		return nil
	}
	prefix := a.domainPrefix(domain)
	keys := make([]string, 0, len(a.keys))
	for _, key := range a.keys {
		if prefix == "" {
			keys = append(keys, key)
		} else {
			keys = append(keys, prefix+key)
		}
	}
	return keys
}

func (a *PreferencesStoreAdapter) withPrefix(values map[string]any, prefix string) map[string]any {
	if prefix == "" || len(values) == 0 {
		return values
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[prefix+key] = value
	}
	return out
}

func normalizePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return ""
	}
	if !strings.HasSuffix(prefix, ".") {
		prefix += "."
	}
	return prefix
}

var _ state.Store[map[string]any] = (*PreferencesStoreAdapter)(nil)
