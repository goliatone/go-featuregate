# Feature Catalog Guide

This guide covers the feature catalog system for providing metadata and descriptions to admin UIs and documentation.

## Overview

The catalog package provides a metadata layer for feature flags, separate from the resolution system. While `FeatureGate` answers "is this feature enabled?", the catalog answers "what is this feature?".

This separation enables:

- Admin UIs that display feature descriptions
- Documentation generation
- Localization of feature names and descriptions
- Consistent feature metadata across your application

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Admin UI / API                        │
└─────────────────────────────────────────────────────────┘
                │                        │
                ▼                        ▼
┌──────────────────────────┐  ┌──────────────────────────┐
│        Catalog           │  │       FeatureGate        │
│  (metadata/descriptions) │  │   (boolean resolution)   │
└──────────────────────────┘  └──────────────────────────┘
        │                              │
        ▼                              ▼
┌──────────────────────────┐  ┌──────────────────────────┐
│    Config / Static       │  │  Defaults + Overrides    │
└──────────────────────────┘  └──────────────────────────┘
```

The catalog is intentionally decoupled from resolution:

- **Catalog**: Provides `FeatureDefinition` with key and description
- **FeatureGate**: Provides boolean enabled/disabled state

This allows you to define rich metadata without affecting resolution performance.

## Core Types

### Message

A `Message` represents a human-friendly string with optional localization support:

```go
type Message struct {
    Key  string         // Localization key (e.g., "feature.users.signup.description")
    Text string         // Plain text fallback
    Args map[string]any // Interpolation arguments
}
```

Usage patterns:

```go
// Simple text (no localization)
Message{Text: "Allow new user signups"}

// Localization key only (resolved by i18n system)
Message{Key: "feature.users.signup.description"}

// Both (Text used as fallback if Key resolution fails)
Message{
    Key:  "feature.users.signup.description",
    Text: "Allow new user signups",
}

// With interpolation arguments
Message{
    Key:  "feature.trial.description",
    Text: "Trial period: {{days}} days",
    Args: map[string]any{"days": 30},
}
```

### FeatureDefinition

A `FeatureDefinition` describes a single feature flag:

```go
type FeatureDefinition struct {
    Key         string  // Normalized feature key
    Description Message // Human-friendly description
}
```

### Catalog Interface

The `Catalog` interface provides access to feature definitions:

```go
type Catalog interface {
    Get(key string) (FeatureDefinition, bool)
    List() []FeatureDefinition
}
```

- `Get`: Retrieves a definition by key (normalized automatically)
- `List`: Returns all definitions sorted by key

## StaticCatalog

`StaticCatalog` is an in-memory, immutable catalog implementation:

```go
import "github.com/goliatone/go-featuregate/catalog"

cat := catalog.NewStatic(map[string]catalog.FeatureDefinition{
    "users.signup": {
        Key: "users.signup",
        Description: catalog.Message{
            Text: "Allow new user signups",
        },
    },
    "users.invite": {
        Key: "users.invite",
        Description: catalog.Message{
            Key:  "feature.users.invite.description",
            Text: "Allow users to invite others",
        },
    },
    "beta.features": {
        Key: "beta.features",
        Description: catalog.Message{
            Text: "Enable beta features for this tenant",
        },
    },
})

// Get a definition
def, ok := cat.Get("users.signup")
if ok {
    fmt.Println(def.Description.Text) // "Allow new user signups"
}

// List all definitions (sorted by key)
for _, def := range cat.List() {
    fmt.Printf("%s: %s\n", def.Key, def.Description.Text)
}
```

### Key Normalization

Keys are automatically normalized when building and querying the catalog:

```go
cat := catalog.NewStatic(map[string]catalog.FeatureDefinition{
    " Users.Signup ": { // Will be normalized to "users.signup"
        Key: "users.signup",
        Description: catalog.Message{Text: "Allow signups"},
    },
})

// All these queries find the same definition
def, _ := cat.Get("users.signup")
def, _ = cat.Get(" Users.Signup ")
def, _ = cat.Get("USERS.SIGNUP")
```

## Config Adapter Integration

The `configadapter.NewCatalog` function builds catalogs from nested configuration maps, making it easy to define feature metadata in YAML, JSON, or environment-based config.

### Basic Usage

```go
import "github.com/goliatone/go-featuregate/adapters/configadapter"

cat := configadapter.NewCatalog(map[string]any{
    "users": map[string]any{
        "signup": "Allow new user signups",
        "invite": "Allow users to invite others",
    },
    "beta": map[string]any{
        "features": "Enable beta features",
    },
})

def, _ := cat.Get("users.signup")
fmt.Println(def.Description.Text) // "Allow new user signups"
```

### Supported Description Formats

The config adapter supports multiple formats for flexibility:

#### 1. Simple String

```go
map[string]any{
    "users": map[string]any{
        "signup": "Allow new user signups",
    },
}
// Result: Key="users.signup", Description.Text="Allow new user signups"
```

#### 2. Description Object

```go
map[string]any{
    "users": map[string]any{
        "signup": map[string]any{
            "description": "Allow new user signups",
        },
    },
}
// Result: Key="users.signup", Description.Text="Allow new user signups"
```

#### 3. Full Message Object

```go
map[string]any{
    "users": map[string]any{
        "password_reset": map[string]any{
            "description": map[string]any{
                "key":  "feature.users.password_reset.description",
                "text": "Allow password reset",
                "args": map[string]any{"window": "30d"},
            },
        },
    },
}
// Result: Key="users.password_reset"
//         Description.Key="feature.users.password_reset.description"
//         Description.Text="Allow password reset"
//         Description.Args={"window": "30d"}
```

#### 4. Separate Key/Text Fields

```go
map[string]any{
    "users.signup": map[string]any{
        "description_key":  "feature.users.signup.description",
        "description_text": "Allow signups",
    },
}
// Result: Key="users.signup"
//         Description.Key="feature.users.signup.description"
//         Description.Text="Allow signups"
```

### Custom Delimiter

Use `WithDelimiter` for different key separators:

```go
cat := configadapter.NewCatalog(map[string]any{
    "users": map[string]any{
        "signup": "Allow signups",
    },
}, configadapter.WithDelimiter("/"))

def, _ := cat.Get("users/signup")
fmt.Println(def.Key) // "users/signup"
```

### Loading from Config Files

Combine with your config library:

```yaml
# features.yaml
features:
  users:
    signup:
      description: Allow new user signups
    invite:
      description:
        key: feature.users.invite.description
        text: Allow users to invite others
  beta:
    features:
      description: Enable beta features for testing
```

```go
// Using go-config or similar
cfg := config.Load("features.yaml")
cat := configadapter.NewCatalog(cfg.Get("features").(map[string]any))
```

## MessageResolver

The `MessageResolver` interface enables localization of feature descriptions:

```go
type MessageResolver interface {
    Resolve(ctx context.Context, locale string, msg Message) (string, error)
}
```

### PlainResolver

The default `PlainResolver` returns the `Text` field, falling back to `Key`:

```go
resolver := catalog.PlainResolver{}

// With text
msg := catalog.Message{
    Key:  "feature.signup.description",
    Text: "Allow signups",
}
text, _ := resolver.Resolve(ctx, "en", msg)
// text = "Allow signups"

// Without text (falls back to key)
msg = catalog.Message{Key: "feature.signup.description"}
text, _ = resolver.Resolve(ctx, "en", msg)
// text = "feature.signup.description"
```

### Custom Resolver for i18n

Implement `MessageResolver` to integrate with your localization system:

```go
type I18nResolver struct {
    translator *i18n.Translator
}

func (r *I18nResolver) Resolve(ctx context.Context, locale string, msg catalog.Message) (string, error) {
    // Try to resolve the localization key
    if msg.Key != "" {
        translated, err := r.translator.Translate(locale, msg.Key, msg.Args)
        if err == nil && translated != "" {
            return translated, nil
        }
    }

    // Fall back to text
    if msg.Text != "" {
        return r.interpolate(msg.Text, msg.Args), nil
    }

    // Last resort: return the key
    return msg.Key, nil
}

func (r *I18nResolver) interpolate(text string, args map[string]any) string {
    result := text
    for key, val := range args {
        placeholder := "{{" + key + "}}"
        result = strings.ReplaceAll(result, placeholder, fmt.Sprint(val))
    }
    return result
}
```

Usage:

```go
resolver := &I18nResolver{translator: myTranslator}

def, _ := cat.Get("users.signup")
description, _ := resolver.Resolve(ctx, "es", def.Description)
// Returns Spanish translation if available
```

## Building Admin UIs

A common use case is building an admin UI that lists all feature flags with their descriptions and current state.

### Combined View Model

```go
type FeatureViewModel struct {
    Key         string
    Description string
    Enabled     bool
}

func BuildFeatureList(ctx context.Context, cat catalog.Catalog, gate gate.FeatureGate, scope gate.ScopeSet) []FeatureViewModel {
    resolver := catalog.PlainResolver{}
    var features []FeatureViewModel

    for _, def := range cat.List() {
        description, _ := resolver.Resolve(ctx, "en", def.Description)
        enabled := gate.Enabled(ctx, def.Key, gate.WithScopeSet(scope))

        features = append(features, FeatureViewModel{
            Key:         def.Key,
            Description: description,
            Enabled:     enabled,
        })
    }

    return features
}
```

### HTTP Handler Example

```go
func (h *AdminHandler) ListFeatures(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    scope := scope.FromContext(ctx)

    features := BuildFeatureList(ctx, h.catalog, h.gate, scope)

    json.NewEncoder(w).Encode(map[string]any{
        "features": features,
    })
}
```

### With Localization

```go
func (h *AdminHandler) ListFeatures(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    locale := getLocale(r) // e.g., from Accept-Language header
    scope := scope.FromContext(ctx)

    var features []FeatureViewModel
    for _, def := range h.catalog.List() {
        description, _ := h.resolver.Resolve(ctx, locale, def.Description)
        enabled := h.gate.Enabled(ctx, def.Key, gate.WithScopeSet(scope))

        features = append(features, FeatureViewModel{
            Key:         def.Key,
            Description: description,
            Enabled:     enabled,
        })
    }

    json.NewEncoder(w).Encode(map[string]any{
        "features": features,
        "locale":   locale,
    })
}
```

## Combining Catalog with FeatureGate

While catalog and gate are separate, you'll often use them together:

```go
type FeatureService struct {
    catalog catalog.Catalog
    gate    gate.MutableFeatureGate
}

// GetFeature returns both metadata and state
func (s *FeatureService) GetFeature(ctx context.Context, key string, scope gate.ScopeSet) (*Feature, error) {
    def, ok := s.catalog.Get(key)
    if !ok {
        return nil, fmt.Errorf("unknown feature: %s", key)
    }

    enabled := s.gate.Enabled(ctx, key, gate.WithScopeSet(scope))

    return &Feature{
        Key:         def.Key,
        Description: def.Description.Text,
        Enabled:     enabled,
    }, nil
}

// SetFeature updates a feature's enabled state
func (s *FeatureService) SetFeature(ctx context.Context, key string, scope gate.ScopeSet, enabled bool, actor activity.Actor) error {
    // Validate that the feature exists in our catalog
    if _, ok := s.catalog.Get(key); !ok {
        return fmt.Errorf("unknown feature: %s", key)
    }

    return s.gate.Set(ctx, key, scope, enabled, actor)
}
```

## Best Practices

### 1. Define Catalog at Startup

Build your catalog once at application startup:

```go
func main() {
    // Load catalog from config
    cat := configadapter.NewCatalog(loadFeatureConfig())

    // Create gate
    gate := resolver.New(
        resolver.WithDefaults(defaults),
        resolver.WithOverrideStore(overrides),
    )

    // Pass both to handlers
    handler := NewAdminHandler(cat, gate)
}
```

### 2. Keep Catalog and Gate in Sync

Ensure your catalog contains all features that your gate resolves:

```go
// Good: Catalog and defaults defined together
config := map[string]any{
    "users": map[string]any{
        "signup": map[string]any{
            "enabled":     true,
            "description": "Allow new user signups",
        },
    },
}

defaults := configadapter.NewDefaults(config)
catalog := configadapter.NewCatalog(config)
```

### 3. Use Localization Keys Consistently

Follow a consistent naming convention for localization keys:

```go
// Convention: feature.<key>.description
Message{
    Key:  "feature.users.signup.description",
    Text: "Allow new user signups", // Fallback
}
```

### 4. Document Required Args

When using interpolation arguments, document what's expected:

```go
// trial.duration expects "days" arg
Message{
    Key:  "feature.trial.duration.description",
    Text: "Trial period: {{days}} days",
    Args: map[string]any{"days": 30},
}
```

## Next Steps

- **[GUIDE_GETTING_STARTED](GUIDE_GETTING_STARTED.md)** - Basic feature gate setup
- **[GUIDE_ADAPTERS](GUIDE_ADAPTERS.md)** - Config adapter details
- **[GUIDE_TEMPLATES](GUIDE_TEMPLATES.md)** - Using features in templates
- **[GUIDE_OVERRIDES](GUIDE_OVERRIDES.md)** - Runtime flag management
