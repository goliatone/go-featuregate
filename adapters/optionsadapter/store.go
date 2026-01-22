package optionsadapter

import (
	"context"
	"fmt"
	"strings"

	opts "github.com/goliatone/go-options"
	"github.com/goliatone/go-options/pkg/state"

	"github.com/goliatone/go-featuregate/ferrors"
	"github.com/goliatone/go-featuregate/gate"
	"github.com/goliatone/go-featuregate/scope"
	"github.com/goliatone/go-featuregate/store"
)

const (
	prioritySystem = 10
	priorityTenant = 20
	priorityOrg    = 30
	priorityUser   = 40
)

// DefaultDomain is the default options domain used for feature overrides.
const DefaultDomain = "feature_flags"

// ErrStoreRequired indicates the underlying state store is missing.
var ErrStoreRequired = ferrors.ErrStoreRequired

// ErrInvalidKey indicates a missing or invalid feature key.
var ErrInvalidKey = ferrors.ErrInvalidKey

// ScopeBuilder maps a ScopeSet into go-options scopes ordered by precedence.
type ScopeBuilder func(scopeSet gate.ScopeSet) []opts.Scope

// MetaBuilder builds storage metadata from an actor reference.
type MetaBuilder func(actor gate.ActorRef) state.Meta

// Option customizes the Store adapter.
type Option func(*Store)

// Store adapts a go-options state.Store into a featuregate override store.
type Store struct {
	stateStore state.Store[map[string]any]
	domain     string
	scopes     ScopeBuilder
	meta       MetaBuilder
}

// NewStore constructs an adapter backed by a go-options state.Store.
func NewStore(stateStore state.Store[map[string]any], opts ...Option) *Store {
	adapter := &Store{
		stateStore: stateStore,
		domain:     DefaultDomain,
		scopes:     defaultScopes,
		meta:       defaultMeta,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(adapter)
		}
	}
	if adapter.domain == "" {
		adapter.domain = DefaultDomain
	}
	if adapter.scopes == nil {
		adapter.scopes = defaultScopes
	}
	if adapter.meta == nil {
		adapter.meta = defaultMeta
	}
	return adapter
}

// WithDomain sets the options domain used for feature overrides.
func WithDomain(domain string) Option {
	return func(adapter *Store) {
		if adapter == nil {
			return
		}
		adapter.domain = strings.TrimSpace(domain)
	}
}

// WithScopeBuilder overrides the default scope mapping.
func WithScopeBuilder(builder ScopeBuilder) Option {
	return func(adapter *Store) {
		if adapter == nil {
			return
		}
		adapter.scopes = builder
	}
}

// WithMetaBuilder overrides the metadata builder used on mutations.
func WithMetaBuilder(builder MetaBuilder) Option {
	return func(adapter *Store) {
		if adapter == nil {
			return
		}
		adapter.meta = builder
	}
}

// Get implements store.Reader.
func (s *Store) Get(ctx context.Context, key string, scopeSet gate.ScopeSet) (store.Override, error) {
	if s == nil || s.stateStore == nil {
		domain := ""
		if s != nil {
			domain = s.domain
		}
		return store.MissingOverride(), storeRequiredError(key, scopeSet, "get", domain)
	}
	trimmed := strings.TrimSpace(key)
	normalized := gate.NormalizeKey(trimmed)
	if normalized == "" {
		return store.MissingOverride(), invalidKeyError(trimmed, normalized, scopeSet, "get", s.domain)
	}

	scopes := s.scopes(scopeSet)
	if len(scopes) == 0 {
		return store.MissingOverride(), nil
	}

	for _, scopeDef := range scopes {
		snapshot, _, ok, err := s.stateStore.Load(ctx, state.Ref{Domain: s.domain, Scope: scopeDef})
		if err != nil {
			meta := storeMeta(scopeDef, "load", s.domain)
			meta[ferrors.MetaFeatureKey] = trimmed
			meta[ferrors.MetaFeatureKeyNormalized] = normalized
			return store.MissingOverride(), ferrors.WrapExternal(err, ferrors.TextCodeStoreReadFailed, "optionsadapter: load failed", meta)
		}
		if !ok || len(snapshot) == 0 {
			continue
		}
		if value, found := lookupPath(snapshot, normalized); found {
			return overrideFromValue(normalized, value, scopeDef, s.domain)
		}
	}

	return store.MissingOverride(), nil
}

// Set implements store.Writer.
func (s *Store) Set(ctx context.Context, key string, scopeSet gate.ScopeSet, enabled bool, actor gate.ActorRef) error {
	if s == nil || s.stateStore == nil {
		domain := ""
		if s != nil {
			domain = s.domain
		}
		return storeRequiredError(key, scopeSet, "set", domain)
	}
	trimmed := strings.TrimSpace(key)
	normalized := gate.NormalizeKey(trimmed)
	if normalized == "" {
		return invalidKeyError(trimmed, normalized, scopeSet, "set", s.domain)
	}

	ref, err := s.writeRef(scopeSet)
	if err != nil {
		return err
	}

	resolver := state.Resolver[map[string]any]{Store: s.stateStore}
	_, _, err = resolver.Mutate(ctx, ref, s.meta(actor), func(snapshot *map[string]any) error {
		if snapshot == nil {
			return ferrors.WrapSentinel(ferrors.ErrSnapshotRequired, "optionsadapter: snapshot is nil", storeMeta(ref.Scope, "set", s.domain))
		}
		if *snapshot == nil {
			*snapshot = map[string]any{}
		}
		return setPath(*snapshot, normalized, enabled)
	})
	if err != nil {
		meta := storeMeta(ref.Scope, "set", s.domain)
		meta[ferrors.MetaFeatureKey] = trimmed
		meta[ferrors.MetaFeatureKeyNormalized] = normalized
		return ferrors.WrapExternal(err, ferrors.TextCodeStoreWriteFailed, "optionsadapter: set failed", meta)
	}
	return nil
}

// Unset implements store.Writer.
func (s *Store) Unset(ctx context.Context, key string, scopeSet gate.ScopeSet, actor gate.ActorRef) error {
	if s == nil || s.stateStore == nil {
		domain := ""
		if s != nil {
			domain = s.domain
		}
		return storeRequiredError(key, scopeSet, "unset", domain)
	}
	trimmed := strings.TrimSpace(key)
	normalized := gate.NormalizeKey(trimmed)
	if normalized == "" {
		return invalidKeyError(trimmed, normalized, scopeSet, "unset", s.domain)
	}

	ref, err := s.writeRef(scopeSet)
	if err != nil {
		return err
	}

	resolver := state.Resolver[map[string]any]{Store: s.stateStore}
	_, _, err = resolver.Mutate(ctx, ref, s.meta(actor), func(snapshot *map[string]any) error {
		if snapshot == nil {
			return ferrors.WrapSentinel(ferrors.ErrSnapshotRequired, "optionsadapter: snapshot is nil", storeMeta(ref.Scope, "unset", s.domain))
		}
		if *snapshot == nil {
			*snapshot = map[string]any{}
		}
		deletePath(*snapshot, normalized)
		return nil
	})
	if err != nil {
		meta := storeMeta(ref.Scope, "unset", s.domain)
		meta[ferrors.MetaFeatureKey] = trimmed
		meta[ferrors.MetaFeatureKeyNormalized] = normalized
		return ferrors.WrapExternal(err, ferrors.TextCodeStoreWriteFailed, "optionsadapter: unset failed", meta)
	}
	return nil
}

func (s *Store) writeRef(scopeSet gate.ScopeSet) (state.Ref, error) {
	scopeDef := writeScope(scopeSet)
	if scopeDef.Name == "" {
		return state.Ref{}, ferrors.WrapSentinel(ferrors.ErrScopeRequired, "optionsadapter: scope is required", storeMeta(scopeDef, "write_ref", s.domain))
	}
	return state.Ref{Domain: s.domain, Scope: scopeDef}, nil
}

func defaultScopes(scopeSet gate.ScopeSet) []opts.Scope {
	if scopeSet.System {
		return []opts.Scope{scoped("system", "System", prioritySystem, "", "")}
	}
	var scopes []opts.Scope
	if scopeSet.UserID != "" {
		scopes = append(scopes, scoped("user", "User", priorityUser, scope.MetadataUserID, scopeSet.UserID))
	}
	if scopeSet.OrgID != "" {
		scopes = append(scopes, scoped("org", "Org", priorityOrg, scope.MetadataOrgID, scopeSet.OrgID))
	}
	if scopeSet.TenantID != "" {
		scopes = append(scopes, scoped("tenant", "Tenant", priorityTenant, scope.MetadataTenantID, scopeSet.TenantID))
	}
	scopes = append(scopes, scoped("system", "System", prioritySystem, "", ""))
	return scopes
}

func writeScope(scopeSet gate.ScopeSet) opts.Scope {
	switch {
	case scopeSet.System:
		return scoped("system", "System", prioritySystem, "", "")
	case scopeSet.UserID != "":
		return scoped("user", "User", priorityUser, scope.MetadataUserID, scopeSet.UserID)
	case scopeSet.OrgID != "":
		return scoped("org", "Org", priorityOrg, scope.MetadataOrgID, scopeSet.OrgID)
	case scopeSet.TenantID != "":
		return scoped("tenant", "Tenant", priorityTenant, scope.MetadataTenantID, scopeSet.TenantID)
	default:
		return scoped("system", "System", prioritySystem, "", "")
	}
}

func scoped(name, label string, priority int, metadataKey, metadataValue string) opts.Scope {
	var metadata map[string]any
	if metadataKey != "" && metadataValue != "" {
		metadata = map[string]any{metadataKey: metadataValue}
	}
	return opts.NewScope(
		name,
		priority,
		opts.WithScopeLabel(label),
		opts.WithScopeMetadata(metadata),
	)
}

func defaultMeta(actor gate.ActorRef) state.Meta {
	extra := map[string]string{}
	if actor.ID != "" {
		extra["actor_id"] = actor.ID
	}
	if actor.Type != "" {
		extra["actor_type"] = actor.Type
	}
	if actor.Name != "" {
		extra["actor_name"] = actor.Name
	}
	if len(extra) == 0 {
		return state.Meta{}
	}
	return state.Meta{Extra: extra}
}

func overrideFromValue(key string, value any, scopeDef opts.Scope, domain string) (store.Override, error) {
	switch typed := value.(type) {
	case nil:
		return store.UnsetOverride(), nil
	case bool:
		if typed {
			return store.EnabledOverride(), nil
		}
		return store.DisabledOverride(), nil
	case *bool:
		if typed == nil {
			return store.UnsetOverride(), nil
		}
		if *typed {
			return store.EnabledOverride(), nil
		}
		return store.DisabledOverride(), nil
	default:
		meta := storeMeta(scopeDef, "decode", domain)
		meta[ferrors.MetaFeatureKeyNormalized] = key
		return store.MissingOverride(), ferrors.NewExternal(ferrors.TextCodeOverrideTypeInvalid, fmt.Sprintf("optionsadapter: unsupported override type %T", value), meta)
	}
}

var _ store.ReadWriter = (*Store)(nil)

func storeRequiredError(key string, scopeSet gate.ScopeSet, operation, domain string) error {
	trimmed := strings.TrimSpace(key)
	normalized := gate.NormalizeKey(trimmed)
	return ferrors.WrapSentinel(ferrors.ErrStoreRequired, "optionsadapter: state store is required", map[string]any{
		ferrors.MetaAdapter:              "options",
		ferrors.MetaStore:                "state",
		ferrors.MetaDomain:               strings.TrimSpace(domain),
		ferrors.MetaScope:                scopeSet,
		ferrors.MetaOperation:            operation,
		ferrors.MetaFeatureKey:           trimmed,
		ferrors.MetaFeatureKeyNormalized: normalized,
	})
}

func invalidKeyError(key, normalized string, scopeSet gate.ScopeSet, operation, domain string) error {
	meta := map[string]any{
		ferrors.MetaAdapter:              "options",
		ferrors.MetaStore:                "state",
		ferrors.MetaDomain:               strings.TrimSpace(domain),
		ferrors.MetaScope:                scopeSet,
		ferrors.MetaOperation:            operation,
		ferrors.MetaFeatureKey:           strings.TrimSpace(key),
		ferrors.MetaFeatureKeyNormalized: normalized,
	}
	return ferrors.WrapSentinel(ferrors.ErrInvalidKey, "optionsadapter: feature key required", meta)
}

func storeMeta(scopeDef opts.Scope, operation, domain string) map[string]any {
	meta := map[string]any{
		ferrors.MetaAdapter:   "options",
		ferrors.MetaStore:     "state",
		ferrors.MetaOperation: operation,
		ferrors.MetaScope:     scopeDef,
	}
	if strings.TrimSpace(domain) != "" {
		meta[ferrors.MetaDomain] = strings.TrimSpace(domain)
	}
	return meta
}
