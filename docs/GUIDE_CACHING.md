# Caching Guide

This guide covers implementing caching for feature flag resolution in go-featuregate.

## Overview

Caching reduces database queries and improves response times for feature flag checks. go-featuregate provides a `Cache` interface with automatic invalidation on mutations.

## Cache Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Resolution Flow                         │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Enabled(ctx, key)                                          │
│       │                                                     │
│       ▼                                                     │
│  ┌─────────┐                                                │
│  │  Cache  │ ──hit──► return cached value                   │
│  └────┬────┘                                                │
│       │ miss                                                │
│       ▼                                                     │
│  ┌──────────┐                                               │
│  │ Override │ ──found──► cache & return                     │
│  │  Store   │                                               │
│  └────┬─────┘                                               │
│       │ not found                                           │
│       ▼                                                     │
│  ┌──────────┐                                               │
│  │ Defaults │ ──found──► cache & return                     │
│  └────┬─────┘                                               │
│       │ not found                                           │
│       ▼                                                     │
│  return false (fallback)                                    │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Cache Interface

```go
type Cache interface {
    Get(ctx context.Context, key string, scope gate.ScopeSet) (Entry, bool)
    Set(ctx context.Context, key string, scope gate.ScopeSet, entry Entry)
    Delete(ctx context.Context, key string, scope gate.ScopeSet)
    Clear(ctx context.Context)
}
```

## Cache Entry

```go
type Entry struct {
    Value bool              // Resolved feature value
    Trace gate.ResolveTrace // Resolution trace for debugging
}
```

The entry stores both the resolved value and its trace, enabling:
- Fast value lookups
- Debugging without re-resolution
- Complete cache hits for `ResolveWithTrace`

## NoopCache (Default)

By default, no caching is performed:

```go
// Default behavior - no caching
featureGate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithOverrideStore(overrides),
)
```

The `NoopCache` ignores all operations:

```go
type NoopCache struct{}

func (NoopCache) Get(context.Context, string, gate.ScopeSet) (Entry, bool) {
    return Entry{}, false // Always miss
}

func (NoopCache) Set(context.Context, string, gate.ScopeSet, Entry) {}
func (NoopCache) Delete(context.Context, string, gate.ScopeSet) {}
func (NoopCache) Clear(context.Context) {}
```

## Enabling Caching

```go
featureGate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithOverrideStore(overrides),
    resolver.WithCache(myCache),
)
```

## Cache Key Composition

Cache keys combine the feature key and scope:

```
key + scope → cache key
```

Example cache keys:
- `dashboard` + `{System: true}` → unique key for system scope
- `dashboard` + `{TenantID: "acme"}` → unique key for acme tenant
- `dashboard` + `{TenantID: "acme", UserID: "123"}` → unique key for user

Each combination is cached independently, ensuring scope isolation.

## Automatic Cache Invalidation

The resolver automatically invalidates cache entries on mutations:

```go
// This automatically calls cache.Delete(ctx, "feature", scope)
featureGate.Set(ctx, "feature", scope, true, actor)

// This also invalidates the cache
featureGate.Unset(ctx, "feature", scope, actor)
```

## Implementing Custom Caches

### In-Memory Cache with TTL

```go
import (
    "context"
    "sync"
    "time"

    "github.com/goliatone/go-featuregate/cache"
    "github.com/goliatone/go-featuregate/gate"
)

type TTLCache struct {
    mu      sync.RWMutex
    entries map[string]ttlEntry
    ttl     time.Duration
}

type ttlEntry struct {
    entry     cache.Entry
    expiresAt time.Time
}

func NewTTLCache(ttl time.Duration) *TTLCache {
    c := &TTLCache{
        entries: make(map[string]ttlEntry),
        ttl:     ttl,
    }
    go c.cleanup()
    return c
}

func (c *TTLCache) Get(ctx context.Context, key string, scope gate.ScopeSet) (cache.Entry, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()

    cacheKey := c.buildKey(key, scope)
    entry, ok := c.entries[cacheKey]
    if !ok {
        return cache.Entry{}, false
    }
    if time.Now().After(entry.expiresAt) {
        return cache.Entry{}, false
    }
    return entry.entry, true
}

func (c *TTLCache) Set(ctx context.Context, key string, scope gate.ScopeSet, entry cache.Entry) {
    c.mu.Lock()
    defer c.mu.Unlock()

    cacheKey := c.buildKey(key, scope)
    c.entries[cacheKey] = ttlEntry{
        entry:     entry,
        expiresAt: time.Now().Add(c.ttl),
    }
}

func (c *TTLCache) Delete(ctx context.Context, key string, scope gate.ScopeSet) {
    c.mu.Lock()
    defer c.mu.Unlock()

    cacheKey := c.buildKey(key, scope)
    delete(c.entries, cacheKey)
}

func (c *TTLCache) Clear(ctx context.Context) {
    c.mu.Lock()
    defer c.mu.Unlock()

    c.entries = make(map[string]ttlEntry)
}

func (c *TTLCache) buildKey(key string, scope gate.ScopeSet) string {
    return fmt.Sprintf("%s:%s:%s:%s:%v",
        key,
        scope.TenantID,
        scope.OrgID,
        scope.UserID,
        scope.System,
    )
}

func (c *TTLCache) cleanup() {
    ticker := time.NewTicker(time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        c.mu.Lock()
        now := time.Now()
        for key, entry := range c.entries {
            if now.After(entry.expiresAt) {
                delete(c.entries, key)
            }
        }
        c.mu.Unlock()
    }
}

// Usage
ttlCache := NewTTLCache(5 * time.Minute)

featureGate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithOverrideStore(overrides),
    resolver.WithCache(ttlCache),
)
```

### Redis Cache

```go
import (
    "context"
    "encoding/json"
    "time"

    "github.com/go-redis/redis/v8"
    "github.com/goliatone/go-featuregate/cache"
    "github.com/goliatone/go-featuregate/gate"
)

type RedisCache struct {
    client *redis.Client
    prefix string
    ttl    time.Duration
}

func NewRedisCache(client *redis.Client, prefix string, ttl time.Duration) *RedisCache {
    return &RedisCache{
        client: client,
        prefix: prefix,
        ttl:    ttl,
    }
}

func (c *RedisCache) Get(ctx context.Context, key string, scope gate.ScopeSet) (cache.Entry, bool) {
    cacheKey := c.buildKey(key, scope)

    data, err := c.client.Get(ctx, cacheKey).Bytes()
    if err != nil {
        return cache.Entry{}, false
    }

    var entry cache.Entry
    if err := json.Unmarshal(data, &entry); err != nil {
        return cache.Entry{}, false
    }

    return entry, true
}

func (c *RedisCache) Set(ctx context.Context, key string, scope gate.ScopeSet, entry cache.Entry) {
    cacheKey := c.buildKey(key, scope)

    data, err := json.Marshal(entry)
    if err != nil {
        return
    }

    c.client.Set(ctx, cacheKey, data, c.ttl)
}

func (c *RedisCache) Delete(ctx context.Context, key string, scope gate.ScopeSet) {
    cacheKey := c.buildKey(key, scope)
    c.client.Del(ctx, cacheKey)
}

func (c *RedisCache) Clear(ctx context.Context) {
    pattern := c.prefix + "*"
    iter := c.client.Scan(ctx, 0, pattern, 0).Iterator()
    for iter.Next(ctx) {
        c.client.Del(ctx, iter.Val())
    }
}

func (c *RedisCache) buildKey(key string, scope gate.ScopeSet) string {
    scopeID := "system"
    if scope.UserID != "" {
        scopeID = "user:" + scope.UserID
    } else if scope.OrgID != "" {
        scopeID = "org:" + scope.OrgID
    } else if scope.TenantID != "" {
        scopeID = "tenant:" + scope.TenantID
    }
    return fmt.Sprintf("%s:%s:%s", c.prefix, key, scopeID)
}

// Usage
redisClient := redis.NewClient(&redis.Options{
    Addr: "localhost:6379",
})

redisCache := NewRedisCache(redisClient, "featuregate", 5*time.Minute)

featureGate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithOverrideStore(overrides),
    resolver.WithCache(redisCache),
)
```

### LRU Cache

```go
import (
    "container/list"
    "context"
    "sync"

    "github.com/goliatone/go-featuregate/cache"
    "github.com/goliatone/go-featuregate/gate"
)

type LRUCache struct {
    mu       sync.RWMutex
    capacity int
    entries  map[string]*list.Element
    order    *list.List
}

type lruEntry struct {
    key   string
    entry cache.Entry
}

func NewLRUCache(capacity int) *LRUCache {
    return &LRUCache{
        capacity: capacity,
        entries:  make(map[string]*list.Element),
        order:    list.New(),
    }
}

func (c *LRUCache) Get(ctx context.Context, key string, scope gate.ScopeSet) (cache.Entry, bool) {
    c.mu.Lock()
    defer c.mu.Unlock()

    cacheKey := c.buildKey(key, scope)
    elem, ok := c.entries[cacheKey]
    if !ok {
        return cache.Entry{}, false
    }

    // Move to front (most recently used)
    c.order.MoveToFront(elem)
    return elem.Value.(*lruEntry).entry, true
}

func (c *LRUCache) Set(ctx context.Context, key string, scope gate.ScopeSet, entry cache.Entry) {
    c.mu.Lock()
    defer c.mu.Unlock()

    cacheKey := c.buildKey(key, scope)

    // Update existing
    if elem, ok := c.entries[cacheKey]; ok {
        c.order.MoveToFront(elem)
        elem.Value.(*lruEntry).entry = entry
        return
    }

    // Evict if at capacity
    if c.order.Len() >= c.capacity {
        oldest := c.order.Back()
        if oldest != nil {
            c.order.Remove(oldest)
            delete(c.entries, oldest.Value.(*lruEntry).key)
        }
    }

    // Add new
    elem := c.order.PushFront(&lruEntry{key: cacheKey, entry: entry})
    c.entries[cacheKey] = elem
}

func (c *LRUCache) Delete(ctx context.Context, key string, scope gate.ScopeSet) {
    c.mu.Lock()
    defer c.mu.Unlock()

    cacheKey := c.buildKey(key, scope)
    if elem, ok := c.entries[cacheKey]; ok {
        c.order.Remove(elem)
        delete(c.entries, cacheKey)
    }
}

func (c *LRUCache) Clear(ctx context.Context) {
    c.mu.Lock()
    defer c.mu.Unlock()

    c.entries = make(map[string]*list.Element)
    c.order = list.New()
}

func (c *LRUCache) buildKey(key string, scope gate.ScopeSet) string {
    return fmt.Sprintf("%s:%s:%s:%s:%v",
        key, scope.TenantID, scope.OrgID, scope.UserID, scope.System)
}
```

## Cache Strategies

### Request-Scoped Caching

For web applications, cache per request:

```go
type RequestCache struct {
    mu      sync.RWMutex
    entries map[string]cache.Entry
}

func NewRequestCache() *RequestCache {
    return &RequestCache{
        entries: make(map[string]cache.Entry),
    }
}

// ... implement Cache interface ...

// Middleware
func CacheMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        reqCache := NewRequestCache()
        ctx := context.WithValue(r.Context(), "feature_cache", reqCache)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

### Tiered Caching

Combine local and distributed caches:

```go
type TieredCache struct {
    local  cache.Cache // Fast, in-memory
    remote cache.Cache // Shared, Redis
}

func (c *TieredCache) Get(ctx context.Context, key string, scope gate.ScopeSet) (cache.Entry, bool) {
    // Try local first
    if entry, ok := c.local.Get(ctx, key, scope); ok {
        return entry, true
    }

    // Try remote
    if entry, ok := c.remote.Get(ctx, key, scope); ok {
        // Populate local cache
        c.local.Set(ctx, key, scope, entry)
        return entry, true
    }

    return cache.Entry{}, false
}

func (c *TieredCache) Set(ctx context.Context, key string, scope gate.ScopeSet, entry cache.Entry) {
    c.local.Set(ctx, key, scope, entry)
    c.remote.Set(ctx, key, scope, entry)
}

func (c *TieredCache) Delete(ctx context.Context, key string, scope gate.ScopeSet) {
    c.local.Delete(ctx, key, scope)
    c.remote.Delete(ctx, key, scope)
}

func (c *TieredCache) Clear(ctx context.Context) {
    c.local.Clear(ctx)
    c.remote.Clear(ctx)
}
```

### Negative Caching

Cache "not found" results to reduce database queries:

```go
type NegativeCache struct {
    inner cache.Cache
    ttl   time.Duration
}

// The cache.Entry with Value=false and empty Trace represents
// a cached "not found" result
```

## Performance Considerations

### TTL Selection

| Scenario | Recommended TTL |
|----------|-----------------|
| High read, low change | 5-15 minutes |
| Frequent changes | 30 seconds - 2 minutes |
| Critical features | No cache or very short TTL |
| Read-only defaults | Long TTL (1+ hours) |

### Memory Usage

For in-memory caches, estimate memory per entry:
- Key: ~100 bytes
- Entry: ~200 bytes
- Overhead: ~50 bytes

1000 entries ≈ 350 KB

### Cache Hit Monitoring

Track cache effectiveness:

```go
type MetricsCache struct {
    inner cache.Cache
    hits  prometheus.Counter
    misses prometheus.Counter
}

func (c *MetricsCache) Get(ctx context.Context, key string, scope gate.ScopeSet) (cache.Entry, bool) {
    entry, ok := c.inner.Get(ctx, key, scope)
    if ok {
        c.hits.Inc()
    } else {
        c.misses.Inc()
    }
    return entry, ok
}
```

### Avoiding Cache Stampedes

When cache expires, prevent thundering herd:

```go
type StampedeProtectedCache struct {
    inner cache.Cache
    mu    sync.Mutex
    locks map[string]*sync.Mutex
}

func (c *StampedeProtectedCache) GetOrSet(
    ctx context.Context,
    key string,
    scope gate.ScopeSet,
    loader func() (cache.Entry, error),
) (cache.Entry, error) {
    // Try cache first
    if entry, ok := c.inner.Get(ctx, key, scope); ok {
        return entry, nil
    }

    // Get lock for this key
    c.mu.Lock()
    cacheKey := c.buildKey(key, scope)
    lock, ok := c.locks[cacheKey]
    if !ok {
        lock = &sync.Mutex{}
        c.locks[cacheKey] = lock
    }
    c.mu.Unlock()

    // Only one goroutine loads
    lock.Lock()
    defer lock.Unlock()

    // Double-check after acquiring lock
    if entry, ok := c.inner.Get(ctx, key, scope); ok {
        return entry, nil
    }

    // Load and cache
    entry, err := loader()
    if err != nil {
        return cache.Entry{}, err
    }

    c.inner.Set(ctx, key, scope, entry)
    return entry, nil
}
```

## Testing with Caching

### Verify Cache Hits

```go
func TestCacheHit(t *testing.T) {
    ttlCache := NewTTLCache(5 * time.Minute)
    featureGate := resolver.New(
        resolver.WithDefaults(defaults),
        resolver.WithCache(ttlCache),
    )

    ctx := context.Background()

    // First call - cache miss, queries defaults
    _, trace1, _ := featureGate.ResolveWithTrace(ctx, "feature")
    assert.False(t, trace1.CacheHit)

    // Second call - cache hit
    _, trace2, _ := featureGate.ResolveWithTrace(ctx, "feature")
    assert.True(t, trace2.CacheHit)
}
```

### Verify Cache Invalidation

```go
func TestCacheInvalidation(t *testing.T) {
    ttlCache := NewTTLCache(5 * time.Minute)
    overrides := store.NewMemoryStore()

    featureGate := resolver.New(
        resolver.WithOverrideStore(overrides),
        resolver.WithCache(ttlCache),
    )

    ctx := context.Background()
    actor := gate.ActorRef{ID: "test"}
    scope := gate.ScopeSet{System: true}

    // Prime cache
    featureGate.Enabled(ctx, "feature")

    // Mutate - should invalidate
    featureGate.Set(ctx, "feature", scope, true, actor)

    // Should be cache miss after invalidation
    _, trace, _ := featureGate.ResolveWithTrace(ctx, "feature")
    assert.False(t, trace.CacheHit)
}
```

## Best Practices

### 1. Start Without Caching

Add caching only when needed:

```go
// Start simple
featureGate := resolver.New(resolver.WithDefaults(defaults))

// Add caching when performance requires it
featureGate := resolver.New(
    resolver.WithDefaults(defaults),
    resolver.WithCache(myCache),
)
```

### 2. Use Short TTLs Initially

Start with conservative TTLs and increase based on observation:

```go
// Conservative start
cache := NewTTLCache(30 * time.Second)

// After monitoring, adjust
cache := NewTTLCache(5 * time.Minute)
```

### 3. Monitor Cache Effectiveness

Track hit rates and adjust strategy:

```go
hitRate := hits / (hits + misses)
// Target: > 80% for read-heavy workloads
```

### 4. Consider Scope Cardinality

High scope cardinality (many unique users) means more cache entries:

```go
// For user-scoped features with many users,
// consider shorter TTLs or LRU eviction
```

## Next Steps

- **[GUIDE_RESOLUTION](GUIDE_RESOLUTION.md)** - Understanding resolution flow
- **[GUIDE_OVERRIDES](GUIDE_OVERRIDES.md)** - Cache invalidation triggers
- **[GUIDE_TESTING](GUIDE_TESTING.md)** - Testing cached behavior
- **[GUIDE_HOOKS](GUIDE_HOOKS.md)** - Monitoring cache performance
