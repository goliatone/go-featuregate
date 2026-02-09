package optionsadapter

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/goliatone/go-featuregate/ferrors"
	opts "github.com/goliatone/go-options"
	"github.com/goliatone/go-options/pkg/state"
)

type memoryPreferencesStore struct {
	data             map[string]map[string]any
	resolveCalls     int
	upsertCalls      int
	deleteCalls      int
	lastResolveInput PreferencesResolveInput
	lastUpsertInput  PreferencesUpsertInput
	lastDeleteInput  PreferencesDeleteInput
}

func newMemoryPreferencesStore() *memoryPreferencesStore {
	return &memoryPreferencesStore{data: map[string]map[string]any{}}
}

func (m *memoryPreferencesStore) Resolve(_ context.Context, input PreferencesResolveInput) (PreferencesSnapshot, error) {
	m.resolveCalls++
	m.lastResolveInput = input

	combined := map[string]any{}
	for _, level := range input.Levels {
		bucket := m.data[m.bucketKey(level, input.Scope)]
		for key, value := range bucket {
			combined[key] = value
		}
	}
	if len(input.Keys) > 0 {
		filtered := map[string]any{}
		for _, key := range input.Keys {
			if value, ok := combined[key]; ok {
				filtered[key] = value
			}
		}
		combined = filtered
	}
	return PreferencesSnapshot{Effective: cloneAnyMap(combined)}, nil
}

func (m *memoryPreferencesStore) Upsert(_ context.Context, input PreferencesUpsertInput) (PreferencesSnapshot, error) {
	m.upsertCalls++
	m.lastUpsertInput = PreferencesUpsertInput{
		Scope:  input.Scope,
		Level:  input.Level,
		Values: cloneAnyMap(input.Values),
	}
	key := m.bucketKey(input.Level, input.Scope)
	if m.data[key] == nil {
		m.data[key] = map[string]any{}
	}
	for prefKey, value := range input.Values {
		m.data[key][prefKey] = value
	}
	return PreferencesSnapshot{Effective: cloneAnyMap(m.data[key])}, nil
}

func (m *memoryPreferencesStore) Delete(_ context.Context, input PreferencesDeleteInput) error {
	m.deleteCalls++
	m.lastDeleteInput = PreferencesDeleteInput{
		Scope: input.Scope,
		Level: input.Level,
		Keys:  append([]string(nil), input.Keys...),
	}
	key := m.bucketKey(input.Level, input.Scope)
	bucket := m.data[key]
	if bucket == nil {
		return nil
	}
	for _, prefKey := range input.Keys {
		delete(bucket, prefKey)
	}
	return nil
}

func (m *memoryPreferencesStore) seed(level PreferenceLevel, scope PreferenceScope, values map[string]any) {
	m.data[m.bucketKey(level, scope)] = cloneAnyMap(values)
}

func (m *memoryPreferencesStore) bucket(level PreferenceLevel, scope PreferenceScope) map[string]any {
	return m.data[m.bucketKey(level, scope)]
}

func (m *memoryPreferencesStore) bucketKey(level PreferenceLevel, scope PreferenceScope) string {
	return fmt.Sprintf("%s|%s|%s|%s", level, scope.TenantID, scope.OrgID, scope.UserID)
}

func cloneAnyMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func TestPreferencesStoreAdapterScopeMapping(t *testing.T) {
	adapter := NewPreferencesStoreAdapter(newMemoryPreferencesStore())

	cases := []struct {
		name      string
		scope     opts.Scope
		wantLevel PreferenceLevel
		wantScope PreferenceScope
		wantErr   error
	}{
		{
			name:      "system",
			scope:     opts.NewScope("system", 10),
			wantLevel: PreferenceLevelSystem,
			wantScope: PreferenceScope{},
		},
		{
			name: "tenant",
			scope: opts.NewScope("tenant", 20, opts.WithScopeMetadata(map[string]any{
				"tenant_id": "tenant-1",
			})),
			wantLevel: PreferenceLevelTenant,
			wantScope: PreferenceScope{TenantID: "tenant-1"},
		},
		{
			name: "org",
			scope: opts.NewScope("org", 30, opts.WithScopeMetadata(map[string]any{
				"tenant_id": "tenant-1",
				"org_id":    "org-1",
			})),
			wantLevel: PreferenceLevelOrg,
			wantScope: PreferenceScope{TenantID: "tenant-1", OrgID: "org-1"},
		},
		{
			name: "user",
			scope: opts.NewScope("user", 40, opts.WithScopeMetadata(map[string]any{
				"tenant_id": "tenant-1",
				"org_id":    "org-1",
				"user_id":   "user-1",
			})),
			wantLevel: PreferenceLevelUser,
			wantScope: PreferenceScope{TenantID: "tenant-1", OrgID: "org-1", UserID: "user-1"},
		},
		{
			name:    "tenant missing metadata",
			scope:   opts.NewScope("tenant", 20),
			wantErr: ErrPreferencesScopeMetadataInvalid,
		},
		{
			name: "user invalid metadata type",
			scope: opts.NewScope("user", 40, opts.WithScopeMetadata(map[string]any{
				"user_id": 9,
			})),
			wantErr: ErrPreferencesScopeMetadataInvalid,
		},
		{
			name:    "unsupported scope",
			scope:   opts.NewScope("role", 50),
			wantErr: ErrPreferencesScopeMetadataInvalid,
		},
	}

	for _, tc := range cases {
		level, gotScope, err := adapter.preferenceScope(tc.scope)
		if tc.wantErr != nil {
			if err == nil {
				t.Fatalf("%s: expected error", tc.name)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("%s: expected errors.Is(%v), got %v", tc.name, tc.wantErr, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if level != tc.wantLevel {
			t.Fatalf("%s: expected level %q, got %q", tc.name, tc.wantLevel, level)
		}
		if gotScope != tc.wantScope {
			t.Fatalf("%s: expected scope %#v, got %#v", tc.name, tc.wantScope, gotScope)
		}
	}
}

func TestPreferencesFlattenUnflattenRoundTripWithArrays(t *testing.T) {
	input := map[string]any{
		"users": map[string]any{
			"signup": true,
			"rules": []any{
				map[string]any{
					"name":    "early",
					"enabled": true,
				},
				map[string]any{
					"name":    "beta",
					"enabled": false,
				},
			},
		},
		"rollouts": []any{
			"alpha",
			map[string]any{"group": "staff"},
		},
	}

	flat, err := flattenSnapshot(input, nil)
	if err != nil {
		t.Fatalf("unexpected flatten error: %v", err)
	}
	got, err := unflattenSnapshot(flat)
	if err != nil {
		t.Fatalf("unexpected unflatten error: %v", err)
	}
	if !reflect.DeepEqual(input, got) {
		t.Fatalf("roundtrip mismatch\nwant: %#v\ngot:  %#v", input, got)
	}
}

func TestPreferencesFlattenRejectsAmbiguousMapKeys(t *testing.T) {
	_, err := flattenSnapshot(map[string]any{
		"users.signup": true,
	}, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, ErrPreferencesPathInvalid) {
		t.Fatalf("expected errors.Is(path invalid), got %v", err)
	}
}

func TestPreferencesUnflattenRejectsInvalidPath(t *testing.T) {
	_, err := unflattenSnapshot(map[string]any{
		"users..signup": true,
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, ErrPreferencesPathInvalid) {
		t.Fatalf("expected errors.Is(path invalid), got %v", err)
	}
}

func TestPreferencesSaveDeleteMissingDefaultFalse(t *testing.T) {
	ctx := context.Background()
	store := newMemoryPreferencesStore()
	scope := PreferenceScope{TenantID: "tenant-1"}
	store.seed(PreferenceLevelTenant, scope, map[string]any{
		"feature_flags.users.signup": true,
		"feature_flags.users.invite": true,
	})
	adapter := NewPreferencesStoreAdapter(store)

	ref := state.Ref{
		Domain: "feature_flags",
		Scope: opts.NewScope("tenant", 20, opts.WithScopeMetadata(map[string]any{
			"tenant_id": "tenant-1",
		})),
	}
	_, err := adapter.Save(ctx, ref, map[string]any{
		"users": map[string]any{
			"signup": false,
		},
	}, state.Meta{})
	if err != nil {
		t.Fatalf("unexpected save error: %v", err)
	}
	if store.upsertCalls != 1 {
		t.Fatalf("expected one upsert, got %d", store.upsertCalls)
	}
	if store.deleteCalls != 0 {
		t.Fatalf("expected zero deletes, got %d", store.deleteCalls)
	}
	if _, ok := store.bucket(PreferenceLevelTenant, scope)["feature_flags.users.invite"]; !ok {
		t.Fatalf("expected invite key to remain when delete_missing=false")
	}
}

func TestPreferencesSaveDiffDeletesMissingWhenEnabled(t *testing.T) {
	ctx := context.Background()
	store := newMemoryPreferencesStore()
	scope := PreferenceScope{TenantID: "tenant-1"}
	store.seed(PreferenceLevelTenant, scope, map[string]any{
		"feature_flags.users.signup": true,
		"feature_flags.users.invite": true,
		"other_domain.unrelated":     true,
	})
	adapter := NewPreferencesStoreAdapter(store, WithDeleteMissing(true))

	ref := state.Ref{
		Domain: "feature_flags",
		Scope: opts.NewScope("tenant", 20, opts.WithScopeMetadata(map[string]any{
			"tenant_id": "tenant-1",
		})),
	}
	_, err := adapter.Save(ctx, ref, map[string]any{
		"users": map[string]any{
			"signup": false,
		},
	}, state.Meta{})
	if err != nil {
		t.Fatalf("unexpected save error: %v", err)
	}
	if store.upsertCalls != 1 {
		t.Fatalf("expected one upsert, got %d", store.upsertCalls)
	}
	if store.deleteCalls != 1 {
		t.Fatalf("expected one delete, got %d", store.deleteCalls)
	}

	if got := store.lastUpsertInput.Values["feature_flags.users.signup"]; got != false {
		t.Fatalf("expected upsert signup=false, got %#v", got)
	}
	deleteKeys := append([]string(nil), store.lastDeleteInput.Keys...)
	sort.Strings(deleteKeys)
	if !reflect.DeepEqual(deleteKeys, []string{"feature_flags.users.invite"}) {
		t.Fatalf("unexpected delete keys: %#v", deleteKeys)
	}

	bucket := store.bucket(PreferenceLevelTenant, scope)
	if _, ok := bucket["feature_flags.users.invite"]; ok {
		t.Fatalf("expected invite key to be deleted")
	}
	if _, ok := bucket["other_domain.unrelated"]; !ok {
		t.Fatalf("expected unrelated key to remain")
	}
}

func TestPreferencesSaveWithAllowlistScopesMutations(t *testing.T) {
	ctx := context.Background()
	store := newMemoryPreferencesStore()
	scope := PreferenceScope{TenantID: "tenant-1"}
	store.seed(PreferenceLevelTenant, scope, map[string]any{
		"feature_flags.users.signup": true,
		"feature_flags.users.invite": true,
	})
	adapter := NewPreferencesStoreAdapter(
		store,
		WithKeys("users.signup"),
		WithDeleteMissing(true),
	)

	ref := state.Ref{
		Domain: "feature_flags",
		Scope: opts.NewScope("tenant", 20, opts.WithScopeMetadata(map[string]any{
			"tenant_id": "tenant-1",
		})),
	}
	_, err := adapter.Save(ctx, ref, map[string]any{
		"users": map[string]any{
			"signup": false,
			"invite": false,
		},
	}, state.Meta{})
	if err != nil {
		t.Fatalf("unexpected save error: %v", err)
	}

	if len(store.lastUpsertInput.Values) != 1 {
		t.Fatalf("expected one allowlisted upsert key, got %#v", store.lastUpsertInput.Values)
	}
	if _, ok := store.lastUpsertInput.Values["feature_flags.users.signup"]; !ok {
		t.Fatalf("expected allowlisted signup upsert")
	}
	if _, ok := store.bucket(PreferenceLevelTenant, scope)["feature_flags.users.invite"]; !ok {
		t.Fatalf("expected non-allowlisted invite key to remain untouched")
	}
	if len(store.lastDeleteInput.Keys) != 0 {
		t.Fatalf("expected no deletes for non-allowlisted keys, got %#v", store.lastDeleteInput.Keys)
	}

	loaded, _, ok, err := adapter.Load(ctx, ref)
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if !ok {
		t.Fatalf("expected snapshot")
	}
	if _, found := lookupPath(loaded, "users.signup"); !found {
		t.Fatalf("expected users.signup in loaded snapshot")
	}
	if _, found := lookupPath(loaded, "users.invite"); found {
		t.Fatalf("did not expect users.invite in allowlisted load")
	}
}

func TestPreferencesErrorContract(t *testing.T) {
	ctx := context.Background()

	systemRef := state.Ref{
		Domain: "feature_flags",
		Scope:  opts.NewScope("system", 10),
	}
	adapter := NewPreferencesStoreAdapter(nil)
	_, _, _, err := adapter.Load(ctx, systemRef)
	if err == nil {
		t.Fatalf("expected missing store error")
	}
	if !errors.Is(err, ErrPreferencesStoreRequired) {
		t.Fatalf("expected errors.Is(missing store), got %v", err)
	}

	adapter = NewPreferencesStoreAdapter(newMemoryPreferencesStore())
	badTenantRef := state.Ref{
		Domain: "feature_flags",
		Scope:  opts.NewScope("tenant", 20),
	}
	_, _, _, err = adapter.Load(ctx, badTenantRef)
	if err == nil {
		t.Fatalf("expected invalid scope metadata error")
	}
	if !errors.Is(err, ErrPreferencesScopeMetadataInvalid) {
		t.Fatalf("expected errors.Is(scope metadata invalid), got %v", err)
	}

	pathStore := newMemoryPreferencesStore()
	pathStore.seed(PreferenceLevelSystem, PreferenceScope{}, map[string]any{
		"feature_flags.users..signup": true,
	})
	adapter = NewPreferencesStoreAdapter(pathStore)
	_, _, _, err = adapter.Load(ctx, systemRef)
	if err == nil {
		t.Fatalf("expected invalid path error")
	}
	if !errors.Is(err, ErrPreferencesPathInvalid) {
		t.Fatalf("expected errors.Is(path invalid), got %v", err)
	}
	rich, ok := ferrors.As(err)
	if !ok {
		t.Fatalf("expected rich error payload")
	}
	if rich.Metadata == nil || rich.Metadata[ferrors.MetaOperation] == nil {
		t.Fatalf("expected wrapped metadata, got %#v", rich.Metadata)
	}
}
