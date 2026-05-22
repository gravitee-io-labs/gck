package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestServer(t *testing.T, root string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.FileServer(http.Dir(root)))
	t.Cleanup(srv.Close)
	return srv
}

func newHTTPResolver(t *testing.T, baseURL string) *HTTPResolver {
	t.Helper()
	sewHome := t.TempDir()
	return &HTTPResolver{
		BaseURL:   baseURL,
		CacheRoot: filepath.Join(sewHome, "cache"),
		SewHome:   sewHome,
	}
}

func TestHTTPResolver_Standalone(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "myctx", "sew.yaml"), `
kind:
  name: my-cluster

components:
  - name: app
    helm:
      chart: repo/app
`)

	srv := newTestServer(t, root)
	resolver := newHTTPResolver(t, srv.URL)

	resolved, err := resolver.Resolve(context.Background(), "myctx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Kind.Name != "my-cluster" {
		t.Fatalf("expected kind name %q, got %q", "my-cluster", resolved.Kind.Name)
	}
	if len(resolved.Components) != 1 || resolved.Components[0].Name != "app" {
		t.Fatalf("expected 1 component 'app', got %v", resolved.Components)
	}
}

func TestHTTPResolver_ParentComposition(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "parent", "sew.yaml"), `
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
	writeFile(t, filepath.Join(root, "child", "sew.yaml"), `
from:
  - parent

kind:
  name: child-cluster

components:
  - name: child-comp
    helm:
      chart: child/chart
`)

	srv := newTestServer(t, root)
	resolver := newHTTPResolver(t, srv.URL)

	resolved, err := resolver.Resolve(context.Background(), "child")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Kind.Name != "child-cluster" {
		t.Fatalf("expected kind name %q, got %q", "child-cluster", resolved.Kind.Name)
	}
	if len(resolved.Kind.Nodes) != 1 || len(resolved.Kind.Nodes[0].ExtraPortMappings) != 1 {
		t.Fatal("expected child to inherit parent port mappings")
	}
	if len(resolved.Repos) != 1 || resolved.Repos[0].Name != "base-repo" {
		t.Fatalf("expected parent repo inherited, got %v", resolved.Repos)
	}
	if len(resolved.Components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(resolved.Components))
	}
}

func TestHTTPResolver_ChildOverridesComponent(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "parent", "sew.yaml"), `
components:
  - name: app
    helm:
      chart: repo/app
      version: "1.0"
      values:
        key1: val1
        key2: val2
`)
	writeFile(t, filepath.Join(root, "child", "sew.yaml"), `
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

	srv := newTestServer(t, root)
	resolver := newHTTPResolver(t, srv.URL)

	resolved, err := resolver.Resolve(context.Background(), "child")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved.Components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(resolved.Components))
	}
	comp := resolved.Components[0]
	if comp.Helm.Chart != "repo/app" {
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

func TestHTTPResolver_FeaturesMerge(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "parent", "sew.yaml"), `
features:
  lb:
    enabled: true
  dns:
    enabled: true
    domain: parent.local

components: []
`)
	writeFile(t, filepath.Join(root, "child", "sew.yaml"), `
from:
  - parent

features:
  dns:
    enabled: true
    domain: child.local
    port: 5353

components: []
`)

	srv := newTestServer(t, root)
	resolver := newHTTPResolver(t, srv.URL)

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

func TestHTTPResolver_CycleDetection(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a", "sew.yaml"), `
from:
  - b
components: []
`)
	writeFile(t, filepath.Join(root, "b", "sew.yaml"), `
from:
  - a
components: []
`)

	srv := newTestServer(t, root)
	resolver := newHTTPResolver(t, srv.URL)

	_, err := resolver.Resolve(context.Background(), "a")
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error message, got: %v", err)
	}
}

func TestHTTPResolver_ThreeLevelComposition(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "grandparent", "sew.yaml"), `
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
	writeFile(t, filepath.Join(root, "mid", "sew.yaml"), `
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
	writeFile(t, filepath.Join(root, "leaf", "sew.yaml"), `
from:
  - mid

kind:
  name: leaf-cluster

components:
  - name: leaf-comp
    helm:
      chart: leaf/chart
`)

	srv := newTestServer(t, root)
	resolver := newHTTPResolver(t, srv.URL)

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

func TestHTTPResolver_CrossRegistryToFS(t *testing.T) {
	fsRoot := t.TempDir()
	writeFile(t, filepath.Join(fsRoot, "parent", "sew.yaml"), `
components:
  - name: from-fs
    helm:
      chart: fs/chart
`)

	httpRoot := t.TempDir()
	writeFile(t, filepath.Join(httpRoot, "child", "sew.yaml"), `
registry: file://`+fsRoot+`
from:
  - parent

components:
  - name: from-http
    helm:
      chart: http/chart
`)

	srv := newTestServer(t, httpRoot)
	resolver := newHTTPResolver(t, srv.URL)

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
	if !names["from-fs"] || !names["from-http"] {
		t.Fatalf("expected both from-fs and from-http, got %v", names)
	}
}

func TestHTTPResolver_CrossRegistryCycleDetection(t *testing.T) {
	fsRoot := t.TempDir()
	httpRoot := t.TempDir()

	srv := newTestServer(t, httpRoot)

	writeFile(t, filepath.Join(httpRoot, "a", "sew.yaml"), `
registry: file://`+fsRoot+`
from:
  - b
components: []
`)
	writeFile(t, filepath.Join(fsRoot, "b", "sew.yaml"), `
registry: `+srv.URL+`
from:
  - a
components: []
`)

	resolver := newHTTPResolver(t, srv.URL)
	_, err := resolver.Resolve(context.Background(), "a")
	if err == nil {
		t.Fatal("expected cycle detection error across registries")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error message, got: %v", err)
	}
}

func TestHTTPResolver_DefaultVariant(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "product", ".default"), `standard`)
	writeFile(t, filepath.Join(root, "product", "standard", "sew.yaml"), `
kind:
  name: standard-cluster

components:
  - name: standard-app
    helm:
      chart: std/chart
`)

	srv := newTestServer(t, root)
	resolver := newHTTPResolver(t, srv.URL)

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

func TestHTTPResolver_ValueFilesDownload(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "ctx", "sew.yaml"), `
components:
  - name: app
    helm:
      chart: repo/app
      valueFiles:
        - custom-values.yaml
`)
	writeFile(t, filepath.Join(root, "ctx", "custom-values.yaml"), `replicas: 3`)

	srv := newTestServer(t, root)
	sewHome := t.TempDir()
	resolver := &HTTPResolver{
		BaseURL:   srv.URL,
		CacheRoot: filepath.Join(sewHome, "cache"),
		SewHome:   sewHome,
	}

	resolved, err := resolver.Resolve(context.Background(), "ctx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved.Components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(resolved.Components))
	}

	cached := filepath.Join(resolver.CacheRoot, "ctx", "custom-values.yaml")
	data, err := os.ReadFile(cached)
	if err != nil {
		t.Fatalf("expected value file cached at %s: %v", cached, err)
	}
	if !strings.Contains(string(data), "replicas: 3") {
		t.Fatalf("expected cached content 'replicas: 3', got %q", string(data))
	}
}

func TestHTTPResolver_NotFound(t *testing.T) {
	root := t.TempDir()

	srv := newTestServer(t, root)
	resolver := newHTTPResolver(t, srv.URL)

	_, err := resolver.Resolve(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing context")
	}
}

func TestHTTPResolver_FlagsFromManifest(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "ctx", "sew.yaml"), `
kind:
  name: my-cluster
components:
  - name: app
    helm:
      chart: repo/app
`)
	writeFile(t, filepath.Join(root, "ctx", "sew.flags.yaml"), `
flags:
  - name: disable-portal
    description: "Disable the developer portal UI"
  - name: disable-ui
    description: "Disable all UIs"
`)
	writeFile(t, filepath.Join(root, "ctx", "sew--disable-portal.yaml"), `
description: "Disable the developer portal UI"
components:
  - name: app
    helm:
      values:
        portal:
          enabled: false
`)
	writeFile(t, filepath.Join(root, "ctx", "sew--disable-ui.yaml"), `
description: "Disable all UIs"
components:
  - name: app
    helm:
      values:
        ui:
          enabled: false
`)

	srv := newTestServer(t, root)
	resolver := newHTTPResolver(t, srv.URL)

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

func TestHTTPResolver_NoFlagsManifest(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "ctx", "sew.yaml"), `
components:
  - name: app
    helm:
      chart: repo/app
`)

	srv := newTestServer(t, root)
	resolver := newHTTPResolver(t, srv.URL)

	resolved, err := resolver.Resolve(context.Background(), "ctx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved.Flags) != 0 {
		t.Fatalf("expected no flags, got %d", len(resolved.Flags))
	}
}

func TestHTTPResolver_FlagsWithComposition(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "parent", "sew.yaml"), `
components:
  - name: app
    helm:
      chart: repo/app
`)
	writeFile(t, filepath.Join(root, "parent", "sew.flags.yaml"), `
flags:
  - name: parent-flag
    description: "A flag from parent"
`)
	writeFile(t, filepath.Join(root, "parent", "sew--parent-flag.yaml"), `
description: "A flag from parent"
components:
  - name: app
    helm:
      values:
        debug: true
`)

	writeFile(t, filepath.Join(root, "child", "sew.yaml"), `
from:
  - parent
components:
  - name: extra
    helm:
      chart: extra/chart
`)
	writeFile(t, filepath.Join(root, "child", "sew.flags.yaml"), `
flags:
  - name: parent-flag
    description: "A flag from parent"
  - name: child-flag
    description: "A flag from child"
`)
	writeFile(t, filepath.Join(root, "child", "sew--parent-flag.yaml"), `
description: "A flag from parent"
components:
  - name: app
    helm:
      values:
        debug: true
`)
	writeFile(t, filepath.Join(root, "child", "sew--child-flag.yaml"), `
description: "A flag from child"
components:
  - name: extra
    helm:
      values:
        verbose: true
`)

	srv := newTestServer(t, root)
	resolver := newHTTPResolver(t, srv.URL)

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
	if !names["parent-flag"] || !names["child-flag"] {
		t.Fatalf("expected parent-flag and child-flag, got %v", names)
	}
}

func TestHTTPResolver_NetrcAuth(t *testing.T) {
	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		if r.URL.Path == "/ctx/sew.yaml" {
			w.Write([]byte("components:\n  - name: app\n    helm:\n      chart: repo/app\n"))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	netrcFile := filepath.Join(dir, "netrc")
	writeFile(t, netrcFile, "machine 127.0.0.1 login deploy password tok3n\n")
	t.Setenv("NETRC", netrcFile)

	sewHome := t.TempDir()
	resolver := NewResolver(srv.URL, sewHome, nil)

	_, err := resolver.Resolve(context.Background(), "ctx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if authHeader == "" {
		t.Fatal("expected Authorization header to be set from .netrc credentials")
	}
	if !strings.HasPrefix(authHeader, "Basic ") {
		t.Fatalf("expected Basic auth, got %q", authHeader)
	}
}

func TestHTTPResolver_NoNetrcNoAuth(t *testing.T) {
	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		if r.URL.Path == "/ctx/sew.yaml" {
			w.Write([]byte("components:\n  - name: app\n    helm:\n      chart: repo/app\n"))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("NETRC", filepath.Join(t.TempDir(), "does-not-exist"))

	sewHome := t.TempDir()
	resolver := NewResolver(srv.URL, sewHome, nil)

	_, err := resolver.Resolve(context.Background(), "ctx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if authHeader != "" {
		t.Fatalf("expected no Authorization header, got %q", authHeader)
	}
}

func TestHTTPResolver_FlagsApply(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "ctx", "sew.yaml"), `
components:
  - name: app
    helm:
      chart: repo/app
      values:
        portal:
          enabled: true
`)
	writeFile(t, filepath.Join(root, "ctx", "sew.flags.yaml"), `
flags:
  - name: disable-portal
    description: "Disable portal"
`)
	writeFile(t, filepath.Join(root, "ctx", "sew--disable-portal.yaml"), `
description: "Disable portal"
components:
  - name: app
    helm:
      values:
        portal:
          enabled: false
`)

	srv := newTestServer(t, root)
	resolver := newHTTPResolver(t, srv.URL)

	resolved, err := resolver.Resolve(context.Background(), "ctx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = ApplyFlags(resolved, []string{"disable-portal"}, nil)
	if err != nil {
		t.Fatalf("unexpected error applying flags: %v", err)
	}

	portal, ok := resolved.Components[0].Helm.Values["portal"].(map[string]interface{})
	if !ok {
		t.Fatal("expected portal values map")
	}
	if portal["enabled"] != false {
		t.Fatalf("expected portal.enabled=false, got %v", portal["enabled"])
	}
}

func TestHTTPResolver_BroadcastSetDoesNotLeakIntoComposedParent(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "datastore", "sew.yaml"), `
vars:
  imageTag:
    default: "7"
  imageRepository:
    default: "mongo"
images:
  preload:
    refs:
      - "{{ .imageRepository }}:{{ .imageTag }}"
components:
  - name: datastore
    type: k8s
    k8s:
      manifests:
        - apiVersion: apps/v1
          kind: Deployment
          metadata:
            name: datastore
`)

	writeFile(t, filepath.Join(root, "product", "sew.yaml"), `
from:
  - datastore
vars:
  imageTag:
    default: "latest"
  datastore:
    imageTag:
      default: "7"
    imageRepository:
      default: "mongo"
components:
  - name: product
    type: k8s
    k8s:
      manifests:
        - apiVersion: apps/v1
          kind: Deployment
          metadata:
            name: product
`)

	srv := newTestServer(t, root)
	sewHome := t.TempDir()
	resolver := &HTTPResolver{
		BaseURL:      srv.URL,
		CacheRoot:    filepath.Join(sewHome, "cache"),
		SewHome:      sewHome,
		SetOverrides: map[string]string{"imageTag": "custom-tag"},
	}

	resolved, err := resolver.Resolve(context.Background(), "product")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	refs := resolved.Images.Preload.Refs
	if len(refs) == 0 {
		t.Fatal("expected at least one preload ref")
	}

	for _, ref := range refs {
		if strings.Contains(ref, "custom-tag") {
			t.Fatalf("broadcast --set imageTag leaked into datastore preload: got %q, "+
				"expected path-scoped override to win with tag '7'", ref)
		}
	}
	if refs[0] != "mongo:7" {
		t.Fatalf("expected preload ref %q, got %q", "mongo:7", refs[0])
	}
}
