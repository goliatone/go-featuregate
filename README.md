# go-featuregate

Shared feature gating with config defaults and runtime overrides.

## Core

Create a gate from defaults and an override store:

```go
defaults := configadapter.NewDefaults(map[string]any{
	"users": map[string]any{
		"signup": config.NewOptionalBool(true),
	},
})

gate := resolver.New(
	resolver.WithDefaults(defaults),
)
```

Create a gate with in-memory overrides:

```go
overrides := store.NewMemoryStore()
gate := resolver.New(
	resolver.WithOverrideStore(overrides),
)
```

## Adapters

### configadapter

`configadapter.NewDefaults` accepts go-config `OptionalBool` values and raw maps. Use
`configadapter.NewDefaultsFromBools` for flat `map[string]bool` input.

### optionsadapter

Wrap a `go-options/pkg/state.Store` as a feature override store:

```go
stateStore := stateStoreImpl // state.Store[map[string]any]
overrides := optionsadapter.NewStore(stateStore, optionsadapter.WithDomain("feature_flags"))

gate := resolver.New(
	resolver.WithOverrideStore(overrides),
)
```

Use the PreferencesStore adapter when go-admin is the backing store:

```go
prefs := admin.NewInMemoryPreferencesStore()
stateStore := optionsadapter.NewPreferencesStoreAdapter(prefs)
overrides := optionsadapter.NewStore(stateStore, optionsadapter.WithDomain("feature_flags"))
```

### bunadapter

Persist overrides in a `feature_flags` table (see `schema/feature_flags.sql`):

```go
db := bun.NewDB(sqlDB, pgdialect.New())
overrides := bunadapter.NewStore(db)
gate := resolver.New(
	resolver.WithOverrideStore(overrides),
)
```

### goauthadapter

Derive scope and actor metadata from go-auth:

```go
scopeResolver := goauthadapter.NewScopeResolver()
gate := resolver.New(
	resolver.WithScopeResolver(scopeResolver),
)
```

Use `goauthadapter.ActorRefFromContext` when persisting overrides.

### routeradapter

Helpers for go-router contexts:

- `routeradapter.Context(ctx)` returns a standard `context.Context`.
- `routeradapter.ScopeSet(ctx)` derives a `gate.ScopeSet`.
- `routeradapter.WithRouterContext(ctx)` provides a resolve option.

### gologgeradapter

Log resolve/update events via go-logger:

```go
hook := gologgeradapter.New(logger)
gate := resolver.New(
	resolver.WithResolveHook(hook),
	resolver.WithActivityHook(hook),
)
```

### urlkitadapter

Wrap a go-urlkit resolver as a `urlbuilder.Builder`:

```go
builder := urlkitadapter.New(manager)
```

## Template helpers

Register helpers with your template engine (e.g., `WithTemplateFunc`):

```go
funcs := templates.TemplateHelpers(gate)
```

Standard template data keys (override with helper options):

- `feature_ctx`: `context.Context` or any value implementing `Context() context.Context`
- `feature_scope`: `gate.ScopeSet` (or `map[string]any` with `tenant_id`, `org_id`, `user_id`)
- `feature_snapshot`: precomputed values (`templates.Snapshot`, `map[string]bool`, or map of traces)

Resolution order:

1. `feature_snapshot` (if it contains the key)
2. `FeatureGate.Enabled` / `ResolveWithTrace`

Helper list:

- `feature(key)` -> bool
- `feature_any(key1, key2, ...)` -> bool
- `feature_all(key1, key2, ...)` -> bool
- `feature_none(key1, key2, ...)` -> bool
- `feature_if(key, whenTrue, whenFalse)` -> any
- `feature_class(key, on, off)` -> any
- `feature_trace(key)` -> `gate.ResolveTrace` (registered only when the gate is traceable)
