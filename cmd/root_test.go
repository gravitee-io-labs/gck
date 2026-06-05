package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gravitee-io-labs/gck/internal/config"
	"github.com/spf13/pflag"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setupCfg(t *testing.T, registryRoot string, from []string) {
	t.Helper()
	resetContextConfigCache()
	gckHome = t.TempDir()
	cfg = &config.Config{
		Registry: "file://" + registryRoot,
		From:     from,
	}
	cfg.Kind.ApplyDefaults()
}

func TestParseSetValues_Valid(t *testing.T) {
	m, err := parseSetValues([]string{"key=value", "a=b=c", "empty="})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m["key"] != "value" {
		t.Fatalf("expected key=value, got %q", m["key"])
	}
	if m["a"] != "b=c" {
		t.Fatalf("expected a=b=c (split on first =), got %q", m["a"])
	}
	if m["empty"] != "" {
		t.Fatalf("expected empty value, got %q", m["empty"])
	}
}

func TestParseSetValues_MissingEquals(t *testing.T) {
	_, err := parseSetValues([]string{"noequals"})
	if err == nil {
		t.Fatal("expected error for missing =")
	}
	if !strings.Contains(err.Error(), "noequals") {
		t.Fatalf("expected error to mention the invalid value, got: %v", err)
	}
}

func TestParseSetValues_Empty(t *testing.T) {
	m, err := parseSetValues(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m) != 0 {
		t.Fatalf("expected empty map, got %v", m)
	}
}

func TestResolveContextConfig_TemplatedContext(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "tmpl-ctx", "gck.yaml"), `vars:
  imageTag: "latest"

kind:
  name: tmpl-cluster

components:
  - name: app
    helm:
      chart: repo/app
      values:
        image:
          tag: "{{ .imageTag }}"
`)

	resetContextConfigCache()
	gckHome = t.TempDir()
	setOverrides = map[string]string{"imageTag": "4.12.0"}
	cfg = &config.Config{
		Registry: "file://" + root,
		From:     []string{"tmpl-ctx"},
	}
	cfg.Kind.ApplyDefaults()

	resolved, err := resolveContextConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved == nil {
		t.Fatal("expected non-nil resolved context")
	}
	tag := resolved.Components[0].Helm.Values["image"].(map[string]interface{})["tag"]
	if tag != "4.12.0" {
		t.Fatalf("expected --set override 4.12.0, got %v", tag)
	}
}

func TestResolveContextConfig_MultiFrom_LeftToRightMerge(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "ctx-a", "gck.yaml"), `
kind:
  name: cluster-a
  nodes:
  - role: control-plane
    extraPortMappings:
    - containerPort: 80
      hostPort: 80

helm:
  repos:
    - name: repo-a
      url: https://example.com/a

components:
  - name: comp-a
    helm:
      chart: a/chart
`)

	writeFile(t, filepath.Join(root, "ctx-b", "gck.yaml"), `
kind:
  name: cluster-b
  nodes:
  - role: control-plane
    extraPortMappings:
    - containerPort: 9090
      hostPort: 9090

helm:
  repos:
    - name: repo-b
      url: https://example.com/b

components:
  - name: comp-b
    helm:
      chart: b/chart
`)

	setupCfg(t, root, []string{"ctx-a", "ctx-b"})

	resolved, err := resolveContextConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved == nil {
		t.Fatal("expected non-nil resolved context")
	}

	compNames := make(map[string]bool)
	for _, c := range resolved.Components {
		compNames[c.Name] = true
	}
	if !compNames["comp-a"] || !compNames["comp-b"] {
		t.Fatalf("expected both comp-a and comp-b, got %v", compNames)
	}

	repoNames := make(map[string]bool)
	for _, r := range resolved.Repos {
		repoNames[r.Name] = true
	}
	if !repoNames["repo-a"] || !repoNames["repo-b"] {
		t.Fatalf("expected both repo-a and repo-b, got %v", repoNames)
	}

	if cfg.Kind.Name != "cluster-b" {
		t.Fatalf("expected kind name from last context %q, got %q", "cluster-b", cfg.Kind.Name)
	}
}

func TestResolveContextConfig_MultiFrom_PortOverride(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "base-infra", "gck.yaml"), `
kind:
  nodes:
  - role: control-plane
    extraPortMappings:
    - containerPort: 30092
      hostPort: 9092
    - containerPort: 80
      hostPort: 80

components:
  - name: kafka
    helm:
      chart: kafka/chart
`)

	writeFile(t, filepath.Join(root, "gateway", "gck.yaml"), `
kind:
  nodes:
  - role: control-plane
    extraPortMappings:
    - containerPort: 30092
      hostPort: 30092

components:
  - name: gw
    helm:
      chart: gw/chart
`)

	setupCfg(t, root, []string{"base-infra", "gateway"})

	resolved, err := resolveContextConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	portMap := make(map[int32]int32)
	for _, p := range resolved.Kind.Nodes[0].ExtraPortMappings {
		portMap[p.ContainerPort] = p.HostPort
	}
	if hp, ok := portMap[30092]; !ok || hp != 30092 {
		t.Fatalf("expected port 30092 overridden to hostPort 30092 by later context, got %d", portMap[30092])
	}
	if _, ok := portMap[80]; !ok {
		t.Fatal("expected port 80 preserved from first context")
	}
}

func TestResolveContextConfig_MultiFrom_FeaturesMerge(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "feat-a", "gck.yaml"), `
features:
  lb:
    enabled: true
  dns:
    enabled: true
    domain: a.local

components:
  - name: comp-a
    helm:
      chart: a/chart
`)

	writeFile(t, filepath.Join(root, "feat-b", "gck.yaml"), `
features:
  dns:
    enabled: true
    domain: b.local
    port: 5353

components:
  - name: comp-b
    helm:
      chart: b/chart
`)

	setupCfg(t, root, []string{"feat-a", "feat-b"})

	_, err := resolveContextConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Features.LB == nil || !cfg.Features.LB.Enabled {
		t.Fatal("expected LB inherited from first context")
	}
	if cfg.Features.DNS == nil || cfg.Features.DNS.Domain != "b.local" {
		t.Fatal("expected DNS domain overridden by second context")
	}
	if cfg.Features.DNS.Port != 5353 {
		t.Fatalf("expected DNS port 5353 from second context, got %d", cfg.Features.DNS.Port)
	}
}

func TestResolveContextConfig_MultiFrom_ImagesMerge(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "img-a", "gck.yaml"), `
images:
  preload:
    refs:
      - mongo:7
      - shared:latest

components:
  - name: comp-a
    helm:
      chart: a/chart
`)

	writeFile(t, filepath.Join(root, "img-b", "gck.yaml"), `
images:
  preload:
    refs:
      - elastic:9
      - shared:latest

components:
  - name: comp-b
    helm:
      chart: b/chart
`)

	setupCfg(t, root, []string{"img-a", "img-b"})

	_, err := resolveContextConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Images.Preload == nil {
		t.Fatal("expected preload from union of all contexts")
	}
	if len(cfg.Images.Preload.Refs) != 3 {
		t.Fatalf("expected 3 refs from union of all contexts, got %d: %v", len(cfg.Images.Preload.Refs), cfg.Images.Preload.Refs)
	}
	refs := make(map[string]bool)
	for _, r := range cfg.Images.Preload.Refs {
		refs[r] = true
	}
	if !refs["mongo:7"] || !refs["shared:latest"] || !refs["elastic:9"] {
		t.Fatalf("expected union of refs from all contexts, got %v", cfg.Images.Preload.Refs)
	}
}

func TestResolveContextConfig_MultiFrom_AbstractSkipped(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "abstract-ctx", "gck.yaml"), `
abstract: true

components:
  - name: base
    helm:
      chart: base/chart
`)

	writeFile(t, filepath.Join(root, "concrete-ctx", "gck.yaml"), `
components:
  - name: extra
    helm:
      chart: extra/chart
`)

	setupCfg(t, root, []string{"abstract-ctx", "concrete-ctx"})

	resolved, err := resolveContextConfig()
	if err != nil {
		t.Fatalf("expected no error when composing with abstract in multi-from, got: %v", err)
	}
	if resolved == nil {
		t.Fatal("expected non-nil resolved context")
	}
	if len(resolved.Components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(resolved.Components))
	}
}

func TestResolveContextConfig_SingleFrom_AbstractBlocked(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "abstract-only", "gck.yaml"), `
abstract: true

components:
  - name: base
    helm:
      chart: base/chart
`)

	setupCfg(t, root, []string{"abstract-only"})

	_, err := resolveContextConfig()
	if err == nil {
		t.Fatal("expected error when deploying a single abstract context directly")
	}
	if !strings.Contains(err.Error(), "abstract") {
		t.Fatalf("expected abstract error message, got: %v", err)
	}
}

func TestResolveContextConfig_SingleFrom_NonAbstract(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "simple", "gck.yaml"), `
kind:
  name: simple-cluster

components:
  - name: app
    helm:
      chart: app/chart
`)

	setupCfg(t, root, []string{"simple"})

	resolved, err := resolveContextConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved == nil {
		t.Fatal("expected non-nil resolved context")
	}
	if len(resolved.Components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(resolved.Components))
	}
	if cfg.Kind.Name != "simple-cluster" {
		t.Fatalf("expected kind name %q, got %q", "simple-cluster", cfg.Kind.Name)
	}
}

func TestResolveContextConfig_NoRegistryReturnsNil(t *testing.T) {
	resetContextConfigCache()
	gckHome = t.TempDir()
	cfg = &config.Config{
		From: []string{"something"},
	}

	resolved, err := resolveContextConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != nil {
		t.Fatal("expected nil when registry is empty")
	}
}

func TestResolveContextConfig_NoFromReturnsNil(t *testing.T) {
	resetContextConfigCache()
	gckHome = t.TempDir()
	cfg = &config.Config{
		Registry: "file:///some/path",
	}

	resolved, err := resolveContextConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != nil {
		t.Fatal("expected nil when from is empty")
	}
}

func TestResolveContextConfig_MultiFrom_ComponentOverride(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "provider", "gck.yaml"), `
components:
  - name: database
    helm:
      chart: bitnami/mongodb
      version: "7.0"
      values:
        auth:
          enabled: false
`)

	writeFile(t, filepath.Join(root, "consumer", "gck.yaml"), `
components:
  - name: database
    helm:
      version: "8.0"
      values:
        auth:
          enabled: true
        replicaCount: 3
`)

	setupCfg(t, root, []string{"provider", "consumer"})

	resolved, err := resolveContextConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resolved.Components) != 1 {
		t.Fatalf("expected 1 component (merged by name), got %d", len(resolved.Components))
	}
	comp := resolved.Components[0]
	if comp.Helm.Chart != "bitnami/mongodb" {
		t.Fatalf("expected chart preserved from first, got %q", comp.Helm.Chart)
	}
	if comp.Helm.Version != "8.0" {
		t.Fatalf("expected version overridden to 8.0, got %q", comp.Helm.Version)
	}
}

func TestResolveContextConfig_MultiFrom_ThreeContexts(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "first", "gck.yaml"), `
kind:
  name: first-cluster
  nodes:
  - role: control-plane
    extraPortMappings:
    - containerPort: 80
      hostPort: 80

components:
  - name: comp-first
    helm:
      chart: first/chart
`)

	writeFile(t, filepath.Join(root, "second", "gck.yaml"), `
kind:
  name: second-cluster
  nodes:
  - role: control-plane
    extraPortMappings:
    - containerPort: 443
      hostPort: 443

components:
  - name: comp-second
    helm:
      chart: second/chart
`)

	writeFile(t, filepath.Join(root, "third", "gck.yaml"), `
kind:
  name: third-cluster

components:
  - name: comp-third
    helm:
      chart: third/chart
`)

	setupCfg(t, root, []string{"first", "second", "third"})

	resolved, err := resolveContextConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resolved.Components) != 3 {
		t.Fatalf("expected 3 components, got %d", len(resolved.Components))
	}

	if cfg.Kind.Name != "third-cluster" {
		t.Fatalf("expected kind name from last context %q, got %q", "third-cluster", cfg.Kind.Name)
	}

	portMap := make(map[int32]int32)
	for _, p := range resolved.Kind.Nodes[0].ExtraPortMappings {
		portMap[p.ContainerPort] = p.HostPort
	}
	if _, ok := portMap[80]; !ok {
		t.Fatal("expected port 80 from first context")
	}
	if _, ok := portMap[443]; !ok {
		t.Fatal("expected port 443 from second context")
	}
}

func TestResolveContextConfig_MultiFrom_UserCfgOverridesContext(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "ctx-a", "gck.yaml"), `
features:
  lb:
    enabled: true

components:
  - name: comp-a
    helm:
      chart: a/chart
`)

	writeFile(t, filepath.Join(root, "ctx-b", "gck.yaml"), `
components:
  - name: comp-b
    helm:
      chart: b/chart
`)

	resetContextConfigCache()
	gckHome = t.TempDir()
	cfg = &config.Config{
		Registry: "file://" + root,
		From:     []string{"ctx-a", "ctx-b"},
		Features: config.FeaturesConfig{
			LB: &config.LBConfig{Enabled: false},
		},
	}
	cfg.Kind.ApplyDefaults()

	_, err := resolveContextConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Features.LB == nil || cfg.Features.LB.Enabled {
		t.Fatal("expected user-level LB=false to override context LB=true")
	}
}

func newFlagSet(t *testing.T, flags map[string]string) *pflag.FlagSet {
	t.Helper()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	for name, def := range flags {
		fs.String(name, def, "")
	}
	return fs
}

func TestExtractActiveFlags_NoContextFlagsPassed(t *testing.T) {
	inherited := newFlagSet(t, map[string]string{"config": "", "registry": ""})
	local := pflag.NewFlagSet("local", pflag.ContinueOnError)
	available := []config.ContextFlag{{Name: "disable-portal"}}

	active, err := extractActiveFlags(
		[]string{"gck", "create", "--config", "gck.yaml"},
		inherited, local, available,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected no active flags, got %v", active)
	}
}

func TestExtractActiveFlags_RecognizesContextFlag(t *testing.T) {
	inherited := newFlagSet(t, map[string]string{"from": ""})
	local := pflag.NewFlagSet("local", pflag.ContinueOnError)
	available := []config.ContextFlag{
		{Name: "disable-portal"},
		{Name: "disable-ui"},
	}

	active, err := extractActiveFlags(
		[]string{"gck", "create", "--from", "ctx", "--disable-portal"},
		inherited, local, available,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(active) != 1 || active[0] != "disable-portal" {
		t.Fatalf("expected [disable-portal], got %v", active)
	}
}

func TestExtractActiveFlags_MultipleContextFlags(t *testing.T) {
	inherited := pflag.NewFlagSet("inherited", pflag.ContinueOnError)
	local := pflag.NewFlagSet("local", pflag.ContinueOnError)
	available := []config.ContextFlag{
		{Name: "disable-portal"},
		{Name: "disable-ui"},
		{Name: "disable-es"},
	}

	active, err := extractActiveFlags(
		[]string{"gck", "create", "--disable-portal", "--disable-es"},
		inherited, local, available,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(active) != 2 || active[0] != "disable-portal" || active[1] != "disable-es" {
		t.Fatalf("expected [disable-portal disable-es], got %v", active)
	}
}

func TestExtractActiveFlags_UnknownFlag(t *testing.T) {
	inherited := pflag.NewFlagSet("inherited", pflag.ContinueOnError)
	local := pflag.NewFlagSet("local", pflag.ContinueOnError)
	available := []config.ContextFlag{{Name: "disable-portal"}}

	_, err := extractActiveFlags(
		[]string{"gck", "create", "--no-such-flag"},
		inherited, local, available,
	)
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
	if !strings.Contains(err.Error(), "--no-such-flag") {
		t.Fatalf("expected error to mention the unknown flag, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--disable-portal") {
		t.Fatalf("expected error to list available flags, got: %v", err)
	}
}

func TestExtractActiveFlags_SkipsKnownCobraFlags(t *testing.T) {
	inherited := newFlagSet(t, map[string]string{"config": "", "registry": ""})
	local := newFlagSet(t, map[string]string{"name": ""})
	available := []config.ContextFlag{{Name: "disable-portal"}}

	active, err := extractActiveFlags(
		[]string{"gck", "create", "--config", "gck.yaml", "--registry", "http://r", "--name", "test", "--disable-portal"},
		inherited, local, available,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(active) != 1 || active[0] != "disable-portal" {
		t.Fatalf("expected [disable-portal], got %v", active)
	}
}

func TestExtractActiveFlags_StopsAtDoubleDash(t *testing.T) {
	inherited := pflag.NewFlagSet("inherited", pflag.ContinueOnError)
	local := pflag.NewFlagSet("local", pflag.ContinueOnError)
	available := []config.ContextFlag{{Name: "disable-portal"}}

	active, err := extractActiveFlags(
		[]string{"gck", "create", "--", "--disable-portal"},
		inherited, local, available,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected no active flags after --, got %v", active)
	}
}

func TestExtractActiveFlags_EqualsForm(t *testing.T) {
	inherited := pflag.NewFlagSet("inherited", pflag.ContinueOnError)
	local := pflag.NewFlagSet("local", pflag.ContinueOnError)
	available := []config.ContextFlag{{Name: "disable-portal"}}

	active, err := extractActiveFlags(
		[]string{"gck", "create", "--disable-portal=true"},
		inherited, local, available,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(active) != 1 || active[0] != "disable-portal" {
		t.Fatalf("expected [disable-portal], got %v", active)
	}
}

func TestExtractActiveFlags_SkipsHelpAndVersion(t *testing.T) {
	inherited := pflag.NewFlagSet("inherited", pflag.ContinueOnError)
	local := pflag.NewFlagSet("local", pflag.ContinueOnError)
	available := []config.ContextFlag{{Name: "disable-portal"}}

	active, err := extractActiveFlags(
		[]string{"gck", "create", "--help"},
		inherited, local, available,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected --help skipped, got %v", active)
	}
}

func TestExtractActiveFlags_IgnoresSingleDash(t *testing.T) {
	inherited := pflag.NewFlagSet("inherited", pflag.ContinueOnError)
	local := pflag.NewFlagSet("local", pflag.ContinueOnError)
	available := []config.ContextFlag{{Name: "disable-portal"}}

	active, err := extractActiveFlags(
		[]string{"gck", "create", "-v", "--disable-portal"},
		inherited, local, available,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(active) != 1 || active[0] != "disable-portal" {
		t.Fatalf("expected [disable-portal], got %v", active)
	}
}
