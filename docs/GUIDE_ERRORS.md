# Errors Guide

This guide covers error handling in go-featuregate using the `ferrors` package.

## Overview

go-featuregate uses structured errors built on `go-errors` for rich error context. Errors include:

- **Category** - Classification (BadInput, Operation, External, Internal)
- **Text Code** - Machine-readable error identifier
- **Message** - Human-readable description
- **Metadata** - Contextual information for debugging

## Error Categories

| Category | Description | Examples |
|----------|-------------|----------|
| `BadInput` | Invalid user input | Empty feature key, invalid scope |
| `Operation` | Business logic failure | Store not configured, resolver missing |
| `External` | External dependency failure | Database error, network timeout |
| `Internal` | Unexpected internal error | Nil pointer, assertion failure |

## Sentinel Errors

go-featuregate defines sentinel errors for common conditions:

```go
import "github.com/goliatone/go-featuregate/ferrors"

// Check for specific error types
if errors.Is(err, ferrors.ErrInvalidKey) {
    // Feature key was empty or invalid
}

if errors.Is(err, ferrors.ErrStoreRequired) {
    // Override store not configured
}
```

### Available Sentinels

| Sentinel | Text Code | Description |
|----------|-----------|-------------|
| `ErrInvalidKey` | `FEATURE_KEY_REQUIRED` | Feature key is empty or invalid |
| `ErrStoreUnavailable` | `OVERRIDE_STORE_REQUIRED` | Override store not configured |
| `ErrStoreRequired` | `STORE_REQUIRED` | Store is nil when required |
| `ErrResolverRequired` | `RESOLVER_REQUIRED` | Resolver is nil |
| `ErrGateRequired` | `FEATURE_GATE_REQUIRED` | Feature gate is nil |
| `ErrScopeRequired` | `SCOPE_REQUIRED` | Scope is required but missing |
| `ErrSnapshotRequired` | `SNAPSHOT_REQUIRED` | Snapshot is nil |
| `ErrPathRequired` | `PATH_REQUIRED` | Path is empty |
| `ErrPathInvalid` | `PATH_INVALID` | Path segment is not a map |
| `ErrPreferencesStoreRequired` | `PREFERENCES_STORE_REQUIRED` | Preferences store is nil |

## Text Codes

Text codes provide machine-readable error identification for logging and monitoring:

### Input Errors

| Code | Description |
|------|-------------|
| `FEATURE_KEY_REQUIRED` | Feature key is empty or invalid |
| `SCOPE_REQUIRED` | Scope is required but missing |
| `SCOPE_INVALID` | Scope format is invalid |
| `PATH_REQUIRED` | Path is empty |
| `PATH_INVALID` | Path segment is not a map |

### Operation Errors

| Code | Description |
|------|-------------|
| `OVERRIDE_STORE_REQUIRED` | Override store not configured |
| `STORE_REQUIRED` | Store is nil when required |
| `RESOLVER_REQUIRED` | Resolver is nil |
| `FEATURE_GATE_REQUIRED` | Feature gate is nil |
| `PREFERENCES_STORE_REQUIRED` | Preferences store is nil |
| `SNAPSHOT_REQUIRED` | Snapshot is nil |

### External Errors

| Code | Description |
|------|-------------|
| `ADAPTER_FAILED` | Adapter operation failed |
| `STORE_READ_FAILED` | Store read operation failed |
| `STORE_WRITE_FAILED` | Store write operation failed |
| `DEFAULT_LOOKUP_FAILED` | Default value lookup failed |
| `SCOPE_RESOLVE_FAILED` | Scope resolution failed |

## Metadata Keys

Errors include metadata for debugging:

```go
const (
    MetaFeatureKey           = "feature_key"      // Original feature key
    MetaFeatureKeyNormalized = "feature_key_norm" // Normalized key
    MetaScope                = "scope"            // ScopeSet used
    MetaStore                = "store"            // Store name
    MetaAdapter              = "adapter"          // Adapter name
    MetaDomain               = "domain"           // Domain name
    MetaTable                = "table"            // Database table
    MetaOperation            = "operation"        // Operation name
    MetaStrict               = "strict"           // Strict mode enabled
    MetaPath                 = "path"             // Path value
)
```

## Extracting Error Details

### Using ferrors.As

Extract rich error information:

```go
if err := gate.Enabled(ctx, key); err != nil {
    if rich, ok := ferrors.As(err); ok {
        log.Printf("Error: %s", rich.Message)
        log.Printf("Code: %s", rich.TextCode)
        log.Printf("Category: %s", rich.Category)

        // Access metadata
        if featureKey, ok := rich.Metadata[ferrors.MetaFeatureKey].(string); ok {
            log.Printf("Feature key: %s", featureKey)
        }
    }
}
```

### Pattern Matching

```go
func handleFeatureError(err error) {
    if err == nil {
        return
    }

    // Check for specific sentinel errors
    switch {
    case errors.Is(err, ferrors.ErrInvalidKey):
        log.Println("Invalid feature key provided")

    case errors.Is(err, ferrors.ErrStoreUnavailable):
        log.Println("Override store not configured")

    default:
        // Extract rich error details
        if rich, ok := ferrors.As(err); ok {
            switch rich.TextCode {
            case ferrors.TextCodeStoreReadFailed:
                log.Printf("Store read failed: %s", rich.Message)
            case ferrors.TextCodeAdapterFailed:
                log.Printf("Adapter failed: %s", rich.Message)
            default:
                log.Printf("Feature error: %s", err)
            }
        } else {
            log.Printf("Unknown error: %s", err)
        }
    }
}
```

## Creating Errors

### Creating New Errors

```go
// Simple error with category
err := ferrors.NewBadInput(
    "CUSTOM_ERROR",
    "custom error message",
    map[string]any{
        "custom_field": "value",
    },
)

// Operation error
err := ferrors.NewOperation(
    ferrors.TextCodeStoreWriteFailed,
    "failed to write override",
    map[string]any{
        ferrors.MetaFeatureKey: key,
        ferrors.MetaStore:      "bun",
    },
)

// External error
err := ferrors.NewExternal(
    ferrors.TextCodeAdapterFailed,
    "database connection failed",
    map[string]any{
        ferrors.MetaAdapter: "bunadapter",
    },
)

// Internal error
err := ferrors.NewInternal(
    "UNEXPECTED_STATE",
    "unexpected nil pointer",
    nil,
)
```

### Wrapping Errors

Wrap existing errors to add context:

```go
// Wrap with category
if err := db.Query(ctx, query); err != nil {
    return ferrors.WrapExternal(
        err,
        ferrors.TextCodeStoreReadFailed,
        "failed to read feature override",
        map[string]any{
            ferrors.MetaFeatureKey: key,
            ferrors.MetaStore:      "bun",
            ferrors.MetaOperation:  "get",
        },
    )
}

// Wrap a sentinel error
return ferrors.WrapSentinel(
    ferrors.ErrInvalidKey,
    "store: feature key required",
    map[string]any{
        ferrors.MetaFeatureKey: key,
        ferrors.MetaStore:      "memory",
    },
)
```

### Category-Specific Wrapping

```go
// Wrap as bad input
err := ferrors.WrapBadInput(err, "INVALID_FORMAT", "invalid key format", meta)

// Wrap as operation failure
err := ferrors.WrapOperation(err, "LOOKUP_FAILED", "failed to lookup default", meta)

// Wrap as external failure
err := ferrors.WrapExternal(err, "DB_ERROR", "database query failed", meta)

// Wrap as internal error
err := ferrors.WrapInternal(err, "ASSERTION_FAILED", "unexpected nil value", meta)
```

## Error Handling Patterns

### HTTP Error Mapping

```go
func featureErrorToHTTP(err error) (int, map[string]any) {
    if rich, ok := ferrors.As(err); ok {
        response := map[string]any{
            "error":   rich.TextCode,
            "message": rich.Message,
        }

        switch rich.Category {
        case goerrors.CategoryBadInput:
            return http.StatusBadRequest, response
        case goerrors.CategoryOperation:
            return http.StatusUnprocessableEntity, response
        case goerrors.CategoryExternal:
            return http.StatusServiceUnavailable, response
        case goerrors.CategoryInternal:
            return http.StatusInternalServerError, response
        default:
            return http.StatusInternalServerError, response
        }
    }

    return http.StatusInternalServerError, map[string]any{
        "error":   "INTERNAL_ERROR",
        "message": err.Error(),
    }
}
```

### Logging Errors

```go
func logFeatureError(logger logger.Logger, err error) {
    if err == nil {
        return
    }

    args := []any{"error", err}

    if rich, ok := ferrors.As(err); ok {
        args = append(args,
            "text_code", rich.TextCode,
            "category", rich.Category.String(),
        )

        // Add all metadata
        for key, value := range rich.Metadata {
            args = append(args, key, value)
        }
    }

    logger.Error("feature gate error", args...)
}
```

### Retry Logic

```go
func withRetry(ctx context.Context, fn func() error) error {
    var lastErr error

    for attempt := 0; attempt < 3; attempt++ {
        err := fn()
        if err == nil {
            return nil
        }

        lastErr = err

        // Only retry external errors
        if rich, ok := ferrors.As(err); ok {
            if rich.Category != goerrors.CategoryExternal {
                return err // Don't retry non-external errors
            }
        }

        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(time.Duration(attempt+1) * 100 * time.Millisecond):
            continue
        }
    }

    return lastErr
}
```

### Template Error Handling

In Pongo2 templates with structured errors enabled:

```go
helpers := templates.TemplateHelpers(gate,
    templates.WithStructuredErrors(true),
)
```

```html
{% with feature_if("my.feature", "enabled", "") as result %}
    {% if result.Helper %}
        <!-- Error occurred -->
        <div class="error">
            <span class="code">{{ result.TextCode }}</span>
            <span class="message">{{ result.Message }}</span>
        </div>
    {% elif result == "enabled" %}
        <div class="feature-content">...</div>
    {% endif %}
{% endwith %}
```

### Error Chain Inspection

```go
func inspectErrorChain(err error) {
    for err != nil {
        if rich, ok := ferrors.As(err); ok {
            fmt.Printf("Error: %s [%s]\n", rich.Message, rich.TextCode)
            fmt.Printf("  Category: %s\n", rich.Category)
            fmt.Printf("  Metadata: %v\n", rich.Metadata)

            err = rich.Source
        } else {
            fmt.Printf("Error: %s\n", err)
            err = errors.Unwrap(err)
        }
    }
}
```

## Common Error Scenarios

### Invalid Feature Key

```go
enabled, err := gate.Enabled(ctx, "")
if err != nil {
    // err wraps ferrors.ErrInvalidKey
    // TextCode: FEATURE_KEY_REQUIRED
    // Category: BadInput
}
```

### Store Not Configured

```go
// Without override store
gate := resolver.New(resolver.WithDefaults(defaults))

err := gate.Set(ctx, "feature", scope, true, actor)
// err wraps ferrors.ErrStoreUnavailable
// TextCode: OVERRIDE_STORE_REQUIRED
// Category: Operation
```

### Database Error

```go
// When database query fails
enabled, err := gate.Enabled(ctx, "feature")
if err != nil {
    if rich, ok := ferrors.As(err); ok {
        // TextCode: STORE_READ_FAILED
        // Category: External
        // Metadata includes: feature_key, store, operation
    }
}
```

### Scope Resolution Error

```go
// When custom scope resolver fails
type FailingScopeResolver struct{}

func (r *FailingScopeResolver) Resolve(ctx context.Context) (gate.ScopeSet, error) {
    return gate.ScopeSet{}, errors.New("auth service unavailable")
}

gate := resolver.New(resolver.WithScopeResolver(&FailingScopeResolver{}))

enabled, err := gate.Enabled(ctx, "feature")
// err wraps the auth error
// TextCode: SCOPE_RESOLVE_FAILED
// Category: External
```

## Testing Error Handling

### Testing Sentinel Errors

```go
func TestInvalidKeyError(t *testing.T) {
    gate := resolver.New()

    _, err := gate.Enabled(context.Background(), "")

    assert.Error(t, err)
    assert.True(t, errors.Is(err, ferrors.ErrInvalidKey))
}
```

### Testing Rich Errors

```go
func TestRichErrorDetails(t *testing.T) {
    gate := resolver.New()

    _, err := gate.Enabled(context.Background(), "")

    rich, ok := ferrors.As(err)
    require.True(t, ok)

    assert.Equal(t, ferrors.TextCodeInvalidKey, rich.TextCode)
    assert.Equal(t, goerrors.CategoryBadInput, rich.Category)
}
```

### Testing Error Metadata

```go
func TestErrorMetadata(t *testing.T) {
    store := store.NewMemoryStore()
    gate := resolver.New(resolver.WithOverrideStore(store))

    // Trigger an error scenario
    _, err := gate.Enabled(context.Background(), "  ")

    rich, ok := ferrors.As(err)
    require.True(t, ok)

    // Check metadata is populated
    key, ok := rich.Metadata[ferrors.MetaFeatureKey]
    assert.True(t, ok)
    assert.NotEmpty(t, key)
}
```

## Best Practices

### 1. Check Sentinel Errors First

```go
if errors.Is(err, ferrors.ErrInvalidKey) {
    // Handle invalid key specifically
    return http.StatusBadRequest, "Invalid feature key"
}
```

### 2. Include Context in Wrapped Errors

```go
return ferrors.WrapExternal(err, code, message, map[string]any{
    ferrors.MetaFeatureKey: key,
    ferrors.MetaScope:      scopeSet,
    ferrors.MetaOperation:  "set",
})
```

### 3. Use Text Codes for Monitoring

```go
if rich, ok := ferrors.As(err); ok {
    metrics.Increment("feature.error", map[string]string{
        "code": rich.TextCode,
    })
}
```

### 4. Don't Expose Internal Details

```go
func publicError(err error) string {
    if rich, ok := ferrors.As(err); ok {
        switch rich.Category {
        case goerrors.CategoryBadInput:
            return rich.Message // Safe to expose
        default:
            return "An error occurred" // Hide internal details
        }
    }
    return "An error occurred"
}
```

## Next Steps

- **[GUIDE_RESOLUTION](GUIDE_RESOLUTION.md)** - Resolution flow and error scenarios
- **[GUIDE_HOOKS](GUIDE_HOOKS.md)** - Logging errors with hooks
- **[GUIDE_TESTING](GUIDE_TESTING.md)** - Testing error conditions
- **[GUIDE_TEMPLATES](GUIDE_TEMPLATES.md)** - Template error handling
