# Migrations Guide

This guide covers database setup for persistent feature flag storage with go-featuregate.

## Overview

The `bunadapter` persists feature flag overrides to a database using the Bun ORM. This enables:

- Persistent overrides across application restarts
- Multi-instance coordination
- Audit trails with `updated_by` and `updated_at`
- Multi-tenant scoping at the database level

## Schema Overview

The `feature_flags` table stores runtime overrides with scope-based primary keys:

```sql
CREATE TABLE feature_flags (
    key text NOT NULL,
    scope_type text NOT NULL,
    scope_id text NOT NULL DEFAULT '',
    enabled boolean NULL,
    updated_by text,
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    PRIMARY KEY (key, scope_type, scope_id)
);
```

### Column Reference

| Column | Type | Description |
|--------|------|-------------|
| `key` | `text` | Normalized feature key (e.g., `dashboard`, `beta.features`) |
| `scope_type` | `text` | Scope level: `system`, `tenant`, `org`, `user` |
| `scope_id` | `text` | Scope identifier (empty for system scope) |
| `enabled` | `boolean NULL` | `true`, `false`, or `NULL` (unset) |
| `updated_by` | `text` | Actor who made the change |
| `updated_at` | `timestamp` | When the change was made |

### Primary Key

The composite primary key `(key, scope_type, scope_id)` ensures:
- One override per feature per scope
- Efficient lookups by feature and scope
- Natural upsert behavior

## Nullable Enabled Semantics

The `enabled` column uses tri-state semantics:

| Value | Meaning | Resolution Behavior |
|-------|---------|---------------------|
| `true` | Feature enabled | Override returns enabled |
| `false` | Feature disabled | Override returns disabled |
| `NULL` | Explicitly unset | Falls back to config default |

This is critical for the `Unset` operation:

```go
// Set to enabled
gate.Set(ctx, "feature", scope, true, actor)
// Row: key=feature, scope_type=system, enabled=true

// Unset (not delete)
gate.Unset(ctx, "feature", scope, actor)
// Row: key=feature, scope_type=system, enabled=NULL

// Delete removes the row entirely
store.Delete(ctx, "feature", scope)
// Row removed
```

## PostgreSQL Schema

### Complete Schema

```sql
-- Feature flag overrides (nullable enabled means explicit unset).
-- scope_type values: system, tenant, org, user.
CREATE TABLE feature_flags (
    key text NOT NULL,
    scope_type text NOT NULL,
    scope_id text NOT NULL DEFAULT '',
    enabled boolean NULL,
    updated_by text,
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    PRIMARY KEY (key, scope_type, scope_id)
);

-- Index for feature key lookups
CREATE INDEX idx_feature_flags_key ON feature_flags(key);

-- Index for scope-based queries
CREATE INDEX idx_feature_flags_scope ON feature_flags(scope_type, scope_id);

-- Index for audit queries
CREATE INDEX idx_feature_flags_updated_at ON feature_flags(updated_at);
```

### Migration File

Save as `migrations/001_create_feature_flags.sql`:

```sql
-- +migrate Up
CREATE TABLE feature_flags (
    key text NOT NULL,
    scope_type text NOT NULL,
    scope_id text NOT NULL DEFAULT '',
    enabled boolean NULL,
    updated_by text,
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    PRIMARY KEY (key, scope_type, scope_id)
);

CREATE INDEX idx_feature_flags_key ON feature_flags(key);
CREATE INDEX idx_feature_flags_scope ON feature_flags(scope_type, scope_id);

-- +migrate Down
DROP TABLE IF EXISTS feature_flags;
```

## SQLite Schema

For SQLite, adjust types:

```sql
CREATE TABLE feature_flags (
    key TEXT NOT NULL,
    scope_type TEXT NOT NULL,
    scope_id TEXT NOT NULL DEFAULT '',
    enabled INTEGER NULL,  -- 0, 1, or NULL
    updated_by TEXT,
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (key, scope_type, scope_id)
);

CREATE INDEX idx_feature_flags_key ON feature_flags(key);
CREATE INDEX idx_feature_flags_scope ON feature_flags(scope_type, scope_id);
```

## MySQL Schema

For MySQL, use appropriate types:

```sql
CREATE TABLE feature_flags (
    `key` VARCHAR(255) NOT NULL,
    scope_type VARCHAR(50) NOT NULL,
    scope_id VARCHAR(255) NOT NULL DEFAULT '',
    enabled BOOLEAN NULL,
    updated_by VARCHAR(255),
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`key`, scope_type, scope_id),
    INDEX idx_feature_flags_key (`key`),
    INDEX idx_feature_flags_scope (scope_type, scope_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

## Custom Table Names

Use `WithTable` to customize the table name:

```go
import "github.com/goliatone/go-featuregate/adapters/bunadapter"

overrides := bunadapter.NewStore(db,
    bunadapter.WithTable("my_feature_flags"),
)
```

Update your schema accordingly:

```sql
CREATE TABLE my_feature_flags (
    -- same structure
);
```

## Scope Storage

### Scope Types

The `scope_type` column stores one of:

| Value | Description | `scope_id` Content |
|-------|-------------|-------------------|
| `system` | Global scope | Empty string `''` |
| `tenant` | Tenant-level | Tenant ID |
| `org` | Organization-level | Organization ID |
| `user` | User-level | User ID |

### Example Data

```
| key        | scope_type | scope_id    | enabled | updated_by |
|------------|------------|-------------|---------|------------|
| dashboard  | system     |             | true    | admin-123  |
| dashboard  | tenant     | acme        | false   | admin-123  |
| dashboard  | user       | user-456    | true    | user-456   |
| beta.features | tenant  | acme        | true    | admin-123  |
| beta.features | tenant  | beta-corp   | true    | admin-123  |
```

### Resolution Order

When resolving `dashboard` for user `user-456` in tenant `acme`:

1. Check `(dashboard, user, user-456)` → Found: `true`
2. (Stop - user scope wins)

If user override missing:
1. Check `(dashboard, user, user-456)` → Not found
2. Check `(dashboard, org, ...)` → Not found (no org)
3. Check `(dashboard, tenant, acme)` → Found: `false`
4. (Stop - tenant scope wins)

## Index Recommendations

### Essential Indexes

```sql
-- Primary key already indexes (key, scope_type, scope_id)

-- Lookup by feature key (for admin panels)
CREATE INDEX idx_feature_flags_key ON feature_flags(key);

-- Lookup by scope (for cleanup/audit)
CREATE INDEX idx_feature_flags_scope ON feature_flags(scope_type, scope_id);
```

### Additional Indexes

```sql
-- Recent changes (for audit dashboards)
CREATE INDEX idx_feature_flags_updated_at ON feature_flags(updated_at DESC);

-- By updater (for audit queries)
CREATE INDEX idx_feature_flags_updated_by ON feature_flags(updated_by);

-- Tenant-specific queries (if common)
CREATE INDEX idx_feature_flags_tenant ON feature_flags(scope_id)
    WHERE scope_type = 'tenant';
```

## Migration Strategies

### New Applications

For new applications, create the table directly:

```go
func CreateFeatureFlagsTable(ctx context.Context, db *bun.DB) error {
    _, err := db.NewCreateTable().
        Model((*bunadapter.FeatureFlagRecord)(nil)).
        IfNotExists().
        Exec(ctx)
    return err
}
```

### Existing Applications

For existing applications, use incremental migrations:

```sql
-- Migration 001: Initial table
CREATE TABLE IF NOT EXISTS feature_flags (...);

-- Migration 002: Add indexes (if not created initially)
CREATE INDEX IF NOT EXISTS idx_feature_flags_key ON feature_flags(key);

-- Migration 003: Add column (if extending schema)
ALTER TABLE feature_flags ADD COLUMN IF NOT EXISTS metadata JSONB;
```

### Using Migration Tools

#### golang-migrate

```bash
migrate create -ext sql -dir migrations -seq create_feature_flags
```

#### goose

```bash
goose create create_feature_flags sql
```

## Audit Trail Extension

For detailed audit trails, consider an audit log table:

```sql
CREATE TABLE feature_flag_audit (
    id SERIAL PRIMARY KEY,
    feature_key text NOT NULL,
    scope_type text NOT NULL,
    scope_id text NOT NULL,
    old_value boolean,
    new_value boolean,
    action text NOT NULL,  -- 'set', 'unset', 'delete'
    actor_id text,
    actor_type text,
    actor_name text,
    created_at timestamp with time zone NOT NULL DEFAULT now()
);

CREATE INDEX idx_feature_flag_audit_key ON feature_flag_audit(feature_key);
CREATE INDEX idx_feature_flag_audit_created ON feature_flag_audit(created_at);
```

Populate via activity hooks:

```go
func (h *AuditHook) OnUpdate(ctx context.Context, event activity.UpdateEvent) {
    h.db.NewInsert().Model(&AuditRecord{
        FeatureKey: event.NormalizedKey,
        ScopeType:  scopeTypeFromSet(event.Scope),
        ScopeID:    scopeIDFromSet(event.Scope),
        NewValue:   event.Value,
        Action:     string(event.Action),
        ActorID:    event.Actor.ID,
        ActorType:  event.Actor.Type,
        ActorName:  event.Actor.Name,
    }).Exec(ctx)
}
```

## Testing with Databases

### Test Database Setup

```go
func setupTestDB(t *testing.T) *bun.DB {
    t.Helper()

    sqlDB, err := sql.Open("postgres", os.Getenv("TEST_DATABASE_URL"))
    require.NoError(t, err)

    db := bun.NewDB(sqlDB, pgdialect.New())

    // Create table
    _, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS feature_flags (
            key text NOT NULL,
            scope_type text NOT NULL,
            scope_id text NOT NULL DEFAULT '',
            enabled boolean NULL,
            updated_by text,
            updated_at timestamp with time zone NOT NULL DEFAULT now(),
            PRIMARY KEY (key, scope_type, scope_id)
        )
    `)
    require.NoError(t, err)

    // Cleanup after test
    t.Cleanup(func() {
        db.Exec("TRUNCATE TABLE feature_flags")
        db.Close()
    })

    return db
}
```

### SQLite for Tests

Use SQLite for fast, in-memory tests:

```go
func setupTestDB(t *testing.T) *bun.DB {
    t.Helper()

    sqlDB, err := sql.Open("sqlite3", ":memory:")
    require.NoError(t, err)

    db := bun.NewDB(sqlDB, sqlitedialect.New())

    _, err = db.Exec(`
        CREATE TABLE feature_flags (
            key TEXT NOT NULL,
            scope_type TEXT NOT NULL,
            scope_id TEXT NOT NULL DEFAULT '',
            enabled INTEGER NULL,
            updated_by TEXT,
            updated_at TEXT NOT NULL DEFAULT (datetime('now')),
            PRIMARY KEY (key, scope_type, scope_id)
        )
    `)
    require.NoError(t, err)

    t.Cleanup(func() { db.Close() })

    return db
}
```

## Maintenance Queries

### Find All Overrides for a Feature

```sql
SELECT * FROM feature_flags WHERE key = 'my.feature';
```

### Find All Tenant Overrides

```sql
SELECT * FROM feature_flags
WHERE scope_type = 'tenant' AND scope_id = 'acme';
```

### Find Recent Changes

```sql
SELECT * FROM feature_flags
ORDER BY updated_at DESC
LIMIT 100;
```

### Clean Up Old Unset Overrides

```sql
-- Remove rows where enabled is NULL and older than 30 days
DELETE FROM feature_flags
WHERE enabled IS NULL
AND updated_at < NOW() - INTERVAL '30 days';
```

### Export Feature Configuration

```sql
COPY (
    SELECT key, scope_type, scope_id, enabled
    FROM feature_flags
    ORDER BY key, scope_type, scope_id
) TO '/tmp/feature_flags.csv' WITH CSV HEADER;
```

## Best Practices

### 1. Use Migrations

Always use versioned migrations for schema changes:

```
migrations/
├── 001_create_feature_flags.up.sql
├── 001_create_feature_flags.down.sql
├── 002_add_indexes.up.sql
└── 002_add_indexes.down.sql
```

### 2. Test Schema Locally

Test migrations against a local database before production:

```bash
# Start local Postgres
docker run -d -p 5432:5432 -e POSTGRES_PASSWORD=test postgres

# Run migrations
migrate -path ./migrations -database "postgres://localhost/test?sslmode=disable" up
```

### 3. Monitor Table Size

Feature flag tables are typically small, but monitor growth:

```sql
SELECT
    COUNT(*) as total_rows,
    pg_size_pretty(pg_table_size('feature_flags')) as table_size
FROM feature_flags;
```

### 4. Backup Before Changes

Always backup before schema modifications:

```bash
pg_dump -t feature_flags mydb > feature_flags_backup.sql
```

### 5. Use Transactions for Bulk Updates

Wrap bulk operations in transactions:

```go
tx, _ := db.BeginTx(ctx, nil)
defer tx.Rollback()

for _, feature := range features {
    store.Set(ctx, feature.Key, feature.Scope, feature.Enabled, actor)
}

tx.Commit()
```

## Next Steps

- **[GUIDE_ADAPTERS](GUIDE_ADAPTERS.md)** - Using the Bun adapter
- **[GUIDE_OVERRIDES](GUIDE_OVERRIDES.md)** - Runtime override management
- **[GUIDE_HOOKS](GUIDE_HOOKS.md)** - Audit trail with hooks
- **[GUIDE_TESTING](GUIDE_TESTING.md)** - Database testing strategies
