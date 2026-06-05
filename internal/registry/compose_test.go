package registry

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gravitee-io-labs/gck/internal/config"
)

// writeFile is a test helper that creates parent dirs and writes data.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFSResolver_ParentComposition(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "parent", "gck.yaml"), `
kind:
  name: parent-cluster
  nodes:
  - role: control-plane
    extraPortMappings:
    - containerPort: 80
      hostPort: 80

helm:
  repos:
    - name: base-repo
      url: https://example.com/base

components:
  - name: base-comp
    helm:
      chart: base/chart
`)

	writeFile(t, filepath.Join(root, "child", "gck.yaml"), `
from:
  - parent

kind:
  name: child-cluster

components:
  - name: child-comp
    helm:
      chart: child/chart
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "child")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Kind.Name != "child-cluster" {
		t.Fatalf("expected kind name %q, got %q", "child-cluster", resolved.Kind.Name)
	}
	if len(resolved.Kind.Nodes) != 1 || len(resolved.Kind.Nodes[0].ExtraPortMappings) != 1 {
		t.Fatal("expected child to inherit parent port mappings (child has nodes but no ports)")
	}
	if resolved.Kind.Nodes[0].ExtraPortMappings[0].ContainerPort != 80 {
		t.Fatalf("expected inherited port 80, got %d", resolved.Kind.Nodes[0].ExtraPortMappings[0].ContainerPort)
	}
	if len(resolved.Repos) != 1 || resolved.Repos[0].Name != "base-repo" {
		t.Fatalf("expected parent repo to be inherited, got %v", resolved.Repos)
	}
	if len(resolved.Components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(resolved.Components))
	}
}

func TestFSResolver_ParentComposition_ChildOverridesComponent(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "parent", "gck.yaml"), `
helm:
  repos:
    - name: repo1
      url: https://example.com/repo1

components:
  - name: app
    helm:
      chart: repo1/app
      version: "1.0"
      values:
        key1: val1
        key2: val2
`)

	writeFile(t, filepath.Join(root, "child", "gck.yaml"), `
from:
  - parent

components:
  - name: app
    helm:
      version: "2.0"
      values:
        key2: overridden
        key3: new
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "child")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resolved.Components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(resolved.Components))
	}
	comp := resolved.Components[0]
	if comp.Helm.Chart != "repo1/app" {
		t.Fatalf("expected chart preserved from parent, got %q", comp.Helm.Chart)
	}
	if comp.Helm.Version != "2.0" {
		t.Fatalf("expected version overridden to 2.0, got %q", comp.Helm.Version)
	}
	if comp.Helm.Values["key1"] != "val1" {
		t.Fatal("expected key1 preserved from parent")
	}
	if comp.Helm.Values["key2"] != "overridden" {
		t.Fatal("expected key2 overridden by child")
	}
	if comp.Helm.Values["key3"] != "new" {
		t.Fatal("expected key3 added by child")
	}
}

func TestFSResolver_ParentComposition_FeaturesMerge(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "parent", "gck.yaml"), `
features:
  lb:
    enabled: true
  dns:
    enabled: true
    domain: parent.local

components: []
`)

	writeFile(t, filepath.Join(root, "child", "gck.yaml"), `
from:
  - parent

features:
  dns:
    enabled: true
    domain: child.local
    port: 5353

components: []
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "child")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Features.LB == nil || !resolved.Features.LB.Enabled {
		t.Fatal("expected LB inherited from parent")
	}
	if resolved.Features.DNS == nil || resolved.Features.DNS.Domain != "child.local" {
		t.Fatal("expected DNS domain overridden by child")
	}
	if resolved.Features.DNS.Port != 5353 {
		t.Fatalf("expected DNS port 5353, got %d", resolved.Features.DNS.Port)
	}
}

func TestFSResolver_CycleDetection(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "a", "gck.yaml"), `
from:
  - b
components: []
`)
	writeFile(t, filepath.Join(root, "b", "gck.yaml"), `
from:
  - a
components: []
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	_, err := resolver.Resolve(context.Background(), "a")
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error message, got: %v", err)
	}
}

func TestFSResolver_SelfCycleDetection(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "self", "gck.yaml"), `
from:
  - self
components: []
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	_, err := resolver.Resolve(context.Background(), "self")
	if err == nil {
		t.Fatal("expected cycle detection error for self-reference")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error message, got: %v", err)
	}
}

func TestFSResolver_ThreeLevelComposition(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "grandparent", "gck.yaml"), `
kind:
  name: gp-cluster
  nodes:
  - role: control-plane
    extraPortMappings:
    - containerPort: 80
      hostPort: 80

helm:
  repos:
    - name: gp-repo
      url: https://example.com/gp

components:
  - name: gp-comp
    helm:
      chart: gp/chart
      values:
        gp-key: gp-val
`)

	writeFile(t, filepath.Join(root, "mid", "gck.yaml"), `
from:
  - grandparent

helm:
  repos:
    - name: mid-repo
      url: https://example.com/mid

components:
  - name: gp-comp
    helm:
      values:
        mid-key: mid-val
  - name: mid-comp
    helm:
      chart: mid/chart
`)

	writeFile(t, filepath.Join(root, "leaf", "gck.yaml"), `
from:
  - mid

kind:
  name: leaf-cluster

components:
  - name: leaf-comp
    helm:
      chart: leaf/chart
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "leaf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Kind.Name != "leaf-cluster" {
		t.Fatalf("expected kind name %q, got %q", "leaf-cluster", resolved.Kind.Name)
	}
	if len(resolved.Kind.Nodes[0].ExtraPortMappings) != 1 {
		t.Fatal("expected grandparent port mappings inherited through chain")
	}

	repoNames := make(map[string]bool)
	for _, r := range resolved.Repos {
		repoNames[r.Name] = true
	}
	if !repoNames["gp-repo"] || !repoNames["mid-repo"] {
		t.Fatalf("expected both gp-repo and mid-repo, got %v", resolved.Repos)
	}

	compNames := make(map[string]bool)
	for _, c := range resolved.Components {
		compNames[c.Name] = true
	}
	if !compNames["gp-comp"] || !compNames["mid-comp"] || !compNames["leaf-comp"] {
		t.Fatalf("expected all three components, got names %v", compNames)
	}
}

func TestFSResolver_SameRegistryImplicit(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "base", "gck.yaml"), `
components:
  - name: shared
    helm:
      chart: shared/chart
`)

	// Child omits registry → uses the same FS registry
	writeFile(t, filepath.Join(root, "derived", "gck.yaml"), `
from:
  - base

components:
  - name: extra
    helm:
      chart: extra/chart
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "derived")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved.Components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(resolved.Components))
	}
}

func TestFSResolver_CrossRegistryComposition(t *testing.T) {
	regA := t.TempDir()
	regB := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(regA, "parent", "gck.yaml"), `
components:
  - name: from-a
    helm:
      chart: a/chart
`)

	writeFile(t, filepath.Join(regB, "child", "gck.yaml"), `
registry: file://`+regA+`
from:
  - parent

components:
  - name: from-b
    helm:
      chart: b/chart
`)

	resolver := &FSResolver{Root: regB, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "child")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved.Components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(resolved.Components))
	}
	names := map[string]bool{}
	for _, c := range resolved.Components {
		names[c.Name] = true
	}
	if !names["from-a"] || !names["from-b"] {
		t.Fatalf("expected both from-a and from-b, got %v", names)
	}
}

func TestFSResolver_RelativeFileRegistryURL(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "org", "base", "gck.yaml"), `
kind:
  name: base-cluster

components:
  - name: base-comp
    helm:
      chart: base/chart
`)

	writeFile(t, filepath.Join(root, "org", "variants", "custom", "gck.yaml"), `
registry: file://../..
from:
  - base

kind:
  name: custom-cluster

components: []
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "org/variants/custom")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Kind.Name != "custom-cluster" {
		t.Fatalf("expected kind name %q, got %q", "custom-cluster", resolved.Kind.Name)
	}
	if len(resolved.Components) != 1 {
		t.Fatalf("expected 1 component from parent, got %d", len(resolved.Components))
	}
}

func TestFSResolver_NoParent(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "standalone", "gck.yaml"), `
kind:
  name: standalone

components:
  - name: app
    helm:
      chart: app/chart
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "standalone")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Kind.Name != "standalone" {
		t.Fatalf("expected kind name %q, got %q", "standalone", resolved.Kind.Name)
	}
	if len(resolved.Components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(resolved.Components))
	}
}

func TestMergeKind_ChildNameWins(t *testing.T) {
	base := config.KindConfig{Name: "base-cluster"}
	child := config.KindConfig{Name: "child-cluster"}
	result := mergeKind(base, child)
	if result.Name != "child-cluster" {
		t.Fatalf("expected %q, got %q", "child-cluster", result.Name)
	}
}

func TestMergeKind_EmptyChildInheritsBase(t *testing.T) {
	base := config.KindConfig{
		Name:       "base-cluster",
		APIVersion: "v1alpha4",
		Nodes: []config.KindNode{{
			Role:              "control-plane",
			ExtraPortMappings: []config.PortMapping{{ContainerPort: 80, HostPort: 80}},
		}},
	}
	child := config.KindConfig{}
	result := mergeKind(base, child)

	if result.Name != "base-cluster" {
		t.Fatalf("expected name inherited, got %q", result.Name)
	}
	if result.APIVersion != "v1alpha4" {
		t.Fatalf("expected apiVersion inherited, got %q", result.APIVersion)
	}
	if len(result.Nodes) != 1 || result.Nodes[0].ExtraPortMappings[0].ContainerPort != 80 {
		t.Fatal("expected nodes inherited from base")
	}
}

func TestMergeKind_ChildNodesInheritParentPorts(t *testing.T) {
	base := config.KindConfig{
		Nodes: []config.KindNode{{
			Role:              "control-plane",
			ExtraPortMappings: []config.PortMapping{{ContainerPort: 80, HostPort: 80}},
		}},
	}
	child := config.KindConfig{
		Nodes: []config.KindNode{{Role: "control-plane"}},
	}
	result := mergeKind(base, child)

	if len(result.Nodes[0].ExtraPortMappings) != 1 {
		t.Fatalf("expected child to inherit parent port mappings, got %d", len(result.Nodes[0].ExtraPortMappings))
	}
}

func TestMergeKind_ChildPortsWin(t *testing.T) {
	base := config.KindConfig{
		Nodes: []config.KindNode{{
			Role:              "control-plane",
			ExtraPortMappings: []config.PortMapping{{ContainerPort: 80, HostPort: 80}},
		}},
	}
	child := config.KindConfig{
		Nodes: []config.KindNode{{
			Role:              "control-plane",
			ExtraPortMappings: []config.PortMapping{{ContainerPort: 9090, HostPort: 9090}},
		}},
	}
	result := mergeKind(base, child)

	if len(result.Nodes[0].ExtraPortMappings) != 2 {
		t.Fatalf("expected 2 port mappings (union), got %d", len(result.Nodes[0].ExtraPortMappings))
	}
	portMap := make(map[int32]struct{})
	for _, p := range result.Nodes[0].ExtraPortMappings {
		portMap[p.ContainerPort] = struct{}{}
	}
	for _, want := range []int32{80, 9090} {
		if _, ok := portMap[want]; !ok {
			t.Fatalf("expected port %d to be present", want)
		}
	}
}

func TestMergeKind_PortsUnionDeduplicated(t *testing.T) {
	base := config.KindConfig{
		Nodes: []config.KindNode{{
			Role: "control-plane",
			ExtraPortMappings: []config.PortMapping{
				{ContainerPort: 80, HostPort: 80},
				{ContainerPort: 443, HostPort: 443},
			},
		}},
	}
	child := config.KindConfig{
		Nodes: []config.KindNode{{
			Role: "control-plane",
			ExtraPortMappings: []config.PortMapping{
				{ContainerPort: 443, HostPort: 8443},
				{ContainerPort: 9090, HostPort: 9090},
			},
		}},
	}
	result := mergeKind(base, child)

	if len(result.Nodes[0].ExtraPortMappings) != 3 {
		t.Fatalf("expected 3 port mappings (union, deduped), got %d", len(result.Nodes[0].ExtraPortMappings))
	}
	portMap := make(map[int32]int32)
	for _, p := range result.Nodes[0].ExtraPortMappings {
		portMap[p.ContainerPort] = p.HostPort
	}
	if hp, ok := portMap[80]; !ok || hp != 80 {
		t.Fatal("expected base-only port 80 with hostPort 80")
	}
	if hp, ok := portMap[443]; !ok || hp != 8443 {
		t.Fatalf("expected child override for port 443 with hostPort 8443, got %d", portMap[443])
	}
	if hp, ok := portMap[9090]; !ok || hp != 9090 {
		t.Fatal("expected child-only port 9090 with hostPort 9090")
	}
}

func TestWithVisited_DetectsCycle(t *testing.T) {
	ctx := context.Background()
	ref := contextRef{Registry: "file:///reg", Context: "path/a"}

	ctx, err := withVisited(ctx, ref)
	if err != nil {
		t.Fatalf("first visit should not error: %v", err)
	}

	_, err = withVisited(ctx, ref)
	if err == nil {
		t.Fatal("expected cycle error on second visit")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error message, got: %v", err)
	}
}

func TestWithVisited_DifferentRefsOK(t *testing.T) {
	ctx := context.Background()

	ctx, err := withVisited(ctx, contextRef{Registry: "file:///reg", Context: "a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = withVisited(ctx, contextRef{Registry: "file:///reg", Context: "b"})
	if err != nil {
		t.Fatalf("different context should not error: %v", err)
	}
}

func TestResolveRegistryURL_AbsoluteFileURL(t *testing.T) {
	result := resolveRegistryURL("file:///absolute/path", "/some/dir")
	if result != "file:///absolute/path" {
		t.Fatalf("expected absolute path preserved, got %q", result)
	}
}

func TestResolveRegistryURL_RelativeFileURL(t *testing.T) {
	result := resolveRegistryURL("file://./..", "/some/context/dir")
	if result != "file:///some/context" {
		t.Fatalf("expected resolved to %q, got %q", "file:///some/context", result)
	}
}

func TestResolveRegistryURL_HTTPPassthrough(t *testing.T) {
	result := resolveRegistryURL("https://example.com/registry", "/some/dir")
	if result != "https://example.com/registry" {
		t.Fatalf("expected HTTP URL unchanged, got %q", result)
	}
}

func TestFSResolver_ParentComposition_ImagesMerge(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "parent", "gck.yaml"), `
images:
  preload:
    refs:
      - img-a
      - img-b

components:
  - name: app
    helm:
      chart: app/chart
`)

	writeFile(t, filepath.Join(root, "child", "gck.yaml"), `
from:
  - parent

images:
  preload:
    refs:
      - img-b
      - img-c

components: []
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "child")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Images.Preload == nil {
		t.Fatal("expected Preload from union of parent and child")
	}
	if len(resolved.Images.Preload.Refs) != 3 {
		t.Fatalf("expected 3 refs from union of parent and child, got %d: %v", len(resolved.Images.Preload.Refs), resolved.Images.Preload.Refs)
	}
	expected := map[string]bool{"img-a": true, "img-b": true, "img-c": true}
	for _, ref := range resolved.Images.Preload.Refs {
		if !expected[ref] {
			t.Fatalf("unexpected ref %q; expected union of parent and child refs", ref)
		}
	}
}

func TestFSResolver_NoParent_ImagesPreserved(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "standalone", "gck.yaml"), `
images:
  preload:
    refs:
      - my-image:latest

components:
  - name: app
    helm:
      chart: app/chart
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "standalone")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Images.Preload == nil {
		t.Fatal("expected Preload to be non-nil")
	}
	if len(resolved.Images.Preload.Refs) != 1 || resolved.Images.Preload.Refs[0] != "my-image:latest" {
		t.Fatalf("expected [my-image:latest], got %v", resolved.Images.Preload.Refs)
	}
}

func TestFSResolver_DefaultVariant(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "product", ".default"), `standard`)
	writeFile(t, filepath.Join(root, "product", "standard", "gck.yaml"), `
kind:
  name: standard-cluster

components:
  - name: standard-app
    helm:
      chart: std/chart
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "product")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Kind.Name != "standard-cluster" {
		t.Fatalf("expected kind name %q, got %q", "standard-cluster", resolved.Kind.Name)
	}
	if len(resolved.Components) != 1 || resolved.Components[0].Name != "standard-app" {
		t.Fatal("expected standard-app component")
	}
}

func TestFSResolver_DefaultVariantWithComposition(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "base", "gck.yaml"), `
helm:
  repos:
    - name: base-repo
      url: https://example.com/base

components:
  - name: base-comp
    helm:
      chart: base/chart
`)
	writeFile(t, filepath.Join(root, "product", ".default"), `standard`)
	writeFile(t, filepath.Join(root, "product", "standard", "gck.yaml"), `
from:
  - base

components:
  - name: extra
    helm:
      chart: extra/chart
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "product")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved.Components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(resolved.Components))
	}
	if len(resolved.Repos) != 1 || resolved.Repos[0].Name != "base-repo" {
		t.Fatalf("expected base repo inherited, got %v", resolved.Repos)
	}
}

func TestFSResolver_ThreeLevelCycleDetection(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "a", "gck.yaml"), `
from:
  - b
components: []
`)
	writeFile(t, filepath.Join(root, "b", "gck.yaml"), `
from:
  - c
components: []
`)
	writeFile(t, filepath.Join(root, "c", "gck.yaml"), `
from:
  - a
components: []
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	_, err := resolver.Resolve(context.Background(), "a")
	if err == nil {
		t.Fatal("expected cycle detection error for a→b→c→a")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error message, got: %v", err)
	}
}

func TestFSResolver_MultiFromComposition(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "mongodb", "standalone", "gck.yaml"), `
helm:
  repos:
    - name: mongodb-repo
      url: https://example.com/mongo

images:
  preload:
    refs:
      - mongo:7

components:
  - name: mongodb
    type: k8s
    k8s:
      manifestFiles:
        - mongodb.yaml
`)
	writeFile(t, filepath.Join(root, "mongodb", "standalone", "mongodb.yaml"), "# mongo manifest")

	writeFile(t, filepath.Join(root, "elastic", "elasticsearch", "standalone", "gck.yaml"), `
helm:
  repos:
    - name: elastic-repo
      url: https://helm.elastic.co

images:
  preload:
    refs:
      - docker.elastic.co/elasticsearch/elasticsearch:9.3.1

components:
  - name: elasticsearch
    type: helm
    helm:
      chart: elastic/elasticsearch
      valueFiles:
        - values-elasticsearch.yaml
`)
	writeFile(t, filepath.Join(root, "elastic", "elasticsearch", "standalone", "values-elasticsearch.yaml"), "# elastic values")

	writeFile(t, filepath.Join(root, "app", "gck.yaml"), `
from:
  - mongodb/standalone
  - elastic/elasticsearch/standalone

kind:
  name: app-cluster

helm:
  repos:
    - name: app-repo
      url: https://example.com/app

components:
  - name: app-comp
    helm:
      chart: app/chart
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Kind.Name != "app-cluster" {
		t.Fatalf("expected kind name %q, got %q", "app-cluster", resolved.Kind.Name)
	}

	repoNames := make(map[string]bool)
	for _, r := range resolved.Repos {
		repoNames[r.Name] = true
	}
	if !repoNames["mongodb-repo"] || !repoNames["elastic-repo"] || !repoNames["app-repo"] {
		t.Fatalf("expected all three repos, got %v", resolved.Repos)
	}

	compNames := make(map[string]bool)
	for _, c := range resolved.Components {
		compNames[c.Name] = true
	}
	if !compNames["mongodb"] || !compNames["elasticsearch"] || !compNames["app-comp"] {
		t.Fatalf("expected mongodb, elasticsearch, and app-comp, got %v", compNames)
	}

	if len(resolved.Images.Preload.Refs) != 2 {
		t.Fatalf("expected 2 preload refs from union of all from entries, got %d: %v", len(resolved.Images.Preload.Refs), resolved.Images.Preload.Refs)
	}
	preloadRefs := make(map[string]bool)
	for _, r := range resolved.Images.Preload.Refs {
		preloadRefs[r] = true
	}
	if !preloadRefs["mongo:7"] || !preloadRefs["docker.elastic.co/elasticsearch/elasticsearch:9.3.1"] {
		t.Fatalf("expected union of mongo and elastic refs, got %v", resolved.Images.Preload.Refs)
	}
}

func TestFSResolver_MultiFromPathAbsolutization(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "parent-a", "gck.yaml"), `
components:
  - name: comp-a
    type: helm
    helm:
      chart: a/chart
      valueFiles:
        - values-a.yaml
`)
	writeFile(t, filepath.Join(root, "parent-a", "values-a.yaml"), "# values a")

	writeFile(t, filepath.Join(root, "parent-b", "gck.yaml"), `
components:
  - name: comp-b
    type: k8s
    k8s:
      manifestFiles:
        - manifest-b.yaml
`)
	writeFile(t, filepath.Join(root, "parent-b", "manifest-b.yaml"), "# manifest b")

	writeFile(t, filepath.Join(root, "child", "gck.yaml"), `
from:
  - parent-a
  - parent-b
components: []
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "child")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resolved.Components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(resolved.Components))
	}

	for _, comp := range resolved.Components {
		switch comp.Name {
		case "comp-a":
			if len(comp.Helm.ValueFiles) != 1 {
				t.Fatalf("expected 1 value file for comp-a, got %d", len(comp.Helm.ValueFiles))
			}
			expected := filepath.Join(root, "parent-a", "values-a.yaml")
			if comp.Helm.ValueFiles[0] != expected {
				t.Fatalf("expected value file %q, got %q", expected, comp.Helm.ValueFiles[0])
			}
		case "comp-b":
			if len(comp.K8s.ManifestFiles) != 1 {
				t.Fatalf("expected 1 manifest file for comp-b, got %d", len(comp.K8s.ManifestFiles))
			}
			expected := filepath.Join(root, "parent-b", "manifest-b.yaml")
			if comp.K8s.ManifestFiles[0] != expected {
				t.Fatalf("expected manifest file %q, got %q", expected, comp.K8s.ManifestFiles[0])
			}
		}
	}
}

func TestFSResolver_AbstractContextDirect(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "base", "gck.yaml"), `
abstract: true

helm:
  repos:
    - name: base-repo
      url: https://example.com/base

components:
  - name: base-comp
    helm:
      chart: base/chart
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "base")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resolved.Abstract {
		t.Fatal("expected Abstract to be true for abstract context")
	}
}

func TestFSResolver_AbstractComposedProducesNonAbstract(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "base", "gck.yaml"), `
abstract: true

helm:
  repos:
    - name: base-repo
      url: https://example.com/base

components:
  - name: base-comp
    helm:
      chart: base/chart
`)

	writeFile(t, filepath.Join(root, "concrete", "gck.yaml"), `
from:
  - base

components:
  - name: extra
    helm:
      chart: extra/chart
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "concrete")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Abstract {
		t.Fatal("expected Abstract to be false when composing from an abstract context")
	}
	if len(resolved.Components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(resolved.Components))
	}
}

func TestFSResolver_AbstractWithFrom(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "grandparent", "gck.yaml"), `
components:
  - name: gp-comp
    helm:
      chart: gp/chart
`)

	writeFile(t, filepath.Join(root, "mid", "gck.yaml"), `
abstract: true
from:
  - grandparent

components:
  - name: mid-comp
    helm:
      chart: mid/chart
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "mid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resolved.Abstract {
		t.Fatal("expected Abstract to be true for abstract context that uses 'from'")
	}
	if len(resolved.Components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(resolved.Components))
	}
}

func TestFSResolver_MultiFromCycleDetection(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "a", "gck.yaml"), `
from:
  - b
  - c
components: []
`)
	writeFile(t, filepath.Join(root, "b", "gck.yaml"), `
components:
  - name: b-comp
    helm:
      chart: b/chart
`)
	writeFile(t, filepath.Join(root, "c", "gck.yaml"), `
from:
  - a
components: []
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	_, err := resolver.Resolve(context.Background(), "a")
	if err == nil {
		t.Fatal("expected cycle detection error for a→c→a")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error message, got: %v", err)
	}
}

// TestFSResolver_NamespacePropagatesThroughAbstractComposition mirrors the
// real ee/kafka composition chain:
//
//	kafka/standalone (type: k8s, no namespace)
//	  ↑ from
//	ee/kafka/base (abstract, sets kafka namespace: gravitee)
//	  ↑ from (with oss/jdbc/postgres)
//	ee/kafka/jdbc/postgres (concrete, via .default chain)
//	  ↑ .default
//	ee/kafka
//
// The test asserts the kafka component in the final resolved context has
// namespace "gravitee".
func TestFSResolver_NamespacePropagatesThroughAbstractComposition(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "kafka", "standalone", "gck.yaml"), `
components:
  - name: kafka
    type: k8s
    k8s:
      manifests:
        - apiVersion: apps/v1
          kind: Deployment
          metadata:
            name: kafka
            labels:
              app: kafka
          spec:
            replicas: 1
            selector:
              matchLabels:
                app: kafka
            template:
              metadata:
                labels:
                  app: kafka
              spec:
                containers:
                  - name: kafka
                    image: apache/kafka:latest
        - apiVersion: v1
          kind: Service
          metadata:
            name: kafka
          spec:
            type: NodePort
            ports:
              - port: 9092
            selector:
              app: kafka
`)

	writeFile(t, filepath.Join(root, "oss", "postgres", "gck.yaml"), `
components:
  - name: apim
    helm:
      chart: graviteeio/apim3
`)

	writeFile(t, filepath.Join(root, "ee", "kafka", "base", "gck.yaml"), `
abstract: true
from:
  - kafka/standalone
components:
  - name: kafka
    namespace: gravitee
    k8s:
      manifests:
        - apiVersion: v1
          kind: Service
          metadata:
            name: kafka
          spec:
            type: ClusterIP
            ports:
              - port: 9092
            selector:
              app: kafka
  - name: apim
    helm:
      values:
        gateway:
          kafka:
            enabled: true
`)

	writeFile(t, filepath.Join(root, "ee", "kafka", "postgres", "gck.yaml"), `
from:
  - oss/postgres
  - ee/kafka/base
`)

	writeFile(t, filepath.Join(root, "ee", "kafka", ".default"), `postgres`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "ee/kafka")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var kafkaComp *config.Component
	for i := range resolved.Components {
		if resolved.Components[i].Name == "kafka" {
			kafkaComp = &resolved.Components[i]
			break
		}
	}
	if kafkaComp == nil {
		t.Fatal("kafka component not found in resolved context")
	}
	if kafkaComp.Namespace != "gravitee" {
		t.Fatalf("expected kafka namespace %q, got %q", "gravitee", kafkaComp.Namespace)
	}
	if kafkaComp.K8s == nil {
		t.Fatal("expected kafka component to have K8s spec")
	}

	foundClusterIP := false
	for _, m := range kafkaComp.K8s.Manifests {
		if m["kind"] == "Service" {
			spec, _ := m["spec"].(map[string]interface{})
			if spec != nil && spec["type"] == "ClusterIP" {
				foundClusterIP = true
			}
		}
	}
	if !foundClusterIP {
		t.Fatal("expected kafka Service to be overridden to ClusterIP")
	}
}

func TestFSResolver_NoParent_DiscoversFlagsInDir(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "ctx", "gck.yaml"), `
components:
  - name: app
    helm:
      chart: app/chart
`)
	writeFile(t, filepath.Join(root, "ctx", "gck--disable-portal.yaml"), `
description: "Disable portal"
components: []
`)
	writeFile(t, filepath.Join(root, "ctx", "gck--disable-ui.yaml"), `
description: "Disable all UIs"
components: []
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "ctx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved.Flags) != 2 {
		t.Fatalf("expected 2 flags, got %d", len(resolved.Flags))
	}
	names := map[string]bool{}
	for _, f := range resolved.Flags {
		names[f.Name] = true
	}
	if !names["disable-portal"] || !names["disable-ui"] {
		t.Fatalf("expected disable-portal and disable-ui flags, got %v", names)
	}
}

func TestFSResolver_ParentFlagsInherited(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "parent", "gck.yaml"), `
components:
  - name: app
    helm:
      chart: app/chart
`)
	writeFile(t, filepath.Join(root, "parent", "gck--disable-portal.yaml"), `
description: "Disable portal"
components: []
`)

	writeFile(t, filepath.Join(root, "child", "gck.yaml"), `
from:
  - parent
components: []
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "child")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved.Flags) != 1 {
		t.Fatalf("expected 1 inherited flag, got %d", len(resolved.Flags))
	}
	if resolved.Flags[0].Name != "disable-portal" {
		t.Fatalf("expected flag %q, got %q", "disable-portal", resolved.Flags[0].Name)
	}
	expectedDir := filepath.Join(root, "parent")
	if resolved.Flags[0].Dir != expectedDir {
		t.Fatalf("expected flag dir %q, got %q", expectedDir, resolved.Flags[0].Dir)
	}
}

func TestFSResolver_ChildFlagOverridesParent(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "parent", "gck.yaml"), `
components:
  - name: app
    helm:
      chart: app/chart
`)
	writeFile(t, filepath.Join(root, "parent", "gck--disable-portal.yaml"), `
description: "Parent: disable portal"
components: []
`)

	writeFile(t, filepath.Join(root, "child", "gck.yaml"), `
from:
  - parent
components: []
`)
	writeFile(t, filepath.Join(root, "child", "gck--disable-portal.yaml"), `
description: "Child: disable portal with extras"
components: []
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "child")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved.Flags) != 1 {
		t.Fatalf("expected 1 flag, got %d", len(resolved.Flags))
	}
	if resolved.Flags[0].Description != "Child: disable portal with extras" {
		t.Fatalf("expected child description to win, got %q", resolved.Flags[0].Description)
	}
	expectedDir := filepath.Join(root, "child")
	if resolved.Flags[0].Dir != expectedDir {
		t.Fatalf("expected child dir %q, got %q", expectedDir, resolved.Flags[0].Dir)
	}
}

func TestFSResolver_ChildAddsFlagsToParent(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "parent", "gck.yaml"), `
components:
  - name: app
    helm:
      chart: app/chart
`)
	writeFile(t, filepath.Join(root, "parent", "gck--disable-portal.yaml"), `
description: "Disable portal"
components: []
`)

	writeFile(t, filepath.Join(root, "child", "gck.yaml"), `
from:
  - parent
components: []
`)
	writeFile(t, filepath.Join(root, "child", "gck--disable-ui.yaml"), `
description: "Disable all UIs"
components: []
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "child")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved.Flags) != 2 {
		t.Fatalf("expected 2 flags, got %d", len(resolved.Flags))
	}
	names := map[string]bool{}
	for _, f := range resolved.Flags {
		names[f.Name] = true
	}
	if !names["disable-portal"] || !names["disable-ui"] {
		t.Fatalf("expected disable-portal and disable-ui, got %v", names)
	}
}

func TestFSResolver_MultiFromFlagsMerged(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "parent-a", "gck.yaml"), `
components:
  - name: comp-a
    helm:
      chart: a/chart
`)
	writeFile(t, filepath.Join(root, "parent-a", "gck--flag-a.yaml"), `
description: "Flag from A"
components: []
`)

	writeFile(t, filepath.Join(root, "parent-b", "gck.yaml"), `
components:
  - name: comp-b
    helm:
      chart: b/chart
`)
	writeFile(t, filepath.Join(root, "parent-b", "gck--flag-b.yaml"), `
description: "Flag from B"
components: []
`)

	writeFile(t, filepath.Join(root, "child", "gck.yaml"), `
from:
  - parent-a
  - parent-b
components: []
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "child")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved.Flags) != 2 {
		t.Fatalf("expected 2 flags from both parents, got %d", len(resolved.Flags))
	}
	names := map[string]bool{}
	for _, f := range resolved.Flags {
		names[f.Name] = true
	}
	if !names["flag-a"] || !names["flag-b"] {
		t.Fatalf("expected flag-a and flag-b, got %v", names)
	}
}

func TestFSResolver_TemplatedContext(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "tmpl-ctx", "gck.yaml"), `vars:
  helmVersion: ""
  imageTag: "latest"

components:
  - name: app
    helm:
      chart: repo/app
      version: "{{ .helmVersion }}"
      values:
        image:
          tag: "{{ .imageTag }}-debian"
`)

	resolver := &FSResolver{
		Root:         root,
		GckHome:      gckHome,
		SetOverrides: map[string]string{"imageTag": "4.12.0"},
	}
	resolved, err := resolver.Resolve(context.Background(), "tmpl-ctx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resolved.Components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(resolved.Components))
	}
	comp := resolved.Components[0]
	if comp.Helm.Version != "" {
		t.Fatalf("expected empty helmVersion (latest), got %q", comp.Helm.Version)
	}
	tag := comp.Helm.Values["image"].(map[string]interface{})["tag"]
	if tag != "4.12.0-debian" {
		t.Fatalf("expected 4.12.0-debian, got %v", tag)
	}
}

func TestFSResolver_TemplatedContext_DefaultVars(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "default-ctx", "gck.yaml"), `vars:
  imageTag: "latest"

components:
  - name: app
    helm:
      chart: repo/app
      values:
        image:
          tag: "{{ .imageTag }}"
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "default-ctx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tag := resolved.Components[0].Helm.Values["image"].(map[string]interface{})["tag"]
	if tag != "latest" {
		t.Fatalf("expected default latest, got %v", tag)
	}
}

func TestFSResolver_TemplatedParentChild(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "parent", "gck.yaml"), `vars:
  dbVersion: "15"

components:
  - name: database
    helm:
      chart: bitnami/postgresql
      version: "{{ .dbVersion }}"
`)

	writeFile(t, filepath.Join(root, "child", "gck.yaml"), `vars:
  imageTag: "latest"

from:
  - parent

components:
  - name: app
    helm:
      chart: repo/app
      values:
        image:
          tag: "{{ .imageTag }}"
`)

	resolver := &FSResolver{
		Root:         root,
		GckHome:      gckHome,
		SetOverrides: map[string]string{"dbVersion": "16", "imageTag": "v2"},
	}
	resolved, err := resolver.Resolve(context.Background(), "child")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	compByName := make(map[string]config.Component)
	for _, c := range resolved.Components {
		compByName[c.Name] = c
	}

	db := compByName["database"]
	if db.Helm.Version != "16" {
		t.Fatalf("expected parent dbVersion overridden to 16, got %q", db.Helm.Version)
	}

	app := compByName["app"]
	tag := app.Helm.Values["image"].(map[string]interface{})["tag"]
	if tag != "v2" {
		t.Fatalf("expected child imageTag overridden to v2, got %v", tag)
	}
}

func TestFSResolver_ThreeLevelFlagInheritance(t *testing.T) {
	root := t.TempDir()
	gckHome := t.TempDir()

	writeFile(t, filepath.Join(root, "grandparent", "gck.yaml"), `
components:
  - name: gp-comp
    helm:
      chart: gp/chart
`)
	writeFile(t, filepath.Join(root, "grandparent", "gck--gp-flag.yaml"), `
description: "Grandparent flag"
components: []
`)

	writeFile(t, filepath.Join(root, "mid", "gck.yaml"), `
from:
  - grandparent
components: []
`)
	writeFile(t, filepath.Join(root, "mid", "gck--mid-flag.yaml"), `
description: "Mid-level flag"
components: []
`)

	writeFile(t, filepath.Join(root, "leaf", "gck.yaml"), `
from:
  - mid
components: []
`)

	resolver := &FSResolver{Root: root, GckHome: gckHome}
	resolved, err := resolver.Resolve(context.Background(), "leaf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved.Flags) != 2 {
		t.Fatalf("expected 2 flags inherited through chain, got %d", len(resolved.Flags))
	}
	names := map[string]bool{}
	for _, f := range resolved.Flags {
		names[f.Name] = true
	}
	if !names["gp-flag"] || !names["mid-flag"] {
		t.Fatalf("expected gp-flag and mid-flag, got %v", names)
	}
}
