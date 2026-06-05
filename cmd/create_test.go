package cmd

import (
	"testing"

	"github.com/gravitee-io-labs/gck/internal/config"
)

func TestGetPreloadRefs_NoBuilds(t *testing.T) {
	c := &config.Config{
		Images: config.ImagesConfig{
			Preload: &config.PreloadConfig{
				Refs: []string{"mongo:7", "postgres:17"},
			},
		},
	}
	refs := getPreloadRefs(c)
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d: %v", len(refs), refs)
	}
}

func TestGetPreloadRefs_BuildImagesSkipped(t *testing.T) {
	c := &config.Config{
		Images: config.ImagesConfig{
			Preload: &config.PreloadConfig{
				Refs: []string{"mongo:7", "graviteeio/apim-gateway:latest", "postgres:17"},
			},
		},
		Builds: []config.Build{
			{Name: "gateway", Image: "graviteeio/apim-gateway:latest"},
		},
	}
	refs := getPreloadRefs(c)
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs (build image skipped), got %d: %v", len(refs), refs)
	}
	for _, r := range refs {
		if r == "graviteeio/apim-gateway:latest" {
			t.Fatal("expected build image to be excluded from preload refs")
		}
	}
}

func TestGetPreloadRefs_MultipleBuildsSkipped(t *testing.T) {
	c := &config.Config{
		Images: config.ImagesConfig{
			Preload: &config.PreloadConfig{
				Refs: []string{
					"mongo:7",
					"graviteeio/apim-gateway:latest",
					"graviteeio/apim-console-ui:latest",
					"postgres:17",
				},
			},
		},
		Builds: []config.Build{
			{Name: "gateway", Image: "graviteeio/apim-gateway:latest"},
			{Name: "ui", Image: "graviteeio/apim-console-ui:latest"},
		},
	}
	refs := getPreloadRefs(c)
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d: %v", len(refs), refs)
	}
	refSet := make(map[string]bool)
	for _, r := range refs {
		refSet[r] = true
	}
	if !refSet["mongo:7"] || !refSet["postgres:17"] {
		t.Fatalf("expected mongo:7 and postgres:17 to remain, got %v", refs)
	}
}

func TestGetPreloadRefs_BuildImageNotInPreload(t *testing.T) {
	c := &config.Config{
		Images: config.ImagesConfig{
			Preload: &config.PreloadConfig{
				Refs: []string{"mongo:7", "postgres:17"},
			},
		},
		Builds: []config.Build{
			{Name: "gateway", Image: "graviteeio/apim-gateway:latest"},
		},
	}
	refs := getPreloadRefs(c)
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs unchanged, got %d: %v", len(refs), refs)
	}
}

func TestGetPreloadRefs_NilPreload(t *testing.T) {
	c := &config.Config{
		Builds: []config.Build{
			{Name: "gateway", Image: "graviteeio/apim-gateway:latest"},
		},
	}
	refs := getPreloadRefs(c)
	if refs != nil {
		t.Fatalf("expected nil refs with nil preload, got %v", refs)
	}
}

func TestGetPreloadRefs_CombinesWithExplicitSkip(t *testing.T) {
	c := &config.Config{
		Images: config.ImagesConfig{
			Preload: &config.PreloadConfig{
				Refs: []string{"mongo:7", "graviteeio/apim-gateway:latest", "elastic:8"},
				Skip: []string{"elastic:8"},
			},
		},
		Builds: []config.Build{
			{Name: "gateway", Image: "graviteeio/apim-gateway:latest"},
		},
	}
	refs := getPreloadRefs(c)
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref (both explicit skip and build skip applied), got %d: %v", len(refs), refs)
	}
	if refs[0] != "mongo:7" {
		t.Fatalf("expected mongo:7 to remain, got %v", refs)
	}
}

func TestGetPreloadRefs_AllBuildImagesSkipped(t *testing.T) {
	c := &config.Config{
		Images: config.ImagesConfig{
			Preload: &config.PreloadConfig{
				Refs: []string{
					"graviteeio/apim-gateway:latest",
					"graviteeio/apim-console-ui:latest",
				},
			},
		},
		Builds: []config.Build{
			{Name: "gateway", Image: "graviteeio/apim-gateway:latest"},
			{Name: "ui", Image: "graviteeio/apim-console-ui:latest"},
		},
	}
	refs := getPreloadRefs(c)
	if len(refs) != 0 {
		t.Fatalf("expected 0 refs when all preload images are builds, got %d: %v", len(refs), refs)
	}
}

func TestGetPreloadRefs_EmptyBuildsSlice(t *testing.T) {
	c := &config.Config{
		Images: config.ImagesConfig{
			Preload: &config.PreloadConfig{
				Refs: []string{"mongo:7"},
			},
		},
		Builds: []config.Build{},
	}
	refs := getPreloadRefs(c)
	if len(refs) != 1 || refs[0] != "mongo:7" {
		t.Fatalf("expected [mongo:7] unchanged with empty builds, got %v", refs)
	}
}

// selectBuilds tests

func TestSelectBuilds_AllWhenNoNames(t *testing.T) {
	all := []config.Build{
		{Name: "gateway", Image: "graviteeio/apim-gateway:latest"},
		{Name: "ui", Image: "graviteeio/apim-console-ui:latest"},
	}
	result, err := selectBuilds(all, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected all 2 builds, got %d", len(result))
	}
}

func TestSelectBuilds_FilterByName(t *testing.T) {
	all := []config.Build{
		{Name: "gateway", Image: "graviteeio/apim-gateway:latest"},
		{Name: "ui", Image: "graviteeio/apim-console-ui:latest"},
		{Name: "api", Image: "graviteeio/apim-rest-api:latest"},
	}
	result, err := selectBuilds(all, []string{"ui"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 build, got %d", len(result))
	}
	if result[0].Name != "ui" {
		t.Fatalf("expected build %q, got %q", "ui", result[0].Name)
	}
}

func TestSelectBuilds_MultipleNames(t *testing.T) {
	all := []config.Build{
		{Name: "gateway", Image: "graviteeio/apim-gateway:latest"},
		{Name: "ui", Image: "graviteeio/apim-console-ui:latest"},
		{Name: "api", Image: "graviteeio/apim-rest-api:latest"},
	}
	result, err := selectBuilds(all, []string{"gateway", "api"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 builds, got %d", len(result))
	}
	if result[0].Name != "gateway" || result[1].Name != "api" {
		t.Fatalf("expected [gateway, api], got [%s, %s]", result[0].Name, result[1].Name)
	}
}

func TestSelectBuilds_UnknownNameError(t *testing.T) {
	all := []config.Build{
		{Name: "gateway", Image: "graviteeio/apim-gateway:latest"},
	}
	_, err := selectBuilds(all, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown build name")
	}
}

func TestSelectBuilds_EmptyBuilds(t *testing.T) {
	result, err := selectBuilds(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 builds, got %d", len(result))
	}
}
