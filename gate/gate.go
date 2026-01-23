package gate

import "context"

// ScopeKind defines supported scope types.
type ScopeKind uint8

const (
	ScopeSystem ScopeKind = iota
	ScopeTenant
	ScopeOrg
	ScopeUser
	ScopeRole
	ScopePerm
)

// ScopeRef identifies a single scope target.
type ScopeRef struct {
	Kind     ScopeKind
	ID       string
	TenantID string
	OrgID    string
}

// ScopeChain is an ordered list of scope references.
type ScopeChain []ScopeRef

// ActorClaims are the minimal inputs required to build a chain.
type ActorClaims struct {
	SubjectID string
	TenantID  string
	OrgID     string
	Roles     []string
	Perms     []string
}

// ClaimsProvider derives claims from context.
type ClaimsProvider interface {
	ClaimsFromContext(ctx context.Context) (ActorClaims, error)
}

// PermissionProvider derives permissions from claims/roles.
type PermissionProvider interface {
	Permissions(ctx context.Context, claims ActorClaims) ([]string, error)
}

// ResolveOption mutates a resolve request.
type ResolveOption func(*ResolveRequest)

// ResolveRequest captures optional inputs for a resolve call.
type ResolveRequest struct {
	ScopeChain *ScopeChain
}

// WithScopeChain forces a specific scope chain instead of deriving it from context.
func WithScopeChain(chain ScopeChain) ResolveOption {
	return func(req *ResolveRequest) {
		if req == nil {
			return
		}
		req.ScopeChain = &chain
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
	Set(ctx context.Context, key string, scope ScopeRef, enabled bool, actor ActorRef) error
	Unset(ctx context.Context, key string, scope ScopeRef, actor ActorRef) error
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
