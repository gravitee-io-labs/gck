package cmd

import (
	"path/filepath"
	"testing"

	"github.com/gravitee-io-labs/gck/internal/config"
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

	st := &state.ClusterState{
		Name:     "gravitee",
		Registry: "file:///registry",
		From:     []string{"gravitee-io/oss/apim/mongodb"},
		Flags:    []string{"disable-es", "disable-portal"},
		Set:      map[string]string{"imagePrefix": "graviteeio.azurecr.io", "imageTag": "4.11.x-latest"},
	}
	if err := state.Save(filepath.Join(gckHome, "clusters"), st); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := inheritClusterState("gravitee")
	if got == nil {
		t.Fatal("expected the loaded state to be returned")
	}
	if len(cfg.From) != 1 || cfg.From[0] != "gravitee-io/oss/apim/mongodb" {
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
	if len(got.Flags) != 2 {
		t.Errorf("expected 2 inherited flags, got %v", got.Flags)
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
