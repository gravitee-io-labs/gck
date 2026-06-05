package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gravitee-io-labs/gck/internal/config"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	cs := &ClusterState{
		Name:      "test-cluster",
		CreatedAt: time.Date(2026, 3, 18, 14, 0, 0, 0, time.UTC),
		Features: config.FeaturesConfig{
			LB: &config.LBConfig{Enabled: true},
		},
		Images: config.ImagesConfig{
			Mirrors: &config.MirrorsConfig{
				Upstreams: []string{"docker.io"},
			},
		},
		Notes: DeleteNotes{Delete: "remember to clean up"},
	}

	if err := Save(dir, cs); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "test-cluster.yaml")); err != nil {
		t.Fatalf("state file not created: %v", err)
	}

	loaded, err := Load(dir, "test-cluster")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Name != cs.Name {
		t.Errorf("Name = %q, want %q", loaded.Name, cs.Name)
	}
	if !loaded.CreatedAt.Equal(cs.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", loaded.CreatedAt, cs.CreatedAt)
	}
	if loaded.Features.LB == nil || !loaded.Features.LB.Enabled {
		t.Error("Features.LB.Enabled should be true")
	}
	if loaded.Images.Mirrors == nil || len(loaded.Images.Mirrors.Upstreams) != 1 {
		t.Error("Images.Mirrors.Upstreams should have one entry")
	}
	if loaded.Notes.Delete != cs.Notes.Delete {
		t.Errorf("Notes.Delete = %q, want %q", loaded.Notes.Delete, cs.Notes.Delete)
	}
}

func TestSaveAndLoadWithContextInfo(t *testing.T) {
	dir := t.TempDir()

	cs := &ClusterState{
		Name:      "gio-apim",
		CreatedAt: time.Date(2026, 3, 18, 14, 0, 0, 0, time.UTC),
		Registry:  "file://./registry",
		From:      []string{"gravitee-io/oss/apim/jdbc/postgres"},
		Features: config.FeaturesConfig{
			DNS: &config.DNSConfig{Enabled: true, Domain: "gck.local"},
		},
	}

	if err := Save(dir, cs); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(dir, "gio-apim")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Registry != cs.Registry {
		t.Errorf("Registry = %q, want %q", loaded.Registry, cs.Registry)
	}
	if len(loaded.From) != 1 || loaded.From[0] != cs.From[0] {
		t.Errorf("From = %v, want %v", loaded.From, cs.From)
	}
}

func TestSaveAndLoadWithoutContextInfo(t *testing.T) {
	dir := t.TempDir()

	cs := &ClusterState{
		Name:      "plain",
		CreatedAt: time.Now(),
	}

	if err := Save(dir, cs); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(dir, "plain")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Registry != "" {
		t.Errorf("Registry should be empty, got %q", loaded.Registry)
	}
	if len(loaded.From) != 0 {
		t.Errorf("From should be empty, got %v", loaded.From)
	}
}

func TestLoadNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing state file")
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()

	names, err := List(dir)
	if err != nil {
		t.Fatalf("List on empty dir: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("expected 0 names, got %d", len(names))
	}

	for _, name := range []string{"alpha", "beta", "gamma"} {
		cs := &ClusterState{Name: name, CreatedAt: time.Now()}
		if err := Save(dir, cs); err != nil {
			t.Fatalf("Save %q: %v", name, err)
		}
	}

	// Create a subdirectory that should be ignored.
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Create a non-yaml file that should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	names, err = List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d: %v", len(names), names)
	}
}

func TestListNonexistentDir(t *testing.T) {
	names, err := List(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("List on nonexistent dir should not error: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("expected 0 names, got %d", len(names))
	}
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()

	cs := &ClusterState{Name: "doomed", CreatedAt: time.Now()}
	if err := Save(dir, cs); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := Remove(dir, "doomed"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "doomed.yaml")); !os.IsNotExist(err) {
		t.Fatal("state file should have been removed")
	}
}

func TestRemoveNonexistent(t *testing.T) {
	dir := t.TempDir()
	if err := Remove(dir, "nope"); err != nil {
		t.Fatalf("Remove nonexistent should not error: %v", err)
	}
}

func TestSaveOverwrites(t *testing.T) {
	dir := t.TempDir()

	cs := &ClusterState{Name: "cluster", CreatedAt: time.Now(), Notes: DeleteNotes{Delete: "v1"}}
	if err := Save(dir, cs); err != nil {
		t.Fatalf("Save v1: %v", err)
	}

	cs.Notes.Delete = "v2"
	if err := Save(dir, cs); err != nil {
		t.Fatalf("Save v2: %v", err)
	}

	loaded, err := Load(dir, "cluster")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Notes.Delete != "v2" {
		t.Errorf("Notes.Delete = %q, want %q", loaded.Notes.Delete, "v2")
	}
}
