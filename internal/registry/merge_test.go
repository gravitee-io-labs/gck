package registry

import (
	"path/filepath"
	"testing"

	"github.com/gravitee-io-labs/gck/internal/config"
)

func TestMergeComponents_MatchingHelmComponent(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{
				Name: "app",
				Helm: &config.HelmSpec{
					Chart:   "repo/app",
					Version: "1.0",
					Values: map[string]interface{}{
						"key1": "base-val",
					},
				},
			},
		},
	}
	overrides := []config.Component{
		{
			Name: "app",
			Helm: &config.HelmSpec{
				Version: "2.0",
				Values: map[string]interface{}{
					"key1": "override-val",
					"key2": "new-val",
				},
			},
		},
	}

	MergeComponents(resolved, overrides, "")

	if len(resolved.Components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(resolved.Components))
	}
	comp := resolved.Components[0]
	if comp.Helm.Chart != "repo/app" {
		t.Fatalf("expected chart preserved, got %q", comp.Helm.Chart)
	}
	if comp.Helm.Version != "2.0" {
		t.Fatalf("expected version overridden, got %q", comp.Helm.Version)
	}
	if comp.Helm.Values["key1"] != "override-val" {
		t.Fatal("expected key1 overridden")
	}
	if comp.Helm.Values["key2"] != "new-val" {
		t.Fatal("expected key2 added")
	}
}

func TestMergeComponents_NewComponent(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{Name: "existing", Helm: &config.HelmSpec{Chart: "existing/chart"}},
		},
	}
	overrides := []config.Component{
		{Name: "added", Helm: &config.HelmSpec{Chart: "added/chart"}},
	}

	MergeComponents(resolved, overrides, "")

	if len(resolved.Components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(resolved.Components))
	}
	names := make(map[string]bool)
	for _, c := range resolved.Components {
		names[c.Name] = true
	}
	if !names["existing"] || !names["added"] {
		t.Fatalf("expected both existing and added, got %v", names)
	}
}

func TestMergeComponents_EmptyOverrides(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{Name: "app", Helm: &config.HelmSpec{Chart: "app/chart"}},
		},
	}

	MergeComponents(resolved, nil, "")

	if len(resolved.Components) != 1 {
		t.Fatalf("expected 1 component unchanged, got %d", len(resolved.Components))
	}
}

func TestMergeComponents_RequirementsDedup(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{
				Name: "app",
				Helm: &config.HelmSpec{Chart: "app/chart"},
				Requires: []config.Requirement{
					{Component: "db"},
				},
			},
		},
	}
	overrides := []config.Component{
		{
			Name: "app",
			Helm: &config.HelmSpec{},
			Requires: []config.Requirement{
				{Component: "db"},
				{Component: "cache"},
			},
		},
	}

	MergeComponents(resolved, overrides, "")

	comp := resolved.Components[0]
	if len(comp.Requires) != 2 {
		t.Fatalf("expected 2 requirements (deduped), got %d", len(comp.Requires))
	}
	reqNames := make(map[string]bool)
	for _, r := range comp.Requires {
		reqNames[r.Component] = true
	}
	if !reqNames["db"] || !reqNames["cache"] {
		t.Fatalf("expected db and cache, got %v", reqNames)
	}
}

func TestMergeComponents_K8sManifestsMerge(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{
				Name: "routes",
				K8s: &config.K8sSpec{
					ManifestFiles: []string{"/abs/path/base.yaml"},
				},
			},
		},
	}
	overrides := []config.Component{
		{
			Name: "routes",
			K8s: &config.K8sSpec{
				ManifestFiles: []string{"extra.yaml"},
			},
		},
	}

	MergeComponents(resolved, overrides, "/config/dir")

	comp := resolved.Components[0]
	if len(comp.K8s.ManifestFiles) != 2 {
		t.Fatalf("expected 2 manifest files, got %d", len(comp.K8s.ManifestFiles))
	}
	if comp.K8s.ManifestFiles[0] != "/abs/path/base.yaml" {
		t.Fatalf("expected base manifest preserved, got %q", comp.K8s.ManifestFiles[0])
	}
	expected := filepath.Join("/config/dir", "extra.yaml")
	if comp.K8s.ManifestFiles[1] != expected {
		t.Fatalf("expected extra manifest resolved to %q, got %q", expected, comp.K8s.ManifestFiles[1])
	}
}

func TestMergeComponents_InlineManifestsOverrideByIdentity(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{
				Name: "kafka",
				K8s: &config.K8sSpec{
					Manifests: []map[string]interface{}{
						{
							"apiVersion": "v1",
							"kind":       "Service",
							"metadata": map[string]interface{}{
								"name": "kafka",
							},
							"spec": map[string]interface{}{
								"type": "NodePort",
							},
						},
						{
							"apiVersion": "apps/v1",
							"kind":       "Deployment",
							"metadata": map[string]interface{}{
								"name": "kafka",
							},
						},
					},
				},
			},
		},
	}
	overrides := []config.Component{
		{
			Name: "kafka",
			K8s: &config.K8sSpec{
				Manifests: []map[string]interface{}{
					{
						"apiVersion": "v1",
						"kind":       "Service",
						"metadata": map[string]interface{}{
							"name": "kafka",
						},
						"spec": map[string]interface{}{
							"type": "ClusterIP",
						},
					},
				},
			},
		},
	}

	MergeComponents(resolved, overrides, "")

	comp := resolved.Components[0]
	if len(comp.K8s.Manifests) != 2 {
		t.Fatalf("expected 2 manifests (union), got %d", len(comp.K8s.Manifests))
	}
	svc := comp.K8s.Manifests[0]
	spec, _ := svc["spec"].(map[string]interface{})
	if spec["type"] != "ClusterIP" {
		t.Fatalf("expected Service overridden to ClusterIP, got %v", spec["type"])
	}
	if comp.K8s.Manifests[1]["kind"] != "Deployment" {
		t.Fatalf("expected Deployment preserved, got %v", comp.K8s.Manifests[1]["kind"])
	}
}

func TestMergeComponents_InlineManifestsAppendNew(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{
				Name: "app",
				K8s: &config.K8sSpec{
					Manifests: []map[string]interface{}{
						{
							"apiVersion": "v1",
							"kind":       "Service",
							"metadata": map[string]interface{}{
								"name": "app",
							},
						},
					},
				},
			},
		},
	}
	overrides := []config.Component{
		{
			Name: "app",
			K8s: &config.K8sSpec{
				Manifests: []map[string]interface{}{
					{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name": "app-config",
						},
					},
				},
			},
		},
	}

	MergeComponents(resolved, overrides, "")

	comp := resolved.Components[0]
	if len(comp.K8s.Manifests) != 2 {
		t.Fatalf("expected 2 manifests, got %d", len(comp.K8s.Manifests))
	}
	if comp.K8s.Manifests[0]["kind"] != "Service" {
		t.Fatalf("expected Service first, got %v", comp.K8s.Manifests[0]["kind"])
	}
	if comp.K8s.Manifests[1]["kind"] != "ConfigMap" {
		t.Fatalf("expected ConfigMap appended, got %v", comp.K8s.Manifests[1]["kind"])
	}
}

func TestMergeComponents_InlineManifestsNamespaceDistinct(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{
				Name: "infra",
				K8s: &config.K8sSpec{
					Manifests: []map[string]interface{}{
						{
							"apiVersion": "v1",
							"kind":       "Service",
							"metadata": map[string]interface{}{
								"name":      "redis",
								"namespace": "cache",
							},
						},
					},
				},
			},
		},
	}
	overrides := []config.Component{
		{
			Name: "infra",
			K8s: &config.K8sSpec{
				Manifests: []map[string]interface{}{
					{
						"apiVersion": "v1",
						"kind":       "Service",
						"metadata": map[string]interface{}{
							"name":      "redis",
							"namespace": "session",
						},
					},
				},
			},
		},
	}

	MergeComponents(resolved, overrides, "")

	comp := resolved.Components[0]
	if len(comp.K8s.Manifests) != 2 {
		t.Fatalf("expected 2 manifests (different namespaces = different identity), got %d", len(comp.K8s.Manifests))
	}
}

func TestMergeComponents_ValueFilePathResolution(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{
				Name: "app",
				Helm: &config.HelmSpec{
					Chart:      "app/chart",
					ValueFiles: []string{"/abs/values.yaml"},
				},
			},
		},
	}
	overrides := []config.Component{
		{
			Name: "app",
			Helm: &config.HelmSpec{
				ValueFiles: []string{"local-values.yaml"},
			},
		},
	}

	MergeComponents(resolved, overrides, "/user/config")

	comp := resolved.Components[0]
	if len(comp.Helm.ValueFiles) != 2 {
		t.Fatalf("expected 2 value files, got %d", len(comp.Helm.ValueFiles))
	}
	if comp.Helm.ValueFiles[0] != "/abs/values.yaml" {
		t.Fatalf("expected base value file preserved, got %q", comp.Helm.ValueFiles[0])
	}
	expected := filepath.Join("/user/config", "local-values.yaml")
	if comp.Helm.ValueFiles[1] != expected {
		t.Fatalf("expected local value file resolved to %q, got %q", expected, comp.Helm.ValueFiles[1])
	}
}

func TestMergeComponents_ChartOverride(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{
				Name: "app",
				Helm: &config.HelmSpec{
					Chart:   "original/chart",
					Version: "1.0",
				},
			},
		},
	}
	overrides := []config.Component{
		{
			Name: "app",
			Helm: &config.HelmSpec{
				Chart: "custom/chart",
			},
		},
	}

	MergeComponents(resolved, overrides, "")

	comp := resolved.Components[0]
	if comp.Helm.Chart != "custom/chart" {
		t.Fatalf("expected chart overridden, got %q", comp.Helm.Chart)
	}
	if comp.Helm.Version != "1.0" {
		t.Fatalf("expected version preserved, got %q", comp.Helm.Version)
	}
}

func TestMergeComponents_K8sOnNewComponent(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{Name: "app", Helm: &config.HelmSpec{Chart: "app/chart"}},
		},
	}
	overrides := []config.Component{
		{
			Name: "routes",
			K8s: &config.K8sSpec{
				ManifestFiles: []string{"gateway.yaml"},
			},
		},
	}

	MergeComponents(resolved, overrides, "/config")

	if len(resolved.Components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(resolved.Components))
	}
	routes := resolved.Components[1]
	if routes.Name != "routes" {
		t.Fatalf("expected routes component, got %q", routes.Name)
	}
	expected := filepath.Join("/config", "gateway.yaml")
	if routes.K8s.ManifestFiles[0] != expected {
		t.Fatalf("expected manifest path resolved to %q, got %q", expected, routes.K8s.ManifestFiles[0])
	}
}

func TestMergeComponents_ConditionsSelectorTimeout(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{
				Name: "apim",
				Helm: &config.HelmSpec{Chart: "apim/chart"},
			},
		},
	}
	overrides := []config.Component{
		{
			Name:       "apim",
			Conditions: config.Conditions{Ready: true},
			Selector: &config.Selector{
				MatchLabels: map[string]string{"app": "apim"},
			},
			Timeout: "10m",
			Helm:    &config.HelmSpec{},
		},
	}

	MergeComponents(resolved, overrides, "")

	comp := resolved.Components[0]
	if !comp.Conditions.Ready {
		t.Fatal("expected conditions.ready to be true")
	}
	if comp.Selector == nil || comp.Selector.MatchLabels["app"] != "apim" {
		t.Fatalf("expected selector overridden, got %v", comp.Selector)
	}
	if comp.Timeout != "10m" {
		t.Fatalf("expected timeout overridden, got %q", comp.Timeout)
	}
}

func TestMergeComponents_NamespaceOverride(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{
				Name: "mongodb",
				K8s:  &config.K8sSpec{},
			},
		},
	}
	overrides := []config.Component{
		{
			Name:      "mongodb",
			Namespace: "gravitee",
		},
	}

	MergeComponents(resolved, overrides, "")

	comp := resolved.Components[0]
	if comp.Namespace != "gravitee" {
		t.Fatalf("expected namespace overridden to %q, got %q", "gravitee", comp.Namespace)
	}
}

// TestMergeComponents_NamespaceSurvivesAppendReallocation reproduces the bug
// where appending a new component before merging an existing one causes the
// byName pointer to become stale after slice reallocation. The namespace
// override is then written to the old backing array instead of the live slice.
func TestMergeComponents_NamespaceSurvivesAppendReallocation(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{Name: "kafka", Type: "k8s", K8s: &config.K8sSpec{}},
		},
	}
	overrides := []config.Component{
		{Name: "license", Type: "k8s"},
		{Name: "kafka", Namespace: "gravitee"},
	}

	MergeComponents(resolved, overrides, "")

	var kafka *config.Component
	for i := range resolved.Components {
		if resolved.Components[i].Name == "kafka" {
			kafka = &resolved.Components[i]
			break
		}
	}
	if kafka == nil {
		t.Fatal("kafka component not found")
	}
	if kafka.Namespace != "gravitee" {
		t.Fatalf("expected namespace %q, got %q (stale pointer after reallocation?)", "gravitee", kafka.Namespace)
	}
}

func TestMergeComponents_NamespacePreservedWhenUnset(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{
				Name:      "mongodb",
				Namespace: "infra",
				K8s:       &config.K8sSpec{},
			},
		},
	}
	overrides := []config.Component{
		{
			Name: "mongodb",
		},
	}

	MergeComponents(resolved, overrides, "")

	comp := resolved.Components[0]
	if comp.Namespace != "infra" {
		t.Fatalf("expected namespace preserved as %q, got %q", "infra", comp.Namespace)
	}
}

func TestMergeComponents_ConditionsNotOverriddenWhenUnset(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{
				Name:       "apim",
				Conditions: config.Conditions{Ready: true},
				Selector: &config.Selector{
					MatchLabels: map[string]string{"app": "apim"},
				},
				Timeout: "5m",
				Helm:    &config.HelmSpec{Chart: "apim/chart"},
			},
		},
	}
	overrides := []config.Component{
		{
			Name: "apim",
			Helm: &config.HelmSpec{},
		},
	}

	MergeComponents(resolved, overrides, "")

	comp := resolved.Components[0]
	if !comp.Conditions.Ready {
		t.Fatal("expected conditions.ready preserved")
	}
	if comp.Selector == nil || comp.Selector.MatchLabels["app"] != "apim" {
		t.Fatal("expected selector preserved")
	}
	if comp.Timeout != "5m" {
		t.Fatalf("expected timeout preserved, got %q", comp.Timeout)
	}
}

func TestMergeComponents_SecretsMerge(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{
				Name: "app",
				K8s: &config.K8sSpec{
					Secrets: []config.LocalResource{
						{Name: "base-secret", FromFile: "/abs/secret.txt"},
					},
				},
			},
		},
	}
	overrides := []config.Component{
		{
			Name: "app",
			K8s: &config.K8sSpec{
				Secrets: []config.LocalResource{
					{Name: "extra-secret", FromFile: "local-secret.txt"},
				},
			},
		},
	}

	MergeComponents(resolved, overrides, "/config/dir")

	comp := resolved.Components[0]
	if len(comp.K8s.Secrets) != 2 {
		t.Fatalf("expected 2 secrets, got %d", len(comp.K8s.Secrets))
	}
	if comp.K8s.Secrets[0].FromFile != "/abs/secret.txt" {
		t.Fatalf("expected base secret preserved, got %q", comp.K8s.Secrets[0].FromFile)
	}
	expected := filepath.Join("/config/dir", "local-secret.txt")
	if comp.K8s.Secrets[1].FromFile != expected {
		t.Fatalf("expected extra secret resolved to %q, got %q", expected, comp.K8s.Secrets[1].FromFile)
	}
}

func TestMergeComponents_ConfigMapsMerge(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{
				Name: "app",
				K8s: &config.K8sSpec{
					ConfigMaps: []config.LocalResource{
						{Name: "base-config", FromFile: "/abs/config.xml"},
					},
				},
			},
		},
	}
	overrides := []config.Component{
		{
			Name: "app",
			K8s: &config.K8sSpec{
				ConfigMaps: []config.LocalResource{
					{Name: "extra-config", FromFile: "logback.xml"},
				},
			},
		},
	}

	MergeComponents(resolved, overrides, "/config/dir")

	comp := resolved.Components[0]
	if len(comp.K8s.ConfigMaps) != 2 {
		t.Fatalf("expected 2 config maps, got %d", len(comp.K8s.ConfigMaps))
	}
	if comp.K8s.ConfigMaps[0].FromFile != "/abs/config.xml" {
		t.Fatalf("expected base config map preserved, got %q", comp.K8s.ConfigMaps[0].FromFile)
	}
	expected := filepath.Join("/config/dir", "logback.xml")
	if comp.K8s.ConfigMaps[1].FromFile != expected {
		t.Fatalf("expected extra config map resolved to %q, got %q", expected, comp.K8s.ConfigMaps[1].FromFile)
	}
}

func TestMergeComponents_LocalResourceEntryPaths(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{Name: "app", K8s: &config.K8sSpec{}},
		},
	}
	overrides := []config.Component{
		{
			Name: "app",
			K8s: &config.K8sSpec{
				Secrets: []config.LocalResource{
					{
						Name: "creds",
						Entries: []config.ResourceEntry{
							{Key: "token", FromFile: "token.txt"},
							{Key: "cert", FromFile: "/abs/cert.pem"},
							{Key: "API_KEY", FromEnv: "MY_API_KEY"},
						},
					},
				},
			},
		},
	}

	MergeComponents(resolved, overrides, "/config")

	entries := resolved.Components[0].K8s.Secrets[0].Entries
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	expected := filepath.Join("/config", "token.txt")
	if entries[0].FromFile != expected {
		t.Fatalf("expected relative entry resolved to %q, got %q", expected, entries[0].FromFile)
	}
	if entries[1].FromFile != "/abs/cert.pem" {
		t.Fatalf("expected absolute entry preserved, got %q", entries[1].FromFile)
	}
	if entries[2].FromEnv != "MY_API_KEY" {
		t.Fatalf("expected env entry unchanged, got %q", entries[2].FromEnv)
	}
}

func TestMergeComponents_NewComponentWithSecrets(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{Name: "existing", Helm: &config.HelmSpec{Chart: "existing/chart"}},
		},
	}
	overrides := []config.Component{
		{
			Name: "license",
			K8s: &config.K8sSpec{
				Secrets: []config.LocalResource{
					{Name: "gravitee-license", FromFile: "./license.key"},
				},
				ConfigMaps: []config.LocalResource{
					{
						Name: "logging",
						Entries: []config.ResourceEntry{
							{Key: "logback.xml", FromFile: "logback.xml"},
						},
					},
				},
			},
		},
	}

	MergeComponents(resolved, overrides, "/user/config")

	if len(resolved.Components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(resolved.Components))
	}
	comp := resolved.Components[1]
	expected := filepath.Join("/user/config", "license.key")
	if comp.K8s.Secrets[0].FromFile != expected {
		t.Fatalf("expected secret path resolved to %q, got %q", expected, comp.K8s.Secrets[0].FromFile)
	}
	expectedEntry := filepath.Join("/user/config", "logback.xml")
	if comp.K8s.ConfigMaps[0].Entries[0].FromFile != expectedEntry {
		t.Fatalf("expected config map entry path resolved to %q, got %q", expectedEntry, comp.K8s.ConfigMaps[0].Entries[0].FromFile)
	}
}

func TestMergeComponents_SecretsInitK8sWhenNil(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{Name: "app", Helm: &config.HelmSpec{Chart: "app/chart"}},
		},
	}
	overrides := []config.Component{
		{
			Name: "app",
			K8s: &config.K8sSpec{
				Secrets: []config.LocalResource{
					{Name: "my-secret", FromFile: "secret.txt"},
				},
			},
		},
	}

	MergeComponents(resolved, overrides, "/cfg")

	comp := resolved.Components[0]
	if comp.K8s == nil {
		t.Fatal("expected K8s to be initialized")
	}
	if len(comp.K8s.Secrets) != 1 {
		t.Fatalf("expected 1 secret, got %d", len(comp.K8s.Secrets))
	}
	expected := filepath.Join("/cfg", "secret.txt")
	if comp.K8s.Secrets[0].FromFile != expected {
		t.Fatalf("expected secret path resolved to %q, got %q", expected, comp.K8s.Secrets[0].FromFile)
	}
}

func TestMergeComponents_EmptyConfigDirSkipsResolution(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{
				Name: "app",
				K8s: &config.K8sSpec{
					Secrets: []config.LocalResource{
						{Name: "s", FromFile: "relative.txt"},
					},
				},
			},
		},
	}
	overrides := []config.Component{
		{
			Name: "app",
			K8s: &config.K8sSpec{
				ConfigMaps: []config.LocalResource{
					{Name: "c", FromFile: "also-relative.txt"},
				},
			},
		},
	}

	MergeComponents(resolved, overrides, "")

	if resolved.Components[0].K8s.ConfigMaps[0].FromFile != "also-relative.txt" {
		t.Fatal("expected path left relative when configDir is empty")
	}
}

func TestMergeComponents_AbsolutePathNotPrepended(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{Name: "app", K8s: &config.K8sSpec{}},
		},
	}
	overrides := []config.Component{
		{
			Name: "app",
			K8s: &config.K8sSpec{
				Secrets: []config.LocalResource{
					{Name: "license", FromFile: "/home/testuser/opt/license.key"},
				},
				ConfigMaps: []config.LocalResource{
					{
						Name: "creds",
						Entries: []config.ResourceEntry{
							{Key: "token", FromFile: "/home/testuser/tokens/token.txt"},
						},
					},
				},
			},
		},
	}

	MergeComponents(resolved, overrides, "/should/not/be/prepended")

	secret := resolved.Components[0].K8s.Secrets[0]
	if secret.FromFile != "/home/testuser/opt/license.key" {
		t.Fatalf("expected absolute path preserved, got %q", secret.FromFile)
	}
	entry := resolved.Components[0].K8s.ConfigMaps[0].Entries[0]
	if entry.FromFile != "/home/testuser/tokens/token.txt" {
		t.Fatalf("expected absolute path preserved in entry, got %q", entry.FromFile)
	}
}

func TestMergeComponents_PatchOverUserOverride(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{
				Name:      "apim",
				Namespace: "gravitee",
				Helm: &config.HelmSpec{
					Chart:   "graviteeio/apim",
					Version: "4.10.0",
					Values: map[string]interface{}{
						"gateway": map[string]interface{}{
							"image": map[string]interface{}{"tag": "latest"},
						},
					},
				},
			},
			{
				Name:      "mongodb",
				Namespace: "gravitee",
				Helm: &config.HelmSpec{
					Chart:   "bitnami/mongodb",
					Version: "7.0.0",
				},
			},
		},
	}

	userOverrides := []config.Component{
		{
			Name: "apim",
			Helm: &config.HelmSpec{
				Values: map[string]interface{}{
					"api": map[string]interface{}{"replicas": 2},
				},
			},
		},
	}
	MergeComponents(resolved, userOverrides, "")

	patch := []config.Component{
		{
			Name: "apim",
			Helm: &config.HelmSpec{
				Values: map[string]interface{}{
					"gateway": map[string]interface{}{
						"image": map[string]interface{}{"tag": "4.11.0"},
					},
				},
			},
		},
	}
	MergeComponents(resolved, patch, "")

	if len(resolved.Components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(resolved.Components))
	}
	comp := resolved.Components[0]
	if comp.Helm.Chart != "graviteeio/apim" {
		t.Fatalf("expected chart preserved, got %q", comp.Helm.Chart)
	}
	if comp.Helm.Version != "4.10.0" {
		t.Fatalf("expected version preserved from user override, got %q", comp.Helm.Version)
	}
	gw, ok := comp.Helm.Values["gateway"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected gateway values, got %T", comp.Helm.Values["gateway"])
	}
	img, ok := gw["image"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected gateway.image values, got %T", gw["image"])
	}
	if img["tag"] != "4.11.0" {
		t.Fatalf("expected patch tag to win, got %v", img["tag"])
	}
	if comp.Helm.Values["api"] == nil {
		t.Fatal("expected user override 'api' key preserved after patch")
	}
}

func TestMergeComponents_DeepMergeValues(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{
				Name: "apim",
				Helm: &config.HelmSpec{
					Chart: "graviteeio/apim",
					Values: map[string]interface{}{
						"gateway": map[string]interface{}{
							"image": map[string]interface{}{"tag": "4.10.0"},
							"env": []interface{}{
								map[string]interface{}{
									"name":  "gravitee_ratelimit_type",
									"value": "jdbc",
								},
							},
						},
						"jdbc": map[string]interface{}{
							"url": "jdbc:postgresql://postgresql:5432/gravitee",
						},
					},
				},
			},
		},
	}
	patch := []config.Component{
		{
			Name: "apim",
			Helm: &config.HelmSpec{
				Values: map[string]interface{}{
					"gateway": map[string]interface{}{
						"image": map[string]interface{}{
							"repository": "graviteeio/apim-gateway",
							"tag":        "4.11.0-alpha",
						},
					},
				},
			},
		},
	}

	MergeComponents(resolved, patch, "")

	comp := resolved.Components[0]
	gw, ok := comp.Helm.Values["gateway"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected gateway values map, got %T", comp.Helm.Values["gateway"])
	}
	img, ok := gw["image"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected gateway.image map, got %T", gw["image"])
	}
	if img["tag"] != "4.11.0-alpha" {
		t.Fatalf("expected tag overridden to 4.11.0-alpha, got %v", img["tag"])
	}
	if img["repository"] != "graviteeio/apim-gateway" {
		t.Fatalf("expected repository added, got %v", img["repository"])
	}
	if gw["env"] == nil {
		t.Fatal("expected gateway.env preserved after deep merge")
	}
	if comp.Helm.Values["jdbc"] == nil {
		t.Fatal("expected jdbc top-level key preserved")
	}
}

func TestMergeComponents_PatchOnlyAffectsNamedComponents(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{
				Name: "apim",
				Helm: &config.HelmSpec{
					Chart:   "graviteeio/apim",
					Version: "4.10.0",
					Values:  map[string]interface{}{"key": "original"},
				},
			},
			{
				Name: "mongodb",
				Helm: &config.HelmSpec{
					Chart:   "bitnami/mongodb",
					Version: "7.0.0",
					Values:  map[string]interface{}{"key": "original"},
				},
			},
		},
	}

	patch := []config.Component{
		{
			Name: "apim",
			Helm: &config.HelmSpec{
				Values: map[string]interface{}{"key": "patched"},
			},
		},
	}
	MergeComponents(resolved, patch, "")

	apim := resolved.Components[0]
	if apim.Helm.Values["key"] != "patched" {
		t.Fatalf("expected apim patched, got %v", apim.Helm.Values["key"])
	}

	mongo := resolved.Components[1]
	if mongo.Helm.Values["key"] != "original" {
		t.Fatalf("expected mongodb untouched, got %v", mongo.Helm.Values["key"])
	}
}

func TestMergeComponents_PatchWithValueFiles(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{
				Name: "apim",
				Helm: &config.HelmSpec{
					Chart:      "graviteeio/apim",
					ValueFiles: []string{"/registry/base-values.yaml"},
				},
			},
		},
	}

	MergeComponents(resolved, []config.Component{
		{
			Name: "apim",
			Helm: &config.HelmSpec{
				ValueFiles: []string{"user-values.yaml"},
			},
		},
	}, "/user")

	MergeComponents(resolved, []config.Component{
		{
			Name: "apim",
			Helm: &config.HelmSpec{
				ValueFiles: []string{"patch-values.yaml"},
			},
		},
	}, "/patch")

	comp := resolved.Components[0]
	if len(comp.Helm.ValueFiles) != 3 {
		t.Fatalf("expected 3 value files, got %d: %v", len(comp.Helm.ValueFiles), comp.Helm.ValueFiles)
	}
	if comp.Helm.ValueFiles[0] != "/registry/base-values.yaml" {
		t.Fatalf("expected base value file first, got %q", comp.Helm.ValueFiles[0])
	}
	expectedUser := filepath.Join("/user", "user-values.yaml")
	if comp.Helm.ValueFiles[1] != expectedUser {
		t.Fatalf("expected user value file second, got %q", comp.Helm.ValueFiles[1])
	}
	expectedPatch := filepath.Join("/patch", "patch-values.yaml")
	if comp.Helm.ValueFiles[2] != expectedPatch {
		t.Fatalf("expected patch value file third, got %q", comp.Helm.ValueFiles[2])
	}
}

func TestManifestKeyOf_Complete(t *testing.T) {
	m := map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":      "my-app",
			"namespace": "production",
		},
	}
	key := manifestKeyOf(m)
	if key.apiVersion != "apps/v1" {
		t.Fatalf("expected apiVersion apps/v1, got %q", key.apiVersion)
	}
	if key.kind != "Deployment" {
		t.Fatalf("expected kind Deployment, got %q", key.kind)
	}
	if key.name != "my-app" {
		t.Fatalf("expected name my-app, got %q", key.name)
	}
	if key.namespace != "production" {
		t.Fatalf("expected namespace production, got %q", key.namespace)
	}
}

func TestManifestKeyOf_NoMetadata(t *testing.T) {
	m := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
	}
	key := manifestKeyOf(m)
	if key.apiVersion != "v1" || key.kind != "ConfigMap" {
		t.Fatalf("expected v1/ConfigMap, got %q/%q", key.apiVersion, key.kind)
	}
	if key.name != "" || key.namespace != "" {
		t.Fatalf("expected empty name/namespace, got %q/%q", key.name, key.namespace)
	}
}

func TestManifestKeyOf_NoNamespace(t *testing.T) {
	m := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata": map[string]interface{}{
			"name": "kafka",
		},
	}
	key := manifestKeyOf(m)
	if key.name != "kafka" {
		t.Fatalf("expected name kafka, got %q", key.name)
	}
	if key.namespace != "" {
		t.Fatalf("expected empty namespace, got %q", key.namespace)
	}
}

func TestManifestKeyOf_Empty(t *testing.T) {
	key := manifestKeyOf(map[string]interface{}{})
	if key != (manifestKey{}) {
		t.Fatalf("expected zero manifestKey, got %+v", key)
	}
}

func TestManifestKeyOf_NonStringValues(t *testing.T) {
	m := map[string]interface{}{
		"apiVersion": 42,
		"kind":       true,
		"metadata": map[string]interface{}{
			"name": 3.14,
		},
	}
	key := manifestKeyOf(m)
	if key.apiVersion != "" || key.kind != "" || key.name != "" {
		t.Fatalf("expected all empty strings for non-string values, got %+v", key)
	}
}

func TestManifestKeyOf_MetadataWrongType(t *testing.T) {
	m := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   "not-a-map",
	}
	key := manifestKeyOf(m)
	if key.name != "" || key.namespace != "" {
		t.Fatalf("expected empty name/namespace for non-map metadata, got %+v", key)
	}
}

func TestMergeManifests_BothEmpty(t *testing.T) {
	result := mergeManifests(nil, nil)
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestMergeManifests_EmptyBase(t *testing.T) {
	override := []map[string]interface{}{
		{"apiVersion": "v1", "kind": "Service", "metadata": map[string]interface{}{"name": "svc"}},
	}
	result := mergeManifests(nil, override)
	if len(result) != 1 {
		t.Fatalf("expected 1 manifest, got %d", len(result))
	}
	if result[0]["kind"] != "Service" {
		t.Fatalf("expected Service, got %v", result[0]["kind"])
	}
}

func TestMergeManifests_EmptyOverride(t *testing.T) {
	base := []map[string]interface{}{
		{"apiVersion": "v1", "kind": "Service", "metadata": map[string]interface{}{"name": "svc"}},
	}
	result := mergeManifests(base, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 manifest, got %d", len(result))
	}
}

func TestMergeManifests_NoOverlap(t *testing.T) {
	base := []map[string]interface{}{
		{"apiVersion": "v1", "kind": "Service", "metadata": map[string]interface{}{"name": "svc-a"}},
	}
	override := []map[string]interface{}{
		{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{"name": "deploy-b"}},
	}
	result := mergeManifests(base, override)
	if len(result) != 2 {
		t.Fatalf("expected 2 manifests, got %d", len(result))
	}
	if result[0]["kind"] != "Service" {
		t.Fatalf("expected Service first, got %v", result[0]["kind"])
	}
	if result[1]["kind"] != "Deployment" {
		t.Fatalf("expected Deployment second, got %v", result[1]["kind"])
	}
}

func TestMergeManifests_FullOverlap(t *testing.T) {
	base := []map[string]interface{}{
		{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]interface{}{"name": "kafka"},
			"spec":     map[string]interface{}{"type": "NodePort"},
		},
		{
			"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": map[string]interface{}{"name": "kafka"},
			"spec":     map[string]interface{}{"replicas": 1},
		},
	}
	override := []map[string]interface{}{
		{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]interface{}{"name": "kafka"},
			"spec":     map[string]interface{}{"type": "ClusterIP"},
		},
		{
			"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": map[string]interface{}{"name": "kafka"},
			"spec":     map[string]interface{}{"replicas": 3},
		},
	}
	result := mergeManifests(base, override)
	if len(result) != 2 {
		t.Fatalf("expected 2 manifests (all replaced), got %d", len(result))
	}
	svcSpec, _ := result[0]["spec"].(map[string]interface{})
	if svcSpec["type"] != "ClusterIP" {
		t.Fatalf("expected Service overridden to ClusterIP, got %v", svcSpec["type"])
	}
	depSpec, _ := result[1]["spec"].(map[string]interface{})
	if depSpec["replicas"] != 3 {
		t.Fatalf("expected Deployment replicas overridden to 3, got %v", depSpec["replicas"])
	}
}

func TestMergeManifests_PartialOverlap(t *testing.T) {
	base := []map[string]interface{}{
		{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]interface{}{"name": "kafka"},
			"spec":     map[string]interface{}{"type": "NodePort"},
		},
		{
			"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": map[string]interface{}{"name": "kafka"},
		},
	}
	override := []map[string]interface{}{
		{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]interface{}{"name": "kafka"},
			"spec":     map[string]interface{}{"type": "ClusterIP"},
		},
		{
			"apiVersion": "v1", "kind": "ConfigMap",
			"metadata": map[string]interface{}{"name": "kafka-config"},
		},
	}
	result := mergeManifests(base, override)
	if len(result) != 3 {
		t.Fatalf("expected 3 manifests (1 replaced, 1 kept, 1 appended), got %d", len(result))
	}
	svcSpec, _ := result[0]["spec"].(map[string]interface{})
	if svcSpec["type"] != "ClusterIP" {
		t.Fatalf("expected Service replaced with ClusterIP, got %v", svcSpec["type"])
	}
	if result[1]["kind"] != "Deployment" {
		t.Fatalf("expected Deployment preserved at index 1, got %v", result[1]["kind"])
	}
	if result[2]["kind"] != "ConfigMap" {
		t.Fatalf("expected ConfigMap appended at index 2, got %v", result[2]["kind"])
	}
}

func TestMergeManifests_DifferentAPIVersionSameKindName(t *testing.T) {
	base := []map[string]interface{}{
		{
			"apiVersion": "networking.k8s.io/v1", "kind": "Ingress",
			"metadata": map[string]interface{}{"name": "my-ingress"},
		},
	}
	override := []map[string]interface{}{
		{
			"apiVersion": "networking.k8s.io/v1beta1", "kind": "Ingress",
			"metadata": map[string]interface{}{"name": "my-ingress"},
		},
	}
	result := mergeManifests(base, override)
	if len(result) != 2 {
		t.Fatalf("expected 2 manifests (different apiVersion = different identity), got %d", len(result))
	}
}

func TestMergeManifests_NamespaceDistinguishesIdentity(t *testing.T) {
	base := []map[string]interface{}{
		{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]interface{}{"name": "redis", "namespace": "cache"},
		},
	}
	override := []map[string]interface{}{
		{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]interface{}{"name": "redis", "namespace": "session"},
		},
	}
	result := mergeManifests(base, override)
	if len(result) != 2 {
		t.Fatalf("expected 2 manifests (different namespace = different identity), got %d", len(result))
	}
}

func TestMergeManifests_SameNamespaceOverrides(t *testing.T) {
	base := []map[string]interface{}{
		{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]interface{}{"name": "redis", "namespace": "cache"},
			"spec":     map[string]interface{}{"type": "NodePort"},
		},
	}
	override := []map[string]interface{}{
		{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]interface{}{"name": "redis", "namespace": "cache"},
			"spec":     map[string]interface{}{"type": "ClusterIP"},
		},
	}
	result := mergeManifests(base, override)
	if len(result) != 1 {
		t.Fatalf("expected 1 manifest (same identity replaced), got %d", len(result))
	}
	spec, _ := result[0]["spec"].(map[string]interface{})
	if spec["type"] != "ClusterIP" {
		t.Fatalf("expected ClusterIP, got %v", spec["type"])
	}
}

func TestMergeManifests_PreservesBaseOrder(t *testing.T) {
	base := []map[string]interface{}{
		{"apiVersion": "v1", "kind": "Service", "metadata": map[string]interface{}{"name": "alpha"}},
		{"apiVersion": "v1", "kind": "Service", "metadata": map[string]interface{}{"name": "beta"}},
		{"apiVersion": "v1", "kind": "Service", "metadata": map[string]interface{}{"name": "gamma"}},
	}
	override := []map[string]interface{}{
		{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]interface{}{"name": "beta"},
			"spec":     map[string]interface{}{"type": "ClusterIP"},
		},
	}
	result := mergeManifests(base, override)
	if len(result) != 3 {
		t.Fatalf("expected 3 manifests, got %d", len(result))
	}
	meta0, _ := result[0]["metadata"].(map[string]interface{})
	meta1, _ := result[1]["metadata"].(map[string]interface{})
	meta2, _ := result[2]["metadata"].(map[string]interface{})
	if meta0["name"] != "alpha" || meta1["name"] != "beta" || meta2["name"] != "gamma" {
		t.Fatalf("expected order alpha/beta/gamma preserved, got %v/%v/%v", meta0["name"], meta1["name"], meta2["name"])
	}
	spec1, _ := result[1]["spec"].(map[string]interface{})
	if spec1["type"] != "ClusterIP" {
		t.Fatalf("expected beta replaced in-place with ClusterIP, got %v", spec1["type"])
	}
}

func TestMergeManifests_DuplicateInOverride(t *testing.T) {
	base := []map[string]interface{}{
		{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]interface{}{"name": "svc"},
			"spec":     map[string]interface{}{"type": "NodePort"},
		},
	}
	override := []map[string]interface{}{
		{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]interface{}{"name": "svc"},
			"spec":     map[string]interface{}{"type": "ClusterIP"},
		},
		{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]interface{}{"name": "svc"},
			"spec":     map[string]interface{}{"type": "LoadBalancer"},
		},
	}
	result := mergeManifests(base, override)
	if len(result) != 1 {
		t.Fatalf("expected 1 manifest (duplicate overrides collapse), got %d", len(result))
	}
	spec, _ := result[0]["spec"].(map[string]interface{})
	if spec["type"] != "LoadBalancer" {
		t.Fatalf("expected last override wins (LoadBalancer), got %v", spec["type"])
	}
}

func TestMergeManifests_BaseNotMutated(t *testing.T) {
	base := []map[string]interface{}{
		{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]interface{}{"name": "svc"},
			"spec":     map[string]interface{}{"type": "NodePort"},
		},
	}
	override := []map[string]interface{}{
		{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]interface{}{"name": "svc"},
			"spec":     map[string]interface{}{"type": "ClusterIP"},
		},
	}
	mergeManifests(base, override)
	spec, _ := base[0]["spec"].(map[string]interface{})
	if spec["type"] != "NodePort" {
		t.Fatalf("expected base slice elements not mutated, got %v", spec["type"])
	}
}

func TestMergeManifests_PortTakeoverScenario(t *testing.T) {
	kafkaStandalone := []map[string]interface{}{
		{
			"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": map[string]interface{}{"name": "kafka"},
			"spec":     map[string]interface{}{"image": "apache/kafka"},
		},
		{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]interface{}{"name": "kafka"},
			"spec": map[string]interface{}{
				"type": "NodePort",
				"ports": []interface{}{
					map[string]interface{}{"port": 9092, "nodePort": 30092},
				},
			},
		},
	}
	kafkaGatewayOverride := []map[string]interface{}{
		{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]interface{}{"name": "kafka"},
			"spec": map[string]interface{}{
				"type": "ClusterIP",
				"ports": []interface{}{
					map[string]interface{}{"port": 9092},
				},
			},
		},
	}
	result := mergeManifests(kafkaStandalone, kafkaGatewayOverride)
	if len(result) != 2 {
		t.Fatalf("expected 2 manifests, got %d", len(result))
	}
	if result[0]["kind"] != "Deployment" {
		t.Fatalf("expected Deployment first (preserved), got %v", result[0]["kind"])
	}
	svcSpec, _ := result[1]["spec"].(map[string]interface{})
	if svcSpec["type"] != "ClusterIP" {
		t.Fatalf("expected Service overridden to ClusterIP (port takeover), got %v", svcSpec["type"])
	}
}

func TestMergeRepos_NoOverlap(t *testing.T) {
	ctx := []config.Repo{
		{Name: "repo-a", URL: "https://a.example.com"},
	}
	local := []config.Repo{
		{Name: "repo-b", URL: "https://b.example.com"},
	}

	result := MergeRepos(ctx, local)

	if len(result) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(result))
	}
}

func TestMergeRepos_OverlapLocalWins(t *testing.T) {
	ctx := []config.Repo{
		{Name: "shared", URL: "https://old.example.com"},
		{Name: "ctx-only", URL: "https://ctx.example.com"},
	}
	local := []config.Repo{
		{Name: "shared", URL: "https://new.example.com"},
	}

	result := MergeRepos(ctx, local)

	if len(result) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(result))
	}
	repoByName := make(map[string]string)
	for _, r := range result {
		repoByName[r.Name] = r.URL
	}
	if repoByName["shared"] != "https://new.example.com" {
		t.Fatalf("expected local URL to win, got %q", repoByName["shared"])
	}
	if repoByName["ctx-only"] != "https://ctx.example.com" {
		t.Fatalf("expected ctx-only preserved, got %q", repoByName["ctx-only"])
	}
}

func TestMergeRepos_EmptyLocal(t *testing.T) {
	ctx := []config.Repo{
		{Name: "repo", URL: "https://example.com"},
	}

	result := MergeRepos(ctx, nil)

	if len(result) != 1 || result[0].Name != "repo" {
		t.Fatalf("expected context repos preserved, got %v", result)
	}
}

func TestMergeRepos_EmptyContext(t *testing.T) {
	local := []config.Repo{
		{Name: "repo", URL: "https://example.com"},
	}

	result := MergeRepos(nil, local)

	if len(result) != 1 || result[0].Name != "repo" {
		t.Fatalf("expected local repos returned, got %v", result)
	}
}

func TestDeepMergeValues_NamedListsMergedByName(t *testing.T) {
	dst := map[string]interface{}{
		"gateway": map[string]interface{}{
			"env": []interface{}{
				map[string]interface{}{"name": "gravitee_ratelimit_type", "value": "jdbc"},
				map[string]interface{}{"name": "gravitee_ds_url", "value": "jdbc:postgresql://pg:5432/gravitee"},
			},
		},
	}
	src := map[string]interface{}{
		"gateway": map[string]interface{}{
			"env": []interface{}{
				map[string]interface{}{"name": "KAFKA_PORT", "value": "9092"},
				map[string]interface{}{"name": "KAFKA_SERVICE_HOST", "value": "kafka"},
				map[string]interface{}{"name": "gravitee_ratelimit_type", "value": "redis"},
			},
		},
	}

	deepMergeValues(dst, src)

	gw, _ := dst["gateway"].(map[string]interface{})
	env, _ := gw["env"].([]interface{})

	if len(env) != 4 {
		t.Fatalf("expected 4 env entries (2 original + 2 new), got %d", len(env))
	}

	byName := make(map[string]string, len(env))
	for _, item := range env {
		m := item.(map[string]interface{})
		byName[m["name"].(string)] = m["value"].(string)
	}
	if byName["gravitee_ratelimit_type"] != "redis" {
		t.Fatalf("expected same-name entry overridden to 'redis', got %q", byName["gravitee_ratelimit_type"])
	}
	if byName["gravitee_ds_url"] != "jdbc:postgresql://pg:5432/gravitee" {
		t.Fatal("expected dst-only entry preserved")
	}
	if byName["KAFKA_PORT"] != "9092" {
		t.Fatal("expected new entry KAFKA_PORT appended")
	}
	if byName["KAFKA_SERVICE_HOST"] != "kafka" {
		t.Fatal("expected new entry KAFKA_SERVICE_HOST appended")
	}
}

func TestDeepMergeValues_UnnamedListsReplaced(t *testing.T) {
	dst := map[string]interface{}{
		"gateway": map[string]interface{}{
			"servers": []interface{}{
				map[string]interface{}{"host": "0.0.0.0", "port": 8082},
			},
		},
	}
	src := map[string]interface{}{
		"gateway": map[string]interface{}{
			"servers": []interface{}{
				map[string]interface{}{"host": "0.0.0.0", "port": 9092},
				map[string]interface{}{"host": "0.0.0.0", "port": 9093},
			},
		},
	}

	deepMergeValues(dst, src)

	gw, _ := dst["gateway"].(map[string]interface{})
	servers, _ := gw["servers"].([]interface{})
	if len(servers) != 2 {
		t.Fatalf("expected servers replaced (2 items), got %d", len(servers))
	}
	first := servers[0].(map[string]interface{})
	if first["port"] != 9092 {
		t.Fatalf("expected first server port 9092 (from src), got %v", first["port"])
	}
}

func TestDeepMergeValues_StringListsReplaced(t *testing.T) {
	dst := map[string]interface{}{
		"es": map[string]interface{}{
			"endpoints": []interface{}{"http://es1:9200", "http://es2:9200"},
		},
	}
	src := map[string]interface{}{
		"es": map[string]interface{}{
			"endpoints": []interface{}{"http://es-new:9200"},
		},
	}

	deepMergeValues(dst, src)

	es, _ := dst["es"].(map[string]interface{})
	endpoints, _ := es["endpoints"].([]interface{})
	if len(endpoints) != 1 {
		t.Fatalf("expected string list replaced (1 item), got %d", len(endpoints))
	}
	if endpoints[0] != "http://es-new:9200" {
		t.Fatalf("expected src endpoint, got %v", endpoints[0])
	}
}

func TestDeepMergeValues_EmptyNamedListNotMerged(t *testing.T) {
	dst := map[string]interface{}{
		"env": []interface{}{
			map[string]interface{}{"name": "A", "value": "1"},
		},
	}
	src := map[string]interface{}{
		"env": []interface{}{},
	}

	deepMergeValues(dst, src)

	env, _ := dst["env"].([]interface{})
	if len(env) != 0 {
		t.Fatalf("expected empty src list to replace dst (empty list is not a named list), got %d items", len(env))
	}
}

func TestDeepMergeValues_NamedListPreservesOrder(t *testing.T) {
	dst := map[string]interface{}{
		"env": []interface{}{
			map[string]interface{}{"name": "A", "value": "1"},
			map[string]interface{}{"name": "B", "value": "2"},
			map[string]interface{}{"name": "C", "value": "3"},
		},
	}
	src := map[string]interface{}{
		"env": []interface{}{
			map[string]interface{}{"name": "B", "value": "override"},
			map[string]interface{}{"name": "D", "value": "4"},
		},
	}

	deepMergeValues(dst, src)

	env, _ := dst["env"].([]interface{})
	if len(env) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(env))
	}
	names := make([]string, len(env))
	for i, item := range env {
		names[i] = item.(map[string]interface{})["name"].(string)
	}
	expected := []string{"A", "B", "C", "D"}
	for i, name := range names {
		if name != expected[i] {
			t.Fatalf("expected order %v, got %v", expected, names)
		}
	}
	bVal := env[1].(map[string]interface{})["value"]
	if bVal != "override" {
		t.Fatalf("expected B overridden, got %v", bVal)
	}
}

func TestMergeRepos_OrderPreserved(t *testing.T) {
	ctx := []config.Repo{
		{Name: "alpha", URL: "https://alpha.example.com"},
		{Name: "beta", URL: "https://beta.example.com"},
		{Name: "gamma", URL: "https://gamma.example.com"},
	}
	local := []config.Repo{
		{Name: "beta", URL: "https://beta-override.example.com"},
		{Name: "delta", URL: "https://delta.example.com"},
	}

	result := MergeRepos(ctx, local)

	if len(result) != 4 {
		t.Fatalf("expected 4 repos, got %d", len(result))
	}
	if result[0].Name != "alpha" {
		t.Fatalf("expected alpha first, got %q", result[0].Name)
	}
	if result[1].Name != "beta" || result[1].URL != "https://beta-override.example.com" {
		t.Fatalf("expected beta (overridden) second, got %v", result[1])
	}
	if result[2].Name != "gamma" {
		t.Fatalf("expected gamma third, got %q", result[2].Name)
	}
	if result[3].Name != "delta" {
		t.Fatalf("expected delta fourth, got %q", result[3].Name)
	}
}

func TestMergeComponents_EnabledPropagated(t *testing.T) {
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{Name: "elasticsearch", Helm: &config.HelmSpec{Chart: "elastic/elasticsearch"}},
			{Name: "apim", Helm: &config.HelmSpec{Chart: "graviteeio/apim"}},
		},
	}
	disabled := false
	patch := []config.Component{
		{Name: "elasticsearch", Enabled: &disabled},
	}
	MergeComponents(resolved, patch, "")
	if resolved.Components[0].IsEnabled() {
		t.Fatal("expected elasticsearch to be disabled after merge")
	}
	if !resolved.Components[1].IsEnabled() {
		t.Fatal("expected apim to remain enabled")
	}
}

func TestMergeComponents_EnabledNotSetDoesNotOverride(t *testing.T) {
	disabled := false
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{Name: "elasticsearch", Enabled: &disabled, Helm: &config.HelmSpec{Chart: "elastic/elasticsearch"}},
		},
	}
	patch := []config.Component{
		{Name: "elasticsearch", Helm: &config.HelmSpec{Values: map[string]interface{}{"replicas": 3}}},
	}
	MergeComponents(resolved, patch, "")
	if resolved.Components[0].IsEnabled() {
		t.Fatal("expected elasticsearch to stay disabled when patch doesn't set enabled")
	}
}

func TestMergeComponents_EnabledReEnabled(t *testing.T) {
	disabled := false
	resolved := &config.ResolvedContext{
		Components: []config.Component{
			{Name: "elasticsearch", Enabled: &disabled, Helm: &config.HelmSpec{Chart: "elastic/elasticsearch"}},
		},
	}
	enabled := true
	patch := []config.Component{
		{Name: "elasticsearch", Enabled: &enabled},
	}
	MergeComponents(resolved, patch, "")
	if !resolved.Components[0].IsEnabled() {
		t.Fatal("expected elasticsearch to be re-enabled")
	}
}
