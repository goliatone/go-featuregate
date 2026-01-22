# Runtime Overrides Guide

This guide explains how to manage runtime feature flag overrides using go-featuregate's mutable gate and store interfaces.

## Overview

Runtime overrides allow you to change feature flag values without redeploying your application:

- **Enable** features for specific tenants/users (beta testing)
- **Disable** features during incidents (kill switch)
- **Unset** overrides to fall back to defaults

## Override Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Override Flow                             │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  gate.Set(ctx, key, scope, enabled, actor)                  │
│       │                                                     │
│       ▼                                                     │
│  ┌─────────────────────────────────────┐                   │
│  │         store.Writer                │                   │
│  │  Set(ctx, key, scope, enabled, actor)│                   │
│  └─────────────────────────────────────┘                   │
│       │                                                     │
│       ▼                                                     │
│  ┌─────────────────────────────────────┐                   │
│  │       Storage Backend               │                   │
│  │  (Memory, Database, Options)        │                   │
│  └─────────────────────────────────────┘                   │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Tri-State Semantics

Overrides use tri-state values:

| State | Meaning | Resolution Behavior |
|-------|---------|---------------------|
| `enabled` | Feature explicitly enabled | Returns `true` |
| `disabled` | Feature explicitly disabled | Returns `false` |
| `missing` | No override exists | Falls through to defaults |
| `unset` | Override explicitly cleared | Falls through to defaults |

```go
import "github.com/goliatone/go-featuregate/gate"

const (
    OverrideStateMissing  OverrideState = "missing"
    OverrideStateEnabled  OverrideState = "enabled"
    OverrideStateDisabled OverrideState = "disabled"
    OverrideStateUnset    OverrideState = "unset"
)
```

## MutableFeatureGate Interface

The `resolver.Gate` implements `MutableFeatureGate`:

```go
type MutableFeatureGate interface {
    FeatureGate
    Set(ctx context.Context, key string, scope ScopeSet, enabled bool, actor ActorRef) error
    Unset(ctx context.Context, key string, scope ScopeSet, actor ActorRef) error
}
```

## Setting Overrides

### Enable a Feature

```go
import (
    "github.com/goliatone/go-featuregate/gate"
    "github.com/goliatone/go-featuregate/resolver"
    "github.com/goliatone/go-featuregate/store"
)

overrides := store.NewMemoryStore()
featureGate := resolver.New(
    resolver.WithOverrideStore(overrides),
)

ctx := context.Background()
scope := gate.ScopeSet{TenantID: "acme-corp"}
actor := gate.ActorRef{
    ID:   "admin-123",
    Type: "user",
    Name: "Admin User",
}

// Enable beta features for this tenant
err := featureGate.Set(ctx, "beta.features", scope, true, actor)
if err != nil {
    log.Fatal(err)
}
```

### Disable a Feature

```go
// Disable during incident
err := featureGate.Set(ctx, "payments.processing", scope, false, actor)
```

### Unset an Override

Remove an override to fall back to the configured default:

```go
// Remove override, revert to default behavior
err := featureGate.Unset(ctx, "beta.features", scope, actor)
```

## Actor Tracking

Track who made changes for audit purposes:

```go
type ActorRef struct {
    ID   string // Unique identifier
    Type string // "user", "system", "api", etc.
    Name string // Human-readable name (optional)
}
```

Examples:

```go
// Human admin
adminActor := gate.ActorRef{
    ID:   "user-456",
    Type: "user",
    Name: "Jane Admin",
}

// Automated system
systemActor := gate.ActorRef{
    ID:   "feature-manager",
    Type: "system",
    Name: "Feature Manager Service",
}

// API client
apiActor := gate.ActorRef{
    ID:   "api-key-abc123",
    Type: "api",
}
```

## Store Implementations

### In-Memory Store

Best for testing and development:

```go
import "github.com/goliatone/go-featuregate/store"

overrides := store.NewMemoryStore()

featureGate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithOverrideStore(overrides),
)
```

Memory store methods:

```go
// Standard interface
overrides.Get(ctx, key, scope)        // Read override
overrides.Set(ctx, key, scope, enabled, actor)  // Write override
overrides.Unset(ctx, key, scope, actor)         // Clear override

// Memory-specific
overrides.Delete(key, scope)  // Remove entry entirely
overrides.Clear()             // Remove all entries
```

### Bun Adapter (Database)

For production use with PostgreSQL/SQLite:

```go
import (
    "github.com/goliatone/go-featuregate/adapters/bunadapter"
    "github.com/uptrace/bun"
)

db := bun.NewDB(sqlDB, pgdialect.New())
overrides := bunadapter.NewStore(db)

featureGate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithOverrideStore(overrides),
)
```

See [GUIDE_ADAPTERS](GUIDE_ADAPTERS.md) and [GUIDE_MIGRATIONS](GUIDE_MIGRATIONS.md) for database setup.

### Options Adapter

For integration with go-options state stores:

```go
import "github.com/goliatone/go-featuregate/adapters/optionsadapter"

stateStore := myStateStore // state.Store[map[string]any]
overrides := optionsadapter.NewStore(stateStore,
    optionsadapter.WithDomain("feature_flags"),
)

featureGate := resolver.New(
    resolver.WithOverrideStore(overrides),
)
```

## Store Interfaces

### Reader Interface

```go
type Reader interface {
    Get(ctx context.Context, key string, scope gate.ScopeSet) (Override, error)
}
```

### Writer Interface

```go
type Writer interface {
    Set(ctx context.Context, key string, scope gate.ScopeSet, enabled bool, actor gate.ActorRef) error
    Unset(ctx context.Context, key string, scope gate.ScopeSet, actor gate.ActorRef) error
}
```

### ReadWriter Interface

```go
type ReadWriter interface {
    Reader
    Writer
}
```

## Override Helpers

Construct override values:

```go
import "github.com/goliatone/go-featuregate/store"

// No override exists
missing := store.MissingOverride()

// Override explicitly cleared
unset := store.UnsetOverride()

// Feature enabled
enabled := store.EnabledOverride()

// Feature disabled
disabled := store.DisabledOverride()

// Check if override has a concrete value
if override.HasValue() {
    // State is enabled or disabled
}
```

## Cache Invalidation

The gate automatically invalidates cache entries on mutations:

```go
featureGate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithOverrideStore(overrides),
    resolver.WithCache(myCache),
)

// Set invalidates the cache for this key/scope
featureGate.Set(ctx, "feature", scope, true, actor)

// Next Enabled() call will re-resolve and cache
enabled, _ := featureGate.Enabled(ctx, "feature", gate.WithScopeSet(scope))
```

## Activity Hooks

Subscribe to override changes:

```go
import "github.com/goliatone/go-featuregate/activity"

featureGate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithOverrideStore(overrides),
    resolver.WithActivityHook(activity.HookFunc(func(ctx context.Context, event activity.UpdateEvent) {
        log.Printf("[%s] %s %s by %s (scope: %+v)",
            event.Action,        // "set" or "unset"
            event.NormalizedKey,
            valueStr(event.Value),
            event.Actor.ID,
            event.Scope,
        )
    })),
)

func valueStr(v *bool) string {
    if v == nil {
        return "nil"
    }
    return fmt.Sprintf("%v", *v)
}
```

### UpdateEvent Structure

```go
type UpdateEvent struct {
    Key           string       // Original key
    NormalizedKey string       // Normalized key
    Scope         gate.ScopeSet
    Actor         gate.ActorRef
    Action        Action       // ActionSet or ActionUnset
    Value         *bool        // nil for unset
}
```

## Common Patterns

### Feature Toggle API

```go
type FeatureToggleRequest struct {
    Key      string `json:"key"`
    TenantID string `json:"tenant_id"`
    Enabled  bool   `json:"enabled"`
}

func handleToggle(w http.ResponseWriter, r *http.Request) {
    var req FeatureToggleRequest
    json.NewDecoder(r.Body).Decode(&req)

    actor := getActorFromContext(r.Context())
    scope := gate.ScopeSet{TenantID: req.TenantID}

    if err := featureGate.Set(r.Context(), req.Key, scope, req.Enabled, actor); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusOK)
}
```

### Emergency Kill Switch

```go
func emergencyDisable(ctx context.Context, featureKey string, reason string) error {
    actor := gate.ActorRef{
        ID:   "ops-runbook",
        Type: "emergency",
        Name: fmt.Sprintf("Emergency: %s", reason),
    }

    // Disable system-wide
    return featureGate.Set(ctx, featureKey, gate.ScopeSet{System: true}, false, actor)
}

func emergencyRestore(ctx context.Context, featureKey string) error {
    actor := gate.ActorRef{
        ID:   "ops-runbook",
        Type: "emergency",
        Name: "Emergency restore",
    }

    // Remove system-wide override
    return featureGate.Unset(ctx, featureKey, gate.ScopeSet{System: true}, actor)
}
```

### Progressive Rollout

```go
func rolloutToTenants(ctx context.Context, featureKey string, tenantIDs []string) error {
    actor := gate.ActorRef{
        ID:   "rollout-manager",
        Type: "system",
    }

    for _, tenantID := range tenantIDs {
        scope := gate.ScopeSet{TenantID: tenantID}
        if err := featureGate.Set(ctx, featureKey, scope, true, actor); err != nil {
            return fmt.Errorf("failed to enable for tenant %s: %w", tenantID, err)
        }
    }
    return nil
}
```

### User Beta Program

```go
func enrollBetaUser(ctx context.Context, userID string, features []string) error {
    actor := gate.ActorRef{
        ID:   "beta-program",
        Type: "system",
    }

    scope := gate.ScopeSet{UserID: userID}

    for _, feature := range features {
        if err := featureGate.Set(ctx, feature, scope, true, actor); err != nil {
            return err
        }
    }
    return nil
}

func unenrollBetaUser(ctx context.Context, userID string, features []string) error {
    actor := gate.ActorRef{
        ID:   "beta-program",
        Type: "system",
    }

    scope := gate.ScopeSet{UserID: userID}

    for _, feature := range features {
        if err := featureGate.Unset(ctx, feature, scope, actor); err != nil {
            return err
        }
    }
    return nil
}
```

## Error Handling

### Missing Store

```go
// No override store configured
featureGate := resolver.New(
    resolver.WithDefaults(defaults),
    // No WithOverrideStore
)

err := featureGate.Set(ctx, "feature", scope, true, actor)
// err == ErrStoreUnavailable
```

### Invalid Key

```go
err := featureGate.Set(ctx, "", scope, true, actor)
// err == ErrInvalidKey
```

### Store Errors

```go
// Wrap external store errors
if err := featureGate.Set(ctx, key, scope, enabled, actor); err != nil {
    if rich, ok := ferrors.As(err); ok {
        log.Printf("Store error: %s (code: %s)", rich.Message, rich.TextCode)
        // TextCode might be "STORE_WRITE_FAILED"
    }
}
```

## Implementing Custom Stores

Implement `store.ReadWriter` for custom backends:

```go
type RedisStore struct {
    client *redis.Client
    prefix string
}

func (s *RedisStore) Get(ctx context.Context, key string, scope gate.ScopeSet) (store.Override, error) {
    redisKey := s.buildKey(key, scope)
    val, err := s.client.Get(ctx, redisKey).Result()
    if err == redis.Nil {
        return store.MissingOverride(), nil
    }
    if err != nil {
        return store.MissingOverride(), err
    }

    if val == "true" {
        return store.EnabledOverride(), nil
    }
    if val == "false" {
        return store.DisabledOverride(), nil
    }
    return store.UnsetOverride(), nil
}

func (s *RedisStore) Set(ctx context.Context, key string, scope gate.ScopeSet, enabled bool, actor gate.ActorRef) error {
    redisKey := s.buildKey(key, scope)
    val := "false"
    if enabled {
        val = "true"
    }
    return s.client.Set(ctx, redisKey, val, 0).Err()
}

func (s *RedisStore) Unset(ctx context.Context, key string, scope gate.ScopeSet, actor gate.ActorRef) error {
    redisKey := s.buildKey(key, scope)
    return s.client.Del(ctx, redisKey).Err()
}

func (s *RedisStore) buildKey(key string, scope gate.ScopeSet) string {
    // Build hierarchical key based on scope
    return fmt.Sprintf("%s:%s:%s:%s:%s",
        s.prefix, key, scope.TenantID, scope.OrgID, scope.UserID)
}
```

## Best Practices

### 1. Always Provide Actor Information

```go
// Good: meaningful actor
actor := gate.ActorRef{
    ID:   userID,
    Type: "admin",
    Name: userName,
}

// Avoid: empty or generic actors
actor := gate.ActorRef{ID: "unknown"}
```

### 2. Use Explicit Scopes for Mutations

```go
// Good: explicit scope
scope := gate.ScopeSet{TenantID: "acme"}
featureGate.Set(ctx, "feature", scope, true, actor)

// Avoid: relying on context-derived scope
```

### 3. Handle Errors Appropriately

```go
if err := featureGate.Set(ctx, key, scope, enabled, actor); err != nil {
    // Log the error with context
    log.Printf("Failed to set override: key=%s, scope=%+v, err=%v", key, scope, err)
    // Return appropriate HTTP status or propagate error
    return fmt.Errorf("failed to update feature flag: %w", err)
}
```

### 4. Use Activity Hooks for Audit Trails

```go
featureGate := resolver.New(
    resolver.WithOverrideStore(overrides),
    resolver.WithActivityHook(auditLogger),
)
```

## Next Steps

- **[GUIDE_ADAPTERS](GUIDE_ADAPTERS.md)** - Storage backend integration
- **[GUIDE_HOOKS](GUIDE_HOOKS.md)** - Event subscriptions
- **[GUIDE_MIGRATIONS](GUIDE_MIGRATIONS.md)** - Database setup
- **[GUIDE_TESTING](GUIDE_TESTING.md)** - Testing with in-memory stores
