# Getting Started with go-featuregate

This guide helps you integrate feature flags into your Go application in under 5 minutes.

## Overview

go-featuregate provides feature flag management with:

- **Config defaults** - Define baseline feature states in configuration
- **Runtime overrides** - Toggle features at runtime without redeployment
- **Multi-tenant scoping** - Scope flags to tenants, organizations, or users
- **Resolution tracing** - Debug why a feature resolved to a specific value

## Installation

```bash
go get github.com/goliatone/go-featuregate
```

Requires Go 1.24+.

## Quick Start

### 1. Config Defaults Only

The simplest setup uses static configuration defaults:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/goliatone/go-featuregate/adapters/configadapter"
    "github.com/goliatone/go-featuregate/resolver"
)

func main() {
    // Define feature defaults as a flat map of dot-delimited keys.
    defaults := configadapter.NewDefaultsFromBools(map[string]bool{
        "users.signup":    true,
        "dashboard":       false,
        "notifications":   true,
    })

    // Create the gate
    gate := resolver.New(
        resolver.WithDefaults(defaults),
    )

    // Check feature enablement
    ctx := context.Background()

    enabled, err := gate.Enabled(ctx, "users.signup")
    if err != nil {
        log.Fatal(err)
    }
    if enabled {
        fmt.Println("User signup is enabled")
    }

    enabled, err = gate.Enabled(ctx, "dashboard")
    if err != nil {
        log.Fatal(err)
    }
    if enabled {
        fmt.Println("Dashboard is enabled")
    } else {
        fmt.Println("Dashboard is disabled")
    }
}
```

Output:
```
User signup is enabled
Dashboard is disabled
```

### 2. With Runtime Overrides

Add an override store to toggle features at runtime:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/goliatone/go-featuregate/adapters/configadapter"
    "github.com/goliatone/go-featuregate/gate"
    "github.com/goliatone/go-featuregate/resolver"
    "github.com/goliatone/go-featuregate/store"
)

func main() {
    defaults := configadapter.NewDefaultsFromBools(map[string]bool{
        "beta.features": false,
    })

    // Add in-memory override store
    overrides := store.NewMemoryStore()

    featureGate := resolver.New(
        resolver.WithDefaults(defaults),
        resolver.WithOverrideStore(overrides),
    )

    ctx := context.Background()
    scope := gate.ScopeSet{TenantID: "acme-corp"}
    actor := gate.ActorRef{ID: "admin-1", Type: "user", Name: "Admin"}

    // Check default value
    enabled, err := featureGate.Enabled(ctx, "beta.features", gate.WithScopeSet(scope))
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("beta.features (default): %v\n", enabled)

    // Enable at runtime for this tenant
    if err := featureGate.Set(ctx, "beta.features", scope, true, actor); err != nil {
        log.Fatal(err)
    }

    enabled, err = featureGate.Enabled(ctx, "beta.features", gate.WithScopeSet(scope))
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("beta.features (override): %v\n", enabled)

    // Revert to default
    if err := featureGate.Unset(ctx, "beta.features", scope, actor); err != nil {
        log.Fatal(err)
    }

    enabled, err = featureGate.Enabled(ctx, "beta.features", gate.WithScopeSet(scope))
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("beta.features (unset): %v\n", enabled)
}
```

Output:
```
beta.features (default): false
beta.features (override): true
beta.features (unset): false
```

## Core Concepts

### Feature Keys

Feature keys are dot-separated strings that identify features:

```go
"users.signup"           // User signup feature
"dashboard"              // Dashboard feature
"notifications.email"    // Email notifications
"beta.new_editor"        // Beta new editor feature
```

Keys are trimmed and aliases are resolved (if configured). Use lowercase dot-delimited
keys by convention, and call `gate.NormalizeKey()` for consistency.

### Resolution Order

When you call `Enabled()`, the gate checks sources in this order:

1. **Override store** - Runtime overrides (if configured)
2. **Config defaults** - Static configuration values
3. **Fallback** - Returns `false` if no value found

### Scopes

Scopes allow different feature states for different contexts:

```go
// System-wide (no tenant/org/user)
systemScope := gate.ScopeSet{System: true}

// Tenant-specific
tenantScope := gate.ScopeSet{TenantID: "acme"}

// User-specific (most specific)
userScope := gate.ScopeSet{
    TenantID: "acme",
    OrgID:    "engineering",
    UserID:   "user-123",
}
```

Resolution precedence: User > Org > Tenant > System

If `System` is true, tenant/org/user IDs are ignored and scope resolves at
system level.

### Actors

Track who made changes with `ActorRef`:

```go
actor := gate.ActorRef{
    ID:   "user-123",
    Type: "user",
    Name: "Jane Admin",
}

featureGate.Set(ctx, "feature.key", scope, true, actor)
```

## Configuration Options

The snippets in this section assume the defaults and override store setup from the
Quick Start examples (including `featureGate`, `ctx`, `scope`, and imports), and
focus on the specific option being added.

### Strict Store Mode

By default, store errors fail open (feature returns default/false). Enable strict mode to fail closed:

```go
featureGate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithOverrideStore(overrides),
    resolver.WithStrictStore(true), // Errors propagate instead of falling back
)
```

### Caching

Add a cache to reduce store lookups:

```go
featureGate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithOverrideStore(overrides),
    resolver.WithCache(myCache), // Implements cache.Cache interface
)
```

### Hooks

Subscribe to resolution and update events:

```go
featureGate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithResolveHook(gate.ResolveHookFunc(func(ctx context.Context, event gate.ResolveEvent) {
        log.Printf("Resolved %s = %v (source: %s)", event.Key, event.Value, event.Source)
    })),
    resolver.WithActivityHook(activity.HookFunc(func(ctx context.Context, event activity.UpdateEvent) {
        log.Printf("Updated %s by %s", event.Key, event.Actor.ID)
    })),
)
```

## Debugging with Traces

Use `ResolveWithTrace` to understand why a feature resolved to a specific value:

```go
enabled, trace, err := featureGate.ResolveWithTrace(ctx, "dashboard", gate.WithScopeSet(scope))
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Feature: %s\n", trace.Key)
fmt.Printf("Value: %v\n", trace.Value)
fmt.Printf("Source: %s\n", trace.Source) // "override", "default", or "fallback"
fmt.Printf("Override State: %s\n", trace.Override.State)
fmt.Printf("Default Set: %v, Value: %v\n", trace.Default.Set, trace.Default.Value)
```

## Running the Examples

The repository includes working examples:

```bash
# Config defaults only
go run ./examples/config_only

# Runtime overrides
go run ./examples/runtime_overrides
```

## Next Steps

- **[GUIDE_RESOLUTION](GUIDE_RESOLUTION.md)** - Deep dive into resolution hierarchy
- **[GUIDE_SCOPES](GUIDE_SCOPES.md)** - Multi-tenant feature flags
- **[GUIDE_OVERRIDES](GUIDE_OVERRIDES.md)** - Runtime override management
- **[GUIDE_ADAPTERS](GUIDE_ADAPTERS.md)** - Database and external system integration
- **[GUIDE_TEMPLATES](GUIDE_TEMPLATES.md)** - Using feature flags in templates
