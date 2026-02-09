package optionsadapter

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPreferencesStoreAdapterExternalCompile(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	moduleRoot := filepath.Clean(filepath.Join(wd, "..", ".."))

	tmp := t.TempDir()
	goMod := fmt.Sprintf(`module example.com/preferencesadaptercompile

go 1.24

require github.com/goliatone/go-featuregate v0.0.0

replace github.com/goliatone/go-featuregate => %s
`, moduleRoot)
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	testFile := `package compiletest

import (
	"context"
	"testing"

	"github.com/goliatone/go-featuregate/adapters/optionsadapter"
	opts "github.com/goliatone/go-options"
	"github.com/goliatone/go-options/pkg/state"
)

type prefStore struct{}

func (prefStore) Resolve(_ context.Context, _ optionsadapter.PreferencesResolveInput) (optionsadapter.PreferencesSnapshot, error) {
	return optionsadapter.PreferencesSnapshot{}, nil
}

func (prefStore) Upsert(_ context.Context, _ optionsadapter.PreferencesUpsertInput) (optionsadapter.PreferencesSnapshot, error) {
	return optionsadapter.PreferencesSnapshot{}, nil
}

func (prefStore) Delete(_ context.Context, _ optionsadapter.PreferencesDeleteInput) error {
	return nil
}

func TestCompile(t *testing.T) {
	var store state.Store[map[string]any] = optionsadapter.NewPreferencesStoreAdapter(
		prefStore{},
		optionsadapter.WithKeyPrefix("feature_flags"),
		optionsadapter.WithKeys("users.signup"),
		optionsadapter.WithDeleteMissing(false),
	)
	ref := state.Ref{
		Domain: "feature_flags",
		Scope:  opts.NewScope("system", 10),
	}
	_, _, _, err := store.Load(context.Background(), ref)
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
}
`
	if err := os.WriteFile(filepath.Join(tmp, "adapter_compile_test.go"), []byte(testFile), 0o644); err != nil {
		t.Fatalf("write compile test: %v", err)
	}

	cmd := exec.Command("go", "test", "-mod=mod", "./...")
	cmd.Dir = tmp
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("external compile test failed: %v\n%s", err, string(out))
	}
}
