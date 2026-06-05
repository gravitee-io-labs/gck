package registry

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gravitee-io-labs/gck/internal/config"
)

func TestDiscoverFlags_NoFlagFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "gck.yaml"), `components: []`)

	flags, err := DiscoverFlags(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(flags) != 0 {
		t.Fatalf("expected no flags, got %d", len(flags))
	}
}

func TestDiscoverFlags_SingleFlag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "gck--disable-portal.yaml"), `
description: "Disable the developer portal UI"
components:
  - name: apim
    helm:
      values:
        portal:
          enabled: false
`)

	flags, err := DiscoverFlags(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(flags) != 1 {
		t.Fatalf("expected 1 flag, got %d", len(flags))
	}
	if flags[0].Name != "disable-portal" {
		t.Fatalf("expected flag name %q, got %q", "disable-portal", flags[0].Name)
	}
	if flags[0].Description != "Disable the developer portal UI" {
		t.Fatalf("expected description %q, got %q", "Disable the developer portal UI", flags[0].Description)
	}
	if flags[0].Dir != dir {
		t.Fatalf("expected dir %q, got %q", dir, flags[0].Dir)
	}
}

func TestDiscoverFlags_MultipleFlags(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "gck--disable-es.yaml"), `
description: "Disable Elasticsearch"
components: []
`)
	writeFile(t, filepath.Join(dir, "gck--disable-portal.yaml"), `
description: "Disable portal"
components: []
`)
	writeFile(t, filepath.Join(dir, "gck--disable-ui.yaml"), `
description: "Disable all UIs"
components: []
`)

	flags, err := DiscoverFlags(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(flags) != 3 {
		t.Fatalf("expected 3 flags, got %d", len(flags))
	}
	names := make([]string, len(flags))
	for i, f := range flags {
		names[i] = f.Name
	}
	expected := []string{"disable-es", "disable-portal", "disable-ui"}
	for i, name := range names {
		if name != expected[i] {
			t.Fatalf("expected sorted order %v, got %v", expected, names)
		}
	}
}

func TestDiscoverFlags_InvalidFlagName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "gck--No_Portal.yaml"), `
description: "bad name"
components: []
`)

	_, err := DiscoverFlags(dir)
	if err == nil {
		t.Fatal("expected error for invalid flag name")
	}
	if !strings.Contains(err.Error(), "must match") {
		t.Fatalf("expected naming convention error, got: %v", err)
	}
}

func TestDiscoverFlags_NoDescription(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "gck--simple.yaml"), `
components:
  - name: app
    helm:
      values:
        debug: true
`)

	flags, err := DiscoverFlags(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(flags) != 1 {
		t.Fatalf("expected 1 flag, got %d", len(flags))
	}
	if flags[0].Description != "" {
		t.Fatalf("expected empty description, got %q", flags[0].Description)
	}
}

func TestDiscoverFlags_IgnoresNonFlagFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "gck.yaml"), `components: []`)
	writeFile(t, filepath.Join(dir, "values.yaml"), `key: val`)
	writeFile(t, filepath.Join(dir, "gck--debug.yaml"), `
description: "Enable debug mode"
components: []
`)

	flags, err := DiscoverFlags(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(flags) != 1 {
		t.Fatalf("expected 1 flag (ignoring non-flag files), got %d", len(flags))
	}
	if flags[0].Name != "debug" {
		t.Fatalf("expected flag name %q, got %q", "debug", flags[0].Name)
	}
}

func TestMergeFlags_EmptyBase(t *testing.T) {
	child := []config.ContextFlag{
		{Name: "disable-portal", Description: "Disable portal", Dir: "/child"},
	}
	result := MergeFlags(nil, child)
	if len(result) != 1 {
		t.Fatalf("expected 1 flag, got %d", len(result))
	}
	if result[0].Name != "disable-portal" {
		t.Fatalf("expected %q, got %q", "disable-portal", result[0].Name)
	}
}

func TestMergeFlags_EmptyChild(t *testing.T) {
	base := []config.ContextFlag{
		{Name: "disable-portal", Description: "Disable portal", Dir: "/base"},
	}
	result := MergeFlags(base, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 flag, got %d", len(result))
	}
	if result[0].Dir != "/base" {
		t.Fatalf("expected dir from base, got %q", result[0].Dir)
	}
}

func TestMergeFlags_ChildOverridesBase(t *testing.T) {
	base := []config.ContextFlag{
		{Name: "disable-portal", Description: "Base description", Dir: "/base"},
		{Name: "disable-es", Description: "Disable ES", Dir: "/base"},
	}
	child := []config.ContextFlag{
		{Name: "disable-portal", Description: "Child description", Dir: "/child"},
	}
	result := MergeFlags(base, child)
	if len(result) != 2 {
		t.Fatalf("expected 2 flags, got %d", len(result))
	}
	if result[0].Description != "Child description" {
		t.Fatalf("expected child description to win, got %q", result[0].Description)
	}
	if result[0].Dir != "/child" {
		t.Fatalf("expected child dir to win, got %q", result[0].Dir)
	}
	if result[1].Name != "disable-es" {
		t.Fatalf("expected disable-es preserved, got %q", result[1].Name)
	}
}

func TestMergeFlags_ChildAddsNew(t *testing.T) {
	base := []config.ContextFlag{
		{Name: "disable-portal", Description: "Disable portal", Dir: "/base"},
	}
	child := []config.ContextFlag{
		{Name: "disable-ui", Description: "Disable all UIs", Dir: "/child"},
	}
	result := MergeFlags(base, child)
	if len(result) != 2 {
		t.Fatalf("expected 2 flags, got %d", len(result))
	}
	if result[0].Name != "disable-portal" {
		t.Fatalf("expected disable-portal first, got %q", result[0].Name)
	}
	if result[1].Name != "disable-ui" {
		t.Fatalf("expected disable-ui appended, got %q", result[1].Name)
	}
}

func TestMergeFlags_BaseNotMutated(t *testing.T) {
	base := []config.ContextFlag{
		{Name: "disable-portal", Description: "Base", Dir: "/base"},
	}
	child := []config.ContextFlag{
		{Name: "disable-portal", Description: "Child", Dir: "/child"},
	}
	MergeFlags(base, child)
	if base[0].Description != "Base" {
		t.Fatal("expected base slice not mutated")
	}
}

func TestApplyFlags_NoActiveFlags(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{Name: "app", Helm: &config.HelmSpec{Chart: "app/chart"}},
		},
		Flags: []config.ContextFlag{
			{Name: "disable-portal", Dir: "/some/dir"},
		},
	}
	err := ApplyFlags(resolved, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved.Components) != 1 {
		t.Fatal("expected components unchanged")
	}
}

func TestApplyFlags_UnknownFlag(t *testing.T) {
	resolved := &config.ResolvedContext{
		Flags: []config.ContextFlag{
			{Name: "disable-portal", Dir: "/some/dir"},
		},
	}
	err := ApplyFlags(resolved, []string{"no-such-flag"}, nil)
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
	if !strings.Contains(err.Error(), "unknown context flag --no-such-flag") {
		t.Fatalf("expected unknown flag error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--disable-portal") {
		t.Fatalf("expected available flags listed, got: %v", err)
	}
}

func TestApplyFlags_AppliesPatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "gck--disable-portal.yaml"), `
description: "Disable the developer portal UI"
components:
  - name: apim
    helm:
      values:
        portal:
          enabled: false
`)

	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{
				Name: "apim",
				Helm: &config.HelmSpec{
					Chart: "graviteeio/apim",
					Values: map[string]interface{}{
						"portal": map[string]interface{}{
							"enabled": true,
							"title":   "Dev Portal",
						},
					},
				},
			},
		},
		Flags: []config.ContextFlag{
			{Name: "disable-portal", Description: "Disable the developer portal UI", Dir: dir},
		},
	}

	err := ApplyFlags(resolved, []string{"disable-portal"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resolved.Components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(resolved.Components))
	}
	portal, ok := resolved.Components[0].Helm.Values["portal"].(map[string]interface{})
	if !ok {
		t.Fatal("expected portal values map")
	}
	if portal["enabled"] != false {
		t.Fatalf("expected portal.enabled=false, got %v", portal["enabled"])
	}
	if portal["title"] != "Dev Portal" {
		t.Fatalf("expected portal.title preserved, got %v", portal["title"])
	}
}

func TestApplyFlags_MultipleFlags(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "gck--disable-portal.yaml"), `
description: "Disable portal"
components:
  - name: apim
    helm:
      values:
        portal:
          enabled: false
`)
	writeFile(t, filepath.Join(dir, "gck--disable-es.yaml"), `
description: "Disable Elasticsearch"
components:
  - name: apim
    helm:
      values:
        es:
          enabled: false
`)

	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{
				Name: "apim",
				Helm: &config.HelmSpec{
					Chart: "graviteeio/apim",
					Values: map[string]interface{}{
						"portal": map[string]interface{}{"enabled": true},
						"es":     map[string]interface{}{"enabled": true},
					},
				},
			},
		},
		Flags: []config.ContextFlag{
			{Name: "disable-portal", Dir: dir},
			{Name: "disable-es", Dir: dir},
		},
	}

	err := ApplyFlags(resolved, []string{"disable-portal", "disable-es"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	vals := resolved.Components[0].Helm.Values
	portal, _ := vals["portal"].(map[string]interface{})
	es, _ := vals["es"].(map[string]interface{})
	if portal["enabled"] != false {
		t.Fatalf("expected portal disabled, got %v", portal["enabled"])
	}
	if es["enabled"] != false {
		t.Fatalf("expected es disabled, got %v", es["enabled"])
	}
}

func TestApplyFlags_AddsNewComponent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "gck--with-monitoring.yaml"), `
description: "Add monitoring stack"
components:
  - name: prometheus
    helm:
      chart: prometheus/prometheus
`)

	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{Name: "app", Helm: &config.HelmSpec{Chart: "app/chart"}},
		},
		Flags: []config.ContextFlag{
			{Name: "with-monitoring", Dir: dir},
		},
	}

	err := ApplyFlags(resolved, []string{"with-monitoring"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resolved.Components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(resolved.Components))
	}
	if resolved.Components[1].Name != "prometheus" {
		t.Fatalf("expected prometheus component added, got %q", resolved.Components[1].Name)
	}
}

func TestApplyFlags_FlagFromDifferentDir(t *testing.T) {
	parentDir := t.TempDir()
	childDir := t.TempDir()

	writeFile(t, filepath.Join(parentDir, "gck--disable-portal.yaml"), `
description: "Disable portal (from parent)"
components:
  - name: apim
    helm:
      values:
        portal:
          enabled: false
`)
	writeFile(t, filepath.Join(childDir, "gck--disable-ui.yaml"), `
description: "Disable all UIs"
components:
  - name: apim
    helm:
      values:
        ui:
          enabled: false
`)

	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{
				Name: "apim",
				Helm: &config.HelmSpec{
					Chart: "graviteeio/apim",
					Values: map[string]interface{}{
						"portal": map[string]interface{}{"enabled": true},
						"ui":     map[string]interface{}{"enabled": true},
					},
				},
			},
		},
		Flags: []config.ContextFlag{
			{Name: "disable-portal", Dir: parentDir},
			{Name: "disable-ui", Dir: childDir},
		},
	}

	err := ApplyFlags(resolved, []string{"disable-portal", "disable-ui"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	vals := resolved.Components[0].Helm.Values
	portal, _ := vals["portal"].(map[string]interface{})
	ui, _ := vals["ui"].(map[string]interface{})
	if portal["enabled"] != false {
		t.Fatalf("expected portal disabled, got %v", portal["enabled"])
	}
	if ui["enabled"] != false {
		t.Fatalf("expected ui disabled, got %v", ui["enabled"])
	}
}

func TestFlagNameFromFile_Valid(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"gck--disable-portal.yaml", "disable-portal"},
		{"gck--disable-ui.yaml", "disable-ui"},
		{"gck--disable-es.yaml", "disable-es"},
		{"gck--debug.yaml", "debug"},
		{"gck--v2.yaml", "v2"},
		{"gck--my-long-flag-name.yaml", "my-long-flag-name"},
	}
	for _, tt := range tests {
		name, err := FlagNameFromFile(tt.filename)
		if err != nil {
			t.Fatalf("FlagNameFromFile(%q) unexpected error: %v", tt.filename, err)
		}
		if name != tt.expected {
			t.Fatalf("FlagNameFromFile(%q) = %q, want %q", tt.filename, name, tt.expected)
		}
	}
}

func TestFlagNameFromFile_Invalid(t *testing.T) {
	tests := []string{
		"gck--No-Portal.yaml",
		"gck--no_portal.yaml",
		"gck--UPPER.yaml",
		"gck--.yaml",
		"gck---leading-dash.yaml",
		"gck--trailing-.yaml",
	}
	for _, filename := range tests {
		_, err := FlagNameFromFile(filename)
		if err == nil {
			t.Fatalf("FlagNameFromFile(%q) expected error, got nil", filename)
		}
	}
}

func TestValidateFlagDescription_Present(t *testing.T) {
	data := []byte(`description: "Disable portal"
components: []
`)
	if err := ValidateFlagDescription(data); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateFlagDescription_Missing(t *testing.T) {
	data := []byte(`components:
  - name: app
    helm:
      values:
        debug: true
`)
	if err := ValidateFlagDescription(data); err == nil {
		t.Fatal("expected error for missing description")
	}
}

func TestValidateFlagDescription_Blank(t *testing.T) {
	data := []byte(`description: "   "
components: []
`)
	if err := ValidateFlagDescription(data); err == nil {
		t.Fatal("expected error for blank description")
	}
}

func TestValidateFlagDescription_Empty(t *testing.T) {
	data := []byte(`description: ""
components: []
`)
	if err := ValidateFlagDescription(data); err == nil {
		t.Fatal("expected error for empty description")
	}
}

func TestApplyFlags_WithValueFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "gck--custom.yaml"), `
description: "Apply custom values"
components:
  - name: apim
    helm:
      valueFiles:
        - custom-values.yaml
`)
	writeFile(t, filepath.Join(dir, "custom-values.yaml"), `key: val`)

	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{
				Name: "apim",
				Helm: &config.HelmSpec{
					Chart:      "graviteeio/apim",
					ValueFiles: []string{"/abs/base-values.yaml"},
				},
			},
		},
		Flags: []config.ContextFlag{
			{Name: "custom", Dir: dir},
		},
	}

	err := ApplyFlags(resolved, []string{"custom"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	comp := resolved.Components[0]
	if len(comp.Helm.ValueFiles) != 2 {
		t.Fatalf("expected 2 value files, got %d", len(comp.Helm.ValueFiles))
	}
	if comp.Helm.ValueFiles[0] != "/abs/base-values.yaml" {
		t.Fatalf("expected base value file preserved, got %q", comp.Helm.ValueFiles[0])
	}
	expected := filepath.Join(dir, "custom-values.yaml")
	if comp.Helm.ValueFiles[1] != expected {
		t.Fatalf("expected flag value file resolved to %q, got %q", expected, comp.Helm.ValueFiles[1])
	}
}
