package resolver

import (
	"context"
	"strings"

	"github.com/goliatone/go-featuregate/activity"
	"github.com/goliatone/go-featuregate/cache"
	"github.com/goliatone/go-featuregate/featureerrors"
	"github.com/goliatone/go-featuregate/gate"
	"github.com/goliatone/go-featuregate/scope"
	"github.com/goliatone/go-featuregate/store"
)

// ErrInvalidKey signals an empty or invalid feature key.
var ErrInvalidKey = featureerrors.ErrInvalidKey

// ErrStoreUnavailable signals a missing runtime override store.
var ErrStoreUnavailable = featureerrors.ErrStoreUnavailable

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
	defaults      Defaults
	overrides     store.Reader
	writer        store.Writer
	scopeResolver gate.ScopeResolver
	cache         cache.Cache
	hooks         []gate.ResolveHook
	updateHooks   []activity.Hook
	strictStore   bool
}

// Option customizes a Gate.
type Option func(*Gate)

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

// WithScopeResolver overrides scope derivation.
func WithScopeResolver(resolver gate.ScopeResolver) Option {
	return func(g *Gate) {
		if g == nil {
			return
		}
		g.scopeResolver = resolver
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
		defaults: NoopDefaults{},
		cache:    cache.NoopCache{},
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
func (g *Gate) Set(ctx context.Context, key string, scopeSet gate.ScopeSet, enabled bool, actor gate.ActorRef) error {
	trimmed := strings.TrimSpace(key)
	normalized := gate.NormalizeKey(trimmed)
	if g.writer == nil {
		return featureerrors.WrapSentinel(featureerrors.ErrStoreUnavailable, "", map[string]any{
			featureerrors.MetaFeatureKey:           trimmed,
			featureerrors.MetaFeatureKeyNormalized: normalized,
			featureerrors.MetaScope:                scopeSet,
			featureerrors.MetaStore:                "override",
			featureerrors.MetaOperation:            "set",
		})
	}
	if normalized == "" {
		return featureerrors.WrapSentinel(featureerrors.ErrInvalidKey, "", map[string]any{
			featureerrors.MetaFeatureKey:           trimmed,
			featureerrors.MetaFeatureKeyNormalized: normalized,
			featureerrors.MetaScope:                scopeSet,
			featureerrors.MetaOperation:            "set",
		})
	}
	if err := g.writer.Set(ctx, normalized, scopeSet, enabled, actor); err != nil {
		return featureerrors.WrapExternal(err, featureerrors.TextCodeStoreWriteFailed, "override store set failed", map[string]any{
			featureerrors.MetaFeatureKey:           trimmed,
			featureerrors.MetaFeatureKeyNormalized: normalized,
			featureerrors.MetaScope:                scopeSet,
			featureerrors.MetaStore:                "override",
			featureerrors.MetaOperation:            "set",
		})
	}
	if g.cache != nil {
		g.cache.Delete(ctx, normalized, scopeSet)
	}
	g.emitUpdate(ctx, activity.UpdateEvent{
		Key:           strings.TrimSpace(key),
		NormalizedKey: normalized,
		Scope:         scopeSet,
		Actor:         actor,
		Action:        activity.ActionSet,
		Value:         boolPtr(enabled),
	})
	return nil
}

// Unset clears a runtime override.
func (g *Gate) Unset(ctx context.Context, key string, scopeSet gate.ScopeSet, actor gate.ActorRef) error {
	trimmed := strings.TrimSpace(key)
	normalized := gate.NormalizeKey(trimmed)
	if g.writer == nil {
		return featureerrors.WrapSentinel(featureerrors.ErrStoreUnavailable, "", map[string]any{
			featureerrors.MetaFeatureKey:           trimmed,
			featureerrors.MetaFeatureKeyNormalized: normalized,
			featureerrors.MetaScope:                scopeSet,
			featureerrors.MetaStore:                "override",
			featureerrors.MetaOperation:            "unset",
		})
	}
	if normalized == "" {
		return featureerrors.WrapSentinel(featureerrors.ErrInvalidKey, "", map[string]any{
			featureerrors.MetaFeatureKey:           trimmed,
			featureerrors.MetaFeatureKeyNormalized: normalized,
			featureerrors.MetaScope:                scopeSet,
			featureerrors.MetaOperation:            "unset",
		})
	}
	if err := g.writer.Unset(ctx, normalized, scopeSet, actor); err != nil {
		return featureerrors.WrapExternal(err, featureerrors.TextCodeStoreWriteFailed, "override store unset failed", map[string]any{
			featureerrors.MetaFeatureKey:           trimmed,
			featureerrors.MetaFeatureKeyNormalized: normalized,
			featureerrors.MetaScope:                scopeSet,
			featureerrors.MetaStore:                "override",
			featureerrors.MetaOperation:            "unset",
		})
	}
	aliasErr := g.unsetAliases(ctx, normalized, scopeSet, actor)
	if g.cache != nil {
		g.cache.Delete(ctx, normalized, scopeSet)
	}
	g.emitUpdate(ctx, activity.UpdateEvent{
		Key:           strings.TrimSpace(key),
		NormalizedKey: normalized,
		Scope:         scopeSet,
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
		err := featureerrors.WrapSentinel(featureerrors.ErrInvalidKey, "", map[string]any{
			featureerrors.MetaFeatureKey:           trimmed,
			featureerrors.MetaFeatureKeyNormalized: normalized,
			featureerrors.MetaOperation:            "resolve",
		})
		trace.Source = gate.ResolveSourceFallback
		g.emitResolve(ctx, trace, err)
		return false, trace, err
	}

	scopeSet, err := g.resolveScope(ctx, opts...)
	if err != nil {
		err = featureerrors.WrapExternal(err, featureerrors.TextCodeScopeResolveFailed, "scope resolution failed", map[string]any{
			featureerrors.MetaFeatureKey:           trimmed,
			featureerrors.MetaFeatureKeyNormalized: normalized,
			featureerrors.MetaOperation:            "resolve_scope",
		})
		trace.Scope = scopeSet
		trace.Source = gate.ResolveSourceFallback
		g.emitResolve(ctx, trace, err)
		return false, trace, err
	}
	trace.Scope = scopeSet

	if g.cache != nil {
		if entry, ok := g.cache.Get(ctx, normalized, scopeSet); ok {
			cached := entry.Trace
			if cached.Key == "" {
				cached.Key = trimmed
			}
			if cached.NormalizedKey == "" {
				cached.NormalizedKey = normalized
			}
			cached.Scope = scopeSet
			cached.Value = entry.Value
			cached.CacheHit = true
			g.emitResolve(ctx, cached, nil)
			return entry.Value, cached, nil
		}
	}

	var storeErr error
	if g.overrides != nil {
		override, err := g.overrides.Get(ctx, normalized, scopeSet)
		if err != nil {
			storeErr = featureerrors.WrapExternal(err, featureerrors.TextCodeStoreReadFailed, "override store read failed", map[string]any{
				featureerrors.MetaFeatureKey:           trimmed,
				featureerrors.MetaFeatureKeyNormalized: normalized,
				featureerrors.MetaScope:                scopeSet,
				featureerrors.MetaStore:                "override",
				featureerrors.MetaOperation:            "get",
				featureerrors.MetaStrict:               g.strictStore,
			})
			trace.Override.Error = storeErr
			if g.strictStore {
				trace.Override.State = gate.OverrideStateMissing
				trace.Source = gate.ResolveSourceFallback
				g.emitResolve(ctx, trace, storeErr)
				return false, trace, storeErr
			}
		} else {
			override = normalizeOverride(override)
			if override.State == gate.OverrideStateMissing {
				aliasOverride, aliasErr := g.aliasOverride(ctx, normalized, scopeSet)
				if aliasErr != nil {
					storeErr = featureerrors.WrapExternal(aliasErr, featureerrors.TextCodeStoreReadFailed, "override store read failed", map[string]any{
						featureerrors.MetaFeatureKey:           trimmed,
						featureerrors.MetaFeatureKeyNormalized: normalized,
						featureerrors.MetaScope:                scopeSet,
						featureerrors.MetaStore:                "override",
						featureerrors.MetaOperation:            "get_alias",
						featureerrors.MetaStrict:               g.strictStore,
					})
					trace.Override.Error = storeErr
					if g.strictStore {
						trace.Override.State = gate.OverrideStateMissing
						trace.Source = gate.ResolveSourceFallback
						g.emitResolve(ctx, trace, storeErr)
						return false, trace, storeErr
					}
				} else {
					override = aliasOverride
				}
			}
			trace.Override.State = override.State
			if override.HasValue() {
				value := override.State == gate.OverrideStateEnabled
				trace.Override.Value = boolPtr(value)
				trace.Value = value
				trace.Source = gate.ResolveSourceOverride
				g.writeCache(ctx, normalized, scopeSet, trace, storeErr)
				g.emitResolve(ctx, trace, nil)
				return value, trace, nil
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
		err = featureerrors.WrapExternal(err, featureerrors.TextCodeDefaultLookupFailed, "default lookup failed", map[string]any{
			featureerrors.MetaFeatureKey:           trimmed,
			featureerrors.MetaFeatureKeyNormalized: normalized,
			featureerrors.MetaScope:                scopeSet,
			featureerrors.MetaOperation:            "default",
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

	g.writeCache(ctx, normalized, scopeSet, trace, storeErr)
	g.emitResolve(ctx, trace, nil)
	return trace.Value, trace, nil
}

func (g *Gate) resolveScope(ctx context.Context, opts ...gate.ResolveOption) (gate.ScopeSet, error) {
	req := gate.ResolveRequest{}
	for _, opt := range opts {
		if opt != nil {
			opt(&req)
		}
	}
	if req.ScopeSet != nil {
		return *req.ScopeSet, nil
	}
	if g.scopeResolver != nil {
		return g.scopeResolver.Resolve(ctx)
	}
	return scope.FromContext(ctx), nil
}

func (g *Gate) writeCache(ctx context.Context, key string, scopeSet gate.ScopeSet, trace gate.ResolveTrace, storeErr error) {
	if g.cache == nil {
		return
	}
	if storeErr != nil {
		return
	}
	g.cache.Set(ctx, key, scopeSet, cache.Entry{
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
		Scope:         trace.Scope,
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

func normalizeOverride(override store.Override) store.Override {
	if override.State == "" {
		override.State = gate.OverrideStateMissing
	}
	return override
}

func (g *Gate) aliasOverride(ctx context.Context, key string, scopeSet gate.ScopeSet) (store.Override, error) {
	if g == nil || g.overrides == nil {
		return store.MissingOverride(), nil
	}
	aliases := gate.AliasesFor(key)
	if len(aliases) == 0 {
		return store.MissingOverride(), nil
	}
	for _, alias := range aliases {
		override, err := g.overrides.Get(ctx, alias, scopeSet)
		if err != nil {
			return store.MissingOverride(), err
		}
		override = normalizeOverride(override)
		if override.State != gate.OverrideStateMissing {
			return override, nil
		}
	}
	return store.MissingOverride(), nil
}

func (g *Gate) unsetAliases(ctx context.Context, normalized string, scopeSet gate.ScopeSet, actor gate.ActorRef) error {
	if g == nil || g.writer == nil {
		return nil
	}
	aliases := gate.AliasesFor(normalized)
	if len(aliases) == 0 {
		return nil
	}
	for _, alias := range aliases {
		if err := g.writer.Unset(ctx, alias, scopeSet, actor); err != nil {
			return err
		}
	}
	return nil
}
