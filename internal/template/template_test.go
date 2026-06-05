package gcktmpl

import (
	"strings"
	"testing"
)

func TestRender_VarsDefaults(t *testing.T) {
	raw := []byte(`vars:
  imageTag: "latest"
  helmVersion: "4.5.0"

image: "repo:{{ .imageTag }}"
chart: "{{ .helmVersion }}"
`)
	out, err := Render(raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `image: "repo:latest"`) {
		t.Errorf("expected imageTag default, got:\n%s", s)
	}
	if !strings.Contains(s, `chart: "4.5.0"`) {
		t.Errorf("expected helmVersion default, got:\n%s", s)
	}
}

func TestRender_SetOverridesVars(t *testing.T) {
	raw := []byte(`vars:
  imageTag: "latest"

image: "repo:{{ .imageTag }}"
`)
	out, err := Render(raw, map[string]string{"imageTag": "4.12.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), `image: "repo:4.12.0"`) {
		t.Errorf("expected --set override, got:\n%s", string(out))
	}
}

func TestRender_NoVarsBlock(t *testing.T) {
	raw := []byte(`name: simple
`)
	out, err := Render(raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), "name: simple") {
		t.Errorf("expected passthrough, got:\n%s", string(out))
	}
}

func TestRender_SetWithoutVarsBlock(t *testing.T) {
	raw := []byte(`image: "repo:{{ .imageTag }}"
`)
	out, err := Render(raw, map[string]string{"imageTag": "v1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), `image: "repo:v1"`) {
		t.Errorf("expected --set value, got:\n%s", string(out))
	}
}

func TestRender_MissingVarErrors(t *testing.T) {
	raw := []byte(`image: "{{ .missing }}"
`)
	_, err := Render(raw, nil)
	if err == nil {
		t.Fatal("expected error for undefined variable")
	}
}

func TestRender_EnvFunction(t *testing.T) {
	t.Setenv("GCK_TEST_VAR", "hello")
	raw := []byte(`val: "{{ env "GCK_TEST_VAR" }}"
`)
	out, err := Render(raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), `val: "hello"`) {
		t.Errorf("expected env value, got:\n%s", string(out))
	}
}

func TestRender_EnvFunctionUnsetVar(t *testing.T) {
	raw := []byte(`val: "{{ env "GCK_UNLIKELY_TO_EXIST_12345" }}"
`)
	out, err := Render(raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), `val: ""`) {
		t.Errorf("expected empty string for unset env var, got:\n%s", string(out))
	}
}

func TestRender_DefaultFunction(t *testing.T) {
	raw := []byte(`vars:
  imageTag: ""

image: '{{ .imageTag | default "latest" }}'
`)
	out, err := Render(raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), "image: 'latest'") {
		t.Errorf("expected default fallback, got:\n%s", string(out))
	}
}

func TestRender_DefaultFunctionValuePresent(t *testing.T) {
	raw := []byte(`vars:
  imageTag: "4.5.0"

image: '{{ .imageTag | default "latest" }}'
`)
	out, err := Render(raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), "image: '4.5.0'") {
		t.Errorf("expected actual value over default, got:\n%s", string(out))
	}
}

func TestRender_RequiredFunctionWithValue(t *testing.T) {
	raw := []byte(`vars:
  licenseKey: "abc123"

key: "{{ .licenseKey | required "licenseKey must be set" }}"
`)
	out, err := Render(raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), `key: "abc123"`) {
		t.Errorf("expected value, got:\n%s", string(out))
	}
}

func TestRender_RequiredFunctionEmpty(t *testing.T) {
	raw := []byte(`vars:
  licenseKey: ""

key: "{{ .licenseKey | required "licenseKey must be set via --set" }}"
`)
	_, err := Render(raw, nil)
	if err == nil {
		t.Fatal("expected error for empty required variable")
	}
	if !strings.Contains(err.Error(), "licenseKey must be set via --set") {
		t.Errorf("expected clear error message, got: %v", err)
	}
}

func TestRender_VarsWithSuffix(t *testing.T) {
	raw := []byte(`vars:
  imageTag: "latest"

gateway: "graviteeio/apim-gateway:{{ .imageTag }}-debian"
ui: "graviteeio/apim-ui:{{ .imageTag }}"
`)
	out, err := Render(raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "graviteeio/apim-gateway:latest-debian") {
		t.Errorf("expected tag with suffix, got:\n%s", s)
	}
	if !strings.Contains(s, "graviteeio/apim-ui:latest") {
		t.Errorf("expected tag without suffix, got:\n%s", s)
	}
}

func TestRender_RealisticSewYAML(t *testing.T) {
	raw := []byte(`vars:
  helmVersion: ""
  imageTag: "latest"

components:
  - name: apim
    helm:
      chart: graviteeio/apim
      version: "{{ .helmVersion }}"
      values:
        gateway:
          image:
            tag: "{{ .imageTag }}-debian"
        api:
          image:
            tag: "{{ .imageTag }}-debian"
        ui:
          image:
            tag: "{{ .imageTag }}"

images:
  preload:
    refs:
      - "graviteeio/apim-gateway:{{ .imageTag }}-debian"
`)
	out, err := Render(raw, map[string]string{"imageTag": "4.12.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `tag: "4.12.0-debian"`) {
		t.Errorf("expected overridden imageTag with suffix, got:\n%s", s)
	}
	if !strings.Contains(s, `tag: "4.12.0"`) {
		t.Errorf("expected overridden imageTag, got:\n%s", s)
	}
	if !strings.Contains(s, `version: ""`) {
		t.Errorf("expected empty helmVersion default, got:\n%s", s)
	}
	if !strings.Contains(s, `"graviteeio/apim-gateway:4.12.0-debian"`) {
		t.Errorf("expected overridden preload ref, got:\n%s", s)
	}
}

func TestExtractVars_EmptyInput(t *testing.T) {
	vars, err := extractVars([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vars) != 0 {
		t.Errorf("expected empty map, got: %v", vars)
	}
}

func TestExtractVars_NoVarsKey(t *testing.T) {
	vars, err := extractVars([]byte("name: foo\nvalue: bar\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vars) != 0 {
		t.Errorf("expected empty map, got: %v", vars)
	}
}

func TestExtractVars_InlineFlowMapping(t *testing.T) {
	vars, err := extractVars([]byte(`vars: {}
name: foo
`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vars) != 0 {
		t.Errorf("expected empty map for flow mapping, got: %v", vars)
	}
}

func TestExtractVars_MultipleVars(t *testing.T) {
	raw := []byte(`vars:
  a: "1"
  b: "2"
  c: "3"

other: stuff
`)
	vars, err := extractVars(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vars) != 3 {
		t.Fatalf("expected 3 vars, got %d: %v", len(vars), vars)
	}
	if vars["a"] != "1" || vars["b"] != "2" || vars["c"] != "3" {
		t.Errorf("unexpected values: %v", vars)
	}
}

func TestExtractVars_VarsAtEndOfFile(t *testing.T) {
	raw := []byte(`name: foo

vars:
  key: "value"
`)
	vars, err := extractVars(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vars["key"] != "value" {
		t.Errorf("expected key=value, got: %v", vars)
	}
}

func TestExtractVars_VarsWithComments(t *testing.T) {
	raw := []byte(`vars:
  # This is a comment
  key: "value"
  # Another comment
  other: "val2"

components: []
`)
	vars, err := extractVars(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vars) != 2 {
		t.Fatalf("expected 2 vars, got %d: %v", len(vars), vars)
	}
	if vars["key"] != "value" || vars["other"] != "val2" {
		t.Errorf("unexpected values: %v", vars)
	}
}

func TestRender_CombinedFunctions(t *testing.T) {
	t.Setenv("GCK_BUILD_TAG", "ci-42")
	raw := []byte(`vars:
  imageTag: ""

image: '{{ .imageTag | default (env "GCK_BUILD_TAG") }}'
`)
	out, err := Render(raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), "image: 'ci-42'") {
		t.Errorf("expected env fallback via default, got:\n%s", string(out))
	}
}

func TestRender_NilOverridesMap(t *testing.T) {
	raw := []byte(`vars:
  tag: "v1"

image: "{{ .tag }}"
`)
	out, err := Render(raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), `image: "v1"`) {
		t.Errorf("expected default, got:\n%s", string(out))
	}
}

func TestRender_EmptyOverridesMap(t *testing.T) {
	raw := []byte(`vars:
  tag: "v1"

image: "{{ .tag }}"
`)
	out, err := Render(raw, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), `image: "v1"`) {
		t.Errorf("expected default, got:\n%s", string(out))
	}
}

func TestRender_ExtendedVarsFormat(t *testing.T) {
	raw := []byte(`vars:
  imageTag:
    default: "latest"
    description: "Docker image tag"
  helmVersion:
    default: "4.5.0"
    description: "Helm chart version"

image: "repo:{{ .imageTag }}"
chart: "{{ .helmVersion }}"
`)
	out, err := Render(raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `image: "repo:latest"`) {
		t.Errorf("expected imageTag default, got:\n%s", s)
	}
	if !strings.Contains(s, `chart: "4.5.0"`) {
		t.Errorf("expected helmVersion default, got:\n%s", s)
	}
}

func TestRender_ExtendedVarsWithSetOverride(t *testing.T) {
	raw := []byte(`vars:
  imageTag:
    default: "latest"
    description: "Docker image tag"

image: "repo:{{ .imageTag }}"
`)
	out, err := Render(raw, map[string]string{"imageTag": "4.12.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), `image: "repo:4.12.0"`) {
		t.Errorf("expected --set override, got:\n%s", string(out))
	}
}

func TestRender_MixedVarsFormats(t *testing.T) {
	raw := []byte(`vars:
  imageTag:
    default: "latest"
    description: "Docker image tag"
  simple: "plain-value"

image: "repo:{{ .imageTag }}"
other: "{{ .simple }}"
`)
	out, err := Render(raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `image: "repo:latest"`) {
		t.Errorf("expected imageTag default, got:\n%s", s)
	}
	if !strings.Contains(s, `other: "plain-value"`) {
		t.Errorf("expected simple value, got:\n%s", s)
	}
}

func TestExtractVarDefs_SimpleFormat(t *testing.T) {
	raw := []byte(`vars:
  imageTag: "latest"
  helmVersion: "4.5.0"

other: stuff
`)
	defs, err := ExtractVarDefs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(defs) != 2 {
		t.Fatalf("expected 2 defs, got %d", len(defs))
	}
	if defs[0].Name != "imageTag" || defs[0].Default != "latest" || defs[0].Description != "" {
		t.Errorf("unexpected first def: %+v", defs[0])
	}
	if defs[1].Name != "helmVersion" || defs[1].Default != "4.5.0" || defs[1].Description != "" {
		t.Errorf("unexpected second def: %+v", defs[1])
	}
}

func TestExtractVarDefs_ExtendedFormat(t *testing.T) {
	raw := []byte(`vars:
  imageTag:
    default: "latest"
    description: "Docker image tag for all APIM components"
  helmVersion:
    default: ""
    description: "Helm chart version constraint"

other: stuff
`)
	defs, err := ExtractVarDefs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(defs) != 2 {
		t.Fatalf("expected 2 defs, got %d", len(defs))
	}
	if defs[0].Name != "imageTag" || defs[0].Default != "latest" || defs[0].Description != "Docker image tag for all APIM components" {
		t.Errorf("unexpected first def: %+v", defs[0])
	}
	if defs[1].Name != "helmVersion" || defs[1].Default != "" || defs[1].Description != "Helm chart version constraint" {
		t.Errorf("unexpected second def: %+v", defs[1])
	}
}

func TestExtractVarDefs_NoVars(t *testing.T) {
	raw := []byte(`name: foo
`)
	defs, err := ExtractVarDefs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if defs != nil {
		t.Errorf("expected nil, got: %v", defs)
	}
}

func TestExtractVarsTree_OwnVarsOnly(t *testing.T) {
	raw := []byte(`vars:
  imageTag:
    default: "latest"
    description: "Docker image tag"
  simple: "plain"

other: stuff
`)
	tree, err := ExtractVarsTree(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tree.Defs) != 2 {
		t.Fatalf("expected 2 defs, got %d", len(tree.Defs))
	}
	if tree.Defs[0].Name != "imageTag" || tree.Defs[0].Default != "latest" {
		t.Errorf("unexpected first def: %+v", tree.Defs[0])
	}
	if tree.Defs[1].Name != "simple" || tree.Defs[1].Default != "plain" {
		t.Errorf("unexpected second def: %+v", tree.Defs[1])
	}
	if len(tree.Overrides) != 0 {
		t.Errorf("expected no overrides, got %d", len(tree.Overrides))
	}
}

func TestExtractVarsTree_OverridesOnly(t *testing.T) {
	raw := []byte(`vars:
  mysql:
    standalone:
      imageTag:
        default: "8"
`)
	tree, err := ExtractVarsTree(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tree.Defs) != 0 {
		t.Errorf("expected no defs, got %d: %+v", len(tree.Defs), tree.Defs)
	}
	if len(tree.Overrides) != 1 {
		t.Fatalf("expected 1 override, got %d", len(tree.Overrides))
	}
	o := tree.Overrides[0]
	if o.ContextPath != "mysql/standalone" {
		t.Errorf("expected context path mysql/standalone, got %q", o.ContextPath)
	}
	if o.Name != "imageTag" {
		t.Errorf("expected var name imageTag, got %q", o.Name)
	}
	if o.Default != "8" {
		t.Errorf("expected default 8, got %q", o.Default)
	}
}

func TestExtractVarsTree_Mixed(t *testing.T) {
	raw := []byte(`vars:
  jdbcDriver:
    default: "mysql"
    description: "JDBC driver"
  mysql:
    standalone:
      imageTag:
        default: "8"
`)
	tree, err := ExtractVarsTree(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tree.Defs) != 1 {
		t.Fatalf("expected 1 def, got %d", len(tree.Defs))
	}
	if tree.Defs[0].Name != "jdbcDriver" {
		t.Errorf("unexpected def: %+v", tree.Defs[0])
	}
	if len(tree.Overrides) != 1 {
		t.Fatalf("expected 1 override, got %d", len(tree.Overrides))
	}
	if tree.Overrides[0].ContextPath != "mysql/standalone" || tree.Overrides[0].Name != "imageTag" {
		t.Errorf("unexpected override: %+v", tree.Overrides[0])
	}
}

func TestExtractVarsTree_DeepPath(t *testing.T) {
	raw := []byte(`vars:
  gravitee-io:
    oss:
      apim:
        base:
          imageTag:
            default: "4.6.0"
`)
	tree, err := ExtractVarsTree(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tree.Overrides) != 1 {
		t.Fatalf("expected 1 override, got %d", len(tree.Overrides))
	}
	o := tree.Overrides[0]
	if o.ContextPath != "gravitee-io/oss/apim/base" {
		t.Errorf("expected path gravitee-io/oss/apim/base, got %q", o.ContextPath)
	}
	if o.Name != "imageTag" || o.Default != "4.6.0" {
		t.Errorf("unexpected override: %+v", o)
	}
}

func TestExtractVarsTree_NoVars(t *testing.T) {
	raw := []byte(`name: foo
`)
	tree, err := ExtractVarsTree(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tree.Defs) != 0 || len(tree.Overrides) != 0 {
		t.Errorf("expected empty tree, got defs=%d overrides=%d", len(tree.Defs), len(tree.Overrides))
	}
}

func TestExtractVarsTree_ExtractVarDefsExcludesOverrides(t *testing.T) {
	raw := []byte(`vars:
  imageTag:
    default: "latest"
  mysql:
    standalone:
      imageTag:
        default: "8"
`)
	defs, err := ExtractVarDefs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(defs) != 1 {
		t.Fatalf("expected 1 def (overrides excluded), got %d", len(defs))
	}
	if defs[0].Name != "imageTag" {
		t.Errorf("unexpected def: %+v", defs[0])
	}
}

func TestRenderWithVars(t *testing.T) {
	raw := []byte(`image: "mysql:{{ .imageTag }}"
`)
	out, err := RenderWithVars(raw, map[string]string{"imageTag": "8.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), `image: "mysql:8.0"`) {
		t.Errorf("expected rendered output, got:\n%s", string(out))
	}
}
