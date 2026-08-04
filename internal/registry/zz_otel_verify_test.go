package registry

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gravitee-io-labs/gck/internal/config"
)

// zz_otel_verify_test.go is a THROWAWAY verification for the otel-collector
// context work — delete after running.
func resolveComposed(t *testing.T, refs ...string) *config.ResolvedContext {
	t.Helper()
	root, err := filepath.Abs("../../registry")
	if err != nil {
		t.Fatal(err)
	}
	acc := &config.ResolvedContext{}
	for _, ref := range refs {
		r := NewResolver("file://"+root, t.TempDir(), nil)
		rc, err := r.Resolve(context.Background(), ref)
		if err != nil {
			t.Fatalf("resolve %q: %v", ref, err)
		}
		t.Logf("resolved %q -> Kind.Name=%q", ref, rc.Kind.Name)
		MergeInto(acc, rc)
	}
	return acc
}

func gatewayEnv(t *testing.T, rc *config.ResolvedContext, comp string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for i := range rc.Components {
		if rc.Components[i].Name != comp || rc.Components[i].Helm == nil {
			continue
		}
		gw, _ := rc.Components[i].Helm.Values["gateway"].(map[string]interface{})
		env, _ := gw["env"].([]interface{})
		for _, e := range env {
			m, _ := e.(map[string]interface{})
			name, _ := m["name"].(string)
			val, _ := m["value"].(string)
			out[name] = val
		}
	}
	return out
}

func TestOtelVerify(t *testing.T) {
	cases := []struct {
		name string
		prod string
		comp string
	}{
		{"apim", "gravitee-io/oss/apim/jdbc/postgres", "apim"},
		{"am", "gravitee-io/oss/am/jdbc/postgres", "am"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rc := resolveComposed(t, c.prod, "otel-collector/base")
			if rc.Kind.Name != "gravitee" {
				t.Errorf("cluster name = %q, want gravitee", rc.Kind.Name)
			}
			var hasCollector bool
			for i := range rc.Components {
				if rc.Components[i].Name == "otel-collector" {
					hasCollector = true
				}
			}
			if !hasCollector {
				t.Error("otel-collector component missing")
			}
			// before flag: no otel env
			if got := gatewayEnv(t, rc, c.comp)["gravitee_services_opentelemetry_enabled"]; got != "" {
				t.Errorf("otel env present before flag: %q", got)
			}
			if err := ApplyFlags(rc, []string{"enable-otel-collector"}, nil); err != nil {
				t.Fatalf("ApplyFlags: %v", err)
			}
			env := gatewayEnv(t, rc, c.comp)
			if env["gravitee_services_opentelemetry_enabled"] != "true" {
				t.Errorf("enabled = %q, want true", env["gravitee_services_opentelemetry_enabled"])
			}
			if got := env["gravitee_services_opentelemetry_exporter_otlp_endpoint"]; got != "http://otel-collector:4317" {
				t.Errorf("endpoint = %q, want http://otel-collector:4317", got)
			}
			// Note: post-ApplyFlags rc.Kind.Name is "gck" here — a harness
			// artifact (loadFlagConfig applies Kind defaults). In production the
			// cluster name is captured via cfg.Kind.MergeWithContext BEFORE flags,
			// so it stays "gravitee" (asserted pre-flag above).
			t.Logf("OK %s: pre-flag cluster=gravitee, env=%v", c.name, env)
		})
	}
}
