package cmd

import (
	"testing"

	"github.com/gravitee-io-labs/gck/internal/config"
	"github.com/gravitee-io-labs/gck/internal/registry"
)

func boolPtr(v bool) *bool { return &v }

func TestFilterDisabledComponents_AllEnabled(t *testing.T) {
	components := []config.Component{
		{Name: "app"},
		{Name: "db"},
	}
	result := filterDisabledComponents(components)
	if len(result) != 2 {
		t.Fatalf("expected 2 components, got %d", len(result))
	}
}

func TestFilterDisabledComponents_OneDisabled(t *testing.T) {
	components := []config.Component{
		{Name: "elasticsearch", Enabled: boolPtr(false)},
		{Name: "apim"},
	}
	result := filterDisabledComponents(components)
	if len(result) != 1 {
		t.Fatalf("expected 1 component, got %d", len(result))
	}
	if result[0].Name != "apim" {
		t.Fatalf("expected apim, got %q", result[0].Name)
	}
}

func TestFilterDisabledComponents_PrunesRequires(t *testing.T) {
	components := []config.Component{
		{Name: "elasticsearch", Enabled: boolPtr(false)},
		{
			Name: "apim",
			Requires: []config.Requirement{
				{Component: "mongodb", Conditions: config.Conditions{Ready: true}},
				{Component: "elasticsearch", Conditions: config.Conditions{Ready: true}},
			},
		},
		{Name: "mongodb"},
	}
	result := filterDisabledComponents(components)
	if len(result) != 2 {
		t.Fatalf("expected 2 components, got %d", len(result))
	}
	apim := result[0]
	if apim.Name != "apim" {
		t.Fatalf("expected apim first, got %q", apim.Name)
	}
	if len(apim.Requires) != 1 {
		t.Fatalf("expected 1 requirement after pruning, got %d", len(apim.Requires))
	}
	if apim.Requires[0].Component != "mongodb" {
		t.Fatalf("expected mongodb requirement kept, got %q", apim.Requires[0].Component)
	}
}

func TestFilterDisabledComponents_AllDisabled(t *testing.T) {
	components := []config.Component{
		{Name: "a", Enabled: boolPtr(false)},
		{Name: "b", Enabled: boolPtr(false)},
	}
	result := filterDisabledComponents(components)
	if len(result) != 0 {
		t.Fatalf("expected 0 components, got %d", len(result))
	}
}

func TestFilterDisabledComponents_ExplicitlyEnabled(t *testing.T) {
	components := []config.Component{
		{Name: "app", Enabled: boolPtr(true)},
	}
	result := filterDisabledComponents(components)
	if len(result) != 1 {
		t.Fatalf("expected 1 component, got %d", len(result))
	}
}

func TestFilterDisabledComponents_DoesNotMutateOriginal(t *testing.T) {
	components := []config.Component{
		{Name: "elasticsearch", Enabled: boolPtr(false)},
		{
			Name: "apim",
			Requires: []config.Requirement{
				{Component: "elasticsearch"},
				{Component: "mongodb"},
			},
		},
	}
	originalLen := len(components[1].Requires)
	filterDisabledComponents(components)
	if len(components[1].Requires) != originalLen {
		t.Fatal("expected original slice not mutated")
	}
}

// A flag that disables components by name across alternatives (e.g.
// --disable-analytics turning off both elasticsearch and opensearch) patches
// a component the selected alternative never brought in. The patch is
// appended as a disabled stub and must be dropped, not deployed.
func TestFilterDisabledComponents_DropsDisabledStubForAbsentComponent(t *testing.T) {
	resolved := &config.ResolvedContext{Components: []config.Component{
		{Name: "elasticsearch", Helm: &config.HelmSpec{Chart: "elastic/elasticsearch"}},
		{Name: "apim", Helm: &config.HelmSpec{Chart: "graviteeio/apim"}, Requires: []config.Requirement{{Component: "elasticsearch"}}},
	}}
	registry.MergeComponents(resolved, []config.Component{
		{Name: "elasticsearch", Enabled: boolPtr(false)},
		{Name: "opensearch", Enabled: boolPtr(false)},
	}, "")

	got := filterDisabledComponents(resolved.Components)
	if len(got) != 1 || got[0].Name != "apim" {
		t.Fatalf("expected only apim to remain, got %v", got)
	}
	if len(got[0].Requires) != 0 {
		t.Fatalf("expected the requirement on the disabled component to be pruned, got %v", got[0].Requires)
	}
}
