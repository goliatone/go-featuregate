# Hooks Guide

This guide covers the hook system in go-featuregate for subscribing to feature flag events.

## Overview

go-featuregate provides two types of hooks:

| Hook Type | Purpose | Event |
|-----------|---------|-------|
| **Resolve Hook** | Observes feature resolution | Every `Enabled()` call |
| **Activity Hook** | Observes mutations | `Set()` and `Unset()` calls |

Hooks enable:
- Logging and debugging
- Metrics collection
- Audit trails
- Real-time notifications
- Analytics integration

## Resolve Hooks

Resolve hooks fire after every feature resolution, regardless of success or failure.

### ResolveHook Interface

```go
type ResolveHook interface {
    OnResolve(ctx context.Context, event ResolveEvent)
}
```

### ResolveEvent Structure

```go
type ResolveEvent struct {
    Key           string        // Original feature key
    NormalizedKey string        // Normalized key used for lookup
    Scope         ScopeSet      // Scope used for resolution
    Value         bool          // Resolved value
    Source        ResolveSource // Where the value came from
    Error         error         // Resolution error (if any)
    Trace         ResolveTrace  // Full resolution trace
}
```

### ResolveSource Values

```go
const (
    ResolveSourceOverride ResolveSource = "override"  // From override store
    ResolveSourceDefault  ResolveSource = "default"   // From config defaults
    ResolveSourceFallback ResolveSource = "fallback"  // No value found (false)
)
```

### Registering Resolve Hooks

```go
import (
    "github.com/goliatone/go-featuregate/resolver"
    "github.com/goliatone/go-featuregate/gate"
)

// Using an interface implementation
type MetricsHook struct {
    metrics MetricsClient
}

func (h *MetricsHook) OnResolve(ctx context.Context, event gate.ResolveEvent) {
    h.metrics.Increment("feature.resolve", map[string]string{
        "key":    event.NormalizedKey,
        "source": string(event.Source),
    })
}

hook := &MetricsHook{metrics: metricsClient}

featureGate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithResolveHook(hook),
)
```

### Using ResolveHookFunc

For simple hooks, use the function adapter:

```go
hook := gate.ResolveHookFunc(func(ctx context.Context, event gate.ResolveEvent) {
    log.Printf("resolved %s = %v (source: %s)",
        event.NormalizedKey,
        event.Value,
        event.Source,
    )
})

featureGate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithResolveHook(hook),
)
```

### ResolveTrace Details

The `Trace` field contains detailed resolution information:

```go
type ResolveTrace struct {
    Key           string        // Original key
    NormalizedKey string        // Normalized key
    Scope         ScopeSet      // Resolution scope
    Value         bool          // Final resolved value
    Source        ResolveSource // Value source
    Override      OverrideTrace // Override layer details
    Default       DefaultTrace  // Default layer details
    CacheHit      bool          // Whether value came from cache
}

type OverrideTrace struct {
    State OverrideState // enabled, disabled, unset, missing
    Value *bool         // The override value (if present)
    Error error         // Store error (if any)
}

type DefaultTrace struct {
    Set   bool  // Whether a default was configured
    Value bool  // The default value
    Error error // Defaults error (if any)
}
```

Example using trace data:

```go
hook := gate.ResolveHookFunc(func(ctx context.Context, event gate.ResolveEvent) {
    trace := event.Trace

    log.Printf("Feature: %s", trace.NormalizedKey)
    log.Printf("  Value: %v (from %s)", trace.Value, trace.Source)
    log.Printf("  Cache hit: %v", trace.CacheHit)

    if trace.Override.State != gate.OverrideStateMissing {
        log.Printf("  Override: state=%s value=%v",
            trace.Override.State,
            trace.Override.Value,
        )
    }

    if trace.Default.Set {
        log.Printf("  Default: %v", trace.Default.Value)
    }
})
```

## Activity Hooks

Activity hooks fire when feature flags are mutated via `Set()` or `Unset()`.

### Activity Hook Interface

```go
type Hook interface {
    OnUpdate(ctx context.Context, event UpdateEvent)
}
```

### UpdateEvent Structure

```go
type UpdateEvent struct {
    Key           string      // Original feature key
    NormalizedKey string      // Normalized key
    Scope         ScopeSet    // Target scope for the mutation
    Actor         ActorRef    // Who made the change
    Action        Action      // "set" or "unset"
    Value         *bool       // New value (nil for unset)
}
```

### Action Constants

```go
const (
    ActionSet   Action = "set"   // Feature enabled or disabled
    ActionUnset Action = "unset" // Override removed
)
```

### Registering Activity Hooks

```go
import (
    "github.com/goliatone/go-featuregate/resolver"
    "github.com/goliatone/go-featuregate/activity"
)

// Using an interface implementation
type AuditHook struct {
    store AuditStore
}

func (h *AuditHook) OnUpdate(ctx context.Context, event activity.UpdateEvent) {
    record := &AuditRecord{
        FeatureKey: event.NormalizedKey,
        Action:     string(event.Action),
        ActorID:    event.Actor.ID,
        ActorType:  event.Actor.Type,
        TenantID:   event.Scope.TenantID,
        Timestamp:  time.Now(),
    }
    if event.Value != nil {
        record.NewValue = *event.Value
    }
    h.store.Save(ctx, record)
}

hook := &AuditHook{store: auditStore}

featureGate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithOverrideStore(overrides),
    resolver.WithActivityHook(hook),
)
```

### Using HookFunc

```go
hook := activity.HookFunc(func(ctx context.Context, event activity.UpdateEvent) {
    log.Printf("feature %s %s by %s (%s)",
        event.NormalizedKey,
        event.Action,
        event.Actor.Name,
        event.Actor.ID,
    )
})

featureGate := resolver.New(
    resolver.WithActivityHook(hook),
)
```

## go-logger Adapter

The `gologgeradapter` provides a pre-built hook that implements both `ResolveHook` and `activity.Hook`.

### Basic Setup

```go
import (
    "github.com/goliatone/go-featuregate/adapters/gologgeradapter"
    "github.com/goliatone/go-logger"
)

lgr := logger.New()
hook := gologgeradapter.New(lgr)

featureGate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithOverrideStore(overrides),
    resolver.WithResolveHook(hook),
    resolver.WithActivityHook(hook),
)
```

### Configuration Options

```go
hook := gologgeradapter.New(lgr,
    // Set log levels
    gologgeradapter.WithResolveLevel("debug"),  // default: "debug"
    gologgeradapter.WithUpdateLevel("info"),     // default: "info"

    // Customize log messages
    gologgeradapter.WithResolveMessage("feature.resolved"),
    gologgeradapter.WithUpdateMessage("feature.updated"),
)
```

### Log Output

Resolve events (debug level):
```
[DEBUG] featuregate.resolve
    feature_key=dashboard
    feature_key_norm=dashboard
    feature_value=true
    feature_source=override
    feature_cache_hit=false
    feature_override=enabled
    tenant_id=acme
```

Update events (info level):
```
[INFO] featuregate.update
    feature_key=dashboard
    feature_key_norm=dashboard
    feature_action=set
    feature_value=true
    actor_id=admin-123
    actor_type=user
    tenant_id=acme
```

## Common Integrations

### Metrics Collection

```go
type PrometheusHook struct {
    resolveCounter *prometheus.CounterVec
    resolveLatency *prometheus.HistogramVec
}

func NewPrometheusHook() *PrometheusHook {
    return &PrometheusHook{
        resolveCounter: promauto.NewCounterVec(
            prometheus.CounterOpts{
                Name: "feature_resolve_total",
                Help: "Total feature flag resolutions",
            },
            []string{"key", "source", "value"},
        ),
        resolveLatency: promauto.NewHistogramVec(
            prometheus.HistogramOpts{
                Name: "feature_resolve_duration_seconds",
                Help: "Feature resolution duration",
            },
            []string{"key"},
        ),
    }
}

func (h *PrometheusHook) OnResolve(ctx context.Context, event gate.ResolveEvent) {
    h.resolveCounter.WithLabelValues(
        event.NormalizedKey,
        string(event.Source),
        strconv.FormatBool(event.Value),
    ).Inc()
}
```

### Audit Trail

```go
type AuditTrailHook struct {
    db *sql.DB
}

func (h *AuditTrailHook) OnUpdate(ctx context.Context, event activity.UpdateEvent) {
    var value *bool
    if event.Value != nil {
        v := *event.Value
        value = &v
    }

    _, err := h.db.ExecContext(ctx, `
        INSERT INTO feature_audit_log
            (feature_key, action, value, actor_id, actor_type, tenant_id, org_id, user_id, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
    `,
        event.NormalizedKey,
        string(event.Action),
        value,
        event.Actor.ID,
        event.Actor.Type,
        event.Scope.TenantID,
        event.Scope.OrgID,
        event.Scope.UserID,
        time.Now(),
    )
    if err != nil {
        log.Printf("audit trail error: %v", err)
    }
}
```

### Real-Time Notifications

```go
type WebhookHook struct {
    client  *http.Client
    url     string
    timeout time.Duration
}

func (h *WebhookHook) OnUpdate(ctx context.Context, event activity.UpdateEvent) {
    payload := map[string]any{
        "event":      "feature.updated",
        "feature":    event.NormalizedKey,
        "action":     string(event.Action),
        "actor":      event.Actor.ID,
        "tenant":     event.Scope.TenantID,
        "timestamp":  time.Now().Unix(),
    }
    if event.Value != nil {
        payload["value"] = *event.Value
    }

    body, _ := json.Marshal(payload)

    ctx, cancel := context.WithTimeout(ctx, h.timeout)
    defer cancel()

    req, _ := http.NewRequestWithContext(ctx, "POST", h.url, bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")

    resp, err := h.client.Do(req)
    if err != nil {
        log.Printf("webhook error: %v", err)
        return
    }
    defer resp.Body.Close()
}
```

### Analytics Integration

```go
type AnalyticsHook struct {
    client AnalyticsClient
}

func (h *AnalyticsHook) OnResolve(ctx context.Context, event gate.ResolveEvent) {
    // Track feature flag usage for analytics
    h.client.Track(analytics.Event{
        Name: "feature_flag_checked",
        Properties: map[string]any{
            "feature_key": event.NormalizedKey,
            "enabled":     event.Value,
            "source":      string(event.Source),
            "cache_hit":   event.Trace.CacheHit,
        },
        UserID:   event.Scope.UserID,
        TenantID: event.Scope.TenantID,
    })
}

func (h *AnalyticsHook) OnUpdate(ctx context.Context, event activity.UpdateEvent) {
    h.client.Track(analytics.Event{
        Name: "feature_flag_changed",
        Properties: map[string]any{
            "feature_key": event.NormalizedKey,
            "action":      string(event.Action),
            "actor_id":    event.Actor.ID,
        },
        TenantID: event.Scope.TenantID,
    })
}
```

### Multiple Hooks

Register multiple hooks by composing them:

```go
type CompositeResolveHook struct {
    hooks []gate.ResolveHook
}

func (c *CompositeResolveHook) OnResolve(ctx context.Context, event gate.ResolveEvent) {
    for _, hook := range c.hooks {
        hook.OnResolve(ctx, event)
    }
}

composite := &CompositeResolveHook{
    hooks: []gate.ResolveHook{
        metricsHook,
        loggingHook,
        analyticsHook,
    },
}

featureGate := resolver.New(
    resolver.WithResolveHook(composite),
)
```

## Performance Considerations

### Async Hooks

For expensive operations, use async hooks:

```go
type AsyncActivityHook struct {
    events chan activity.UpdateEvent
    hook   activity.Hook
}

func NewAsyncActivityHook(hook activity.Hook, bufferSize int) *AsyncActivityHook {
    h := &AsyncActivityHook{
        events: make(chan activity.UpdateEvent, bufferSize),
        hook:   hook,
    }
    go h.process()
    return h
}

func (h *AsyncActivityHook) OnUpdate(ctx context.Context, event activity.UpdateEvent) {
    select {
    case h.events <- event:
    default:
        // Buffer full, log and drop
        log.Println("activity hook buffer full, event dropped")
    }
}

func (h *AsyncActivityHook) process() {
    for event := range h.events {
        h.hook.OnUpdate(context.Background(), event)
    }
}

func (h *AsyncActivityHook) Close() {
    close(h.events)
}
```

### Sampling Resolve Events

For high-traffic systems, sample resolve events:

```go
type SampledResolveHook struct {
    hook       gate.ResolveHook
    sampleRate float64 // 0.0 to 1.0
}

func (h *SampledResolveHook) OnResolve(ctx context.Context, event gate.ResolveEvent) {
    if rand.Float64() < h.sampleRate {
        h.hook.OnResolve(ctx, event)
    }
}

// Log 10% of resolve events
sampledHook := &SampledResolveHook{
    hook:       loggingHook,
    sampleRate: 0.1,
}
```

### Conditional Hooks

Skip hooks for certain features:

```go
type FilteredResolveHook struct {
    hook    gate.ResolveHook
    include map[string]bool
}

func (h *FilteredResolveHook) OnResolve(ctx context.Context, event gate.ResolveEvent) {
    if h.include[event.NormalizedKey] {
        h.hook.OnResolve(ctx, event)
    }
}

// Only log specific features
filteredHook := &FilteredResolveHook{
    hook: loggingHook,
    include: map[string]bool{
        "critical.feature": true,
        "monitored.flag":   true,
    },
}
```

## Testing Hooks

### Capture Hook for Tests

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
    capture := &CaptureResolveHook{}

    gate := resolver.New(
        resolver.WithResolveHook(capture),
    )

    gate.Enabled(context.Background(), "test.feature")

    assert.Len(t, capture.Events, 1)
    assert.Equal(t, "test.feature", capture.Events[0].NormalizedKey)
    assert.Equal(t, gate.ResolveSourceFallback, capture.Events[0].Source)
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
    capture := &CaptureActivityHook{}
    overrides := store.NewMemoryStore()

    gate := resolver.New(
        resolver.WithOverrideStore(overrides),
        resolver.WithActivityHook(capture),
    )

    actor := gate.ActorRef{ID: "test-user", Type: "user"}
    scope := gate.ScopeSet{TenantID: "acme"}

    gate.Set(context.Background(), "feature", scope, true, actor)

    assert.Len(t, capture.Events, 1)
    assert.Equal(t, activity.ActionSet, capture.Events[0].Action)
    assert.Equal(t, "test-user", capture.Events[0].Actor.ID)
}
```

## Best Practices

### 1. Keep Hooks Fast

Hooks run synchronously in the resolution path. Keep them lightweight:

```go
// Good - fast operation
func (h *Hook) OnResolve(ctx context.Context, event gate.ResolveEvent) {
    h.counter.Inc()
}

// Avoid - slow operation in sync hook
func (h *Hook) OnResolve(ctx context.Context, event gate.ResolveEvent) {
    h.db.Insert(ctx, event) // Slow!
}
```

### 2. Handle Errors Gracefully

Never panic in hooks:

```go
func (h *Hook) OnUpdate(ctx context.Context, event activity.UpdateEvent) {
    defer func() {
        if r := recover(); r != nil {
            log.Printf("hook panic recovered: %v", r)
        }
    }()

    if err := h.process(event); err != nil {
        log.Printf("hook error: %v", err)
    }
}
```

### 3. Use Context for Cancellation

Respect context cancellation in async hooks:

```go
func (h *AsyncHook) process(ctx context.Context, event activity.UpdateEvent) {
    select {
    case <-ctx.Done():
        return
    default:
        h.hook.OnUpdate(ctx, event)
    }
}
```

### 4. Document Hook Behavior

```go
// MetricsHook increments Prometheus counters for feature resolutions.
// It is safe for concurrent use and adds minimal latency (<1ms).
type MetricsHook struct {
    // ...
}
```

## Next Steps

- **[GUIDE_RESOLUTION](GUIDE_RESOLUTION.md)** - Understanding resolution flow
- **[GUIDE_OVERRIDES](GUIDE_OVERRIDES.md)** - Runtime mutations
- **[GUIDE_ADAPTERS](GUIDE_ADAPTERS.md)** - Using gologgeradapter
- **[GUIDE_TESTING](GUIDE_TESTING.md)** - Testing with hooks
