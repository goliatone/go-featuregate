package resolver

import (
	"context"
	"sort"
	"strings"

	"github.com/goliatone/go-featuregate/activity"
	"github.com/goliatone/go-featuregate/cache"
	"github.com/goliatone/go-featuregate/ferrors"
	"github.com/goliatone/go-featuregate/gate"
	"github.com/goliatone/go-featuregate/scope"
	"github.com/goliatone/go-featuregate/store"
)

// ErrInvalidKey signals an empty or invalid feature key.
var ErrInvalidKey = ferrors.ErrInvalidKey

// ErrStoreUnavailable signals a missing runtime override store.
var ErrStoreUnavailable = ferrors.ErrStoreUnavailable

// DefaultResult captures a config default lookup.
type DefaultResult struct {
	Set   bool
	Value bool
}

// Defaults resolves config defaults for a feature key.
type Defaults interface {
	Default(ctx context.Context, key string) (DefaultResult, error)
}

// NoopDefaults returns no defaults.
type NoopDefaults struct{}

// Default implements Defaults.
func (NoopDefaults) Default(context.Context, string) (DefaultResult, error) {
	return DefaultResult{}, nil
}

// Gate resolves feature values using overrides, defaults, and fallbacks.
type Gate struct {
	defaults                 Defaults
	overrides                store.Reader
	writer                   store.Writer
	claimsProvider           gate.ClaimsProvider
	permissionProvider       gate.PermissionProvider
	cache                    cache.Cache
	hooks                    []gate.ResolveHook
	updateHooks              []activity.Hook
	strictStore              bool
	scopeOrder               []gate.ScopeKind
	strategy                 ResolveStrategy
	failureMode              ClaimsFailureMode
	failureFallbackChain      gate.ScopeChain
	appendSystemOnFailure     bool
	appendSystemOnProvidedChain bool
	preserveRolePermOrder     bool
	rolePermNormalizer        IdentifierNormalizer
}

// Option customizes a Gate.
type Option func(*Gate)

// ClaimsFailureMode controls behavior when claims resolution fails.
type ClaimsFailureMode string

const (
	FailOpen   ClaimsFailureMode = "fail_open"
	FailClosed ClaimsFailureMode = "fail_closed"
)

// IdentifierNormalizer normalizes role/perm identifiers.
type IdentifierNormalizer func(string) string

// ResolveOptions are passed to the strategy for context.
type ResolveOptions struct {
	ScopeOrder []gate.ScopeKind
}

// OverrideDecision captures a strategy decision.
type OverrideDecision struct {
	Matched  bool
	Value    bool
	Match    gate.ScopeRef
	Matches  []store.OverrideMatch
	Strategy string
}

// ResolveStrategy evaluates matches for a chain.
type ResolveStrategy func(ctx context.Context, key string, chain gate.ScopeChain, matches []store.OverrideMatch, opts ResolveOptions) (OverrideDecision, gate.ResolveTrace, error)

// WithDefaults sets the default resolver.
func WithDefaults(defaults Defaults) Option {
	return func(g *Gate) {
		if g == nil {
			return
		}
		g.defaults = defaults
	}
}

// WithOverrideStore sets the runtime override reader.
func WithOverrideStore(reader store.Reader) Option {
	return func(g *Gate) {
		if g == nil {
			return
		}
		g.overrides = reader
		if writer, ok := reader.(store.Writer); ok {
			g.writer = writer
		}
	}
}

// WithOverrideWriter sets the runtime override writer.
func WithOverrideWriter(writer store.Writer) Option {
	return func(g *Gate) {
		if g == nil {
			return
		}
		g.writer = writer
	}
}

// WithClaimsProvider overrides claims derivation.
func WithClaimsProvider(provider gate.ClaimsProvider) Option {
	return func(g *Gate) {
		if g == nil {
			return
		}
		g.claimsProvider = provider
	}
}

// WithPermissionProvider sets a permission provider.
func WithPermissionProvider(provider gate.PermissionProvider) Option {
	return func(g *Gate) {
		if g == nil {
			return
		}
		g.permissionProvider = provider
	}
}

// WithScopeOrder sets the chain construction order.
func WithScopeOrder(order ...gate.ScopeKind) Option {
	return func(g *Gate) {
		if g == nil {
			return
		}
		g.scopeOrder = append([]gate.ScopeKind(nil), order...)
	}
}

// WithResolveStrategy overrides the default strategy.
func WithResolveStrategy(strategy ResolveStrategy) Option {
	return func(g *Gate) {
		if g == nil {
			return
		}
		g.strategy = strategy
	}
}

// WithClaimsFailureMode sets claims failure behavior.
func WithClaimsFailureMode(mode ClaimsFailureMode) Option {
	return func(g *Gate) {
		if g == nil {
			return
		}
		g.failureMode = mode
	}
}

// WithFailureFallbackChain sets the fallback chain used on claims failure.
func WithFailureFallbackChain(chain gate.ScopeChain) Option {
	return func(g *Gate) {
		if g == nil {
			return
		}
		g.failureFallbackChain = append(gate.ScopeChain(nil), chain...)
	}
}

// WithAppendSystemOnFailure toggles appending system scope on claims failure.
func WithAppendSystemOnFailure(enabled bool) Option {
	return func(g *Gate) {
		if g == nil {
			return
		}
		g.appendSystemOnFailure = enabled
	}
}

// WithAppendSystemOnProvidedChain controls appending system to explicit chains.
func WithAppendSystemOnProvidedChain(enabled bool) Option {
	return func(g *Gate) {
		if g == nil {
			return
		}
		g.appendSystemOnProvidedChain = enabled
	}
}

// WithPreserveRolePermOrder preserves provider order for roles/perms.
func WithPreserveRolePermOrder(enabled bool) Option {
	return func(g *Gate) {
		if g == nil {
			return
		}
		g.preserveRolePermOrder = enabled
	}
}

// WithRolePermNormalizer overrides role/perm normalization.
func WithRolePermNormalizer(normalizer IdentifierNormalizer) Option {
	return func(g *Gate) {
		if g == nil {
			return
		}
		g.rolePermNormalizer = normalizer
	}
}

// WithCache sets the cache implementation.
func WithCache(c cache.Cache) Option {
	return func(g *Gate) {
		if g == nil {
			return
		}
		g.cache = c
	}
}

// WithResolveHook registers a resolve hook.
func WithResolveHook(hook gate.ResolveHook) Option {
	return func(g *Gate) {
		if g == nil || hook == nil {
			return
		}
		g.hooks = append(g.hooks, hook)
	}
}

// WithActivityHook registers an update hook.
func WithActivityHook(hook activity.Hook) Option {
	return func(g *Gate) {
		if g == nil || hook == nil {
			return
		}
		g.updateHooks = append(g.updateHooks, hook)
	}
}

// WithStrictStore toggles strict override resolution (fail closed on store errors).
func WithStrictStore(strict bool) Option {
	return func(g *Gate) {
		if g == nil {
			return
		}
		g.strictStore = strict
	}
}

// New constructs a Gate with the provided options.
func New(options ...Option) *Gate {
	g := &Gate{
		defaults:             NoopDefaults{},
		cache:                cache.NoopCache{},
		scopeOrder:           defaultScopeOrder(),
		strategy:             defaultResolveStrategy,
		failureMode:          FailOpen,
		appendSystemOnFailure: true,
		appendSystemOnProvidedChain: false,
		rolePermNormalizer:   defaultRolePermNormalizer,
	}
	for _, opt := range options {
		if opt != nil {
			opt(g)
		}
	}
	if g.defaults == nil {
		g.defaults = NoopDefaults{}
	}
	if g.cache == nil {
		g.cache = cache.NoopCache{}
	}
	if g.claimsProvider == nil {
		g.claimsProvider = contextClaimsProvider{}
	}
	if g.strategy == nil {
		g.strategy = defaultResolveStrategy
	}
	if g.scopeOrder == nil {
		g.scopeOrder = defaultScopeOrder()
	}
	if g.rolePermNormalizer == nil {
		g.rolePermNormalizer = defaultRolePermNormalizer
	}
	return g
}

// Enabled resolves a feature value without returning trace data.
func (g *Gate) Enabled(ctx context.Context, key string, opts ...gate.ResolveOption) (bool, error) {
	value, _, err := g.resolve(ctx, key, opts...)
	return value, err
}

// ResolveWithTrace resolves a feature value and returns trace data.
func (g *Gate) ResolveWithTrace(ctx context.Context, key string, opts ...gate.ResolveOption) (bool, gate.ResolveTrace, error) {
	value, trace, err := g.resolve(ctx, key, opts...)
	return value, trace, err
}

// Set stores a runtime override.
func (g *Gate) Set(ctx context.Context, key string, scopeRef gate.ScopeRef, enabled bool, actor gate.ActorRef) error {
	trimmed := strings.TrimSpace(key)
	normalized := gate.NormalizeKey(trimmed)
	scopeRef = g.normalizeScopeRef(scopeRef)
	if g.writer == nil {
		return ferrors.WrapSentinel(ferrors.ErrStoreUnavailable, "", map[string]any{
			ferrors.MetaFeatureKey:           trimmed,
			ferrors.MetaFeatureKeyNormalized: normalized,
			ferrors.MetaScope:                scopeRef,
			ferrors.MetaStore:                "override",
			ferrors.MetaOperation:            "set",
		})
	}
	if normalized == "" {
		return ferrors.WrapSentinel(ferrors.ErrInvalidKey, "", map[string]any{
			ferrors.MetaFeatureKey:           trimmed,
			ferrors.MetaFeatureKeyNormalized: normalized,
			ferrors.MetaScope:                scopeRef,
			ferrors.MetaOperation:            "set",
		})
	}
	if err := g.writer.Set(ctx, normalized, scopeRef, enabled, actor); err != nil {
		return ferrors.WrapExternal(err, ferrors.TextCodeStoreWriteFailed, "override store set failed", map[string]any{
			ferrors.MetaFeatureKey:           trimmed,
			ferrors.MetaFeatureKeyNormalized: normalized,
			ferrors.MetaScope:                scopeRef,
			ferrors.MetaStore:                "override",
			ferrors.MetaOperation:            "set",
		})
	}
	if g.cache != nil {
		g.invalidateCache(ctx, normalized, scopeRef)
	}
	g.emitUpdate(ctx, activity.UpdateEvent{
		Key:           strings.TrimSpace(key),
		NormalizedKey: normalized,
		Scope:         scopeRef,
		Actor:         actor,
		Action:        activity.ActionSet,
		Value:         boolPtr(enabled),
	})
	return nil
}

// Unset clears a runtime override.
func (g *Gate) Unset(ctx context.Context, key string, scopeRef gate.ScopeRef, actor gate.ActorRef) error {
	trimmed := strings.TrimSpace(key)
	normalized := gate.NormalizeKey(trimmed)
	scopeRef = g.normalizeScopeRef(scopeRef)
	if g.writer == nil {
		return ferrors.WrapSentinel(ferrors.ErrStoreUnavailable, "", map[string]any{
			ferrors.MetaFeatureKey:           trimmed,
			ferrors.MetaFeatureKeyNormalized: normalized,
			ferrors.MetaScope:                scopeRef,
			ferrors.MetaStore:                "override",
			ferrors.MetaOperation:            "unset",
		})
	}
	if normalized == "" {
		return ferrors.WrapSentinel(ferrors.ErrInvalidKey, "", map[string]any{
			ferrors.MetaFeatureKey:           trimmed,
			ferrors.MetaFeatureKeyNormalized: normalized,
			ferrors.MetaScope:                scopeRef,
			ferrors.MetaOperation:            "unset",
		})
	}
	if err := g.writer.Unset(ctx, normalized, scopeRef, actor); err != nil {
		return ferrors.WrapExternal(err, ferrors.TextCodeStoreWriteFailed, "override store unset failed", map[string]any{
			ferrors.MetaFeatureKey:           trimmed,
			ferrors.MetaFeatureKeyNormalized: normalized,
			ferrors.MetaScope:                scopeRef,
			ferrors.MetaStore:                "override",
			ferrors.MetaOperation:            "unset",
		})
	}
	aliasErr := g.unsetAliases(ctx, normalized, scopeRef, actor)
	if g.cache != nil {
		g.invalidateCache(ctx, normalized, scopeRef)
	}
	g.emitUpdate(ctx, activity.UpdateEvent{
		Key:           strings.TrimSpace(key),
		NormalizedKey: normalized,
		Scope:         scopeRef,
		Actor:         actor,
		Action:        activity.ActionUnset,
		Value:         nil,
	})
	if aliasErr != nil {
		return aliasErr
	}
	return nil
}

func (g *Gate) resolve(ctx context.Context, key string, opts ...gate.ResolveOption) (bool, gate.ResolveTrace, error) {
	trimmed := strings.TrimSpace(key)
	normalized := gate.NormalizeKey(trimmed)
	trace := gate.ResolveTrace{
		Key:           trimmed,
		NormalizedKey: normalized,
	}
	if normalized == "" {
		err := ferrors.WrapSentinel(ferrors.ErrInvalidKey, "", map[string]any{
			ferrors.MetaFeatureKey:           trimmed,
			ferrors.MetaFeatureKeyNormalized: normalized,
			ferrors.MetaOperation:            "resolve",
		})
		trace.Source = gate.ResolveSourceFallback
		g.emitResolve(ctx, trace, err)
		return false, trace, err
	}

	chain, failureMode, err := g.resolveChain(ctx, opts...)
	if err != nil {
		err = ferrors.WrapExternal(err, ferrors.TextCodeScopeResolveFailed, "claims resolution failed", map[string]any{
			ferrors.MetaFeatureKey:           trimmed,
			ferrors.MetaFeatureKeyNormalized: normalized,
			ferrors.MetaOperation:            "resolve_claims",
		})
		trace.Chain = chain
		trace.Source = gate.ResolveSourceFallback
		trace.ClaimsFailureMode = string(failureMode)
		g.emitResolve(ctx, trace, err)
		return false, trace, err
	}
	trace.Chain = chain
	trace.ClaimsFailureMode = string(failureMode)

	if g.cache != nil {
		if entry, ok := g.cache.Get(ctx, normalized, chain); ok {
			cached := entry.Trace
			if cached.Key == "" {
				cached.Key = trimmed
			}
			if cached.NormalizedKey == "" {
				cached.NormalizedKey = normalized
			}
			cached.Chain = chain
			cached.Value = entry.Value
			cached.CacheHit = true
			g.emitResolve(ctx, cached, nil)
			return entry.Value, cached, nil
		}
	}

	var storeErr error
	var decision OverrideDecision
	var overrideTrace gate.ResolveTrace
	if g.overrides != nil {
		decision, overrideTrace, storeErr = g.resolveOverrides(ctx, normalized, chain)
		if storeErr != nil {
			storeErr = ferrors.WrapExternal(storeErr, ferrors.TextCodeStoreReadFailed, "override store read failed", map[string]any{
				ferrors.MetaFeatureKey:           trimmed,
				ferrors.MetaFeatureKeyNormalized: normalized,
				ferrors.MetaStore:                "override",
				ferrors.MetaOperation:            "get_all",
				ferrors.MetaStrict:               g.strictStore,
			})
			trace.Override.Error = storeErr
			if g.strictStore {
				trace.Override.State = gate.OverrideStateMissing
				trace.Source = gate.ResolveSourceFallback
				g.emitResolve(ctx, trace, storeErr)
				return false, trace, storeErr
			}
		} else {
			trace.Override = overrideTrace.Override
			trace.Strategy = overrideTrace.Strategy
			if decision.Matched {
				trace.Value = decision.Value
				trace.Source = gate.ResolveSourceOverride
				g.writeCache(ctx, normalized, chain, trace, storeErr)
				g.emitResolve(ctx, trace, nil)
				return decision.Value, trace, nil
			}
		}
	} else {
		trace.Override.State = gate.OverrideStateMissing
	}

	defaults := g.defaults
	if defaults == nil {
		defaults = NoopDefaults{}
	}
	def, err := defaults.Default(ctx, normalized)
	if err != nil {
		err = ferrors.WrapExternal(err, ferrors.TextCodeDefaultLookupFailed, "default lookup failed", map[string]any{
			ferrors.MetaFeatureKey:           trimmed,
			ferrors.MetaFeatureKeyNormalized: normalized,
			ferrors.MetaChain:                chain,
			ferrors.MetaOperation:            "default",
		})
		trace.Default.Error = err
		trace.Source = gate.ResolveSourceFallback
		g.emitResolve(ctx, trace, err)
		return false, trace, err
	}
	trace.Default.Set = def.Set
	trace.Default.Value = def.Value
	if def.Set {
		trace.Value = def.Value
		trace.Source = gate.ResolveSourceDefault
	} else {
		trace.Value = false
		trace.Source = gate.ResolveSourceFallback
	}

	g.writeCache(ctx, normalized, chain, trace, storeErr)
	g.emitResolve(ctx, trace, nil)
	return trace.Value, trace, nil
}

func (g *Gate) resolveChain(ctx context.Context, opts ...gate.ResolveOption) (gate.ScopeChain, ClaimsFailureMode, error) {
	req := gate.ResolveRequest{}
	for _, opt := range opts {
		if opt != nil {
			opt(&req)
		}
	}
	if req.ScopeChain != nil {
		chain := append(gate.ScopeChain(nil), *req.ScopeChain...)
		if g.appendSystemOnProvidedChain {
			chain = appendSystemIfMissing(chain)
		}
		return chain, g.failureMode, nil
	}
	claims, err := g.claimsProvider.ClaimsFromContext(ctx)
	if err != nil {
		if g.failureMode == FailClosed {
			return nil, g.failureMode, err
		}
		fallback := append(gate.ScopeChain(nil), g.failureFallbackChain...)
		if g.appendSystemOnFailure {
			fallback = appendSystemIfMissing(fallback)
		}
		return fallback, g.failureMode, nil
	}
	if g.permissionProvider != nil {
		perms, permErr := g.permissionProvider.Permissions(ctx, claims)
		if permErr != nil {
			if g.failureMode == FailClosed {
				return nil, g.failureMode, permErr
			}
			fallback := append(gate.ScopeChain(nil), g.failureFallbackChain...)
			if g.appendSystemOnFailure {
				fallback = appendSystemIfMissing(fallback)
			}
			return fallback, g.failureMode, nil
		}
		claims.Perms = mergePerms(claims.Perms, perms)
	}
	chain := g.buildChain(claims)
	return appendSystemIfMissing(chain), g.failureMode, nil
}

func (g *Gate) writeCache(ctx context.Context, key string, chain gate.ScopeChain, trace gate.ResolveTrace, storeErr error) {
	if g.cache == nil {
		return
	}
	if storeErr != nil {
		return
	}
	g.cache.Set(ctx, key, chain, cache.Entry{
		Value: trace.Value,
		Trace: trace,
	})
}

func (g *Gate) emitResolve(ctx context.Context, trace gate.ResolveTrace, err error) {
	if len(g.hooks) == 0 {
		return
	}
	event := gate.ResolveEvent{
		Key:           trace.Key,
		NormalizedKey: trace.NormalizedKey,
		Chain:         trace.Chain,
		Value:         trace.Value,
		Source:        trace.Source,
		Error:         err,
		Trace:         trace,
	}
	for _, hook := range g.hooks {
		if hook == nil {
			continue
		}
		hook.OnResolve(ctx, event)
	}
}

func (g *Gate) emitUpdate(ctx context.Context, event activity.UpdateEvent) {
	if len(g.updateHooks) == 0 {
		return
	}
	for _, hook := range g.updateHooks {
		if hook == nil {
			continue
		}
		hook.OnUpdate(ctx, event)
	}
}

func boolPtr(value bool) *bool {
	return &value
}

type contextClaimsProvider struct{}

func (contextClaimsProvider) ClaimsFromContext(ctx context.Context) (gate.ActorClaims, error) {
	return scope.ClaimsFromContext(ctx), nil
}

func defaultScopeOrder() []gate.ScopeKind {
	return []gate.ScopeKind{
		gate.ScopeUser,
		gate.ScopeRole,
		gate.ScopePerm,
		gate.ScopeOrg,
		gate.ScopeTenant,
		gate.ScopeSystem,
	}
}

func defaultRolePermNormalizer(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (g *Gate) normalizeScopeRef(ref gate.ScopeRef) gate.ScopeRef {
	ref.ID = strings.TrimSpace(ref.ID)
	ref.TenantID = strings.TrimSpace(ref.TenantID)
	ref.OrgID = strings.TrimSpace(ref.OrgID)
	if ref.Kind == gate.ScopeRole || ref.Kind == gate.ScopePerm {
		ref.ID = g.rolePermNormalizer(ref.ID)
	}
	return ref
}

func (g *Gate) buildChain(claims gate.ActorClaims) gate.ScopeChain {
	roles := normalizeList(claims.Roles, g.rolePermNormalizer)
	perms := normalizeList(claims.Perms, g.rolePermNormalizer)
	if !g.preserveRolePermOrder {
		roles = sortAndDedupe(roles)
		perms = sortAndDedupe(perms)
	} else {
		roles = dedupeStable(roles)
		perms = dedupeStable(perms)
	}
	chain := make(gate.ScopeChain, 0, len(roles)+len(perms)+4)
	for _, kind := range g.scopeOrder {
		switch kind {
		case gate.ScopeUser:
			if claims.SubjectID != "" {
				chain = append(chain, gate.ScopeRef{
					Kind:     gate.ScopeUser,
					ID:       claims.SubjectID,
					TenantID: claims.TenantID,
					OrgID:    claims.OrgID,
				})
			}
		case gate.ScopeRole:
			chain = append(chain, buildRolePermRefs(gate.ScopeRole, roles, claims)...)
		case gate.ScopePerm:
			chain = append(chain, buildRolePermRefs(gate.ScopePerm, perms, claims)...)
		case gate.ScopeOrg:
			if claims.OrgID != "" {
				chain = append(chain, gate.ScopeRef{
					Kind:     gate.ScopeOrg,
					ID:       claims.OrgID,
					TenantID: claims.TenantID,
					OrgID:    claims.OrgID,
				})
			}
		case gate.ScopeTenant:
			if claims.TenantID != "" {
				chain = append(chain, gate.ScopeRef{
					Kind:     gate.ScopeTenant,
					ID:       claims.TenantID,
					TenantID: claims.TenantID,
				})
			}
		case gate.ScopeSystem:
			chain = append(chain, gate.ScopeRef{Kind: gate.ScopeSystem})
		}
	}
	return chain
}

func buildRolePermRefs(kind gate.ScopeKind, items []string, claims gate.ActorClaims) gate.ScopeChain {
	if len(items) == 0 {
		return nil
	}
	refs := make(gate.ScopeChain, 0, len(items)*2)
	for _, id := range items {
		if id == "" {
			continue
		}
		refs = append(refs, gate.ScopeRef{
			Kind: kind,
			ID:   id,
		})
		if claims.TenantID != "" || claims.OrgID != "" {
			refs = append(refs, gate.ScopeRef{
				Kind:     kind,
				ID:       id,
				TenantID: claims.TenantID,
				OrgID:    claims.OrgID,
			})
		}
	}
	return refs
}

func appendSystemIfMissing(chain gate.ScopeChain) gate.ScopeChain {
	for _, ref := range chain {
		if ref.Kind == gate.ScopeSystem {
			return chain
		}
	}
	return append(chain, gate.ScopeRef{Kind: gate.ScopeSystem})
}

func mergePerms(existing, extra []string) []string {
	if len(extra) == 0 {
		return existing
	}
	out := make([]string, 0, len(existing)+len(extra))
	out = append(out, existing...)
	out = append(out, extra...)
	return out
}

func normalizeList(values []string, normalizer IdentifierNormalizer) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if normalizer != nil {
			trimmed = normalizer(trimmed)
		}
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func sortAndDedupe(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	uniq := map[string]struct{}{}
	for _, value := range values {
		uniq[value] = struct{}{}
	}
	out := make([]string, 0, len(uniq))
	for value := range uniq {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func dedupeStable(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func (g *Gate) resolveOverrides(ctx context.Context, key string, chain gate.ScopeChain) (OverrideDecision, gate.ResolveTrace, error) {
	var trace gate.ResolveTrace
	trace.Strategy = "default"
	matches, err := g.overrides.GetAll(ctx, key, chain)
	if err != nil {
		return OverrideDecision{}, trace, err
	}
	matches = normalizeMatches(matches)
	if decision, trace, err := g.applyStrategy(ctx, key, chain, matches); err != nil {
		return OverrideDecision{}, trace, err
	} else if decision.Matched {
		return decision, trace, nil
	}
	aliases := gate.AliasesFor(key)
	for _, alias := range aliases {
		aliasMatches, aliasErr := g.overrides.GetAll(ctx, alias, chain)
		if aliasErr != nil {
			return OverrideDecision{}, trace, aliasErr
		}
		aliasMatches = normalizeMatches(aliasMatches)
		if decision, aliasTrace, err := g.applyStrategy(ctx, alias, chain, aliasMatches); err != nil {
			return OverrideDecision{}, aliasTrace, err
		} else if decision.Matched {
			return decision, aliasTrace, nil
		}
	}
	return OverrideDecision{}, trace, nil
}

func (g *Gate) applyStrategy(ctx context.Context, key string, chain gate.ScopeChain, matches []store.OverrideMatch) (OverrideDecision, gate.ResolveTrace, error) {
	if g.strategy == nil {
		g.strategy = defaultResolveStrategy
	}
	decision, trace, err := g.strategy(ctx, key, chain, matches, ResolveOptions{
		ScopeOrder: g.scopeOrder,
	})
	if err != nil {
		trace.Override.Error = err
		return decision, trace, err
	}
	return decision, trace, nil
}

func defaultResolveStrategy(ctx context.Context, key string, chain gate.ScopeChain, matches []store.OverrideMatch, opts ResolveOptions) (OverrideDecision, gate.ResolveTrace, error) {
	_ = ctx
	_ = key
	trace := gate.ResolveTrace{
		Strategy: "default",
	}
	if len(matches) == 0 {
		trace.Override.State = gate.OverrideStateMissing
		return OverrideDecision{Matched: false, Strategy: "default"}, trace, nil
	}
	matchMap := map[string]store.OverrideMatch{}
	for _, match := range matches {
		matchMap[scopeKey(match.Scope)] = match
	}
	groupOrder := groupOrderFor(opts.ScopeOrder)
	for _, group := range groupOrder {
		groupMatches := collectGroupMatches(group, chain, matchMap)
		if len(groupMatches) == 0 {
			continue
		}
		decision, groupTrace := evaluateGroup(group, groupMatches)
		if !decision.Matched {
			continue
		}
		trace.Override = groupTrace
		return decision, trace, nil
	}
	trace.Override.State = gate.OverrideStateMissing
	return OverrideDecision{Matched: false, Strategy: "default"}, trace, nil
}

func groupOrderFor(scopeOrder []gate.ScopeKind) []groupKind {
	order := make([]groupKind, 0, 5)
	for _, kind := range scopeOrder {
		switch kind {
		case gate.ScopeUser:
			order = append(order, groupUser)
		case gate.ScopeRole, gate.ScopePerm:
			if !containsGroup(order, groupRolePerm) {
				order = append(order, groupRolePerm)
			}
		case gate.ScopeOrg:
			order = append(order, groupOrg)
		case gate.ScopeTenant:
			order = append(order, groupTenant)
		case gate.ScopeSystem:
			order = append(order, groupSystem)
		}
	}
	if len(order) == 0 {
		return []groupKind{groupUser, groupRolePerm, groupOrg, groupTenant, groupSystem}
	}
	return order
}

type groupKind string

const (
	groupUser    groupKind = "user"
	groupRolePerm groupKind = "role_perm"
	groupOrg     groupKind = "org"
	groupTenant  groupKind = "tenant"
	groupSystem  groupKind = "system"
)

func containsGroup(groups []groupKind, target groupKind) bool {
	for _, group := range groups {
		if group == target {
			return true
		}
	}
	return false
}

func collectGroupMatches(group groupKind, chain gate.ScopeChain, matchMap map[string]store.OverrideMatch) []store.OverrideMatch {
	out := make([]store.OverrideMatch, 0)
	for _, ref := range chain {
		if !scopeKindInGroup(ref.Kind, group) {
			continue
		}
		if match, ok := matchMap[scopeKey(ref)]; ok {
			out = append(out, match)
		}
	}
	return out
}

func scopeKindInGroup(kind gate.ScopeKind, group groupKind) bool {
	switch group {
	case groupUser:
		return kind == gate.ScopeUser
	case groupRolePerm:
		return kind == gate.ScopeRole || kind == gate.ScopePerm
	case groupOrg:
		return kind == gate.ScopeOrg
	case groupTenant:
		return kind == gate.ScopeTenant
	case groupSystem:
		return kind == gate.ScopeSystem
	default:
		return false
	}
}

func evaluateGroup(group groupKind, matches []store.OverrideMatch) (OverrideDecision, gate.OverrideTrace) {
	trace := gate.OverrideTrace{
		State:   gate.OverrideStateMissing,
		Matches: toMatchTraces(matches),
	}
	switch group {
	case groupRolePerm:
		for _, match := range matches {
			if match.Override.State == gate.OverrideStateDisabled {
				trace.State = gate.OverrideStateDisabled
				trace.Value = boolPtr(false)
				trace.Match = match.Scope
				return OverrideDecision{
					Matched:  true,
					Value:    false,
					Match:    match.Scope,
					Matches:  matches,
					Strategy: "default",
				}, trace
			}
		}
		for _, match := range matches {
			if match.Override.State == gate.OverrideStateEnabled {
				trace.State = gate.OverrideStateEnabled
				trace.Value = boolPtr(true)
				trace.Match = match.Scope
				return OverrideDecision{
					Matched:  true,
					Value:    true,
					Match:    match.Scope,
					Matches:  matches,
					Strategy: "default",
				}, trace
			}
		}
	default:
		for _, match := range matches {
			if match.Override.State == gate.OverrideStateEnabled {
				trace.State = gate.OverrideStateEnabled
				trace.Value = boolPtr(true)
				trace.Match = match.Scope
				return OverrideDecision{
					Matched:  true,
					Value:    true,
					Match:    match.Scope,
					Matches:  matches,
					Strategy: "default",
				}, trace
			}
			if match.Override.State == gate.OverrideStateDisabled {
				trace.State = gate.OverrideStateDisabled
				trace.Value = boolPtr(false)
				trace.Match = match.Scope
				return OverrideDecision{
					Matched:  true,
					Value:    false,
					Match:    match.Scope,
					Matches:  matches,
					Strategy: "default",
				}, trace
			}
		}
	}
	return OverrideDecision{Matched: false, Strategy: "default"}, trace
}

func toMatchTraces(matches []store.OverrideMatch) []gate.OverrideMatchTrace {
	if len(matches) == 0 {
		return nil
	}
	out := make([]gate.OverrideMatchTrace, 0, len(matches))
	for _, match := range matches {
		out = append(out, gate.OverrideMatchTrace{
			Scope: match.Scope,
			State: match.Override.State,
			Value: valueFromOverride(match.Override),
		})
	}
	return out
}

func normalizeMatches(matches []store.OverrideMatch) []store.OverrideMatch {
	for i := range matches {
		if matches[i].Override.State == "" {
			matches[i].Override.State = gate.OverrideStateMissing
		}
	}
	return matches
}

func valueFromOverride(override store.Override) *bool {
	if override.State == gate.OverrideStateEnabled {
		return boolPtr(true)
	}
	if override.State == gate.OverrideStateDisabled {
		return boolPtr(false)
	}
	return nil
}

func scopeKey(ref gate.ScopeRef) string {
	return strings.Join([]string{
		scopeKindString(ref.Kind),
		ref.TenantID,
		ref.OrgID,
		ref.ID,
	}, "|")
}

func scopeKindString(kind gate.ScopeKind) string {
	switch kind {
	case gate.ScopeSystem:
		return "system"
	case gate.ScopeTenant:
		return "tenant"
	case gate.ScopeOrg:
		return "org"
	case gate.ScopeUser:
		return "user"
	case gate.ScopeRole:
		return "role"
	case gate.ScopePerm:
		return "perm"
	default:
		return "unknown"
	}
}

func (g *Gate) invalidateCache(ctx context.Context, key string, scopeRef gate.ScopeRef) {
	if g.cache == nil {
		return
	}
	if scopeRef.Kind == gate.ScopeRole || scopeRef.Kind == gate.ScopePerm {
		g.cache.Clear(ctx)
		return
	}
	g.cache.Clear(ctx)
}

func (g *Gate) unsetAliases(ctx context.Context, normalized string, scopeRef gate.ScopeRef, actor gate.ActorRef) error {
	if g == nil || g.writer == nil {
		return nil
	}
	aliases := gate.AliasesFor(normalized)
	if len(aliases) == 0 {
		return nil
	}
	for _, alias := range aliases {
		if err := g.writer.Unset(ctx, alias, scopeRef, actor); err != nil {
			return err
		}
	}
	return nil
}

