package routeradapter

import (
	"context"

	"github.com/goliatone/go-featuregate/gate"
	"github.com/goliatone/go-featuregate/scope"
	"github.com/goliatone/go-router"
)

// Context extracts the standard context from a router context.
func Context(ctx router.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx.Context()
}

// ScopeSet derives a ScopeSet from a router context.
func ScopeSet(ctx router.Context) gate.ScopeSet {
	return scope.FromContext(Context(ctx))
}

// WithRouterContext returns a resolve option that uses scope derived from router context.
func WithRouterContext(ctx router.Context) gate.ResolveOption {
	return gate.WithScopeSet(ScopeSet(ctx))
}
