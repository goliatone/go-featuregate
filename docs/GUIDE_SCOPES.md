# Scopes Guide

This guide explains how to implement multi-tenant feature flags using go-featuregate's scope system.

## Overview

Scopes allow feature flags to have different values for different contexts:

- **System-wide** - Affects all tenants
- **Tenant-specific** - Affects a specific tenant
- **Organization-specific** - Affects a specific org within a tenant
- **User-specific** - Affects a specific user

This enables progressive rollouts, tenant-specific features, and user-level customization.

## ScopeRef and ScopeChain

```go
type ScopeRef struct {
    Kind     ScopeKind
    ID       string
    TenantID string
    OrgID    string
}

type ScopeChain []ScopeRef
```

## Scope Precedence

When multiple scopes could apply, the most specific scope wins:

```
User > Org > Tenant > System
```

If `System` is true, tenant/org/user IDs are ignored and the scope resolves at
system level regardless of other values.

Example scenario:
- System default: `dashboard = false`
- Tenant "acme" override: `dashboard = true`
- User "user-123" override: `dashboard = false`

For user-123 in tenant acme: `dashboard = false` (user override wins)

## Creating Scopes

### System Scope

For system-wide features that apply globally:

```go
import "github.com/goliatone/go-featuregate/gate"

// System-wide scope
systemChain := gate.ScopeChain{
    {Kind: gate.ScopeSystem},
}

// Check system-level feature
enabled, _ := featureGate.Enabled(ctx, "maintenance.mode", gate.WithScopeChain(systemChain))
```

### Tenant Scope

For features scoped to a specific tenant:

```go
tenantScope := gate.ScopeRef{Kind: gate.ScopeTenant, ID: "acme-corp", TenantID: "acme-corp"}
tenantChain := gate.ScopeChain{
    tenantScope,
    {Kind: gate.ScopeSystem},
}

// Enable feature for this tenant
featureGate.Set(ctx, "beta.features", tenantScope, true, actor)

// Check feature for this tenant
enabled, _ := featureGate.Enabled(ctx, "beta.features", gate.WithScopeChain(tenantChain))
```

### Organization Scope

For features scoped to an organization within a tenant:

```go
orgScope := gate.ScopeRef{
    Kind:     gate.ScopeOrg,
    ID:       "engineering",
    TenantID: "acme-corp",
    OrgID:    "engineering",
}

// Enable feature for engineering org
featureGate.Set(ctx, "advanced.tools", orgScope, true, actor)
```

Org-only scopes are supported when tenant information is not available:

```go
orgOnlyScope := gate.ScopeRef{
    Kind: gate.ScopeOrg,
    ID:   "engineering",
    OrgID: "engineering",
}

featureGate.Set(ctx, "advanced.tools", orgOnlyScope, true, actor)
```

Org-only overrides match only org-only checks (no implicit tenant wildcarding).

### User Scope

For user-specific feature flags:

```go
userScope := gate.ScopeRef{
    Kind:     gate.ScopeUser,
    ID:       "user-123",
    TenantID: "acme-corp",
    OrgID:    "engineering",
}

// Enable feature for specific user (beta tester)
featureGate.Set(ctx, "experimental.ui", userScope, true, actor)
```

## Deriving Scope from Context

### Using scope Package Helpers

Store scope identifiers in context and extract them automatically:

```go
import (
    "context"
    "github.com/goliatone/go-featuregate/scope"
)

// Store scope in context (typically in middleware)
ctx := context.Background()
ctx = scope.WithTenantID(ctx, "acme-corp")
ctx = scope.WithOrgID(ctx, "engineering")
ctx = scope.WithUserID(ctx, "user-123")

// Extract claims from context
claims := scope.ClaimsFromContext(ctx)
// claims.TenantID == "acme-corp"
// claims.OrgID == "engineering"
// claims.SubjectID == "user-123"
```

`scope.WithTenantID`, `scope.WithOrgID`, and `scope.WithUserID` ignore empty or
whitespace-only values. Use `scope.ClearTenantID`, `scope.ClearOrgID`, and
`scope.ClearUserID` to clear values explicitly.

To force system scope via context, set the system flag:

```go
ctx = scope.WithSystem(ctx, true)
```

### Individual Extractors

Extract individual scope components:

```go
tenantID := scope.TenantID(ctx)  // "acme-corp"
orgID := scope.OrgID(ctx)        // "engineering"
userID := scope.UserID(ctx)      // "user-123"
```

### Automatic Scope Resolution

When you don't pass an explicit scope, the gate derives it from context:

```go
// Middleware sets scope in context
ctx = scope.WithTenantID(ctx, "acme-corp")

// Gate automatically uses claims from context
enabled, _ := featureGate.Enabled(ctx, "feature.key")
```

## Explicit Scope Override

Override context-derived scope with `gate.WithScopeChain`:

```go
// Context has tenant scope
ctx = scope.WithTenantID(ctx, "acme-corp")

// Override with system scope for this specific check
systemChain := gate.ScopeChain{{Kind: gate.ScopeSystem}}
enabled, _ := featureGate.Enabled(ctx, "global.setting", gate.WithScopeChain(systemChain))
```

## Custom Scope Resolvers

Implement `gate.ClaimsProvider` for custom claims derivation:

```go
type ClaimsProvider interface {
    ClaimsFromContext(ctx context.Context) (gate.ActorClaims, error)
}
```

### Example: JWT Claims Resolver

```go
type JWTClaimsProvider struct{}

func (r *JWTClaimsProvider) ClaimsFromContext(ctx context.Context) (gate.ActorClaims, error) {
    claims, ok := ctx.Value("jwt_claims").(map[string]any)
    if !ok {
        return gate.ActorClaims{}, nil
    }

    return gate.ActorClaims{
        TenantID:  getString(claims, "tenant_id"),
        OrgID:     getString(claims, "org_id"),
        SubjectID: getString(claims, "sub"),
    }, nil
}

func getString(m map[string]any, key string) string {
    if v, ok := m[key].(string); ok {
        return v
    }
    return ""
}

// Use custom resolver
featureGate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithClaimsProvider(&JWTClaimsProvider{}),
)
```

### Using go-auth Adapter

The go-auth adapter provides scope resolution from authentication context:

```go
import "github.com/goliatone/go-auth/adapters/featuregate"

claimsProvider := featuregate.NewClaimsProvider()

featureGate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithClaimsProvider(claimsProvider),
)
```

## HTTP Middleware Integration

### Setting Scope in Middleware

```go
func ScopeMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()

        // Extract from headers, JWT, session, etc.
        tenantID := r.Header.Get("X-Tenant-ID")
        userID := getUserFromSession(r)

        ctx = scope.WithTenantID(ctx, tenantID)
        ctx = scope.WithUserID(ctx, userID)

        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

### Using Scope in Handlers

```go
func MyHandler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // Scope is automatically derived from context
    if enabled, _ := featureGate.Enabled(ctx, "new.dashboard"); enabled {
        renderNewDashboard(w, r)
    } else {
        renderLegacyDashboard(w, r)
    }
}
```

## Scope Patterns

### Progressive Rollout

Roll out features gradually across tenants:

```go
// Phase 1: Enable for internal tenant
featureGate.Set(ctx, "new.checkout", gate.ScopeRef{Kind: gate.ScopeTenant, ID: "internal", TenantID: "internal"}, true, actor)

// Phase 2: Enable for beta tenants
for _, tenantID := range betaTenants {
    featureGate.Set(ctx, "new.checkout", gate.ScopeRef{Kind: gate.ScopeTenant, ID: tenantID, TenantID: tenantID}, true, actor)
}

// Phase 3: Enable system-wide (or change default)
featureGate.Set(ctx, "new.checkout", gate.ScopeRef{Kind: gate.ScopeSystem}, true, actor)
```

### Beta User Testing

Enable features for specific beta testers:

```go
func enableBetaFeature(ctx context.Context, featureKey string, userIDs []string) error {
    actor := gate.ActorRef{ID: "system", Type: "automation"}

    for _, userID := range userIDs {
        scope := gate.ScopeRef{Kind: gate.ScopeUser, ID: userID}
        if err := featureGate.Set(ctx, featureKey, scope, true, actor); err != nil {
            return err
        }
    }
    return nil
}
```

### Tenant-Specific Features

Premium features for specific tenants:

```go
func checkPremiumFeature(ctx context.Context, featureKey string) bool {
    // Get tenant from context
    scopeSet := scope.FromContext(ctx)

    enabled, _ := featureGate.Enabled(ctx, featureKey, gate.WithScopeSet(scopeSet))
    return enabled
}

// In handler
if checkPremiumFeature(ctx, "advanced.analytics") {
    // Show premium analytics
}
```

### Emergency Kill Switch

Disable features system-wide in emergencies:

```go
func emergencyDisable(ctx context.Context, featureKey string) error {
    actor := gate.ActorRef{
        ID:   "ops-team",
        Type: "emergency",
        Name: "Emergency Disable",
    }

    // System-wide disable overrides all tenant/user settings
    return featureGate.Set(ctx, featureKey, gate.ScopeRef{Kind: gate.ScopeSystem}, false, actor)
}
```

## Scope Metadata Keys

When storing scope in maps or templates:

```go
const (
    MetadataTenantID = "tenant_id"
    MetadataOrgID    = "org_id"
    MetadataUserID   = "user_id"
)

// For template data
templateData := map[string]any{
    "feature_scope": map[string]any{
        "tenant_id": "acme-corp",
        "org_id":    "engineering",
        "user_id":   "user-123",
    },
}
```

## Testing with Scopes

### Unit Testing Different Scopes

```go
func TestFeatureByScope(t *testing.T) {
    defaults := configadapter.NewDefaultsFromBools(map[string]bool{
        "feature": false,
    })
    overrides := store.NewMemoryStore()

    featureGate := resolver.New(
        resolver.WithDefaults(defaults),
        resolver.WithOverrideStore(overrides),
    )

    ctx := context.Background()
    actor := gate.ActorRef{ID: "test"}

    // Enable for tenant A
    tenantA := gate.ScopeRef{Kind: gate.ScopeTenant, ID: "tenant-a", TenantID: "tenant-a"}
    tenantAChain := gate.ScopeChain{tenantA, {Kind: gate.ScopeSystem}}
    featureGate.Set(ctx, "feature", tenantA, true, actor)

    // Test tenant A - enabled
    enabled, _ := featureGate.Enabled(ctx, "feature", gate.WithScopeChain(tenantAChain))
    assert.True(t, enabled)

    // Test tenant B - falls back to default (false)
    tenantB := gate.ScopeRef{Kind: gate.ScopeTenant, ID: "tenant-b", TenantID: "tenant-b"}
    tenantBChain := gate.ScopeChain{tenantB, {Kind: gate.ScopeSystem}}
    enabled, _ = featureGate.Enabled(ctx, "feature", gate.WithScopeChain(tenantBChain))
    assert.False(t, enabled)
}
```

### Testing Scope Precedence

```go
func TestScopePrecedence(t *testing.T) {
    overrides := store.NewMemoryStore()
    featureGate := resolver.New(
        resolver.WithOverrideStore(overrides),
    )

    ctx := context.Background()
    actor := gate.ActorRef{ID: "test"}

    // System: enabled
    featureGate.Set(ctx, "feature", gate.ScopeRef{Kind: gate.ScopeSystem}, true, actor)

    // Tenant: disabled (overrides system)
    tenantScope := gate.ScopeRef{Kind: gate.ScopeTenant, ID: "acme", TenantID: "acme"}
    tenantChain := gate.ScopeChain{tenantScope, {Kind: gate.ScopeSystem}}
    featureGate.Set(ctx, "feature", tenantScope, false, actor)

    // User: enabled (overrides tenant)
    userScope := gate.ScopeRef{Kind: gate.ScopeUser, ID: "beta-user", TenantID: "acme"}
    userChain := gate.ScopeChain{userScope, tenantScope, {Kind: gate.ScopeSystem}}
    featureGate.Set(ctx, "feature", userScope, true, actor)

    // System scope: true
    enabled, _ := featureGate.Enabled(ctx, "feature", gate.WithScopeChain(gate.ScopeChain{{Kind: gate.ScopeSystem}}))
    assert.True(t, enabled)

    // Tenant scope: false
    enabled, _ = featureGate.Enabled(ctx, "feature", gate.WithScopeChain(tenantChain))
    assert.False(t, enabled)

    // User scope: true
    enabled, _ = featureGate.Enabled(ctx, "feature", gate.WithScopeChain(userChain))
    assert.True(t, enabled)
}
```

## Best Practices

### 1. Always Use Explicit Scopes for Mutations

```go
// Good: explicit scope
featureGate.Set(ctx, "feature", gate.ScopeRef{Kind: gate.ScopeTenant, ID: "acme", TenantID: "acme"}, true, actor)

// Avoid: relying on context claims for mutations
// claims := scope.ClaimsFromContext(ctx)
// _ = claims
```

### 2. Use System Scope for Boot/Test Flows

```go
// During application bootstrap
systemChain := gate.ScopeChain{{Kind: gate.ScopeSystem}}
if enabled, _ := featureGate.Enabled(ctx, "feature", gate.WithScopeChain(systemChain)); enabled {
    // Initialize feature
}
```

### 3. Document Scope Requirements

```go
// checkDashboard requires tenant scope in context.
// Returns false if no tenant scope is present.
func checkDashboard(ctx context.Context) bool {
    claims := scope.ClaimsFromContext(ctx)
    if claims.TenantID == "" {
        return false
    }
    chain := gate.ScopeChain{
        {Kind: gate.ScopeTenant, ID: claims.TenantID, TenantID: claims.TenantID},
        {Kind: gate.ScopeSystem},
    }
    enabled, _ := featureGate.Enabled(ctx, "dashboard", gate.WithScopeChain(chain))
    return enabled
}
```

### 4. Validate Scope Before Operations

```go
func EnableFeatureForTenant(ctx context.Context, tenantID, featureKey string) error {
    if tenantID == "" {
        return errors.New("tenant ID required")
    }

    scope := gate.ScopeRef{Kind: gate.ScopeTenant, ID: tenantID, TenantID: tenantID}
    return featureGate.Set(ctx, featureKey, scope, true, getActor(ctx))
}
```

## Next Steps

- **[GUIDE_OVERRIDES](GUIDE_OVERRIDES.md)** - Runtime override management
- **[GUIDE_ADAPTERS](GUIDE_ADAPTERS.md)** - Integration with external systems
- **[GUIDE_TESTING](GUIDE_TESTING.md)** - Testing strategies with scopes
