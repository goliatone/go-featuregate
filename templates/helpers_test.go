package templates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	if !hasArgPair(logStub.args, "category", goerrors.CategoryBadInput.String()) {
		t.Fatalf("expected category arg pair to be logged")
	}
}

func TestTemplateHelpersSanitizeErrorOutput(t *testing.T) {
	cause := errors.New("private-source")
	rich := goerrors.Wrap(cause, goerrors.CategoryExternal, "private-message").
		WithCode(503).
		WithTextCode(ferrors.TextCodeStoreReadFailed).
		WithRequestID("request-1").
		WithMetadata(map[string]any{"token": "private-metadata"})
	sentinel := ferrors.WrapSentinel(ferrors.ErrInvalidKey, "private-message", map[string]any{
		ferrors.MetaFeatureKey: "private-metadata",
	})
	var nilRich *goerrors.Error
	tests := []struct {
		name     string
		err      error
		message  string
		category string
		textCode string
	}{
		{"rich", rich, "An external service error occurred.", "external", ferrors.TextCodeStoreReadFailed},
		{"wrapped", fmt.Errorf("private-wrapper: %w", rich), "An external service error occurred.", "external", ferrors.TextCodeStoreReadFailed},
		{"sentinel", sentinel, "The request is invalid.", "bad_input", ferrors.TextCodeInvalidKey},
		{"plain", errors.New("private-plain"), "An unexpected error occurred.", "internal", ""},
		{"typed nil", nilRich, "An unexpected error occurred.", "internal", ""},
	}
	for _, tt := range tests {
		for _, helper := range []string{"feature_if", "feature_class"} {
			t.Run(tt.name+"/"+helper, func(t *testing.T) {
				logStub := &captureLogger{}
				helpers := TemplateHelpers(&captureGate{err: tt.err},
					WithStructuredErrors(true), WithErrorLogging(true), WithLogger(logStub))
				fn := helpers[helper].(func(*pongo2.ExecutionContext, any, any, ...any) any)
				value := fn(nil, "users.signup", "on", "off")
				output, ok := value.(TemplateError)
				if !ok {
					t.Fatalf("expected TemplateError, got %T", value)
				}
				if output.Helper != helper || output.Message != tt.message || output.Category != tt.category || output.TextCode != tt.textCode {
					t.Fatalf("unexpected public error: %+v", output)
				}
				wantType := tt.textCode
				if wantType == "" {
					wantType = tt.category
				}
				if output.Type != wantType || output.Context != nil || output.Metadata != nil {
					t.Fatalf("unexpected type or diagnostic fields: %+v", output)
				}
				encoded, err := json.Marshal(output)
				if err != nil {
					t.Fatal(err)
				}
				for _, forbidden := range []string{"private-", `"context"`, `"metadata"`} {
					if strings.Contains(string(encoded), forbidden) {
						t.Fatalf("template output contains %q: %s", forbidden, encoded)
					}
				}
				if logStub.call != 1 || logStub.msg != "featuregate.helper_error" {
					t.Fatalf("unexpected log event: %+v", logStub)
				}
				if !hasArgPair(logStub.args, "helper", helper) || !hasArgPair(logStub.args, "category", tt.category) {
					t.Fatalf("missing helper or category: %v", logStub.args)
				}
				summary := "[" + tt.category
				if tt.textCode != "" {
					summary += ":" + tt.textCode
					if !hasArgPair(logStub.args, "text_code", tt.textCode) {
						t.Fatalf("missing text code: %v", logStub.args)
					}
				}
				summary += "] " + tt.message
				if !hasArgPair(logStub.args, "error", summary) || !hasArgPair(logStub.args, "severity", "ERROR") {
					t.Fatalf("missing safe summary or severity: %v", logStub.args)
				}
				if tt.name == "rich" || tt.name == "wrapped" {
					if !hasArgPair(logStub.args, "error_code", int64(503)) || !hasArgPair(logStub.args, "request_id", "request-1") {
						t.Fatalf("missing safe diagnostic identifiers: %v", logStub.args)
					}
				}
				for i := 0; i < len(logStub.args); i += 2 {
					switch logStub.args[i] {
					case "metadata", "source", "location", "stack_trace":
						t.Fatalf("unexpected diagnostic field: %v", logStub.args[i])
					}
					if _, raw := logStub.args[i+1].(error); raw {
						t.Fatalf("raw error passed to logger: %T", logStub.args[i+1])
					}
				}
				if strings.Contains(fmt.Sprint(logStub.args), "private-") {
					t.Fatalf("private error data passed to logger: %v", logStub.args)
				}
			})
		}
	}
	if !errors.Is(rich, cause) || rich.Message != "private-message" || rich.Metadata["token"] != "private-metadata" {
		t.Fatal("rendering mutated the original error or its cause")
	}
	if !errors.Is(sentinel, ferrors.ErrInvalidKey) {
		t.Fatal("rendering broke sentinel matching")
	}
}

func TestTemplateHelpersRenderPublicError(t *testing.T) {
	helpers := TemplateHelpers(&captureGate{err: errors.New("private-backend-error")}, WithStructuredErrors(true))
	tpl, err := pongo2.FromString(`{% with feature_if("users.signup", "on", "off") as result %}{{ result.Helper }}|{{ result.Type }}|{{ result.Message }}|{% if result.Metadata %}unexpected-metadata{% endif %}{% endwith %}`)
	if err != nil {
		t.Fatal(err)
	}
	output, err := tpl.Execute(pongo2.Context(helpers))
	if err != nil {
		t.Fatal(err)
	}
	if output != "feature_if|internal|An unexpected error occurred.|" {
		t.Fatalf("unexpected rendered error: %q", output)
	}
}

func TestTemplateErrorNil(t *testing.T) {
	output := templateError("feature_if", nil)
	if output.Helper != "feature_if" || output.Type != "" || output.Message != "" || output.Category != "" || output.TextCode != "" || output.Context != nil || output.Metadata != nil {
		t.Fatalf("unexpected nil error output: %+v", output)
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
