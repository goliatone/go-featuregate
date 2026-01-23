package templates

import (
	"context"
	"errors"
	"testing"

	"github.com/flosch/pongo2/v6"

	goerrors "github.com/goliatone/go-errors"

	"github.com/goliatone/go-featuregate/ferrors"
	"github.com/goliatone/go-featuregate/gate"
	"github.com/goliatone/go-featuregate/logger"
)

type captureGate struct {
	value     bool
	err       error
	calls     int
	lastKey   string
	lastChain *gate.ScopeChain
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
	if req.ScopeChain != nil {
		chainCopy := append(gate.ScopeChain(nil), (*req.ScopeChain)...)
		g.lastChain = &chainCopy
	} else {
		g.lastChain = nil
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
	if gateStub.lastChain == nil || len(*gateStub.lastChain) == 0 || (*gateStub.lastChain)[0].ID != "user-1" {
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

type captureLogger struct {
	msg  string
	args []any
	call int
	ctx  context.Context
}

func (l *captureLogger) Trace(msg string, args ...any) { l.record(msg, args...) }
func (l *captureLogger) Debug(msg string, args ...any) { l.record(msg, args...) }
func (l *captureLogger) Info(msg string, args ...any)  { l.record(msg, args...) }
func (l *captureLogger) Warn(msg string, args ...any)  { l.record(msg, args...) }
func (l *captureLogger) Error(msg string, args ...any) { l.record(msg, args...) }
func (l *captureLogger) Fatal(msg string, args ...any) { l.record(msg, args...) }

func (l *captureLogger) WithContext(ctx context.Context) logger.Logger {
	l.ctx = ctx
	return l
}

func (l *captureLogger) record(msg string, args ...any) {
	l.call++
	l.msg = msg
	l.args = append([]any(nil), args...)
}

func TestTemplateHelpersErrorLoggingUsesArgs(t *testing.T) {
	logStub := &captureLogger{}
	helpers := TemplateHelpers(nil, WithErrorLogging(true), WithLogger(logStub))
	fn, ok := helpers["feature_if"].(func(*pongo2.ExecutionContext, any, any, ...any) any)
	if !ok {
		t.Fatalf("feature_if helper not found")
	}
	execCtx := &pongo2.ExecutionContext{
		Public: pongo2.Context{},
	}

	_ = fn(execCtx, "", "on", "off")
	if logStub.call != 1 {
		t.Fatalf("expected logger to be called once, got %d", logStub.call)
	}
	if logStub.msg != "featuregate.helper_error" {
		t.Fatalf("unexpected log message: %s", logStub.msg)
	}
	if !hasArgPair(logStub.args, "helper", "feature_if") {
		t.Fatalf("expected helper arg pair to be logged")
	}
	if !hasArgPair(logStub.args, "text_code", ferrors.TextCodeInvalidKey) {
		t.Fatalf("expected text_code arg pair to be logged")
	}
	if !hasArgPair(logStub.args, "category", goerrors.CategoryBadInput) {
		t.Fatalf("expected category arg pair to be logged")
	}
}

func TestTemplateHelpersErrorLoggingDefaultLogger(t *testing.T) {
	helpers := TemplateHelpers(nil, WithErrorLogging(true))
	fn, ok := helpers["feature_if"].(func(*pongo2.ExecutionContext, any, any, ...any) any)
	if !ok {
		t.Fatalf("feature_if helper not found")
	}
	execCtx := &pongo2.ExecutionContext{
		Public: pongo2.Context{},
	}

	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("unexpected panic: %v", rec)
		}
	}()
	_ = fn(execCtx, "", "on", "off")
}

func hasArgPair(args []any, key string, value any) bool {
	for idx := 0; idx+1 < len(args); idx += 2 {
		if args[idx] == key && args[idx+1] == value {
			return true
		}
	}
	return false
}
