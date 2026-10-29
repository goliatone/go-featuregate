package templates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/flosch/pongo2/v6"
	goerrors "github.com/goliatone/go-errors"

	"github.com/goliatone/go-featuregate/adapters/configadapter"
	"github.com/goliatone/go-featuregate/ferrors"
	"github.com/goliatone/go-featuregate/gate"
	"github.com/goliatone/go-featuregate/resolver"
	"github.com/goliatone/go-featuregate/store"
)

type failingTraceStore struct{ err error }

func (s failingTraceStore) GetAll(context.Context, string, gate.ScopeChain) ([]store.OverrideMatch, error) {
	return nil, s.err
}

func TestFeatureTraceSanitizesLiveErrors(t *testing.T) {
	cause := errors.New("private-cause")
	rich := goerrors.Wrap(cause, goerrors.CategoryExternal, "private-message").
		WithTextCode(ferrors.TextCodeStoreReadFailed).
		WithCode(503).
		WithRequestID("request-1").
		WithMetadata(map[string]any{"token": "private-token"})
	fg := resolver.New(
		resolver.WithOverrideStore(failingTraceStore{err: rich}),
		resolver.WithDefaults(configadapter.NewDefaultsFromBools(map[string]bool{"probe.feature": true})),
	)
	value, diagnostic, err := fg.ResolveWithTrace(context.Background(), "probe.feature")
	if err != nil || !value || diagnostic.Override.Error == nil {
		t.Fatalf("expected non-strict fallback with diagnostic error: value=%v, trace=%+v, err=%v", value, diagnostic, err)
	}
	for _, structured := range []bool{false, true} {
		t.Run(fmt.Sprintf("structured=%v", structured), func(t *testing.T) {
			helpers := TemplateHelpers(fg, WithStructuredErrors(structured))
			assertPublicTrace(t, helpers, pongo2.Context{}, diagnostic,
				"An external service error occurred.", "")
		})
	}
	_, after, err := fg.ResolveWithTrace(context.Background(), "probe.feature")
	if err != nil {
		t.Fatal(err)
	}
	for _, trace := range []gate.ResolveTrace{diagnostic, after} {
		detail, ok := ferrors.As(trace.Override.Error)
		if !ok || detail.Message != "private-message" || detail.Metadata["token"] != "private-token" || !errors.Is(trace.Override.Error, cause) {
			t.Fatalf("Go diagnostic trace was sanitized or lost its cause: %+v", trace)
		}
	}
}

func TestFeatureTraceSanitizesSnapshotErrors(t *testing.T) {
	cause := errors.New("private-cause")
	rich := goerrors.Wrap(cause, goerrors.CategoryExternal, "private-message").
		WithTextCode(ferrors.TextCodeStoreReadFailed).
		WithMetadata(map[string]any{"token": "private-token"})
	plain := errors.New("private-default")
	diagnostic := gate.ResolveTrace{
		Key: "probe.feature", NormalizedKey: "probe.feature", Value: true,
		Source: gate.ResolveSourceDefault, CacheHit: true, Strategy: "custom",
		Chain: gate.ScopeChain{{Kind: gate.ScopeSystem}},
		Override: gate.OverrideTrace{
			State: gate.OverrideStateMissing, Value: new(false),
			Error:   fmt.Errorf("private-wrapper: %w", rich),
			Matches: []gate.OverrideMatchTrace{{State: gate.OverrideStateMissing}},
		},
		Default: gate.DefaultTrace{Set: true, Value: true, Error: plain},
	}
	snapshot := Snapshot{Traces: map[string]gate.ResolveTrace{"probe.feature": diagnostic}}
	fg := resolver.New(resolver.WithOverrideStore(failingTraceStore{err: errors.New("unexpected-live-call")}), resolver.WithStrictStore(true))
	helpers := TemplateHelpers(fg)
	for name, input := range map[string]any{
		"value map":   map[string]gate.ResolveTrace{"probe.feature": diagnostic},
		"pointer map": map[string]*gate.ResolveTrace{"probe.feature": &diagnostic},
		"reader":      snapshot,
	} {
		t.Run(name, func(t *testing.T) {
			assertPublicTrace(t, helpers, pongo2.Context{TemplateSnapshotKey: input}, diagnostic,
				"An external service error occurred.", "An unexpected error occurred.")
			original, ok := snapshotTrace(input, "probe.feature")
			if !ok || original.Override.Error != diagnostic.Override.Error || original.Default.Error != plain {
				t.Fatal("snapshot errors were replaced")
			}
		})
	}
	original, ok := snapshot.Trace("probe.feature")
	if !ok || !errors.Is(original.Override.Error, cause) || original.Default.Error != plain || rich.Message != "private-message" || rich.Metadata["token"] != "private-token" {
		t.Fatal("snapshot Go API lost diagnostic data")
	}
}

func TestFeatureTracePreservesErrorFreeTrace(t *testing.T) {
	fg := resolver.New()
	_, diagnostic, err := fg.ResolveWithTrace(context.Background(), "probe.feature")
	if err != nil {
		t.Fatal(err)
	}
	assertPublicTrace(t, TemplateHelpers(fg), pongo2.Context{}, diagnostic, "", "")
}

func assertPublicTrace(t *testing.T, helpers map[string]any, data pongo2.Context, diagnostic gate.ResolveTrace, overrideMessage, defaultMessage string) {
	t.Helper()
	fn := helpers["feature_trace"].(func(*pongo2.ExecutionContext, any) any)
	value := fn(&pongo2.ExecutionContext{Public: data}, "probe.feature")
	output, ok := value.(gate.ResolveTrace)
	if !ok {
		t.Fatalf("expected ResolveTrace, got %T", value)
	}
	for _, pair := range [][2]error{{output.Override.Error, diagnostic.Override.Error}, {output.Default.Error, diagnostic.Default.Error}} {
		if (pair[0] == nil) != (pair[1] == nil) {
			t.Fatal("changed whether a trace error is present")
		}
		if pair[0] != nil && (errors.Unwrap(pair[0]) != nil || strings.Contains(pair[0].Error(), "private-")) {
			t.Fatal("template trace exposes a private error or its cause")
		}
		if rich, ok := ferrors.As(pair[1]); ok {
			encoded, err := json.Marshal(pair[0])
			if err != nil {
				t.Fatal(err)
			}
			var public goerrors.PublicError
			if err := json.Unmarshal(encoded, &public); err != nil {
				t.Fatal(err)
			}
			if public.Category != rich.Category || public.TextCode != rich.TextCode || public.Code != rich.Code || public.RequestID != rich.RequestID {
				t.Fatalf("public trace lost error classification or identifiers: %+v", public)
			}
		}
	}
	encoded, err := json.Marshal(output)
	if err != nil || strings.Contains(string(encoded), "private-") {
		t.Fatalf("unsafe or invalid trace JSON: %s, err=%v", encoded, err)
	}
	output.Override.Error, output.Default.Error = nil, nil
	diagnostic.Override.Error, diagnostic.Default.Error = nil, nil
	if !reflect.DeepEqual(output, diagnostic) {
		t.Fatal("template projection changed non-error trace fields")
	}
	for key, helper := range helpers {
		data[key] = helper
	}
	tpl, err := pongo2.FromString(`{% with feature_trace("probe.feature") as trace %}{{ trace.Override.Error.Message }}|{{ trace.Default.Error.Message }}|{{ trace.Override.Error.Metadata.token }}{{ trace.Default.Error.Metadata.token }}{{ trace.Override.Error.Source }}{{ trace.Default.Error.Source }}{{ trace.Override.Error.Location }}{{ trace.Override.Error.StackTrace }}{% endwith %}`)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := tpl.Execute(data)
	if err != nil {
		t.Fatal(err)
	}
	if want := overrideMessage + "|" + defaultMessage + "|"; rendered != want {
		t.Fatalf("unexpected trace rendered by Pongo2: got %q, want %q", rendered, want)
	}
}
