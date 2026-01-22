# Feature Resolution Guide

This guide explains how go-featuregate resolves feature flag values, including the resolution hierarchy, key normalization, and debugging with traces.

## Resolution Hierarchy

When you call `Enabled()` or `ResolveWithTrace()`, the gate checks sources in a specific order:

```
┌─────────────────────────────────────────────────────────────┐
│                     Resolution Flow                          │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  1. Cache (if configured)                                   │
│     └─ Hit? Return cached value                             │
│                                                             │
│  2. Override Store (if configured)                          │
│     └─ Has value? Return override                           │
│     └─ Check aliases if primary key missing                 │
│                                                             │
│  3. Config Defaults                                         │
│     └─ Has value? Return default                            │
│                                                             │
│  4. Fallback                                                │
│     └─ Return false                                         │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### Source Priority

| Priority | Source | Description |
|----------|--------|-------------|
| 1 | Cache | Previously resolved value (if caching enabled) |
| 2 | Override | Runtime override from store (enabled, disabled, or unset) |
| 3 | Default | Static configuration value |
| 4 | Fallback | Always returns `false` |

## Config Defaults

Config defaults define the baseline state of features before any runtime overrides.

### Using OptionalBool (go-config)

```go
import (
    "github.com/goliatone/go-config/config"
    "github.com/goliatone/go-featuregate/adapters/configadapter"
    "github.com/goliatone/go-featuregate/resolver"
)

defaults := configadapter.NewDefaults(map[string]any{
    "users": map[string]any{
        "signup":         config.NewOptionalBool(true),
        "password_reset": config.NewOptionalBool(true),
    },
    "dashboard": config.NewOptionalBool(false),
    "beta": map[string]any{
        "new_editor": config.NewOptionalBool(false),
    },
})

gate := resolver.New(
    resolver.WithDefaults(defaults),
)
```

### Using Simple Booleans

For simpler cases, use `NewDefaultsFromBools`:

```go
defaults := configadapter.NewDefaultsFromBools(map[string]bool{
    "users.signup":    true,
    "dashboard":       false,
    "notifications":   true,
})

gate := resolver.New(
    resolver.WithDefaults(defaults),
)
```

### Custom Delimiter

By default, nested maps use `.` as the key delimiter. Customize with `WithDelimiter`:

```go
defaults := configadapter.NewDefaults(map[string]any{
    "users": map[string]any{
        "signup": true,
    },
}, configadapter.WithDelimiter("/"))

// Key becomes "users/signup" instead of "users.signup"
```

### Custom Defaults Implementation

Implement the `resolver.Defaults` interface for custom sources:

```go
type Defaults interface {
    Default(ctx context.Context, key string) (DefaultResult, error)
}

type DefaultResult struct {
    Set   bool  // Whether a default exists
    Value bool  // The default value
}
```

Example:

```go
type EnvDefaults struct{}

func (d *EnvDefaults) Default(ctx context.Context, key string) (resolver.DefaultResult, error) {
    envKey := "FEATURE_" + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
    if value, ok := os.LookupEnv(envKey); ok {
        return resolver.DefaultResult{
            Set:   true,
            Value: value == "true" || value == "1",
        }, nil
    }
    return resolver.DefaultResult{Set: false}, nil
}
```

## Runtime Overrides

Runtime overrides take precedence over defaults. See [GUIDE_OVERRIDES](GUIDE_OVERRIDES.md) for details.

### Tri-State Values

Overrides have three possible states:

| State | Description | Resolution Behavior |
|-------|-------------|---------------------|
| `enabled` | Explicitly enabled | Returns `true` |
| `disabled` | Explicitly disabled | Returns `false` |
| `missing`/`unset` | No override set | Falls through to defaults |

```go
// Enable a feature
gate.Set(ctx, "beta.features", scope, true, actor)

// Disable a feature
gate.Set(ctx, "beta.features", scope, false, actor)

// Remove override (fall back to default)
gate.Unset(ctx, "beta.features", scope, actor)
```

## Fallback Behavior

When no override or default exists, the gate returns `false`:

```go
// No default configured for "unknown.feature"
enabled, _ := gate.Enabled(ctx, "unknown.feature")
// enabled == false
```

## Key Normalization

Feature keys are normalized before resolution:

1. **Whitespace trimmed** - Leading/trailing spaces removed
2. **Aliases resolved** - Legacy keys mapped to canonical keys (if configured)

```go
import "github.com/goliatone/go-featuregate/gate"

// All equivalent after normalization
gate.NormalizeKey("  users.signup  ")  // "users.signup"
gate.NormalizeKey("users.signup")      // "users.signup"
```

### Key Aliases

Aliases allow legacy keys to resolve to canonical keys:

```go
// Check if a key is an alias
if gate.IsAlias("old.feature.name") {
    canonical, _ := gate.ResolveAlias("old.feature.name")
    fmt.Println("Canonical key:", canonical)
}

// Get all aliases for a key
aliases := gate.AliasesFor("users.signup")
```

**Note**: Legacy aliases are currently disabled in the default configuration.

### Standard Feature Keys

Common feature keys used across the ecosystem:

```go
const (
    FeatureUsersSignup               = "users.signup"
    FeatureUsersPasswordReset        = "users.password_reset"
    FeatureUsersPasswordResetFinalize = "users.password_reset.finalize"
)
```

## Error Handling

### Strict vs Permissive Mode

By default, store errors **fail open** (return default/fallback):

```go
// Default: permissive mode
gate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithOverrideStore(overrides),
)

// Store error → falls back to default
enabled, err := gate.Enabled(ctx, "feature")
// err == nil, enabled == default value
```

Enable **strict mode** to fail closed on store errors:

```go
gate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithOverrideStore(overrides),
    resolver.WithStrictStore(true),
)

// Store error → error returned
enabled, err := gate.Enabled(ctx, "feature")
// err != nil if store fails
```

### Error Types

| Error | Cause |
|-------|-------|
| `ErrInvalidKey` | Empty or invalid feature key |
| `ErrStoreUnavailable` | No override store configured (for Set/Unset) |
| Store errors | Database/network failures (in strict mode) |

## Resolution Tracing

Use `ResolveWithTrace` to debug resolution:

```go
enabled, trace, err := gate.ResolveWithTrace(ctx, "dashboard", gate.WithScopeSet(scope))
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Key: %s\n", trace.Key)
fmt.Printf("Normalized: %s\n", trace.NormalizedKey)
fmt.Printf("Value: %v\n", trace.Value)
fmt.Printf("Source: %s\n", trace.Source)
fmt.Printf("Cache Hit: %v\n", trace.CacheHit)

// Override details
fmt.Printf("Override State: %s\n", trace.Override.State)
if trace.Override.Value != nil {
    fmt.Printf("Override Value: %v\n", *trace.Override.Value)
}
if trace.Override.Error != nil {
    fmt.Printf("Override Error: %v\n", trace.Override.Error)
}

// Default details
fmt.Printf("Default Set: %v\n", trace.Default.Set)
fmt.Printf("Default Value: %v\n", trace.Default.Value)
```

### Trace Structure

```go
type ResolveTrace struct {
    Key           string        // Original key
    NormalizedKey string        // After normalization
    Scope         ScopeSet      // Resolution scope
    Value         bool          // Final resolved value
    Source        ResolveSource // Where value came from
    Override      OverrideTrace // Override resolution details
    Default       DefaultTrace  // Default resolution details
    CacheHit      bool          // Whether served from cache
}

type ResolveSource string
const (
    ResolveSourceOverride ResolveSource = "override"
    ResolveSourceDefault  ResolveSource = "default"
    ResolveSourceFallback ResolveSource = "fallback"
)
```

### Common Trace Scenarios

**Override Active**:
```
Source: override
Override.State: enabled
Override.Value: true
```

**Default Used**:
```
Source: default
Override.State: missing
Default.Set: true
Default.Value: false
```

**Fallback (no config)**:
```
Source: fallback
Override.State: missing
Default.Set: false
```

## Caching

Enable caching to reduce store lookups:

```go
gate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithOverrideStore(overrides),
    resolver.WithCache(myCache),
)
```

Cache entries are keyed by `(feature_key, scope)` and automatically invalidated on `Set`/`Unset`.

See [GUIDE_CACHING](GUIDE_CACHING.md) for implementation details.

## Resolve Hooks

Subscribe to resolution events for logging/monitoring:

```go
gate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithResolveHook(gate.ResolveHookFunc(func(ctx context.Context, event gate.ResolveEvent) {
        log.Printf("[%s] %s = %v (source: %s)",
            event.Scope.TenantID,
            event.Key,
            event.Value,
            event.Source,
        )
        if event.Error != nil {
            log.Printf("Resolution error: %v", event.Error)
        }
    })),
)
```

See [GUIDE_HOOKS](GUIDE_HOOKS.md) for more hook patterns.

## Best Practices

### 1. Define Sensible Defaults

Always configure defaults so features have predictable baseline behavior:

```go
defaults := configadapter.NewDefaultsFromBools(map[string]bool{
    "users.signup":    true,   // Core features enabled
    "beta.features":   false,  // New features disabled by default
})
```

### 2. Use Consistent Key Naming

Follow a hierarchical naming convention:

```
<module>.<feature>
<module>.<submodule>.<feature>
```

Examples:
- `users.signup`
- `users.password_reset`
- `dashboard.widgets.charts`
- `notifications.email`

### 3. Prefer Explicit Scopes

Pass scopes explicitly rather than relying on context extraction:

```go
// Preferred: explicit scope
enabled, _ := gate.Enabled(ctx, "feature", gate.WithScopeSet(scope))

// Implicit: derived from context
enabled, _ := gate.Enabled(ctx, "feature")
```

### 4. Use Tracing for Debugging

When features behave unexpectedly, use `ResolveWithTrace`:

```go
_, trace, _ := gate.ResolveWithTrace(ctx, "problem.feature", gate.WithScopeSet(scope))
fmt.Printf("Debug: %+v\n", trace)
```

## Next Steps

- **[GUIDE_SCOPES](GUIDE_SCOPES.md)** - Multi-tenant feature flags
- **[GUIDE_OVERRIDES](GUIDE_OVERRIDES.md)** - Runtime override management
- **[GUIDE_CACHING](GUIDE_CACHING.md)** - Performance optimization
- **[GUIDE_HOOKS](GUIDE_HOOKS.md)** - Event subscriptions
