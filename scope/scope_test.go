package scope

import (
	"context"
	"testing"
)

func TestScopeHelpersNoopAndClear(t *testing.T) {
	ctx := context.Background()
	ctx = WithTenantID(ctx, "  acme ")
	ctx = WithOrgID(ctx, " engineering ")
	ctx = WithUserID(ctx, " user-123 ")

	if got := TenantID(ctx); got != "acme" {
		t.Fatalf("TenantID() = %q, want %q", got, "acme")
	}
	if got := OrgID(ctx); got != "engineering" {
		t.Fatalf("OrgID() = %q, want %q", got, "engineering")
	}
	if got := UserID(ctx); got != "user-123" {
		t.Fatalf("UserID() = %q, want %q", got, "user-123")
	}

	ctx = WithTenantID(ctx, " ")
	ctx = WithOrgID(ctx, "")
	ctx = WithUserID(ctx, "\n\t")

	if got := TenantID(ctx); got != "acme" {
		t.Fatalf("TenantID() after no-op = %q, want %q", got, "acme")
	}
	if got := OrgID(ctx); got != "engineering" {
		t.Fatalf("OrgID() after no-op = %q, want %q", got, "engineering")
	}
	if got := UserID(ctx); got != "user-123" {
		t.Fatalf("UserID() after no-op = %q, want %q", got, "user-123")
	}

	ctx = ClearTenantID(ctx)
	ctx = ClearOrgID(ctx)
	ctx = ClearUserID(ctx)

	if got := TenantID(ctx); got != "" {
		t.Fatalf("TenantID() after clear = %q, want empty", got)
	}
	if got := OrgID(ctx); got != "" {
		t.Fatalf("OrgID() after clear = %q, want empty", got)
	}
	if got := UserID(ctx); got != "" {
		t.Fatalf("UserID() after clear = %q, want empty", got)
	}
}

func TestClaimsFromContext(t *testing.T) {
	ctx := context.Background()
	ctx = WithTenantID(ctx, "acme")
	ctx = WithOrgID(ctx, "engineering")
	ctx = WithUserID(ctx, "user-123")
	ctx = WithSystem(ctx, true)

	got := ClaimsFromContext(ctx)
	if got.SubjectID != "user-123" || got.TenantID != "acme" || got.OrgID != "engineering" {
		t.Fatalf("ClaimsFromContext() = %+v, want subject/tenant/org", got)
	}
}

func TestClaimsFromContextNil(t *testing.T) {
	var ctx context.Context
	got := ClaimsFromContext(ctx)
	if got.SubjectID != "" || got.TenantID != "" || got.OrgID != "" || len(got.Roles) != 0 || len(got.Perms) != 0 {
		t.Fatalf("ClaimsFromContext(nil) = %+v, want empty claims", got)
	}
}
