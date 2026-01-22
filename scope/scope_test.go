package scope

import (
	"context"
	"testing"

	"github.com/goliatone/go-featuregate/gate"
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

func TestFromContextSystemOverride(t *testing.T) {
	ctx := context.Background()
	ctx = WithTenantID(ctx, "acme")
	ctx = WithOrgID(ctx, "engineering")
	ctx = WithUserID(ctx, "user-123")
	ctx = WithSystem(ctx, true)

	got := FromContext(ctx)
	want := gate.ScopeSet{System: true}
	if got != want {
		t.Fatalf("FromContext() = %+v, want %+v", got, want)
	}
}

func TestFromContextNil(t *testing.T) {
	var ctx context.Context
	got := FromContext(ctx)
	if got != (gate.ScopeSet{}) {
		t.Fatalf("FromContext(nil) = %+v, want empty ScopeSet", got)
	}
}
