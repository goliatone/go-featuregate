# Testing Guide

This guide covers testing strategies for applications using go-featuregate.

## Overview

go-featuregate is designed for testability:

- **In-memory store** - Fast, isolated tests without external dependencies
- **Interface-based** - Easy mocking with standard Go interfaces
- **Deterministic** - No hidden state or side effects

## In-Memory Store

The `MemoryStore` is the primary tool for unit testing:

```go
import (
    "context"
    "testing"

    "github.com/goliatone/go-featuregate/resolver"
    "github.com/goliatone/go-featuregate/store"
    "github.com/goliatone/go-featuregate/gate"
    "github.com/stretchr/testify/assert"
)

func TestFeatureEnabled(t *testing.T) {
    ctx := context.Background()
    overrides := store.NewMemoryStore()
    featureGate := resolver.New(resolver.WithOverrideStore(overrides))

    // Set up test data
    actor := gate.ActorRef{ID: "test"}
    scope := gate.ScopeSet{System: true}
    featureGate.Set(ctx, "my.feature", scope, true, actor)

    // Test
    enabled, err := featureGate.Enabled(ctx, "my.feature")

    assert.NoError(t, err)
    assert.True(t, enabled)
}
```

### Clearing State Between Tests

```go
func TestWithClearState(t *testing.T) {
    overrides := store.NewMemoryStore()
    featureGate := resolver.New(resolver.WithOverrideStore(overrides))

    t.Run("first test", func(t *testing.T) {
        // Set feature
        featureGate.Set(ctx, "feature", scope, true, actor)

        enabled, _ := featureGate.Enabled(ctx, "feature")
        assert.True(t, enabled)

        // Clear for next test
        overrides.Clear()
    })

    t.Run("second test", func(t *testing.T) {
        // Feature is now disabled (default fallback)
        enabled, _ := featureGate.Enabled(ctx, "feature")
        assert.False(t, enabled)
    })
}
```

## Mocking the FeatureGate Interface

### Basic Mock

```go
type MockFeatureGate struct {
    features map[string]bool
}

func NewMockFeatureGate(features map[string]bool) *MockFeatureGate {
    return &MockFeatureGate{features: features}
}

func (m *MockFeatureGate) Enabled(ctx context.Context, key string, opts ...gate.ResolveOption) (bool, error) {
    if value, ok := m.features[key]; ok {
        return value, nil
    }
    return false, nil
}
```

Usage:

```go
func TestServiceWithMock(t *testing.T) {
    mockGate := NewMockFeatureGate(map[string]bool{
        "premium.feature": true,
        "beta.feature":    false,
    })

    service := NewMyService(mockGate)

    // Test service behavior with features enabled/disabled
    result := service.DoSomething()
    assert.Equal(t, "premium result", result)
}
```

### Mock with Error Simulation

```go
type ErrorMockFeatureGate struct {
    err error
}

func (m *ErrorMockFeatureGate) Enabled(ctx context.Context, key string, opts ...gate.ResolveOption) (bool, error) {
    return false, m.err
}

func TestServiceHandlesGateError(t *testing.T) {
    mockGate := &ErrorMockFeatureGate{
        err: errors.New("store unavailable"),
    }

    service := NewMyService(mockGate)

    err := service.DoSomething()
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "store unavailable")
}
```

### Full Mock with MutableFeatureGate

```go
type FullMockFeatureGate struct {
    mu       sync.RWMutex
    features map[string]bool
    calls    []string
}

func NewFullMockFeatureGate() *FullMockFeatureGate {
    return &FullMockFeatureGate{
        features: make(map[string]bool),
    }
}

func (m *FullMockFeatureGate) Enabled(ctx context.Context, key string, opts ...gate.ResolveOption) (bool, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    m.calls = append(m.calls, "Enabled:"+key)
    return m.features[key], nil
}

func (m *FullMockFeatureGate) Set(ctx context.Context, key string, scope gate.ScopeSet, enabled bool, actor gate.ActorRef) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.features[key] = enabled
    m.calls = append(m.calls, "Set:"+key)
    return nil
}

func (m *FullMockFeatureGate) Unset(ctx context.Context, key string, scope gate.ScopeSet, actor gate.ActorRef) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    delete(m.features, key)
    m.calls = append(m.calls, "Unset:"+key)
    return nil
}

func (m *FullMockFeatureGate) Calls() []string {
    m.mu.RLock()
    defer m.mu.RUnlock()
    return append([]string{}, m.calls...)
}
```

## Testing Resolution Scenarios

### Testing Override Precedence

```go
func TestOverridePrecedence(t *testing.T) {
    ctx := context.Background()

    // Defaults
    defaults := configadapter.NewDefaultsFromBools(map[string]bool{
        "feature": false, // default is false
    })

    overrides := store.NewMemoryStore()
    featureGate := resolver.New(
        resolver.WithDefaults(defaults),
        resolver.WithOverrideStore(overrides),
    )

    actor := gate.ActorRef{ID: "test"}

    t.Run("default value when no override", func(t *testing.T) {
        enabled, _ := featureGate.Enabled(ctx, "feature")
        assert.False(t, enabled) // Uses default
    })

    t.Run("override takes precedence over default", func(t *testing.T) {
        featureGate.Set(ctx, "feature", gate.ScopeSet{System: true}, true, actor)

        enabled, _ := featureGate.Enabled(ctx, "feature")
        assert.True(t, enabled) // Override wins
    })

    t.Run("unset falls back to default", func(t *testing.T) {
        featureGate.Unset(ctx, "feature", gate.ScopeSet{System: true}, actor)

        enabled, _ := featureGate.Enabled(ctx, "feature")
        assert.False(t, enabled) // Back to default
    })
}
```

### Testing Scope Isolation

```go
func TestScopeIsolation(t *testing.T) {
    ctx := context.Background()
    overrides := store.NewMemoryStore()
    featureGate := resolver.New(resolver.WithOverrideStore(overrides))

    actor := gate.ActorRef{ID: "test"}

    // Enable for tenant A only
    tenantA := gate.ScopeSet{TenantID: "tenant-a"}
    featureGate.Set(ctx, "feature", tenantA, true, actor)

    t.Run("enabled for tenant A", func(t *testing.T) {
        enabled, _ := featureGate.Enabled(ctx, "feature", gate.WithScopeSet(tenantA))
        assert.True(t, enabled)
    })

    t.Run("disabled for tenant B", func(t *testing.T) {
        tenantB := gate.ScopeSet{TenantID: "tenant-b"}
        enabled, _ := featureGate.Enabled(ctx, "feature", gate.WithScopeSet(tenantB))
        assert.False(t, enabled)
    })

    t.Run("disabled for system scope", func(t *testing.T) {
        systemScope := gate.ScopeSet{System: true}
        enabled, _ := featureGate.Enabled(ctx, "feature", gate.WithScopeSet(systemScope))
        assert.False(t, enabled)
    })
}
```

### Testing Scope Precedence

```go
func TestScopePrecedence(t *testing.T) {
    ctx := context.Background()
    overrides := store.NewMemoryStore()
    featureGate := resolver.New(resolver.WithOverrideStore(overrides))

    actor := gate.ActorRef{ID: "test"}

    // System: enabled
    featureGate.Set(ctx, "feature", gate.ScopeSet{System: true}, true, actor)

    // Tenant: disabled
    tenantScope := gate.ScopeSet{TenantID: "acme"}
    featureGate.Set(ctx, "feature", tenantScope, false, actor)

    // User: enabled
    userScope := gate.ScopeSet{TenantID: "acme", UserID: "user-123"}
    featureGate.Set(ctx, "feature", userScope, true, actor)

    tests := []struct {
        name     string
        scope    gate.ScopeSet
        expected bool
    }{
        {"system scope", gate.ScopeSet{System: true}, true},
        {"tenant scope", tenantScope, false},
        {"user scope", userScope, true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            enabled, _ := featureGate.Enabled(ctx, "feature", gate.WithScopeSet(tt.scope))
            assert.Equal(t, tt.expected, enabled)
        })
    }
}
```

### Testing Default Fallbacks

```go
func TestDefaultFallback(t *testing.T) {
    ctx := context.Background()

    defaults := configadapter.NewDefaultsFromBools(map[string]bool{
        "existing.feature": true,
    })

    featureGate := resolver.New(resolver.WithDefaults(defaults))

    t.Run("returns default for existing feature", func(t *testing.T) {
        enabled, err := featureGate.Enabled(ctx, "existing.feature")
        assert.NoError(t, err)
        assert.True(t, enabled)
    })

    t.Run("returns false for unknown feature", func(t *testing.T) {
        enabled, err := featureGate.Enabled(ctx, "unknown.feature")
        assert.NoError(t, err)
        assert.False(t, enabled) // Fallback
    })
}
```

## Testing Guards

```go
func TestGuardRequire(t *testing.T) {
    ctx := context.Background()
    overrides := store.NewMemoryStore()
    featureGate := resolver.New(resolver.WithOverrideStore(overrides))

    actor := gate.ActorRef{ID: "test"}

    t.Run("returns nil when feature enabled", func(t *testing.T) {
        featureGate.Set(ctx, "feature", gate.ScopeSet{System: true}, true, actor)

        err := guard.Require(ctx, featureGate, "feature")
        assert.NoError(t, err)
    })

    t.Run("returns error when feature disabled", func(t *testing.T) {
        featureGate.Unset(ctx, "feature", gate.ScopeSet{System: true}, actor)

        err := guard.Require(ctx, featureGate, "feature")
        assert.Error(t, err)
        assert.ErrorIs(t, err, guard.ErrFeatureDisabled)
    })

    t.Run("returns custom error when configured", func(t *testing.T) {
        customErr := errors.New("premium required")

        err := guard.Require(ctx, featureGate, "feature",
            guard.WithDisabledError(customErr),
        )
        assert.ErrorIs(t, err, customErr)
    })

    t.Run("allows via override key", func(t *testing.T) {
        featureGate.Set(ctx, "override.key", gate.ScopeSet{System: true}, true, actor)

        err := guard.Require(ctx, featureGate, "disabled.feature",
            guard.WithOverrides("override.key"),
        )
        assert.NoError(t, err)
    })
}
```

## Testing Template Helpers

```go
func TestTemplateHelpers(t *testing.T) {
    ctx := context.Background()
    overrides := store.NewMemoryStore()
    featureGate := resolver.New(resolver.WithOverrideStore(overrides))

    actor := gate.ActorRef{ID: "test"}
    featureGate.Set(ctx, "enabled.feature", gate.ScopeSet{System: true}, true, actor)

    helpers := templates.TemplateHelpers(featureGate)

    t.Run("feature helper", func(t *testing.T) {
        tpl, _ := pongo2.FromString(`{% if feature("enabled.feature") %}yes{% else %}no{% endif %}`)

        out, err := tpl.Execute(pongo2.Context{
            "feature_ctx": ctx,
        })
        assert.NoError(t, err)
        assert.Equal(t, "yes", out)
    })

    t.Run("feature_if helper", func(t *testing.T) {
        tpl, _ := pongo2.FromString(`{{ feature_if("enabled.feature", "on", "off") }}`)

        out, err := tpl.Execute(pongo2.Context{
            "feature_ctx": ctx,
        })
        assert.NoError(t, err)
        assert.Equal(t, "on", out)
    })
}
```

### Testing with Snapshots

```go
func TestTemplateWithSnapshot(t *testing.T) {
    helpers := templates.TemplateHelpers(nil) // No gate needed with snapshot

    snapshot := map[string]bool{
        "feature.a": true,
        "feature.b": false,
    }

    tpl, _ := pongo2.FromString(`{{ feature_if("feature.a", "yes", "no") }}`)

    out, err := tpl.Execute(pongo2.Context{
        "feature_snapshot": snapshot,
    })
    assert.NoError(t, err)
    assert.Equal(t, "yes", out)
}
```

## Testing Hooks

### Testing Resolve Hooks

```go
type CaptureResolveHook struct {
    mu     sync.Mutex
    Events []gate.ResolveEvent
}

func (h *CaptureResolveHook) OnResolve(ctx context.Context, event gate.ResolveEvent) {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.Events = append(h.Events, event)
}

func (h *CaptureResolveHook) Clear() {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.Events = nil
}

func TestResolveHook(t *testing.T) {
    ctx := context.Background()
    capture := &CaptureResolveHook{}

    featureGate := resolver.New(
        resolver.WithResolveHook(capture),
    )

    featureGate.Enabled(ctx, "test.feature")

    assert.Len(t, capture.Events, 1)
    event := capture.Events[0]
    assert.Equal(t, "test.feature", event.NormalizedKey)
    assert.Equal(t, gate.ResolveSourceFallback, event.Source)
    assert.False(t, event.Value)
}
```

### Testing Activity Hooks

```go
type CaptureActivityHook struct {
    mu     sync.Mutex
    Events []activity.UpdateEvent
}

func (h *CaptureActivityHook) OnUpdate(ctx context.Context, event activity.UpdateEvent) {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.Events = append(h.Events, event)
}

func TestActivityHook(t *testing.T) {
    ctx := context.Background()
    capture := &CaptureActivityHook{}
    overrides := store.NewMemoryStore()

    featureGate := resolver.New(
        resolver.WithOverrideStore(overrides),
        resolver.WithActivityHook(capture),
    )

    actor := gate.ActorRef{ID: "admin", Type: "user", Name: "Admin User"}
    scope := gate.ScopeSet{TenantID: "acme"}

    t.Run("captures set events", func(t *testing.T) {
        featureGate.Set(ctx, "feature", scope, true, actor)

        assert.Len(t, capture.Events, 1)
        event := capture.Events[0]
        assert.Equal(t, activity.ActionSet, event.Action)
        assert.Equal(t, "admin", event.Actor.ID)
        assert.Equal(t, "acme", event.Scope.TenantID)
        assert.True(t, *event.Value)
    })

    t.Run("captures unset events", func(t *testing.T) {
        featureGate.Unset(ctx, "feature", scope, actor)

        assert.Len(t, capture.Events, 2)
        event := capture.Events[1]
        assert.Equal(t, activity.ActionUnset, event.Action)
        assert.Nil(t, event.Value)
    })
}
```

## Integration Testing

### With Database Store

```go
func TestWithDatabaseStore(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }

    // Set up test database
    db := setupTestDatabase(t)
    defer db.Close()

    overrides := bunadapter.NewStore(db)
    featureGate := resolver.New(resolver.WithOverrideStore(overrides))

    ctx := context.Background()
    actor := gate.ActorRef{ID: "test"}
    scope := gate.ScopeSet{TenantID: "test-tenant"}

    t.Run("persist and retrieve override", func(t *testing.T) {
        err := featureGate.Set(ctx, "test.feature", scope, true, actor)
        require.NoError(t, err)

        enabled, err := featureGate.Enabled(ctx, "test.feature", gate.WithScopeSet(scope))
        require.NoError(t, err)
        assert.True(t, enabled)
    })

    t.Run("override persists across gate instances", func(t *testing.T) {
        // Create new gate instance with same store
        newGate := resolver.New(resolver.WithOverrideStore(bunadapter.NewStore(db)))

        enabled, err := newGate.Enabled(ctx, "test.feature", gate.WithScopeSet(scope))
        require.NoError(t, err)
        assert.True(t, enabled)
    })
}

func setupTestDatabase(t *testing.T) *bun.DB {
    t.Helper()

    sqlDB, err := sql.Open("postgres", os.Getenv("TEST_DATABASE_URL"))
    require.NoError(t, err)

    db := bun.NewDB(sqlDB, pgdialect.New())

    // Run migrations
    _, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS feature_flags (
            key TEXT NOT NULL,
            scope_type TEXT NOT NULL,
            scope_id TEXT NOT NULL,
            enabled BOOLEAN,
            updated_by TEXT,
            updated_at TIMESTAMP,
            PRIMARY KEY (key, scope_type, scope_id)
        )
    `)
    require.NoError(t, err)

    t.Cleanup(func() {
        db.Exec("DROP TABLE IF EXISTS feature_flags")
        db.Close()
    })

    return db
}
```

## Test Fixtures

### Feature Gate Fixture

```go
func newTestGate(t *testing.T, defaults map[string]bool) *resolver.Gate {
    t.Helper()

    defaultsProvider := configadapter.NewDefaultsFromBools(defaults)
    overrides := store.NewMemoryStore()

    return resolver.New(
        resolver.WithDefaults(defaultsProvider),
        resolver.WithOverrideStore(overrides),
    )
}

func TestWithFixture(t *testing.T) {
    gate := newTestGate(t, map[string]bool{
        "feature.a": true,
        "feature.b": false,
    })

    ctx := context.Background()

    enabled, _ := gate.Enabled(ctx, "feature.a")
    assert.True(t, enabled)

    enabled, _ = gate.Enabled(ctx, "feature.b")
    assert.False(t, enabled)
}
```

### Scope Fixture

```go
func testScope(tenantID, userID string) gate.ScopeSet {
    return gate.ScopeSet{
        TenantID: tenantID,
        UserID:   userID,
    }
}

func testActor(id string) gate.ActorRef {
    return gate.ActorRef{
        ID:   id,
        Type: "test",
        Name: "Test Actor",
    }
}
```

## Table-Driven Tests

```go
func TestFeatureResolution(t *testing.T) {
    ctx := context.Background()

    defaults := configadapter.NewDefaultsFromBools(map[string]bool{
        "default.enabled":  true,
        "default.disabled": false,
    })

    overrides := store.NewMemoryStore()
    featureGate := resolver.New(
        resolver.WithDefaults(defaults),
        resolver.WithOverrideStore(overrides),
    )

    actor := gate.ActorRef{ID: "test"}

    // Set up overrides
    featureGate.Set(ctx, "override.enabled", gate.ScopeSet{System: true}, true, actor)
    featureGate.Set(ctx, "override.disabled", gate.ScopeSet{System: true}, false, actor)

    tests := []struct {
        name     string
        key      string
        expected bool
        source   gate.ResolveSource
    }{
        {"default enabled", "default.enabled", true, gate.ResolveSourceDefault},
        {"default disabled", "default.disabled", false, gate.ResolveSourceDefault},
        {"override enabled", "override.enabled", true, gate.ResolveSourceOverride},
        {"override disabled", "override.disabled", false, gate.ResolveSourceOverride},
        {"unknown feature", "unknown.feature", false, gate.ResolveSourceFallback},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            enabled, trace, err := featureGate.ResolveWithTrace(ctx, tt.key)

            assert.NoError(t, err)
            assert.Equal(t, tt.expected, enabled)
            assert.Equal(t, tt.source, trace.Source)
        })
    }
}
```

## Best Practices

### 1. Use In-Memory Store for Unit Tests

```go
// Fast, isolated, no external dependencies
overrides := store.NewMemoryStore()
```

### 2. Clear State Between Tests

```go
func TestSuite(t *testing.T) {
    overrides := store.NewMemoryStore()

    t.Run("test1", func(t *testing.T) {
        // ...
        t.Cleanup(func() { overrides.Clear() })
    })

    t.Run("test2", func(t *testing.T) {
        // Starts with clean state
    })
}
```

### 3. Test Both Enabled and Disabled States

```go
func TestFeatureBehavior(t *testing.T) {
    t.Run("when enabled", func(t *testing.T) {
        gate := newTestGate(t, map[string]bool{"feature": true})
        // Test enabled behavior
    })

    t.Run("when disabled", func(t *testing.T) {
        gate := newTestGate(t, map[string]bool{"feature": false})
        // Test disabled behavior
    })
}
```

### 4. Test Error Conditions

```go
func TestErrorHandling(t *testing.T) {
    t.Run("invalid key", func(t *testing.T) {
        gate := resolver.New()
        _, err := gate.Enabled(context.Background(), "")
        assert.Error(t, err)
    })

    t.Run("store error propagation", func(t *testing.T) {
        // Test with failing store mock
    })
}
```

### 5. Use Subtests for Organization

```go
func TestFeatureGate(t *testing.T) {
    t.Run("Resolution", func(t *testing.T) {
        t.Run("uses override when present", func(t *testing.T) { /* ... */ })
        t.Run("falls back to default", func(t *testing.T) { /* ... */ })
        t.Run("returns false for unknown keys", func(t *testing.T) { /* ... */ })
    })

    t.Run("Mutations", func(t *testing.T) {
        t.Run("set enables feature", func(t *testing.T) { /* ... */ })
        t.Run("unset removes override", func(t *testing.T) { /* ... */ })
    })
}
```

## Next Steps

- **[GUIDE_GETTING_STARTED](GUIDE_GETTING_STARTED.md)** - Basic setup
- **[GUIDE_GUARDS](GUIDE_GUARDS.md)** - Testing guards
- **[GUIDE_HOOKS](GUIDE_HOOKS.md)** - Testing hooks
- **[GUIDE_ERRORS](GUIDE_ERRORS.md)** - Testing error conditions
