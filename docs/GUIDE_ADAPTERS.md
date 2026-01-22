# Adapters Guide

This guide covers the adapter system in go-featuregate for integrating with external configuration sources, databases, and framework components.

## Overview

Adapters bridge go-featuregate to external systems:

| Adapter | Purpose |
|---------|---------|
| **configadapter** | Wraps go-config `OptionalBool` values as defaults |
| **bunadapter** | Persists overrides to PostgreSQL/SQLite via Bun ORM |
| **optionsadapter** | Wraps go-options state stores as override stores |
| **goauthadapter** | Extracts scope from go-auth context |
| **gologgeradapter** | Logging hooks for go-logger |

## Config Adapter

The config adapter converts configuration maps into feature defaults.

### NewDefaults with OptionalBool

Use `go-config`'s `OptionalBool` for tri-state configuration values:

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

### NewDefaultsFromBools

For simple boolean maps:

```go
defaults := configadapter.NewDefaultsFromBools(map[string]bool{
    "users.signup":    true,
    "users.invite":    true,
    "dashboard":       false,
    "notifications":   true,
})

gate := resolver.New(
    resolver.WithDefaults(defaults),
)
```

### Custom Delimiter

Change the nested key delimiter (default is `.`):

```go
defaults := configadapter.NewDefaults(map[string]any{
    "users": map[string]any{
        "signup": true,
    },
}, configadapter.WithDelimiter("/"))

// Key becomes "users/signup"
```

### Supported Value Types

The config adapter supports:

| Type | Behavior |
|------|----------|
| `config.OptionalBool` | Uses `IsSet()` and `Value()` |
| `*config.OptionalBool` | Same, nil returns unset |
| `bool` | Always set with the given value |
| `*bool` | Nil returns unset |
| `map[string]any` | Recursively flattened |
| `map[string]bool` | Recursively flattened |

## Bun Adapter

The Bun adapter persists feature overrides to a database.

### Basic Setup

```go
import (
    "github.com/goliatone/go-featuregate/adapters/bunadapter"
    "github.com/uptrace/bun"
    "github.com/uptrace/bun/dialect/pgdialect"
)

db := bun.NewDB(sqlDB, pgdialect.New())
overrides := bunadapter.NewStore(db)

gate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithOverrideStore(overrides),
)
```

### Custom Table Name

```go
overrides := bunadapter.NewStore(db,
    bunadapter.WithTable("my_feature_flags"),
)
```

### Custom UpdatedBy Builder

Control the `updated_by` audit column value:

```go
overrides := bunadapter.NewStore(db,
    bunadapter.WithUpdatedByBuilder(func(actor gate.ActorRef) string {
        if actor.ID != "" {
            return fmt.Sprintf("%s:%s", actor.Type, actor.ID)
        }
        return "system"
    }),
)
```

### Custom Timestamp

For testing or consistent timestamps:

```go
overrides := bunadapter.NewStore(db,
    bunadapter.WithNowFunc(func() time.Time {
        return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
    }),
)
```

### Database Schema

The Bun adapter expects a table with this structure:

```sql
CREATE TABLE feature_flags (
    key        TEXT NOT NULL,
    scope_type TEXT NOT NULL,  -- 'system', 'tenant', 'org', 'user'
    scope_id   TEXT NOT NULL,  -- '' for system, ID for others
    enabled    BOOLEAN,        -- NULL = unset (fall back to default)
    updated_by TEXT,
    updated_at TIMESTAMP,
    PRIMARY KEY (key, scope_type, scope_id)
);

CREATE INDEX idx_feature_flags_key ON feature_flags(key);
CREATE INDEX idx_feature_flags_scope ON feature_flags(scope_type, scope_id);
```

See [GUIDE_MIGRATIONS](GUIDE_MIGRATIONS.md) for complete schema setup.

### Delete vs Unset

The adapter supports both operations:

```go
// Unset: Sets enabled = NULL (falls back to default)
overrides.Unset(ctx, "feature", scope, actor)

// Delete: Removes the row entirely
overrides.Delete(ctx, "feature", scope)
```

## Options Adapter

The options adapter wraps a `go-options` state store.

### Basic Setup

```go
import (
    "github.com/goliatone/go-featuregate/adapters/optionsadapter"
    "github.com/goliatone/go-options/pkg/state"
)

stateStore := myStateStore // state.Store[map[string]any]

overrides := optionsadapter.NewStore(stateStore)

gate := resolver.New(
    resolver.WithOverrideStore(overrides),
)
```

### Custom Domain

Feature flags are stored under a domain (default: `feature_flags`):

```go
overrides := optionsadapter.NewStore(stateStore,
    optionsadapter.WithDomain("app_features"),
)
```

### Custom Scope Builder

Control how `ScopeSet` maps to go-options scopes:

```go
import opts "github.com/goliatone/go-options"

overrides := optionsadapter.NewStore(stateStore,
    optionsadapter.WithScopeBuilder(func(scopeSet gate.ScopeSet) []opts.Scope {
        var scopes []opts.Scope

        // Custom priority and naming
        if scopeSet.UserID != "" {
            scopes = append(scopes, opts.NewScope("user", 100,
                opts.WithScopeMetadata(map[string]any{"user_id": scopeSet.UserID}),
            ))
        }
        if scopeSet.TenantID != "" {
            scopes = append(scopes, opts.NewScope("tenant", 50,
                opts.WithScopeMetadata(map[string]any{"tenant_id": scopeSet.TenantID}),
            ))
        }
        scopes = append(scopes, opts.NewScope("global", 10))

        return scopes
    }),
)
```

### Custom Meta Builder

Control mutation metadata:

```go
overrides := optionsadapter.NewStore(stateStore,
    optionsadapter.WithMetaBuilder(func(actor gate.ActorRef) state.Meta {
        return state.Meta{
            Extra: map[string]string{
                "modified_by":   actor.ID,
                "modified_type": actor.Type,
                "source":        "feature-gate",
            },
        }
    }),
)
```

### With go-admin Preferences Store

Use the go-admin preferences store as a backend:

```go
import (
    "github.com/goliatone/go-admin/featuregate/adapter"
    "github.com/goliatone/go-featuregate/adapters/optionsadapter"
)

prefs := admin.NewInMemoryPreferencesStore()
stateStore := adapter.NewPreferencesStoreAdapter(prefs)

overrides := optionsadapter.NewStore(stateStore,
    optionsadapter.WithDomain("feature_flags"),
)
```

## go-auth Adapter

The go-auth adapter extracts scope and actor information from authentication context.

**Note**: The go-auth adapter has moved to the go-auth package: `github.com/goliatone/go-auth/adapters/featuregate`

### Scope Resolver

```go
import "github.com/goliatone/go-auth/adapters/featuregate"

scopeResolver := featuregate.NewScopeResolver()

gate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithScopeResolver(scopeResolver),
)
```

### Actor Reference

Extract actor for mutations:

```go
import "github.com/goliatone/go-auth/adapters/featuregate"

func handleFeatureToggle(ctx context.Context, key string, enabled bool) error {
    actor := featuregate.ActorRefFromContext(ctx)
    scope := featuregate.ScopeFromContext(ctx)

    return gate.Set(ctx, key, scope, enabled, actor)
}
```

## go-logger Adapter

The go-logger adapter provides logging hooks.

### Setup

```go
import (
    "github.com/goliatone/go-featuregate/adapters/gologgeradapter"
    "github.com/goliatone/go-logger"
)

lgr := logger.New()
hook := gologgeradapter.New(lgr)

gate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithResolveHook(hook),
    resolver.WithActivityHook(hook),
)
```

### Logged Events

**Resolve Events** (debug level):
```
[DEBUG] feature resolved key=dashboard value=true source=override scope={TenantID:acme}
```

**Activity Events** (info level):
```
[INFO] feature override updated key=dashboard action=set actor=admin-123 scope={TenantID:acme}
```

## Writing Custom Adapters

### Custom Defaults Adapter

Implement `resolver.Defaults`:

```go
type Defaults interface {
    Default(ctx context.Context, key string) (DefaultResult, error)
}

type DefaultResult struct {
    Set   bool
    Value bool
}
```

Example - environment variable defaults:

```go
type EnvDefaults struct {
    prefix string
}

func NewEnvDefaults(prefix string) *EnvDefaults {
    return &EnvDefaults{prefix: prefix}
}

func (d *EnvDefaults) Default(ctx context.Context, key string) (resolver.DefaultResult, error) {
    envKey := d.prefix + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))

    if value, ok := os.LookupEnv(envKey); ok {
        enabled := value == "true" || value == "1" || value == "yes"
        return resolver.DefaultResult{Set: true, Value: enabled}, nil
    }

    return resolver.DefaultResult{Set: false}, nil
}

// Usage: FEATURE_USERS_SIGNUP=true
defaults := NewEnvDefaults("FEATURE_")
```

### Custom Override Store

Implement `store.ReadWriter`:

```go
type Reader interface {
    Get(ctx context.Context, key string, scope gate.ScopeSet) (Override, error)
}

type Writer interface {
    Set(ctx context.Context, key string, scope gate.ScopeSet, enabled bool, actor gate.ActorRef) error
    Unset(ctx context.Context, key string, scope gate.ScopeSet, actor gate.ActorRef) error
}

type ReadWriter interface {
    Reader
    Writer
}
```

Example - Redis store:

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

    switch val {
    case "true":
        return store.EnabledOverride(), nil
    case "false":
        return store.DisabledOverride(), nil
    default:
        return store.UnsetOverride(), nil
    }
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
    scopeID := "system"
    if scope.UserID != "" {
        scopeID = "user:" + scope.UserID
    } else if scope.OrgID != "" {
        scopeID = "org:" + scope.OrgID
    } else if scope.TenantID != "" {
        scopeID = "tenant:" + scope.TenantID
    }
    return fmt.Sprintf("%s:%s:%s", s.prefix, key, scopeID)
}
```

### Custom Scope Resolver

Implement `gate.ScopeResolver`:

```go
type ScopeResolver interface {
    Resolve(ctx context.Context) (ScopeSet, error)
}
```

Example - HTTP header resolver:

```go
type HeaderScopeResolver struct{}

func (r *HeaderScopeResolver) Resolve(ctx context.Context) (gate.ScopeSet, error) {
    req, ok := ctx.Value("http_request").(*http.Request)
    if !ok {
        return gate.ScopeSet{}, nil
    }

    return gate.ScopeSet{
        TenantID: req.Header.Get("X-Tenant-ID"),
        OrgID:    req.Header.Get("X-Org-ID"),
        UserID:   req.Header.Get("X-User-ID"),
    }, nil
}
```

## Adapter Composition

Combine adapters for complex setups:

```go
// Defaults from config file
fileDefaults := configadapter.NewDefaults(loadConfigFile())

// Overrides from database with caching
dbOverrides := bunadapter.NewStore(db)

// Scope from authentication
authScopeResolver := featuregate.NewScopeResolver()

// Logging
logHook := gologgeradapter.New(logger)

gate := resolver.New(
    resolver.WithDefaults(fileDefaults),
    resolver.WithOverrideStore(dbOverrides),
    resolver.WithScopeResolver(authScopeResolver),
    resolver.WithResolveHook(logHook),
    resolver.WithActivityHook(logHook),
    resolver.WithCache(myCache),
)
```

## Error Handling

Adapters use the `ferrors` package for rich errors:

```go
if err := overrides.Set(ctx, key, scope, enabled, actor); err != nil {
    if rich, ok := ferrors.As(err); ok {
        log.Printf("Adapter: %s, Operation: %s, Error: %s",
            rich.Metadata["adapter"],
            rich.Metadata["operation"],
            rich.Message,
        )
    }
}
```

Common error codes:
- `STORE_REQUIRED` - Missing store/database
- `STORE_READ_FAILED` - Read operation failed
- `STORE_WRITE_FAILED` - Write operation failed
- `FEATURE_KEY_REQUIRED` - Empty or invalid key

## Next Steps

- **[GUIDE_MIGRATIONS](GUIDE_MIGRATIONS.md)** - Database schema setup
- **[GUIDE_HOOKS](GUIDE_HOOKS.md)** - Event subscriptions
- **[GUIDE_ERRORS](GUIDE_ERRORS.md)** - Error handling patterns
