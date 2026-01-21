package optionsadapter

import (
	"context"
	"testing"

	"github.com/goliatone/go-admin/admin"

	"github.com/goliatone/go-featuregate/gate"
)

func TestPreferencesStoreAdapterSetAndGet(t *testing.T) {
	ctx := context.Background()
	prefs := admin.NewInMemoryPreferencesStore()
	stateStore := NewPreferencesStoreAdapter(prefs)
	store := NewStore(stateStore)

	scopeSet := gate.ScopeSet{OrgID: "org-1"}
	if err := store.Set(ctx, "users.signup", scopeSet, true, gate.ActorRef{ID: "actor-1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	override, err := store.Get(ctx, "users.signup", scopeSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if override.State != gate.OverrideStateEnabled {
		t.Fatalf("expected enabled override, got %q", override.State)
	}

	snapshot, err := prefs.Resolve(ctx, admin.PreferencesResolveInput{
		Scope:  admin.PreferenceScope{OrgID: "org-1"},
		Levels: []admin.PreferenceLevel{admin.PreferenceLevelOrg},
		Keys:   []string{"feature_flags.users.signup"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snapshot.Effective["feature_flags.users.signup"] != true {
		t.Fatalf("expected stored preference value to be true")
	}
}
