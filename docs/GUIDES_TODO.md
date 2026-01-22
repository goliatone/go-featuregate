# Documentation Guides

This document tracks user guides for `go-featuregate`. Each guide helps users understand and use specific features of the package.

## Guide Status Legend

- `pending` - Not started
- `in-progress` - Currently being written
- `review` - Draft complete, needs review
- `done` - Published and complete

---

## 1. GUIDE_GETTING_STARTED.md

**Status**: `review`

**Purpose**: Get users checking feature flags in under 5 minutes.

**Sections**:
- Overview of go-featuregate
- Installation
- Minimal setup (resolver + in-memory store)
- Creating your first gate
- Checking feature enablement with `Enabled`
- Basic configuration options
- Next steps / where to go from here

**Primary Audience**: New users
**Complexity**: Beginner

---

## 2. GUIDE_RESOLUTION.md

**Status**: `review`

**Purpose**: Deep dive into how feature flags are resolved.

**Sections**:
- Resolution hierarchy overview (override → default → fallback)
- Config defaults with `configadapter`
- Runtime overrides with `store.Reader`
- Fallback behavior (always `false`)
- Strict vs permissive store errors (`WithStrictStore`)
- Key normalization and aliases
- Resolution tracing with `ResolveWithTrace`
- Common resolution patterns

**Primary Audience**: Backend developers
**Complexity**: Intermediate

---

## 3. GUIDE_SCOPES.md

**Status**: `review`

**Purpose**: Understanding and implementing multi-tenant feature flags.

**Sections**:
- Scope architecture overview
- `ScopeSet` structure (System, TenantID, OrgID, UserID)
- Scope precedence (user > org > tenant > system)
- Deriving scope from context:
  - `scope.FromContext`
  - `scope.WithTenantID`, `scope.WithOrgID`, `scope.WithUserID`
- Explicit scope with `gate.WithScopeSet`
- Custom scope resolvers (`ScopeResolver` interface)
- System-level flags vs tenant-scoped flags
- Common multitenancy patterns

**Primary Audience**: Backend developers
**Complexity**: Intermediate

---

## 4. GUIDE_OVERRIDES.md

**Status**: `review`

**Purpose**: Managing runtime feature flag overrides.

**Sections**:
- Override architecture overview
- Tri-state semantics (enabled, disabled, unset)
- `MutableFeatureGate` interface (`Set`, `Unset`)
- In-memory store (`store.NewMemoryStore`)
- Bun adapter for database persistence
- Options adapter for go-options integration
- Unset semantics (fall back to defaults)
- Actor tracking for audit trails
- Cache invalidation on mutations

**Primary Audience**: Backend developers
**Complexity**: Intermediate

---

## 5. GUIDE_ADAPTERS.md

**Status**: `review`

**Purpose**: Integrating go-featuregate with external systems.

**Sections**:
- Adapter architecture overview
- Config adapter:
  - `NewDefaults` with `OptionalBool`
  - `NewDefaultsFromBools` for flat maps
  - `WithDelimiter` for nested keys
- Options adapter:
  - Wrapping `go-options` state stores
  - `WithDomain` for key namespacing
  - `WithScopeBuilder` and `WithMetaBuilder`
- Bun adapter:
  - Database schema (`feature_flags` table)
  - `WithTable` for custom table names
  - `WithUpdatedByBuilder` for audit columns
- go-auth adapter (scope and actor resolution)
- Writing custom adapters

**Primary Audience**: Backend developers
**Complexity**: Intermediate

---

## 6. GUIDE_TEMPLATES.md

**Status**: `review`

**Purpose**: Using feature flags in Pongo2 templates.

**Sections**:
- Template helpers overview
- Registering helpers with `TemplateHelpers`
- Template data keys:
  - `feature_ctx` (context)
  - `feature_scope` (scope set)
  - `feature_snapshot` (precomputed values)
- Helper functions:
  - `feature(key)` - single flag check
  - `feature_any(keys...)` - any enabled
  - `feature_all(keys...)` - all enabled
  - `feature_none(keys...)` - none enabled
  - `feature_if(key, whenTrue, whenFalse)` - conditional value
  - `feature_class(key, on, off)` - CSS class helper
  - `feature_trace(key)` - debug trace
- Snapshot resolution vs live resolution
- Error handling (`WithStructuredErrors`, `WithErrorLogging`)
- Custom key overrides (`WithContextKey`, `WithScopeKey`, `WithSnapshotKey`)

**Primary Audience**: Full-stack developers
**Complexity**: Intermediate

---

## 7. GUIDE_GUARDS.md

**Status**: `review`

**Purpose**: Enforcing feature flag requirements in code.

**Sections**:
- Guard pattern overview
- `guard.Require` for enforcement
- Guard options:
  - `WithDisabledError` (custom error when disabled)
  - `WithOverrides` (fallback keys)
  - `WithErrorMapper` (error transformation)
- Common guard patterns:
  - API endpoint protection
  - Command/handler gating
  - Middleware integration
- Error handling and responses

**Primary Audience**: Backend developers
**Complexity**: Intermediate

---

## 8. GUIDE_HOOKS.md

**Status**: `review`

**Purpose**: Subscribing to feature flag events.

**Sections**:
- Hooks architecture overview
- Resolve hooks:
  - `ResolveHook` interface
  - `ResolveEvent` structure
  - `ResolveTrace` for debugging
- Activity hooks:
  - `activity.Hook` interface
  - `UpdateEvent` structure
  - Actor tracking
- Common integrations:
  - Logging with `gologgeradapter`
  - Analytics/metrics collection
  - Audit trail persistence
  - Real-time notifications

**Primary Audience**: Backend developers
**Complexity**: Intermediate

---

## 9. GUIDE_ERRORS.md

**Status**: `review`

**Purpose**: Understanding and handling go-featuregate errors.

**Sections**:
- Error taxonomy overview
- Error categories:
  - `BadInput` - invalid keys, missing requirements
  - `Operation` - resolution failures
  - `External` - store/adapter failures
  - `Internal` - unexpected errors
- Text codes reference
- Metadata keys reference
- Rich error handling:
  - `ferrors.WrapSentinel` for sentinel errors
  - `ferrors.WrapExternal` for dependency failures
  - `ferrors.As` for extracting payloads
- Error patterns in templates
- Logging and debugging errors

**Primary Audience**: Backend developers
**Complexity**: Intermediate

---

## 10. GUIDE_TESTING.md

**Status**: `review`

**Purpose**: Testing strategies for applications using go-featuregate.

**Sections**:
- Testing architecture overview
- In-memory store for unit tests
- Mocking the `FeatureGate` interface
- Testing resolution scenarios:
  - Override precedence
  - Scope isolation
  - Default fallbacks
- Testing template helpers
- Testing guard enforcement
- Integration testing with database stores
- Example test patterns

**Primary Audience**: Developers
**Complexity**: Intermediate

---

## 11. GUIDE_CACHING.md

**Status**: `review`

**Purpose**: Implementing caching for feature flag resolution.

**Sections**:
- Cache architecture overview
- `Cache` interface
- `NoopCache` (default)
- Cache key composition (key + scope)
- Cache invalidation on `Set`/`Unset`
- Implementing custom caches:
  - In-memory with TTL
  - Redis/distributed caching
- Cache entry structure (`Entry` with value and trace)
- Performance considerations

**Primary Audience**: Platform engineers
**Complexity**: Advanced

---

## 12. GUIDE_MIGRATIONS.md

**Status**: `review`

**Purpose**: Database setup for persistent feature flag storage.

**Sections**:
- Schema overview
- `feature_flags` table structure:
  - `id`, `key`, `enabled` (nullable)
  - Scope columns (`tenant_id`, `org_id`, `user_id`)
  - Audit columns (`created_at`, `updated_at`, `updated_by`)
- PostgreSQL schema (`schema/feature_flags.sql`)
- Nullable `enabled` semantics (NULL = unset)
- Indexes for scope-based queries
- Custom table naming with `bunadapter.WithTable`
- Migration strategies

**Primary Audience**: DevOps/DBAs
**Complexity**: Intermediate

---

## Summary

| Guide | Audience | Complexity | Status |
|-------|----------|------------|--------|
| GUIDE_GETTING_STARTED | New users | Beginner | `review` |
| GUIDE_RESOLUTION | Backend developers | Intermediate | `review` |
| GUIDE_SCOPES | Backend developers | Intermediate | `review` |
| GUIDE_OVERRIDES | Backend developers | Intermediate | `review` |
| GUIDE_ADAPTERS | Backend developers | Intermediate | `review` |
| GUIDE_TEMPLATES | Full-stack developers | Intermediate | `review` |
| GUIDE_GUARDS | Backend developers | Intermediate | `review` |
| GUIDE_HOOKS | Backend developers | Intermediate | `review` |
| GUIDE_ERRORS | Backend developers | Intermediate | `review` |
| GUIDE_TESTING | Developers | Intermediate | `review` |
| GUIDE_CACHING | Platform engineers | Advanced | `review` |
| GUIDE_MIGRATIONS | DevOps/DBAs | Intermediate | `review` |

---

## Suggested Priority Order

1. **GUIDE_GETTING_STARTED** - Essential for onboarding new users
2. **GUIDE_RESOLUTION** - Core concept users need to understand first
3. **GUIDE_SCOPES** - Critical for multi-tenant applications
4. **GUIDE_OVERRIDES** - Runtime flag management
5. **GUIDE_ADAPTERS** - Integration with existing systems
6. **GUIDE_GUARDS** - Common enforcement patterns
7. **GUIDE_TEMPLATES** - UI integration
8. **GUIDE_HOOKS** - Event-driven patterns
9. **GUIDE_ERRORS** - Debugging and error handling
10. **GUIDE_TESTING** - Quality assurance patterns
11. **GUIDE_CACHING** - Performance optimization
12. **GUIDE_MIGRATIONS** - Database setup reference

---

## Notes

- Each guide should include runnable code examples
- Reference the existing README.md for API details
- Use `examples/` directory patterns where applicable
- Code examples should use in-memory stores for simplicity where appropriate
- Cross-reference related guides (e.g., GUIDE_SCOPES links to GUIDE_OVERRIDES)
