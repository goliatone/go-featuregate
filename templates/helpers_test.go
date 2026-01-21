package templates

import (
	"context"
	"errors"
	"testing"

	"github.com/flosch/pongo2/v6"

	"github.com/goliatone/go-featuregate/gate"
)

type captureGate struct {
	value     bool
	err       error
	calls     int
	lastKey   string
	lastScope *gate.ScopeSet
	lastCtx   context.Context
}

func (g *captureGate) Enabled(ctx context.Context, key string, opts ...gate.ResolveOption) (bool, error) {
	g.calls++
	g.lastKey = key
	g.lastCtx = ctx
	req := gate.ResolveRequest{}
	for _, opt := range opts {
		if opt != nil {
			opt(&req)
		}
	}
	if req.ScopeSet != nil {
		scopeCopy := *req.ScopeSet
		g.lastScope = &scopeCopy
	} else {
		g.lastScope = nil
	}
	return g.value, g.err
}

func TestTemplateHelpersScopeOverride(t *testing.T) {
	gateStub := &captureGate{value: true}
	helpers := TemplateHelpers(gateStub)
	fn, ok := helpers["feature"].(func(*pongo2.ExecutionContext, any) bool)
	if !ok {
		t.Fatalf("feature helper not found")
	}
	execCtx := &pongo2.ExecutionContext{
		Public: pongo2.Context{
			TemplateScopeKey: map[string]any{
				"tenant_id": "tenant-1",
				"org_id":    "org-1",
				"user_id":   "user-1",
			},
		},
	}

	value := fn(execCtx, "users.signup")
	if !value {
		t.Fatalf("expected feature helper to return true")
	}
	if gateStub.lastScope == nil || gateStub.lastScope.UserID != "user-1" {
		t.Fatalf("expected scope override to be applied")
	}
}

func TestTemplateHelpersSnapshotPrecedence(t *testing.T) {
	gateStub := &captureGate{value: false}
	helpers := TemplateHelpers(gateStub)
	fn, ok := helpers["feature"].(func(*pongo2.ExecutionContext, any) bool)
	if !ok {
		t.Fatalf("feature helper not found")
	}
	execCtx := &pongo2.ExecutionContext{
		Public: pongo2.Context{
			TemplateSnapshotKey: map[string]bool{
				"users.signup": true,
			},
		},
	}

	value := fn(execCtx, "users.signup")
	if !value {
		t.Fatalf("expected snapshot value to be used")
	}
	if gateStub.calls != 0 {
		t.Fatalf("expected gate not to be called when snapshot contains key")
	}
}

func TestTemplateHelpersErrorFallback(t *testing.T) {
	gateStub := &captureGate{err: errors.New("boom")}
	helpers := TemplateHelpers(gateStub)
	fn, ok := helpers["feature_if"].(func(*pongo2.ExecutionContext, any, any, ...any) any)
	if !ok {
		t.Fatalf("feature_if helper not found")
	}
	execCtx := &pongo2.ExecutionContext{
		Public: pongo2.Context{},
	}

	value := fn(execCtx, "users.signup", "on", "off")
	if value != "off" {
		t.Fatalf("expected fallback value, got %v", value)
	}
}
