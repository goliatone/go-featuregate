package gate

import "context"

// ResolveSource captures which layer produced the final value.
type ResolveSource string

const (
	ResolveSourceOverride ResolveSource = "override"
	ResolveSourceDefault  ResolveSource = "default"
	ResolveSourceFallback ResolveSource = "fallback"
)

// OverrideTrace captures override resolution details.
type OverrideTrace struct {
	State OverrideState
	Value *bool
	Error error
}

// DefaultTrace captures config default resolution details.
type DefaultTrace struct {
	Set   bool
	Value bool
	Error error
}

// ResolveTrace captures provenance for a single feature resolution.
type ResolveTrace struct {
	Key           string
	NormalizedKey string
	Scope         ScopeSet
	Value         bool
	Source        ResolveSource
	Override      OverrideTrace
	Default       DefaultTrace
	CacheHit      bool
}

// ResolveEvent is emitted after resolution for hooks.
type ResolveEvent struct {
	Key           string
	NormalizedKey string
	Scope         ScopeSet
	Value         bool
	Source        ResolveSource
	Error         error
	Trace         ResolveTrace
}

// ResolveHook receives resolution events.
type ResolveHook interface {
	OnResolve(ctx context.Context, event ResolveEvent)
}

// ResolveHookFunc wraps a function as a ResolveHook.
type ResolveHookFunc func(context.Context, ResolveEvent)

// OnResolve implements ResolveHook.
func (fn ResolveHookFunc) OnResolve(ctx context.Context, event ResolveEvent) {
	if fn == nil {
		return
	}
	fn(ctx, event)
}
