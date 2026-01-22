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

## Concepts

### Key naming and normalization

Feature keys are dot-scoped strings (for example `users.signup`, `dashboard`). Common keys include
`users.invite`, `users.password_reset`, `users.password_reset.finalize`, `users.signup`, `cms`,
`dashboard`, `notifications`, and `debug`. Normalize keys with `gate.NormalizeKey` to trim
whitespace. The legacy key `users.self_registration` is not normalized or checked; use
`users.signup`.

### Scope derivation and overrides

Scopes are represented by `gate.ScopeSet` (`System`, `TenantID`, `OrgID`, `UserID`). `System`
explicitly forces system scope and ignores tenant/org/user IDs. By default, `resolver.Gate`
derives scope from `context.Context` using `scope.FromContext` (see `scope.WithSystem`,
`scope.WithTenantID`, `scope.WithOrgID`, `scope.WithUserID`). Scope metadata keys are `tenant_id`,
`org_id`, and `user_id`. Scope helpers ignore empty values; use `scope.ClearTenantID`,
`scope.ClearOrgID`, and `scope.ClearUserID` to clear values explicitly. Override scope explicitly
with `gate.WithScopeSet`, especially for boot/test flows that should resolve at system scope (use
`ScopeSet{System: true}`). Precedence is user > org > tenant > system when `System` is false.

### Resolution order and unset semantics

Resolution order:
1. runtime overrides (store)
2. config defaults
3. fallback false

Overrides are tri-state (`enabled`, `disabled`, `unset`). Use `Unset` to explicitly clear a value
and fall back to config defaults. Store errors fail open by default; enable strict behavior with
`resolver.WithStrictStore(true)` to fail closed and surface the error.

### Guard helpers

Use `gate/guard` to enforce feature checks with optional override keys and custom error mapping:

```go
if err := guard.Require(ctx, gate, "users.signup",
	guard.WithDisabledError(errors.New("signup disabled")),
	guard.WithOverrides("users.signup.override"),
	guard.WithErrorMapper(mapGateErr),
); err != nil {
	return err
}
```

### Runtime overrides and storage

Runtime overrides flow through `store.Reader`/`store.Writer`. The `resolver.Gate` type implements
`gate.MutableFeatureGate` with `Set` and `Unset`, and `store.NewMemoryStore` is available for tests
and examples.

The default SQL schema lives in `schema/feature_flags.sql`. `enabled` is nullable: `NULL` represents
an explicit unset (fall back to config defaults). The bun adapter sets `enabled = NULL` on `Unset`;
stores that expose `Delete` remove the row entirely for cleanup. The options adapter deletes the key
path from the snapshot to represent an unset.

### Hooks and events

Use `resolver.WithResolveHook` to subscribe to per-resolve events (`gate.ResolveEvent` includes the
full `gate.ResolveTrace`). Use `resolver.WithActivityHook` for runtime override updates
(`activity.UpdateEvent` includes the actor, scope, and action).

### Errors and taxonomy

Rich errors are built on `github.com/goliatone/go-errors` with helpers in `ferrors`. Categories map
to `BadInput`, `Operation`, `External`, and `Internal`.

Text codes include:
- `FEATURE_KEY_REQUIRED`, `OVERRIDE_STORE_REQUIRED`, `STORE_REQUIRED`
- `RESOLVER_REQUIRED`, `FEATURE_GATE_REQUIRED`, `SCOPE_REQUIRED`, `SNAPSHOT_REQUIRED`
- `PATH_REQUIRED`, `PATH_INVALID`, `OVERRIDE_TYPE_INVALID`
- `PREFERENCES_STORE_REQUIRED`, `SCOPE_INVALID`, `SCOPE_METADATA_MISSING`, `SCOPE_METADATA_INVALID`
- `ADAPTER_FAILED`, `STORE_READ_FAILED`, `STORE_WRITE_FAILED`
- `DEFAULT_LOOKUP_FAILED`, `SCOPE_RESOLVE_FAILED`

Common metadata keys include `feature_key`, `feature_key_norm`, `scope`, `store`, `adapter`,
`domain`, `table`, `operation`, `strict`, and `path`.

Use `ferrors.WrapSentinel` to preserve `errors.Is` behavior for sentinel errors and
`ferrors.WrapExternal` when wrapping dependency failures. Use `ferrors.As` to extract the rich
error payload for logging or template output.

## Adapters

### configadapter

`configadapter.NewDefaults` accepts go-config `OptionalBool` values and raw maps. Use
`configadapter.NewDefaultsFromBools` for flat `map[string]bool` input. `WithDelimiter` customizes
the nested map delimiter (defaults to ".").

### optionsadapter

Wrap a `go-options/pkg/state.Store` as a feature override store:

```go
stateStore := stateStoreImpl // state.Store[map[string]any]
overrides := optionsadapter.NewStore(stateStore, optionsadapter.WithDomain("feature_flags"))

gate := resolver.New(
	resolver.WithOverrideStore(overrides),
)
```

Use the PreferencesStore adapter from go-admin (`github.com/goliatone/go-admin/featuregate/adapter`)
when go-admin is the backing store:

```go
prefs := admin.NewInMemoryPreferencesStore()
stateStore := featuregateadapter.NewPreferencesStoreAdapter(prefs)
overrides := optionsadapter.NewStore(stateStore, optionsadapter.WithDomain("feature_flags"))
```

Use `optionsadapter.WithScopeBuilder` or `optionsadapter.WithMetaBuilder` to customize scope
ordering or stored metadata.

### bunadapter

Persist overrides in a `feature_flags` table (see `schema/feature_flags.sql`):

```go
db := bun.NewDB(sqlDB, pgdialect.New())
overrides := bunadapter.NewStore(db)
gate := resolver.New(
	resolver.WithOverrideStore(overrides),
)
```

Use `bunadapter.WithTable` to point to a custom table name and `bunadapter.WithUpdatedByBuilder`
to control the `updated_by` audit value.

### goauthadapter

Derive scope and actor metadata from go-auth (import from `github.com/goliatone/go-auth/adapters/featuregate`):

```go
scopeResolver := goauthadapter.NewScopeResolver()
gate := resolver.New(
	resolver.WithScopeResolver(scopeResolver),
)
```

Use `goauthadapter.ActorRefFromContext` when persisting overrides.

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

Helper options let you override template data keys (`WithContextKey`, `WithScopeKey`,
`WithSnapshotKey`), enable structured errors (`WithStructuredErrors`), or log helper failures
(`WithErrorLogging`, `WithLogger`). When `feature_snapshot` includes trace data, `feature_trace`
prefers it before calling the gate.

## Examples

- `examples/config_only/main.go` shows config defaults only (no runtime store).
- `examples/runtime_overrides/main.go` shows runtime overrides with `Set`/`Unset`.

Run them with:

```bash
go run ./examples/config_only
go run ./examples/runtime_overrides
```
