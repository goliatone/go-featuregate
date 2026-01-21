package gate

import "context"

// ScopeSet captures the resolution scope for a feature check.
type ScopeSet struct {
	System   bool
	TenantID string
	OrgID    string
	UserID   string
}

// ScopeResolver derives a ScopeSet from context.
type ScopeResolver interface {
	Resolve(ctx context.Context) (ScopeSet, error)
}

// ResolveOption mutates a resolve request.
type ResolveOption func(*ResolveRequest)

// ResolveRequest captures optional inputs for a resolve call.
type ResolveRequest struct {
	ScopeSet *ScopeSet
}

// WithScopeSet forces a specific scope instead of deriving it from context.
func WithScopeSet(s ScopeSet) ResolveOption {
	return func(req *ResolveRequest) {
		if req == nil {
			return
		}
		req.ScopeSet = &s
	}
}

// FeatureGate resolves feature enablement for the current scope.
type FeatureGate interface {
	Enabled(ctx context.Context, key string, opts ...ResolveOption) (bool, error)
}

// TraceableFeatureGate adds explainability for feature resolution.
type TraceableFeatureGate interface {
	FeatureGate
	ResolveWithTrace(ctx context.Context, key string, opts ...ResolveOption) (bool, ResolveTrace, error)
}

// MutableFeatureGate supports runtime overrides for feature values.
type MutableFeatureGate interface {
	FeatureGate
	Set(ctx context.Context, key string, scope ScopeSet, enabled bool, actor ActorRef) error
	Unset(ctx context.Context, key string, scope ScopeSet, actor ActorRef) error
}

// ActorRef identifies the actor making a change to runtime overrides.
type ActorRef struct {
	ID   string
	Type string
	Name string
}

// OverrideState captures the tri-state override status.
type OverrideState string

const (
	OverrideStateMissing  OverrideState = "missing"
	OverrideStateEnabled  OverrideState = "enabled"
	OverrideStateDisabled OverrideState = "disabled"
	OverrideStateUnset    OverrideState = "unset"
)
