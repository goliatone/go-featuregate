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
	Get(ctx context.Context, key string, chain gate.ScopeChain) (Entry, bool)
	Set(ctx context.Context, key string, chain gate.ScopeChain, entry Entry)
	Delete(ctx context.Context, key string, chain gate.ScopeChain)
	Clear(ctx context.Context)
}

// NoopCache ignores all cache operations.
type NoopCache struct{}

// Get implements Cache.
func (NoopCache) Get(context.Context, string, gate.ScopeChain) (Entry, bool) {
	return Entry{}, false
}

// Set implements Cache.
func (NoopCache) Set(context.Context, string, gate.ScopeChain, Entry) {}

// Delete implements Cache.
func (NoopCache) Delete(context.Context, string, gate.ScopeChain) {}

// Clear implements Cache.
func (NoopCache) Clear(context.Context) {}
