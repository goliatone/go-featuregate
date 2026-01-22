# Templates Guide

This guide covers using feature flags in Pongo2 templates with go-featuregate's template helpers.

## Overview

The `templates` package provides Pongo2 template functions for checking feature flags directly in your HTML templates. This enables conditional rendering based on feature enablement without requiring backend logic for every UI variation.

## Setup

### Basic Registration

Register template helpers with your Pongo2 environment:

```go
import (
    "github.com/flosch/pongo2/v6"
    "github.com/goliatone/go-featuregate/resolver"
    "github.com/goliatone/go-featuregate/templates"
)

// Create feature gate
featureGate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithOverrideStore(overrides),
)

// Get template helpers
helpers := templates.TemplateHelpers(featureGate)

// Register with Pongo2 global context
for name, fn := range helpers {
    pongo2.RegisterFilter(name, fn.(pongo2.FilterFunction))
}
```

### With go-router

When using go-router, helpers are typically registered during app setup:

```go
import (
    "github.com/goliatone/go-router"
    "github.com/goliatone/go-featuregate/templates"
)

r := router.New()

// Register helpers with router's template engine
helpers := templates.TemplateHelpers(featureGate)
for name, fn := range helpers {
    r.AddTemplateFunc(name, fn)
}
```

## Template Data Keys

The helpers look for these keys in template data:

| Key | Type | Purpose |
|-----|------|---------|
| `feature_ctx` | `context.Context` | Request context for live resolution |
| `feature_scope` | `gate.ScopeSet` or `map[string]any` | Explicit scope override |
| `feature_snapshot` | `map[string]bool` or `Snapshot` | Precomputed values |

### Setting Template Data

In your handler, pass the feature context data:

```go
func MyHandler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    data := pongo2.Context{
        // Pass context for live resolution
        "feature_ctx": ctx,

        // Or pass explicit scope
        "feature_scope": map[string]any{
            "tenant_id": "acme",
            "user_id":   "user-123",
        },

        // Other template data
        "user": currentUser,
    }

    tpl.ExecuteWriter(data, w)
}
```

## Helper Functions

### feature(key)

Check if a single feature is enabled:

```html
{% if feature("dashboard.new_layout") %}
    <div class="new-dashboard">...</div>
{% else %}
    <div class="legacy-dashboard">...</div>
{% endif %}
```

Returns `false` if the key is invalid or resolution fails.

### feature_any(keys...)

Check if ANY of the listed features are enabled:

```html
{% if feature_any("beta.features", "early.access", "preview.mode") %}
    <div class="beta-banner">
        You have access to beta features!
    </div>
{% endif %}
```

Returns `true` if at least one feature is enabled.

### feature_all(keys...)

Check if ALL listed features are enabled:

```html
{% if feature_all("payments.enabled", "checkout.v2", "stripe.integration") %}
    <button>Pay with Stripe</button>
{% else %}
    <button disabled>Payment unavailable</button>
{% endif %}
```

Returns `true` only if every feature is enabled.

### feature_none(keys...)

Check if NONE of the listed features are enabled:

```html
{% if feature_none("maintenance.mode", "system.readonly") %}
    <form method="POST">...</form>
{% else %}
    <p>System is in read-only mode</p>
{% endif %}
```

Returns `true` only if no features are enabled.

### feature_if(key, whenTrue, whenFalse)

Return a value based on feature state:

```html
<div class="{{ feature_if("dark.mode", "theme-dark", "theme-light") }}">
    ...
</div>

<button>{{ feature_if("checkout.v2", "Proceed to Checkout", "Continue") }}</button>
```

The third argument (`whenFalse`) is optional and defaults to empty string:

```html
<span class="{{ feature_if("premium.user", "badge-premium") }}">
    {{ user.name }}
</span>
```

### feature_class(key, onClass, offClass)

CSS class helper (alias for `feature_if` with class semantics):

```html
<nav class="nav {{ feature_class("sidebar.expanded", "nav-wide", "nav-compact") }}">
    ...
</nav>

<div class="{{ feature_class("animations.enabled", "animate-fade") }}">
    Content with optional animation
</div>
```

### feature_trace(key)

Debug helper that returns resolution trace information:

```html
{% if debug_mode %}
    <pre>{{ feature_trace("my.feature") | json }}</pre>
{% endif %}
```

Returns a `ResolveTrace` object with:
- `Key` - Normalized feature key
- `Value` - Resolved boolean value
- `Source` - Where the value came from (override, default, fallback)
- `Scope` - The scope used for resolution

Only available when the feature gate implements `TraceableFeatureGate`.

## Resolution Strategies

### Live Resolution

By default, helpers resolve features in real-time using the gate:

```go
data := pongo2.Context{
    "feature_ctx": r.Context(),  // Pass request context
}
```

Each `feature()` call queries the gate, which may hit the override store.

### Snapshot Resolution

For performance or consistency, precompute values before rendering:

```go
// Precompute feature values
snapshot := map[string]bool{
    "dashboard":      true,
    "beta.features":  false,
    "new.editor":     true,
}

data := pongo2.Context{
    "feature_snapshot": snapshot,
}
```

When a snapshot is provided, helpers check it first before falling back to live resolution.

### Using the Snapshot Type

For richer snapshot data including traces:

```go
import "github.com/goliatone/go-featuregate/templates"

snapshot := templates.Snapshot{
    Values: map[string]bool{
        "dashboard":     true,
        "beta.features": false,
    },
    Traces: map[string]gate.ResolveTrace{
        "dashboard": {
            Key:    "dashboard",
            Value:  true,
            Source: gate.SourceOverride,
        },
    },
}

data := pongo2.Context{
    "feature_snapshot": snapshot,
}
```

### Custom Snapshot Reader

Implement `SnapshotReader` for custom snapshot sources:

```go
type SnapshotReader interface {
    Enabled(key string) (bool, bool) // value, found
}

// Optional: include trace data
type TraceSnapshotReader interface {
    SnapshotReader
    Trace(key string) (gate.ResolveTrace, bool)
}
```

Example implementation:

```go
type CachedSnapshot struct {
    cache *redis.Client
    prefix string
}

func (s *CachedSnapshot) Enabled(key string) (bool, bool) {
    val, err := s.cache.Get(ctx, s.prefix+key).Result()
    if err != nil {
        return false, false
    }
    return val == "true", true
}
```

## Configuration Options

### Custom Data Keys

Override the default template data keys:

```go
helpers := templates.TemplateHelpers(featureGate,
    templates.WithContextKey("ctx"),
    templates.WithScopeKey("scope"),
    templates.WithSnapshotKey("features"),
)
```

Now use in templates:

```go
data := pongo2.Context{
    "ctx":      r.Context(),
    "scope":    scopeSet,
    "features": snapshot,
}
```

### Structured Errors

Enable structured error output for debugging:

```go
helpers := templates.TemplateHelpers(featureGate,
    templates.WithStructuredErrors(true),
)
```

When enabled, helpers like `feature_if` return a `TemplateError` struct on failure:

```go
type TemplateError struct {
    Helper   string         `json:"helper"`
    Type     string         `json:"type,omitempty"`
    Message  string         `json:"message,omitempty"`
    Category string         `json:"category,omitempty"`
    TextCode string         `json:"text_code,omitempty"`
    Context  map[string]any `json:"context,omitempty"`
    Metadata map[string]any `json:"metadata,omitempty"`
}
```

### Error Logging

Enable automatic error logging:

```go
import "github.com/goliatone/go-logger"

lgr := logger.New()

helpers := templates.TemplateHelpers(featureGate,
    templates.WithErrorLogging(true),
    templates.WithLogger(lgr),
)
```

Errors are logged at ERROR level with helper name, error details, and metadata.

## Passing Scope

### Via Template Data

Pass scope explicitly in template data:

```go
data := pongo2.Context{
    "feature_scope": gate.ScopeSet{
        TenantID: "acme",
        UserID:   "user-123",
    },
}
```

Or as a map:

```go
data := pongo2.Context{
    "feature_scope": map[string]any{
        "tenant_id": "acme",
        "org_id":    "engineering",
        "user_id":   "user-123",
    },
}
```

### Via Context

If scope is stored in context, pass the context:

```go
ctx := scope.WithTenantID(r.Context(), "acme")
ctx = scope.WithUserID(ctx, "user-123")

data := pongo2.Context{
    "feature_ctx": ctx,
}
```

The gate's scope resolver will extract scope from context automatically.

## Common Patterns

### Progressive Enhancement

Show enhanced UI when feature is available:

```html
<div class="editor">
    {% if feature("editor.rich_text") %}
        <div class="toolbar">
            <button>Bold</button>
            <button>Italic</button>
            <button>Link</button>
        </div>
    {% endif %}
    <textarea name="content">{{ content }}</textarea>
</div>
```

### Feature-Based Navigation

Conditionally show navigation items:

```html
<nav>
    <a href="/">Home</a>
    <a href="/dashboard">Dashboard</a>
    {% if feature("analytics.enabled") %}
        <a href="/analytics">Analytics</a>
    {% endif %}
    {% if feature_any("admin.panel", "super.user") %}
        <a href="/admin">Admin</a>
    {% endif %}
</nav>
```

### Gradual Rollout Banner

Inform users about new features:

```html
{% if feature("new.checkout") %}
    <div class="banner banner-info">
        <strong>New!</strong> Try our redesigned checkout experience.
        <a href="/checkout">Try it now</a>
    </div>
{% endif %}
```

### A/B Testing

Render different variants:

```html
{% if feature("pricing.variant_b") %}
    {% include "pricing/variant_b.html" %}
{% else %}
    {% include "pricing/variant_a.html" %}
{% endif %}
```

### Maintenance Mode

Show maintenance notice:

```html
{% if feature("maintenance.scheduled") %}
    <div class="alert alert-warning">
        Scheduled maintenance at 2:00 AM UTC.
    </div>
{% endif %}

{% if feature("maintenance.active") %}
    <div class="maintenance-page">
        <h1>We'll be back soon</h1>
        <p>We're performing scheduled maintenance.</p>
    </div>
{% else %}
    {% include "layouts/main.html" %}
{% endif %}
```

### Premium Features

Gate premium features:

```html
{% if feature("premium.analytics") %}
    <section class="analytics-dashboard">
        {% include "analytics/advanced.html" %}
    </section>
{% else %}
    <section class="analytics-teaser">
        <h3>Unlock Advanced Analytics</h3>
        <p>Upgrade to Premium for detailed insights.</p>
        <a href="/upgrade" class="btn">Upgrade Now</a>
    </section>
{% endif %}
```

### Debug Panel

Show debug info in development:

```html
{% if feature("debug.panel") %}
    <div class="debug-panel">
        <h4>Feature Flags</h4>
        <table>
            <tr>
                <td>dashboard</td>
                <td>{{ feature("dashboard") }}</td>
            </tr>
            <tr>
                <td>beta.features</td>
                <td>{{ feature("beta.features") }}</td>
            </tr>
        </table>

        <h4>Resolution Trace</h4>
        <pre>{{ feature_trace("dashboard") | json }}</pre>
    </div>
{% endif %}
```

## Performance Tips

### Use Snapshots for Repeated Checks

If checking the same features multiple times, use a snapshot:

```go
// Precompute once
snapshot := make(map[string]bool)
for _, key := range []string{"dashboard", "beta", "premium"} {
    enabled, _ := featureGate.Enabled(ctx, key)
    snapshot[key] = enabled
}

data := pongo2.Context{
    "feature_snapshot": snapshot,
}
```

### Batch Resolution

Resolve multiple features in one operation:

```go
keys := []string{"feature.a", "feature.b", "feature.c"}
snapshot := make(map[string]bool, len(keys))

for _, key := range keys {
    enabled, _ := featureGate.Enabled(ctx, key)
    snapshot[key] = enabled
}
```

### Cache at Request Level

For multi-template rendering, compute snapshot once per request:

```go
func FeatureMiddleware(gate gate.FeatureGate) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ctx := r.Context()

            // Precompute common features
            snapshot := map[string]bool{
                "dashboard":     mustEnabled(gate, ctx, "dashboard"),
                "beta.features": mustEnabled(gate, ctx, "beta.features"),
                "premium":       mustEnabled(gate, ctx, "premium"),
            }

            ctx = context.WithValue(ctx, "feature_snapshot", snapshot)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

func mustEnabled(g gate.FeatureGate, ctx context.Context, key string) bool {
    enabled, _ := g.Enabled(ctx, key)
    return enabled
}
```

## Error Handling

### Silent Failures (Default)

By default, helpers return safe defaults on error:
- `feature()` returns `false`
- `feature_any()` returns `false`
- `feature_all()` returns `false`
- `feature_none()` returns `false` (treats errors as "enabled")
- `feature_if()` returns the fallback value
- `feature_class()` returns the "off" class

### Debugging Errors

Enable structured errors for debugging:

```go
helpers := templates.TemplateHelpers(featureGate,
    templates.WithStructuredErrors(true),
    templates.WithErrorLogging(true),
    templates.WithLogger(logger),
)
```

Then in templates:

```html
{% with feature_if("my.feature", "enabled", "") as result %}
    {% if result.Helper %}
        <!-- Error occurred -->
        <div class="error">Feature check failed: {{ result.Message }}</div>
    {% else %}
        <!-- Success -->
        <div>Feature is {{ result }}</div>
    {% endif %}
{% endwith %}
```

## Next Steps

- **[GUIDE_SCOPES](GUIDE_SCOPES.md)** - Multi-tenant scope configuration
- **[GUIDE_RESOLUTION](GUIDE_RESOLUTION.md)** - Understanding resolution hierarchy
- **[GUIDE_ERRORS](GUIDE_ERRORS.md)** - Error handling patterns
