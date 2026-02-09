package optionsadapter

import (
	"context"
	"errors"
	"strings"

	goerrors "github.com/goliatone/go-errors"
	"github.com/goliatone/go-featuregate/ferrors"
	"github.com/goliatone/go-featuregate/scope"
	opts "github.com/goliatone/go-options"
	"github.com/goliatone/go-options/pkg/state"
)

// PreferenceLevel identifies preference scope granularity.
type PreferenceLevel string

const (
	PreferenceLevelSystem PreferenceLevel = "system"
	PreferenceLevelTenant PreferenceLevel = "tenant"
	PreferenceLevelOrg    PreferenceLevel = "org"
	PreferenceLevelUser   PreferenceLevel = "user"
)

// PreferenceScope identifies one concrete scope target.
type PreferenceScope struct {
	TenantID string
	OrgID    string
	UserID   string
}

// PreferencesSnapshot contains resolved key/value pairs.
type PreferencesSnapshot struct {
	Effective map[string]any
}

// PreferencesResolveInput configures scoped preference resolution.
type PreferencesResolveInput struct {
	Scope  PreferenceScope
	Levels []PreferenceLevel
	Keys   []string
}

// PreferencesUpsertInput configures scoped preference writes.
type PreferencesUpsertInput struct {
	Scope  PreferenceScope
	Level  PreferenceLevel
	Values map[string]any
}

// PreferencesDeleteInput configures scoped preference key deletions.
type PreferencesDeleteInput struct {
	Scope PreferenceScope
	Level PreferenceLevel
	Keys  []string
}

// PreferencesStore is the minimal backend contract used by the adapter.
type PreferencesStore interface {
	Resolve(ctx context.Context, input PreferencesResolveInput) (PreferencesSnapshot, error)
	Upsert(ctx context.Context, input PreferencesUpsertInput) (PreferencesSnapshot, error)
	Delete(ctx context.Context, input PreferencesDeleteInput) error
}

var (
	// ErrPreferencesStoreRequired indicates a missing preferences backend.
	ErrPreferencesStoreRequired = ferrors.ErrPreferencesStoreRequired
	// ErrPreferencesScopeMetadataInvalid indicates malformed options scope metadata.
	ErrPreferencesScopeMetadataInvalid = goerrors.New(
		"preferences scope metadata is invalid",
		goerrors.CategoryBadInput,
	).WithTextCode(ferrors.TextCodeScopeMetadataInvalid).WithCode(goerrors.CodeBadRequest)
	// ErrPreferencesPathInvalid indicates an invalid flatten/unflatten path.
	ErrPreferencesPathInvalid = goerrors.New(
		"preferences path is invalid",
		goerrors.CategoryBadInput,
	).WithTextCode(ferrors.TextCodePathInvalid).WithCode(goerrors.CodeBadRequest)
)

// PreferencesOption customizes the preferences-backed state store adapter.
type PreferencesOption func(*PreferencesStoreAdapter)

// PreferencesStoreAdapter adapts a preferences backend to state.Store.
type PreferencesStoreAdapter struct {
	store         PreferencesStore
	keyPrefix     string
	keys          []string
	keysSet       map[string]struct{}
	deleteMissing bool
}

// NewPreferencesStoreAdapter constructs a preferences-backed state adapter.
func NewPreferencesStoreAdapter(store PreferencesStore, opts ...PreferencesOption) *PreferencesStoreAdapter {
	adapter := &PreferencesStoreAdapter{
		store:         store,
		deleteMissing: false,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(adapter)
		}
	}
	return adapter
}

// WithKeyPrefix sets a fixed key prefix. If empty, state.Ref.Domain is used.
func WithKeyPrefix(prefix string) PreferencesOption {
	return func(adapter *PreferencesStoreAdapter) {
		if adapter == nil {
			return
		}
		adapter.keyPrefix = strings.TrimSpace(prefix)
	}
}

// WithKeys limits Load/Save/Delete to the provided logical keys.
func WithKeys(keys ...string) PreferencesOption {
	return func(adapter *PreferencesStoreAdapter) {
		if adapter == nil {
			return
		}
		cleaned := make([]string, 0, len(keys))
		seen := map[string]struct{}{}
		for _, key := range keys {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			cleaned = append(cleaned, key)
		}
		adapter.keys = cleaned
		adapter.keysSet = seen
	}
}

// WithDeleteMissing controls whether Save deletes persisted keys missing in snapshot.
func WithDeleteMissing(enabled bool) PreferencesOption {
	return func(adapter *PreferencesStoreAdapter) {
		if adapter == nil {
			return
		}
		adapter.deleteMissing = enabled
	}
}

// Load implements state.Store.
func (a *PreferencesStoreAdapter) Load(ctx context.Context, ref state.Ref) (map[string]any, state.Meta, bool, error) {
	if a == nil || a.store == nil {
		return nil, state.Meta{}, false, preferencesStoreRequiredError(ref, "load")
	}
	level, prefScope, err := a.preferenceScope(ref.Scope)
	if err != nil {
		return nil, state.Meta{}, false, scopeMetaWrap(err, ref, "load_scope")
	}

	resolved, err := a.store.Resolve(ctx, PreferencesResolveInput{
		Scope:  prefScope,
		Levels: []PreferenceLevel{level},
		Keys:   a.prefixedKeys(ref.Domain),
	})
	if err != nil {
		return nil, state.Meta{}, false, ferrors.WrapExternal(
			err,
			ferrors.TextCodeStoreReadFailed,
			"optionsadapter: preferences resolve failed",
			preferencesMeta(ref, "load"),
		)
	}
	if len(resolved.Effective) == 0 {
		return nil, state.Meta{}, false, nil
	}

	prefix := a.domainPrefix(ref.Domain)
	snapshot := map[string]any{}
	for rawKey, value := range resolved.Effective {
		logicalKey, ok, keyErr := a.logicalKey(rawKey, prefix)
		if keyErr != nil {
			return nil, state.Meta{}, false, pathMetaWrap(keyErr, ref, "load_path")
		}
		if !ok || !a.isAllowed(logicalKey) {
			continue
		}
		if err := setPathStrict(snapshot, logicalKey, value); err != nil {
			return nil, state.Meta{}, false, pathMetaWrap(err, ref, "load_path")
		}
	}
	if len(snapshot) == 0 {
		return nil, state.Meta{}, false, nil
	}
	return snapshot, state.Meta{}, true, nil
}

// Save implements state.Store.
func (a *PreferencesStoreAdapter) Save(ctx context.Context, ref state.Ref, snapshot map[string]any, _ state.Meta) (state.Meta, error) {
	if a == nil || a.store == nil {
		return state.Meta{}, preferencesStoreRequiredError(ref, "save")
	}
	level, prefScope, err := a.preferenceScope(ref.Scope)
	if err != nil {
		return state.Meta{}, scopeMetaWrap(err, ref, "save_scope")
	}

	flat, err := flattenSnapshot(snapshot, a.keysSet)
	if err != nil {
		return state.Meta{}, pathMetaWrap(err, ref, "save_flatten")
	}
	prefix := a.domainPrefix(ref.Domain)
	flat = a.withPrefix(flat, prefix)

	if len(flat) > 0 {
		if _, err := a.store.Upsert(ctx, PreferencesUpsertInput{
			Scope:  prefScope,
			Level:  level,
			Values: flat,
		}); err != nil {
			return state.Meta{}, ferrors.WrapExternal(
				err,
				ferrors.TextCodeStoreWriteFailed,
				"optionsadapter: preferences upsert failed",
				preferencesMeta(ref, "save_upsert"),
			)
		}
	}

	if !a.deleteMissing {
		return state.Meta{}, nil
	}

	existingKeys, err := a.loadExistingKeys(ctx, ref, prefScope, level, prefix)
	if err != nil {
		return state.Meta{}, err
	}
	var deleteKeys []string
	for key := range existingKeys {
		if _, keep := flat[key]; !keep {
			deleteKeys = append(deleteKeys, key)
		}
	}
	if len(deleteKeys) == 0 {
		return state.Meta{}, nil
	}

	if err := a.store.Delete(ctx, PreferencesDeleteInput{
		Scope: prefScope,
		Level: level,
		Keys:  deleteKeys,
	}); err != nil {
		return state.Meta{}, ferrors.WrapExternal(
			err,
			ferrors.TextCodeStoreWriteFailed,
			"optionsadapter: preferences delete failed",
			preferencesMeta(ref, "save_delete"),
		)
	}
	return state.Meta{}, nil
}

func (a *PreferencesStoreAdapter) loadExistingKeys(
	ctx context.Context,
	ref state.Ref,
	prefScope PreferenceScope,
	level PreferenceLevel,
	prefix string,
) (map[string]struct{}, error) {
	resolved, err := a.store.Resolve(ctx, PreferencesResolveInput{
		Scope:  prefScope,
		Levels: []PreferenceLevel{level},
		Keys:   a.prefixedKeys(ref.Domain),
	})
	if err != nil {
		return nil, ferrors.WrapExternal(
			err,
			ferrors.TextCodeStoreReadFailed,
			"optionsadapter: preferences resolve failed",
			preferencesMeta(ref, "load_existing"),
		)
	}
	out := map[string]struct{}{}
	for rawKey := range resolved.Effective {
		logicalKey, ok, keyErr := a.logicalKey(rawKey, prefix)
		if keyErr != nil {
			return nil, pathMetaWrap(keyErr, ref, "load_existing_path")
		}
		if !ok || !a.isAllowed(logicalKey) {
			continue
		}
		namespaced := logicalKey
		if prefix != "" {
			namespaced = prefix + logicalKey
		}
		out[namespaced] = struct{}{}
	}
	return out, nil
}

func (a *PreferencesStoreAdapter) preferenceScope(scopeDef opts.Scope) (PreferenceLevel, PreferenceScope, error) {
	switch strings.ToLower(strings.TrimSpace(scopeDef.Name)) {
	case "system":
		return PreferenceLevelSystem, PreferenceScope{}, nil
	case "tenant":
		tenantID, err := requiredScopeMetadata(scopeDef, scope.MetadataTenantID)
		if err != nil {
			return "", PreferenceScope{}, err
		}
		return PreferenceLevelTenant, PreferenceScope{TenantID: tenantID}, nil
	case "org":
		orgID, err := requiredScopeMetadata(scopeDef, scope.MetadataOrgID)
		if err != nil {
			return "", PreferenceScope{}, err
		}
		tenantID, err := optionalScopeMetadata(scopeDef, scope.MetadataTenantID)
		if err != nil {
			return "", PreferenceScope{}, err
		}
		return PreferenceLevelOrg, PreferenceScope{TenantID: tenantID, OrgID: orgID}, nil
	case "user":
		userID, err := requiredScopeMetadata(scopeDef, scope.MetadataUserID)
		if err != nil {
			return "", PreferenceScope{}, err
		}
		tenantID, err := optionalScopeMetadata(scopeDef, scope.MetadataTenantID)
		if err != nil {
			return "", PreferenceScope{}, err
		}
		orgID, err := optionalScopeMetadata(scopeDef, scope.MetadataOrgID)
		if err != nil {
			return "", PreferenceScope{}, err
		}
		return PreferenceLevelUser, PreferenceScope{TenantID: tenantID, OrgID: orgID, UserID: userID}, nil
	default:
		return "", PreferenceScope{}, ferrors.WrapSentinel(
			ErrPreferencesScopeMetadataInvalid,
			"optionsadapter: unsupported scope for preferences adapter",
			map[string]any{
				ferrors.MetaAdapter: "options_preferences",
				ferrors.MetaStore:   "preferences",
				ferrors.MetaScope:   scopeDef,
			},
		)
	}
}

func requiredScopeMetadata(scopeDef opts.Scope, key string) (string, error) {
	value, ok, err := metadataString(scopeDef, key)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ferrors.WrapSentinel(
			ErrPreferencesScopeMetadataInvalid,
			"optionsadapter: required scope metadata missing",
			map[string]any{
				ferrors.MetaAdapter: "options_preferences",
				ferrors.MetaStore:   "preferences",
				ferrors.MetaScope:   scopeDef,
				"metadata_key":      key,
			},
		)
	}
	return value, nil
}

func optionalScopeMetadata(scopeDef opts.Scope, key string) (string, error) {
	value, ok, err := metadataString(scopeDef, key)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", nil
	}
	return value, nil
}

func metadataString(scopeDef opts.Scope, key string) (string, bool, error) {
	if scopeDef.Metadata == nil {
		return "", false, nil
	}
	raw, ok := scopeDef.Metadata[key]
	if !ok || raw == nil {
		return "", false, nil
	}
	value, isString := raw.(string)
	if !isString {
		return "", false, ferrors.WrapSentinel(
			ErrPreferencesScopeMetadataInvalid,
			"optionsadapter: invalid scope metadata type",
			map[string]any{
				ferrors.MetaAdapter: "options_preferences",
				ferrors.MetaStore:   "preferences",
				ferrors.MetaScope:   scopeDef,
				"metadata_key":      key,
				"metadata_value":    raw,
			},
		)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false, ferrors.WrapSentinel(
			ErrPreferencesScopeMetadataInvalid,
			"optionsadapter: invalid empty scope metadata value",
			map[string]any{
				ferrors.MetaAdapter: "options_preferences",
				ferrors.MetaStore:   "preferences",
				ferrors.MetaScope:   scopeDef,
				"metadata_key":      key,
			},
		)
	}
	return value, true, nil
}

func (a *PreferencesStoreAdapter) domainPrefix(domain string) string {
	if trimmed := strings.TrimSpace(a.keyPrefix); trimmed != "" {
		return normalizePrefix(trimmed)
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
			continue
		}
		keys = append(keys, prefix+key)
	}
	return keys
}

func (a *PreferencesStoreAdapter) withPrefix(values map[string]any, prefix string) map[string]any {
	if len(values) == 0 || prefix == "" {
		return values
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[prefix+key] = value
	}
	return out
}

func (a *PreferencesStoreAdapter) isAllowed(key string) bool {
	if len(a.keysSet) == 0 {
		return true
	}
	_, ok := a.keysSet[key]
	return ok
}

func (a *PreferencesStoreAdapter) logicalKey(rawKey, prefix string) (string, bool, error) {
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return "", false, nil
	}
	if prefix != "" {
		if !strings.HasPrefix(rawKey, prefix) {
			return "", false, nil
		}
		rawKey = strings.TrimPrefix(rawKey, prefix)
	}
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return "", false, ferrors.WrapSentinel(
			ErrPreferencesPathInvalid,
			"optionsadapter: resolved key path is empty",
			map[string]any{
				ferrors.MetaPath: rawKey,
			},
		)
	}
	return rawKey, true, nil
}

func preferencesStoreRequiredError(ref state.Ref, operation string) error {
	return ferrors.WrapSentinel(
		ErrPreferencesStoreRequired,
		"optionsadapter: preferences store is required",
		preferencesMeta(ref, operation),
	)
}

func scopeMetaWrap(err error, ref state.Ref, operation string) error {
	if !errors.Is(err, ErrPreferencesScopeMetadataInvalid) {
		return err
	}
	meta := preferencesMeta(ref, operation)
	if rich, ok := ferrors.As(err); ok {
		for key, value := range rich.Metadata {
			meta[key] = value
		}
	}
	return ferrors.WrapSentinel(
		ErrPreferencesScopeMetadataInvalid,
		"optionsadapter: invalid preferences scope metadata",
		meta,
	)
}

func pathMetaWrap(err error, ref state.Ref, operation string) error {
	if !errors.Is(err, ErrPreferencesPathInvalid) {
		return err
	}
	meta := preferencesMeta(ref, operation)
	if rich, ok := ferrors.As(err); ok {
		for key, value := range rich.Metadata {
			meta[key] = value
		}
	}
	return ferrors.WrapSentinel(
		ErrPreferencesPathInvalid,
		"optionsadapter: invalid preferences path",
		meta,
	)
}

func preferencesMeta(ref state.Ref, operation string) map[string]any {
	meta := map[string]any{
		ferrors.MetaAdapter:   "options_preferences",
		ferrors.MetaStore:     "preferences",
		ferrors.MetaOperation: operation,
		ferrors.MetaScope:     ref.Scope,
	}
	if domain := strings.TrimSpace(ref.Domain); domain != "" {
		meta[ferrors.MetaDomain] = domain
	}
	return meta
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
