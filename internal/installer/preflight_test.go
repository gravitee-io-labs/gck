package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gravitee-io-labs/gck/internal/config"
)

func TestPreflightLocalResources(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "present.key")
	if err := os.WriteFile(present, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing.key")
	off := false

	components := []config.Component{
		{Name: "ok", K8s: &config.K8sSpec{Secrets: []config.LocalResource{{Name: "a", FromFile: present}}}},
		{Name: "optional", K8s: &config.K8sSpec{Secrets: []config.LocalResource{{Name: "b", FromFile: missing, OnMissing: "ignore"}}}},
		{Name: "disabled", Enabled: &off, K8s: &config.K8sSpec{Secrets: []config.LocalResource{{Name: "c", FromFile: missing}}}},
	}
	if err := PreflightLocalResources(components); err != nil {
		t.Fatalf("present, ignorable and disabled resources must pass: %v", err)
	}

	components = append(components,
		config.Component{Name: "needs-file", K8s: &config.K8sSpec{Secrets: []config.LocalResource{{Name: "d", FromFile: missing, OnMissing: "fail"}}}},
		config.Component{Name: "needs-env", K8s: &config.K8sSpec{ConfigMaps: []config.LocalResource{{Name: "e", Entries: []config.ResourceEntry{{Key: "k", FromEnv: "GCK_PREFLIGHT_UNSET_VAR"}}}}}},
	)
	err := PreflightLocalResources(components)
	if err == nil {
		t.Fatal("expected missing inputs to be reported")
	}
	for _, want := range []string{`component "needs-file": secret "d"`, `component "needs-env": config map "e"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}
