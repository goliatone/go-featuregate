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
	priorityRole   = 50
	priorityPerm   = 60
)

// DefaultDomain is the default options domain used for feature overrides.
const DefaultDomain = "feature_flags"

// ErrStoreRequired indicates the underlying state store is missing.
var ErrStoreRequired = ferrors.ErrStoreRequired

// ErrInvalidKey indicates a missing or invalid feature key.
var ErrInvalidKey = ferrors.ErrInvalidKey

// ScopeBuilder maps a ScopeRef into a go-options scope.
type ScopeBuilder func(ref gate.ScopeRef) opts.Scope

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
		scopes:     defaultScopeFromRef,
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
		adapter.scopes = defaultScopeFromRef
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

// GetAll implements store.Reader.
func (s *Store) GetAll(ctx context.Context, key string, chain gate.ScopeChain) ([]store.OverrideMatch, error) {
	if s == nil || s.stateStore == nil {
		domain := ""
		if s != nil {
			domain = s.domain
		}
		return nil, storeRequiredError(key, gate.ScopeRef{}, "get_all", domain)
	}
	trimmed := strings.TrimSpace(key)
	normalized := gate.NormalizeKey(trimmed)
	if normalized == "" {
		return nil, invalidKeyError(trimmed, normalized, gate.ScopeRef{}, "get_all", s.domain)
	}
	matches := make([]store.OverrideMatch, 0)
	for _, ref := range chain {
		scopeDef := s.scopes(ref)
		snapshot, _, ok, err := s.stateStore.Load(ctx, state.Ref{Domain: s.domain, Scope: scopeDef})
		if err != nil {
			meta := storeMeta(scopeDef, "load", s.domain)
			meta[ferrors.MetaFeatureKey] = trimmed
			meta[ferrors.MetaFeatureKeyNormalized] = normalized
			return nil, ferrors.WrapExternal(err, ferrors.TextCodeStoreReadFailed, "optionsadapter: load failed", meta)
		}
		if !ok || len(snapshot) == 0 {
			continue
		}
		if value, found := lookupPath(snapshot, normalized); found {
			override, err := overrideFromValue(normalized, value, scopeDef, s.domain)
			if err != nil {
				return nil, err
			}
			matches = append(matches, store.OverrideMatch{
				Scope:    ref,
				Override: override,
			})
		}
	}
	return matches, nil
}

// Set implements store.Writer.
func (s *Store) Set(ctx context.Context, key string, scopeRef gate.ScopeRef, enabled bool, actor gate.ActorRef) error {
	if s == nil || s.stateStore == nil {
		domain := ""
		if s != nil {
			domain = s.domain
		}
		return storeRequiredError(key, scopeRef, "set", domain)
	}
	trimmed := strings.TrimSpace(key)
	normalized := gate.NormalizeKey(trimmed)
	if normalized == "" {
		return invalidKeyError(trimmed, normalized, scopeRef, "set", s.domain)
	}

	ref, err := s.writeRef(scopeRef)
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
func (s *Store) Unset(ctx context.Context, key string, scopeRef gate.ScopeRef, actor gate.ActorRef) error {
	if s == nil || s.stateStore == nil {
		domain := ""
		if s != nil {
			domain = s.domain
		}
		return storeRequiredError(key, scopeRef, "unset", domain)
	}
	trimmed := strings.TrimSpace(key)
	normalized := gate.NormalizeKey(trimmed)
	if normalized == "" {
		return invalidKeyError(trimmed, normalized, scopeRef, "unset", s.domain)
	}

	ref, err := s.writeRef(scopeRef)
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

func (s *Store) writeRef(scopeRef gate.ScopeRef) (state.Ref, error) {
	scopeDef := s.scopes(scopeRef)
	if scopeDef.Name == "" {
		return state.Ref{}, ferrors.WrapSentinel(ferrors.ErrScopeRequired, "optionsadapter: scope is required", storeMeta(scopeDef, "write_ref", s.domain))
	}
	return state.Ref{Domain: s.domain, Scope: scopeDef}, nil
}

func defaultScopeFromRef(ref gate.ScopeRef) opts.Scope {
	switch ref.Kind {
	case gate.ScopeSystem:
		return scoped("system", "System", prioritySystem, map[string]any{})
	case gate.ScopeUser:
		return scoped(scopeName("user", ref.ID), "User", priorityUser, scopeMetadata(ref, scope.MetadataUserID))
	case gate.ScopeOrg:
		return scoped(scopeName("org", ref.ID), "Org", priorityOrg, scopeMetadata(ref, scope.MetadataOrgID))
	case gate.ScopeTenant:
		return scoped(scopeName("tenant", ref.ID), "Tenant", priorityTenant, scopeMetadata(ref, scope.MetadataTenantID))
	case gate.ScopeRole:
		return scoped(scopeName("role", ref.ID), "Role", priorityRole, scopeMetadata(ref, metadataRoleID))
	case gate.ScopePerm:
		return scoped(scopeName("perm", ref.ID), "Perm", priorityPerm, scopeMetadata(ref, metadataPermID))
	default:
		return scoped("system", "System", prioritySystem, map[string]any{})
	}
}

func scoped(name, label string, priority int, metadata map[string]any) opts.Scope {
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

const (
	metadataRoleID = "role_id"
	metadataPermID = "perm_id"
)

func scopeName(kind, id string) string {
	_ = id
	return kind
}

func scopeMetadata(ref gate.ScopeRef, idKey string) map[string]any {
	metadata := map[string]any{}
	if idKey != "" && ref.ID != "" {
		metadata[idKey] = ref.ID
	}
	if ref.TenantID != "" {
		metadata[scope.MetadataTenantID] = ref.TenantID
	}
	if ref.OrgID != "" {
		metadata[scope.MetadataOrgID] = ref.OrgID
	}
	return metadata
}

func storeRequiredError(key string, scopeRef gate.ScopeRef, operation, domain string) error {
	trimmed := strings.TrimSpace(key)
	normalized := gate.NormalizeKey(trimmed)
	return ferrors.WrapSentinel(ferrors.ErrStoreRequired, "optionsadapter: state store is required", map[string]any{
		ferrors.MetaAdapter:              "options",
		ferrors.MetaStore:                "state",
		ferrors.MetaDomain:               strings.TrimSpace(domain),
		ferrors.MetaScope:                scopeRef,
		ferrors.MetaOperation:            operation,
		ferrors.MetaFeatureKey:           trimmed,
		ferrors.MetaFeatureKeyNormalized: normalized,
	})
}

func invalidKeyError(key, normalized string, scopeRef gate.ScopeRef, operation, domain string) error {
	meta := map[string]any{
		ferrors.MetaAdapter:              "options",
		ferrors.MetaStore:                "state",
		ferrors.MetaDomain:               strings.TrimSpace(domain),
		ferrors.MetaScope:                scopeRef,
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
