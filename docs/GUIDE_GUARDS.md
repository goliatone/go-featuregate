# Guards Guide

This guide covers the guard pattern for enforcing feature flag requirements in your Go code.

## Overview

Guards provide a declarative way to require feature flags before executing code. Instead of manually checking flags and handling the disabled case, guards encapsulate this logic with consistent error handling.

```go
// Instead of this:
enabled, err := gate.Enabled(ctx, "premium.api")
if err != nil {
    return err
}
if !enabled {
    return errors.New("feature disabled")
}
// ... proceed with operation

// Use this:
if err := guard.Require(ctx, gate, "premium.api"); err != nil {
    return err
}
// ... proceed with operation
```

## Basic Usage

### Simple Guard

```go
import (
    "context"
    "github.com/goliatone/go-featuregate/gate/guard"
)

func ProcessPayment(ctx context.Context, order *Order) error {
    if err := guard.Require(ctx, featureGate, "payments.enabled"); err != nil {
        return err
    }

    // Feature is enabled, proceed with payment
    return processPaymentInternal(order)
}
```

### Nil Gate Handling

Guards safely handle nil gates by allowing access:

```go
var featureGate gate.FeatureGate = nil

// Returns nil (no error) - feature access allowed
err := guard.Require(ctx, featureGate, "any.feature")
```

This enables optional feature gating where the gate may not be configured.

## Error Types

### DisabledError

When a feature is disabled, `Require` returns a `DisabledError`:

```go
err := guard.Require(ctx, gate, "premium.feature")
if err != nil {
    var disabled guard.DisabledError
    if errors.As(err, &disabled) {
        fmt.Printf("Feature %s is disabled\n", disabled.Key)
    }
}
```

`DisabledError` unwraps to `guard.ErrFeatureDisabled`:

```go
if errors.Is(err, guard.ErrFeatureDisabled) {
    // Handle disabled feature
}
```

### Gate Errors

Resolution errors from the gate are passed through:

```go
err := guard.Require(ctx, gate, "feature")
if err != nil {
    if errors.Is(err, guard.ErrFeatureDisabled) {
        // Feature is disabled
    } else {
        // Gate resolution error (store failure, etc.)
    }
}
```

## Configuration Options

### WithDisabledError

Customize the error returned when a feature is disabled:

```go
import "net/http"

var ErrPremiumRequired = errors.New("premium subscription required")

func PremiumHandler(w http.ResponseWriter, r *http.Request) {
    err := guard.Require(r.Context(), gate, "premium.access",
        guard.WithDisabledError(ErrPremiumRequired),
    )
    if err != nil {
        if errors.Is(err, ErrPremiumRequired) {
            http.Error(w, "Upgrade to Premium", http.StatusPaymentRequired)
            return
        }
        http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
        return
    }

    // Serve premium content
}
```

### WithOverrides

Specify fallback feature keys when the primary key is disabled:

```go
// Access granted if ANY of these features is enabled
err := guard.Require(ctx, gate, "feature.v2",
    guard.WithOverrides("feature.v1", "feature.beta"),
)
```

Use cases:
- Gradual migration from old to new features
- Beta access alongside production features
- Admin override keys

```go
// Allow if new_checkout OR admin.bypass is enabled
err := guard.Require(ctx, gate, "checkout.v2",
    guard.WithOverrides("admin.bypass"),
)
```

### WithErrorMapper

Transform errors before returning:

```go
func mapToHTTPError(err error) error {
    if errors.Is(err, guard.ErrFeatureDisabled) {
        return &HTTPError{
            Code:    http.StatusForbidden,
            Message: "Feature not available",
        }
    }
    return &HTTPError{
        Code:    http.StatusInternalServerError,
        Message: "Service error",
    }
}

err := guard.Require(ctx, gate, "feature",
    guard.WithErrorMapper(mapToHTTPError),
)
```

## Common Patterns

### API Endpoint Protection

Guard entire endpoints:

```go
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
    if err := guard.Require(r.Context(), h.gate, "orders.create"); err != nil {
        h.handleFeatureError(w, err)
        return
    }

    // Process order creation
}

func (h *Handler) handleFeatureError(w http.ResponseWriter, err error) {
    if errors.Is(err, guard.ErrFeatureDisabled) {
        http.Error(w, "Feature not available", http.StatusForbidden)
        return
    }
    http.Error(w, "Internal error", http.StatusInternalServerError)
}
```

### Command/Handler Gating

Guard CQRS command handlers:

```go
type CreateUserCommand struct {
    Email string
    Name  string
}

type CreateUserHandler struct {
    gate gate.FeatureGate
    repo UserRepository
}

func (h *CreateUserHandler) Handle(ctx context.Context, cmd CreateUserCommand) error {
    if err := guard.Require(ctx, h.gate, "users.registration"); err != nil {
        return fmt.Errorf("registration disabled: %w", err)
    }

    user := &User{Email: cmd.Email, Name: cmd.Name}
    return h.repo.Create(ctx, user)
}
```

### Service Method Gating

Guard service layer methods:

```go
type PaymentService struct {
    gate     gate.FeatureGate
    provider PaymentProvider
}

func (s *PaymentService) ProcessRefund(ctx context.Context, orderID string) error {
    if err := guard.Require(ctx, s.gate, "payments.refunds"); err != nil {
        return err
    }

    return s.provider.Refund(ctx, orderID)
}

func (s *PaymentService) ProcessSubscription(ctx context.Context, plan string) error {
    // Require subscriptions feature, fallback to payments.enabled
    if err := guard.Require(ctx, s.gate, "payments.subscriptions",
        guard.WithOverrides("payments.enabled"),
    ); err != nil {
        return err
    }

    return s.provider.Subscribe(ctx, plan)
}
```

### Middleware Integration

Create reusable middleware:

```go
func RequireFeature(gate gate.FeatureGate, key string, opts ...guard.Option) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if err := guard.Require(r.Context(), gate, key, opts...); err != nil {
                if errors.Is(err, guard.ErrFeatureDisabled) {
                    http.Error(w, "Feature not available", http.StatusForbidden)
                    return
                }
                http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}

// Usage
mux.Handle("/api/v2/", RequireFeature(gate, "api.v2")(apiV2Handler))
mux.Handle("/admin/", RequireFeature(gate, "admin.panel",
    guard.WithOverrides("super.admin"),
)(adminHandler))
```

### Chi Router Integration

```go
import "github.com/go-chi/chi/v5"

r := chi.NewRouter()

// Apply to route group
r.Route("/premium", func(r chi.Router) {
    r.Use(RequireFeature(gate, "premium.features"))
    r.Get("/dashboard", premiumDashboard)
    r.Get("/analytics", premiumAnalytics)
})
```

### Gin Integration

```go
import "github.com/gin-gonic/gin"

func FeatureGuard(gate gate.FeatureGate, key string) gin.HandlerFunc {
    return func(c *gin.Context) {
        if err := guard.Require(c.Request.Context(), gate, key); err != nil {
            if errors.Is(err, guard.ErrFeatureDisabled) {
                c.AbortWithStatusJSON(403, gin.H{"error": "Feature not available"})
                return
            }
            c.AbortWithStatusJSON(500, gin.H{"error": "Service error"})
            return
        }
        c.Next()
    }
}

// Usage
r.GET("/premium", FeatureGuard(gate, "premium"), premiumHandler)
```

### gRPC Interceptor

```go
import (
    "google.golang.org/grpc"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
)

func FeatureUnaryInterceptor(gate gate.FeatureGate, featureKey string) grpc.UnaryServerInterceptor {
    return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
        if err := guard.Require(ctx, gate, featureKey); err != nil {
            if errors.Is(err, guard.ErrFeatureDisabled) {
                return nil, status.Error(codes.PermissionDenied, "feature disabled")
            }
            return nil, status.Error(codes.Internal, "feature check failed")
        }
        return handler(ctx, req)
    }
}
```

### Conditional Logic Gating

Guard specific code paths:

```go
func (s *OrderService) CalculateDiscount(ctx context.Context, order *Order) (float64, error) {
    var discount float64

    // Apply loyalty discount if feature enabled
    if err := guard.Require(ctx, s.gate, "discounts.loyalty"); err == nil {
        discount += s.calculateLoyaltyDiscount(order)
    }

    // Apply promo codes if feature enabled
    if err := guard.Require(ctx, s.gate, "discounts.promo_codes"); err == nil {
        discount += s.calculatePromoDiscount(order)
    }

    return discount, nil
}
```

### Multiple Guards

Check multiple independent features:

```go
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    data := &DashboardData{}

    // Each feature independently guarded
    if err := guard.Require(ctx, h.gate, "dashboard.analytics"); err == nil {
        data.Analytics = h.loadAnalytics(ctx)
    }

    if err := guard.Require(ctx, h.gate, "dashboard.reports"); err == nil {
        data.Reports = h.loadReports(ctx)
    }

    if err := guard.Require(ctx, h.gate, "dashboard.realtime"); err == nil {
        data.Realtime = h.loadRealtime(ctx)
    }

    h.render(w, "dashboard", data)
}
```

### Scoped Guards

Use guards with explicit scopes:

```go
func (s *TenantService) EnableFeature(ctx context.Context, tenantID, feature string) error {
    // Create scoped context
    scopedCtx := scope.WithTenantID(ctx, tenantID)

    // Guard checks feature for this specific tenant
    if err := guard.Require(scopedCtx, s.gate, "tenant.self_service"); err != nil {
        return fmt.Errorf("self-service disabled for tenant %s: %w", tenantID, err)
    }

    // Enable the feature for the tenant
    return s.gate.Set(ctx, feature, gate.ScopeSet{TenantID: tenantID}, true, s.actor(ctx))
}
```

## Error Handling Patterns

### HTTP Response Mapping

```go
func featureErrorToHTTP(err error) (int, string) {
    if err == nil {
        return http.StatusOK, ""
    }

    if errors.Is(err, guard.ErrFeatureDisabled) {
        var disabled guard.DisabledError
        if errors.As(err, &disabled) {
            return http.StatusForbidden, fmt.Sprintf("Feature '%s' is not available", disabled.Key)
        }
        return http.StatusForbidden, "Feature not available"
    }

    // Gate resolution error
    return http.StatusServiceUnavailable, "Unable to check feature availability"
}
```

### Logging Guard Failures

```go
func (h *Handler) guardedHandler(w http.ResponseWriter, r *http.Request) {
    err := guard.Require(r.Context(), h.gate, "my.feature")
    if err != nil {
        h.logger.Warn("feature guard failed",
            "feature", "my.feature",
            "error", err,
            "user_id", getUserID(r),
            "path", r.URL.Path,
        )

        if errors.Is(err, guard.ErrFeatureDisabled) {
            http.Error(w, "Feature not available", http.StatusForbidden)
            return
        }
        http.Error(w, "Service error", http.StatusInternalServerError)
        return
    }

    // Continue with handler
}
```

### Custom Error Types

```go
type FeatureError struct {
    Feature    string
    Reason     string
    StatusCode int
}

func (e *FeatureError) Error() string {
    return fmt.Sprintf("feature %s: %s", e.Feature, e.Reason)
}

func guardWithContext(ctx context.Context, gate gate.FeatureGate, key string) error {
    err := guard.Require(ctx, gate, key)
    if err == nil {
        return nil
    }

    if errors.Is(err, guard.ErrFeatureDisabled) {
        return &FeatureError{
            Feature:    key,
            Reason:     "disabled",
            StatusCode: http.StatusForbidden,
        }
    }

    return &FeatureError{
        Feature:    key,
        Reason:     "check failed",
        StatusCode: http.StatusInternalServerError,
    }
}
```

## Testing Guards

### Basic Guard Test

```go
func TestGuard(t *testing.T) {
    overrides := store.NewMemoryStore()
    gate := resolver.New(resolver.WithOverrideStore(overrides))

    ctx := context.Background()
    actor := gate.ActorRef{ID: "test"}

    // Feature disabled by default
    err := guard.Require(ctx, gate, "my.feature")
    assert.ErrorIs(t, err, guard.ErrFeatureDisabled)

    // Enable feature
    gate.Set(ctx, "my.feature", gate.ScopeSet{System: true}, true, actor)

    // Guard should pass
    err = guard.Require(ctx, gate, "my.feature")
    assert.NoError(t, err)
}
```

### Testing Override Keys

```go
func TestGuardWithOverrides(t *testing.T) {
    overrides := store.NewMemoryStore()
    gate := resolver.New(resolver.WithOverrideStore(overrides))

    ctx := context.Background()
    actor := gate.ActorRef{ID: "test"}

    // Primary feature disabled
    err := guard.Require(ctx, gate, "feature.v2",
        guard.WithOverrides("feature.v1"),
    )
    assert.ErrorIs(t, err, guard.ErrFeatureDisabled)

    // Enable fallback
    gate.Set(ctx, "feature.v1", gate.ScopeSet{System: true}, true, actor)

    // Guard should pass via override
    err = guard.Require(ctx, gate, "feature.v2",
        guard.WithOverrides("feature.v1"),
    )
    assert.NoError(t, err)
}
```

### Testing Custom Errors

```go
func TestGuardCustomError(t *testing.T) {
    gate := resolver.New()
    ctx := context.Background()

    customErr := errors.New("premium required")

    err := guard.Require(ctx, gate, "premium.feature",
        guard.WithDisabledError(customErr),
    )

    assert.ErrorIs(t, err, customErr)
}
```

## Best Practices

### 1. Use Descriptive Feature Keys

```go
// Good
guard.Require(ctx, gate, "payments.stripe.subscription")

// Avoid
guard.Require(ctx, gate, "feature1")
```

### 2. Guard at Service Boundaries

Guard at API/service entry points rather than deep in business logic:

```go
// Good - guard at handler level
func (h *Handler) CreateSubscription(w http.ResponseWriter, r *http.Request) {
    if err := guard.Require(r.Context(), h.gate, "subscriptions.create"); err != nil {
        h.handleError(w, err)
        return
    }
    h.service.CreateSubscription(r.Context(), req)
}

// Avoid - guard buried in service
func (s *Service) CreateSubscription(ctx context.Context, req *Request) error {
    if err := guard.Require(ctx, s.gate, "subscriptions.create"); err != nil {
        return err
    }
    // ...
}
```

### 3. Document Guard Requirements

```go
// CreateOrder creates a new order.
// Requires: orders.create feature flag
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
    if err := guard.Require(r.Context(), h.gate, "orders.create"); err != nil {
        // ...
    }
}
```

### 4. Use Consistent Error Handling

Create helpers for consistent responses:

```go
func (h *Handler) requireFeature(w http.ResponseWriter, r *http.Request, key string) bool {
    if err := guard.Require(r.Context(), h.gate, key); err != nil {
        h.handleFeatureError(w, err, key)
        return false
    }
    return true
}

func (h *Handler) MyHandler(w http.ResponseWriter, r *http.Request) {
    if !h.requireFeature(w, r, "my.feature") {
        return
    }
    // Continue...
}
```

## Next Steps

- **[GUIDE_RESOLUTION](GUIDE_RESOLUTION.md)** - Understanding feature resolution
- **[GUIDE_SCOPES](GUIDE_SCOPES.md)** - Multi-tenant scoping
- **[GUIDE_ERRORS](GUIDE_ERRORS.md)** - Error handling patterns
- **[GUIDE_TESTING](GUIDE_TESTING.md)** - Testing strategies
