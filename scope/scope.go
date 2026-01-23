package scope

import (
	"context"
	"strings"

	"github.com/goliatone/go-featuregate/gate"
)

type contextKey string

const (
	systemKey   contextKey = "featuregate.system"
	tenantIDKey contextKey = "featuregate.tenant_id"
	orgIDKey    contextKey = "featuregate.org_id"
	userIDKey   contextKey = "featuregate.user_id"
)

const (
	MetadataTenantID = "tenant_id"
	MetadataOrgID    = "org_id"
	MetadataUserID   = "user_id"
)

// WithSystem stores a system scope flag in context.
func WithSystem(ctx context.Context, system bool) context.Context {
	return context.WithValue(ctx, systemKey, system)
}

// WithTenantID stores a tenant identifier in context.
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	trimmed := strings.TrimSpace(tenantID)
	if trimmed == "" {
		return ctx
	}
	return context.WithValue(ctx, tenantIDKey, trimmed)
}

// WithOrgID stores an org identifier in context.
func WithOrgID(ctx context.Context, orgID string) context.Context {
	trimmed := strings.TrimSpace(orgID)
	if trimmed == "" {
		return ctx
	}
	return context.WithValue(ctx, orgIDKey, trimmed)
}

// WithUserID stores a user identifier in context.
func WithUserID(ctx context.Context, userID string) context.Context {
	trimmed := strings.TrimSpace(userID)
	if trimmed == "" {
		return ctx
	}
	return context.WithValue(ctx, userIDKey, trimmed)
}

// ClearTenantID clears a tenant identifier from context.
func ClearTenantID(ctx context.Context) context.Context {
	return context.WithValue(ctx, tenantIDKey, "")
}

// ClearOrgID clears an org identifier from context.
func ClearOrgID(ctx context.Context) context.Context {
	return context.WithValue(ctx, orgIDKey, "")
}

// ClearUserID clears a user identifier from context.
func ClearUserID(ctx context.Context) context.Context {
	return context.WithValue(ctx, userIDKey, "")
}

// System extracts the system scope flag from context.
func System(ctx context.Context) bool {
	return toBool(ctx.Value(systemKey))
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

// ClaimsFromContext builds ActorClaims from context values.
func ClaimsFromContext(ctx context.Context) gate.ActorClaims {
	if ctx == nil {
		return gate.ActorClaims{}
	}
	return gate.ActorClaims{
		SubjectID: UserID(ctx),
		TenantID:  TenantID(ctx),
		OrgID:     OrgID(ctx),
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

func toBool(value any) bool {
	if value == nil {
		return false
	}
	if flag, ok := value.(bool); ok {
		return flag
	}
	return false
}
