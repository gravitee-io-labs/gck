package cmd

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gravitee-io-labs/gck/internal/config"
	"github.com/gravitee-io-labs/gck/internal/registry"
	"github.com/gravitee-io-labs/gck/internal/state"
)

func TestMergeSet_OverridesWin(t *testing.T) {
	inherited := map[string]string{"imagePrefix": "azurecr", "imageTag": "4.11"}
	overrides := map[string]string{"imageTag": "4.12", "helmVersion": "4.12.0"}

	merged := mergeSet(inherited, overrides)

	if merged["imagePrefix"] != "azurecr" {
		t.Errorf("imagePrefix = %q, want azurecr (inherited)", merged["imagePrefix"])
	}
	if merged["imageTag"] != "4.12" {
		t.Errorf("imageTag = %q, want 4.12 (override wins)", merged["imageTag"])
	}
	if merged["helmVersion"] != "4.12.0" {
		t.Errorf("helmVersion = %q, want 4.12.0 (override-only key)", merged["helmVersion"])
	}
	if inherited["imageTag"] != "4.11" {
		t.Error("mergeSet must not mutate its inputs")
	}
}

func TestMergeSet_NilInputs(t *testing.T) {
	if got := mergeSet(nil, nil); len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
	if got := mergeSet(map[string]string{"a": "1"}, nil); got["a"] != "1" {
		t.Errorf("expected inherited a=1, got %v", got)
	}
}

func TestUnionFlags_OrderAndDedup(t *testing.T) {
	got := unionFlags(
		[]string{"disable-es", "disable-portal"},
		[]string{"disable-portal", "enable-redis"},
	)
	want := []string{"disable-es", "disable-portal", "enable-redis"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v (order preserved, deduplicated)", got, want)
		}
	}
}

func TestInheritClusterState_ReusesCreateContext(t *testing.T) {
	gckHome = t.TempDir()
	// Patch-time inputs: bump imageTag only, no --from / --registry.
	setOverrides = map[string]string{"imageTag": "master-latest"}
	cfg = &config.Config{}
	inheritedUse, inheritedFlags, inheritedSelected = nil, nil, nil

	st := &state.ClusterState{
		Name:     "gravitee",
		Registry: "file:///registry",
		From:     []string{"gravitee-io/apim"},
		Flags:    []string{"use-elasticsearch", "use-mongodb", "disable-portal"},
		Set:      map[string]string{"imagePrefix": "graviteeio.azurecr.io", "imageTag": "4.11.x-latest"},
	}
	if err := state.Save(filepath.Join(gckHome, "clusters"), st); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := inheritClusterState("gravitee")
	if got == nil {
		t.Fatal("expected the loaded state to be returned")
	}
	if len(cfg.From) != 1 || cfg.From[0] != "gravitee-io/apim" {
		t.Errorf("cfg.From = %v, want the create-time from inherited", cfg.From)
	}
	if cfg.Registry != "file:///registry" {
		t.Errorf("cfg.Registry = %q, want the create-time registry inherited", cfg.Registry)
	}
	if setOverrides["imagePrefix"] != "graviteeio.azurecr.io" {
		t.Errorf("imagePrefix not inherited from create-time --set: %q", setOverrides["imagePrefix"])
	}
	if setOverrides["imageTag"] != "master-latest" {
		t.Errorf("imageTag = %q, want master-latest (patch-time wins)", setOverrides["imageTag"])
	}
	if len(got.Flags) != 3 {
		t.Errorf("expected 3 inherited flags, got %v", got.Flags)
	}
	// The selected alternatives are re-selected so the context resolves to
	// the same implementations it was created with.
	if want := []string{"use-elasticsearch", "use-mongodb"}; !slices.Equal(inheritedUse, want) {
		t.Errorf("inheritedUse = %v, want %v", inheritedUse, want)
	}
	if want := []string{"disable-portal"}; !slices.Equal(inheritedFlags, want) {
		t.Errorf("inheritedFlags = %v, want %v", inheritedFlags, want)
	}
}

func TestCheckPatchUse(t *testing.T) {
	st := &state.ClusterState{Name: "gravitee", Flags: []string{"use-elasticsearch", "use-jdbc-postgres"}}

	if err := checkPatchUse(st, []string{"use-jdbc-postgres"}); err != nil {
		t.Errorf("re-selecting the create-time alternative: %v", err)
	}
	if err := checkPatchUse(nil, []string{"use-mongodb"}); err != nil {
		t.Errorf("a cluster without state must not be checked: %v", err)
	}
	err := checkPatchUse(st, []string{"use-mongodb"})
	if err == nil || !strings.Contains(err.Error(), "recreate it to switch") {
		t.Errorf("expected switching alternative on patch to be refused, got %v", err)
	}
}

func TestInheritClusterState_DoesNotOverrideExplicitInputs(t *testing.T) {
	gckHome = t.TempDir()
	setOverrides = map[string]string{}
	cfg = &config.Config{
		From:     []string{"explicit/from"},
		Registry: "file:///explicit",
	}

	st := &state.ClusterState{
		Name:     "gravitee",
		Registry: "file:///stored",
		From:     []string{"stored/from"},
	}
	if err := state.Save(filepath.Join(gckHome, "clusters"), st); err != nil {
		t.Fatalf("Save: %v", err)
	}

	inheritClusterState("gravitee")

	if len(cfg.From) != 1 || cfg.From[0] != "explicit/from" {
		t.Errorf("cfg.From = %v, want the explicit from preserved", cfg.From)
	}
	if cfg.Registry != "file:///explicit" {
		t.Errorf("cfg.Registry = %q, want the explicit registry preserved", cfg.Registry)
	}
}

func TestInheritClusterState_NoStateFile(t *testing.T) {
	gckHome = t.TempDir()
	setOverrides = map[string]string{"imageTag": "x"}
	cfg = &config.Config{}

	if got := inheritClusterState("missing"); got != nil {
		t.Errorf("expected nil for a cluster with no state file, got %v", got)
	}
	if len(cfg.From) != 0 || setOverrides["imageTag"] != "x" {
		t.Error("inheritClusterState must not mutate state when no file exists")
	}
}

func TestInheritClusterState_EmptyName(t *testing.T) {
	if got := inheritClusterState(""); got != nil {
		t.Errorf("expected nil for an empty cluster name, got %v", got)
	}
}

// writeTwoProducts writes products a and b, each with a datasource group of
// members mongo and pg; a defaults to pg, b to mongo.
func writeTwoProducts(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, p := range []struct{ name, def string }{{"a", "pg"}, {"b", "mongo"}} {
		writeFile(t, filepath.Join(root, p.name, "gck.yaml"), `
components:
  - name: `+p.name+`
    helm:
      chart: repo/`+p.name+`
`)
		for _, m := range []string{"mongo", "pg"} {
			writeFile(t, filepath.Join(root, p.name, "gck--use-"+m+".yaml"), fmt.Sprintf(`
description: %q
group: datasource
default: %t
`, p.name+" on "+m, m == p.def))
		}
	}
	return root
}

func TestPatch_ReplaysEachContextsSelection(t *testing.T) {
	root := writeTwoProducts(t)
	resetContextConfigCache()
	gckHome = t.TempDir()
	setOverrides = map[string]string{}
	cfg = &config.Config{}
	inheritedUse, inheritedFlags, inheritedSelected = nil, nil, nil
	t.Cleanup(func() { inheritedUse, inheritedFlags, inheritedSelected = nil, nil, nil })

	// Both groups are named datasource and both have mongo and pg: by name,
	// the state's flags cannot say which product had which.
	created := map[string]string{
		registry.SelectionKey("a", "datasource"): "use-mongo",
		registry.SelectionKey("b", "datasource"): "use-pg",
	}
	st := &state.ClusterState{
		Name:     "gravitee",
		Registry: "file://" + root,
		From:     []string{"a", "b"},
		Flags:    []string{"use-mongo", "use-pg"},
		Selected: created,
	}
	if err := state.Save(filepath.Join(gckHome, "clusters"), st); err != nil {
		t.Fatalf("Save: %v", err)
	}
	inherited := inheritClusterState("gravitee")
	cfg.Kind.ApplyDefaults()
	if len(inheritedUse) != 0 {
		t.Fatalf("inheritedUse = %v, want none when the state records Selected", inheritedUse)
	}

	resolved, err := resolveContextConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !maps.Equal(resolved.Selected, created) {
		t.Fatalf("Selected = %v, want the create-time %v", resolved.Selected, created)
	}
	if err := checkPatchSelection(inherited, resolved); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// --use-pg on patch passes checkPatchUse, since b was created with it,
	// but it would switch a.
	if err := checkPatchUse(inherited, []string{"use-pg"}); err != nil {
		t.Fatalf("checkPatchUse: %v", err)
	}
	resolved.Selected[registry.SelectionKey("a", "datasource")] = "use-pg"
	err = checkPatchSelection(inherited, resolved)
	if err == nil || !strings.Contains(err.Error(), "created with --use-mongo for group datasource of a") {
		t.Fatalf("expected the switch of a to be refused, got %v", err)
	}
}
