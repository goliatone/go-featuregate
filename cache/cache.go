package cache

import (
	"context"

	"github.com/goliatone/go-featuregate/gate"
)

// Entry stores a resolved feature value and optional trace.
type Entry struct {
	Value bool
	Trace gate.ResolveTrace
}

// Cache stores resolved feature values by key and scope.
type Cache interface {
	Get(ctx context.Context, key string, scope gate.ScopeSet) (Entry, bool)
	Set(ctx context.Context, key string, scope gate.ScopeSet, entry Entry)
	Delete(ctx context.Context, key string, scope gate.ScopeSet)
	Clear(ctx context.Context)
}

// NoopCache ignores all cache operations.
type NoopCache struct{}

// Get implements Cache.
func (NoopCache) Get(context.Context, string, gate.ScopeSet) (Entry, bool) {
	return Entry{}, false
}

// Set implements Cache.
func (NoopCache) Set(context.Context, string, gate.ScopeSet, Entry) {}

// Delete implements Cache.
func (NoopCache) Delete(context.Context, string, gate.ScopeSet) {}

// Clear implements Cache.
func (NoopCache) Clear(context.Context) {}
