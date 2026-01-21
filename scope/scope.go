package scope

import (
	"context"
	"strings"

	"github.com/goliatone/go-featuregate/gate"
)

type contextKey string

const (
	tenantIDKey contextKey = "featuregate.tenant_id"
	orgIDKey    contextKey = "featuregate.org_id"
	userIDKey   contextKey = "featuregate.user_id"
)

// WithTenantID stores a tenant identifier in context.
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantIDKey, strings.TrimSpace(tenantID))
}

// WithOrgID stores an org identifier in context.
func WithOrgID(ctx context.Context, orgID string) context.Context {
	return context.WithValue(ctx, orgIDKey, strings.TrimSpace(orgID))
}

// WithUserID stores a user identifier in context.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, strings.TrimSpace(userID))
}

// TenantID extracts the tenant identifier from context.
func TenantID(ctx context.Context) string {
	return toString(ctx.Value(tenantIDKey))
}

// OrgID extracts the org identifier from context.
func OrgID(ctx context.Context) string {
	return toString(ctx.Value(orgIDKey))
}

// UserID extracts the user identifier from context.
func UserID(ctx context.Context) string {
	return toString(ctx.Value(userIDKey))
}

// FromContext builds a ScopeSet from context values.
func FromContext(ctx context.Context) gate.ScopeSet {
	if ctx == nil {
		return gate.ScopeSet{}
	}
	return gate.ScopeSet{
		TenantID: TenantID(ctx),
		OrgID:    OrgID(ctx),
		UserID:   UserID(ctx),
	}
}

func toString(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}
