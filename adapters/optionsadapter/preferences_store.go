package optionsadapter

import (
	"context"
	"fmt"
	"strings"

	"github.com/goliatone/go-admin/admin"
	"github.com/goliatone/go-featuregate/ferrors"
	"github.com/goliatone/go-featuregate/scope"
	opts "github.com/goliatone/go-options"
	"github.com/goliatone/go-options/pkg/state"
)

// ErrPreferencesStoreRequired indicates a missing preferences store.
var ErrPreferencesStoreRequired = ferrors.ErrPreferencesStoreRequired

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
		return nil, state.Meta{}, false, prefStoreRequiredError(ref.Scope, ref.Domain, "load")
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
		return nil, state.Meta{}, false, ferrors.WrapExternal(err, ferrors.TextCodeStoreReadFailed, "optionsadapter: preferences resolve failed", map[string]any{
			ferrors.MetaAdapter:   "options",
			ferrors.MetaStore:     "preferences",
			ferrors.MetaDomain:    strings.TrimSpace(ref.Domain),
			ferrors.MetaScope:     ref.Scope,
			ferrors.MetaOperation: "resolve",
		})
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
			meta := map[string]any{
				ferrors.MetaAdapter:   "options",
				ferrors.MetaStore:     "preferences",
				ferrors.MetaDomain:    strings.TrimSpace(ref.Domain),
				ferrors.MetaScope:     ref.Scope,
				ferrors.MetaOperation: "load",
				ferrors.MetaPath:      key,
			}
			return nil, state.Meta{}, false, ferrors.WrapBadInput(err, ferrors.TextCodePathInvalid, "optionsadapter: invalid path", meta)
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
		return state.Meta{}, prefStoreRequiredError(ref.Scope, ref.Domain, "save")
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
		return state.Meta{}, ferrors.WrapExternal(err, ferrors.TextCodeStoreReadFailed, "optionsadapter: preferences load failed", map[string]any{
			ferrors.MetaAdapter:   "options",
			ferrors.MetaStore:     "preferences",
			ferrors.MetaDomain:    strings.TrimSpace(ref.Domain),
			ferrors.MetaScope:     ref.Scope,
			ferrors.MetaOperation: "load",
		})
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
			return state.Meta{}, ferrors.WrapExternal(err, ferrors.TextCodeStoreWriteFailed, "optionsadapter: preferences upsert failed", map[string]any{
				ferrors.MetaAdapter:   "options",
				ferrors.MetaStore:     "preferences",
				ferrors.MetaDomain:    strings.TrimSpace(ref.Domain),
				ferrors.MetaScope:     ref.Scope,
				ferrors.MetaOperation: "upsert",
			})
		}
	}

	if len(deleteKeys) > 0 {
		if err := a.store.Delete(ctx, admin.PreferencesDeleteInput{
			Scope: prefScope,
			Level: level,
			Keys:  deleteKeys,
		}); err != nil {
			return state.Meta{}, ferrors.WrapExternal(err, ferrors.TextCodeStoreWriteFailed, "optionsadapter: preferences delete failed", map[string]any{
				ferrors.MetaAdapter:   "options",
				ferrors.MetaStore:     "preferences",
				ferrors.MetaDomain:    strings.TrimSpace(ref.Domain),
				ferrors.MetaScope:     ref.Scope,
				ferrors.MetaOperation: "delete",
			})
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
		return "", admin.PreferenceScope{}, ferrors.NewBadInput(ferrors.TextCodeScopeInvalid, fmt.Sprintf("optionsadapter: unsupported scope %q", scopeDef.Name), map[string]any{
			ferrors.MetaAdapter:   "options",
			ferrors.MetaStore:     "preferences",
			ferrors.MetaScope:     scopeDef.Name,
			ferrors.MetaOperation: "scope",
		})
	}
}

func extractScopeID(scopeDef opts.Scope, key string) (string, error) {
	if scopeDef.Metadata == nil {
		return "", ferrors.NewBadInput(ferrors.TextCodeScopeMetadataMissing, fmt.Sprintf("optionsadapter: missing metadata for scope %q", scopeDef.Name), map[string]any{
			ferrors.MetaAdapter:   "options",
			ferrors.MetaStore:     "preferences",
			ferrors.MetaScope:     scopeDef.Name,
			ferrors.MetaOperation: "scope_metadata",
		})
	}
	raw, ok := scopeDef.Metadata[key]
	if !ok {
		return "", ferrors.NewBadInput(ferrors.TextCodeScopeMetadataMissing, fmt.Sprintf("optionsadapter: missing metadata key %q for scope %q", key, scopeDef.Name), map[string]any{
			ferrors.MetaAdapter:   "options",
			ferrors.MetaStore:     "preferences",
			ferrors.MetaScope:     scopeDef.Name,
			ferrors.MetaOperation: "scope_metadata",
		})
	}
	id, ok := raw.(string)
	if !ok || strings.TrimSpace(id) == "" {
		return "", ferrors.NewBadInput(ferrors.TextCodeScopeMetadataInvalid, fmt.Sprintf("optionsadapter: invalid metadata key %q for scope %q", key, scopeDef.Name), map[string]any{
			ferrors.MetaAdapter:   "options",
			ferrors.MetaStore:     "preferences",
			ferrors.MetaScope:     scopeDef.Name,
			ferrors.MetaOperation: "scope_metadata",
		})
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

func prefStoreRequiredError(scopeDef opts.Scope, domain, operation string) error {
	return ferrors.WrapSentinel(ferrors.ErrPreferencesStoreRequired, "optionsadapter: preferences store is required", map[string]any{
		ferrors.MetaAdapter:   "options",
		ferrors.MetaStore:     "preferences",
		ferrors.MetaDomain:    strings.TrimSpace(domain),
		ferrors.MetaScope:     scopeDef,
		ferrors.MetaOperation: operation,
	})
}
