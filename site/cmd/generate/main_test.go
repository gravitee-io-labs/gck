package main

import (
	"os"
	"path/filepath"
	"testing"
)

// loadRegistry parses every gck.yaml in the real registry, the way main does.
func loadRegistry(t *testing.T) (string, map[string]*gckConfig) {
	t.Helper()
	registryDir, err := filepath.Abs(filepath.Join("..", "..", "..", "registry"))
	if err != nil {
		t.Fatalf("resolving registry dir: %v", err)
	}
	configs := map[string]*gckConfig{}
	err = filepath.Walk(registryDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || info.Name() != "gck.yaml" {
			return err
		}
		rel, err := filepath.Rel(registryDir, filepath.Dir(path))
		if err != nil {
			return err
		}
		config, err := parseGckConfig(path)
		if err != nil {
			return err
		}
		configs[rel] = config
		return nil
	})
	if err != nil {
		t.Fatalf("walking registry: %v", err)
	}
	return registryDir, configs
}

func endpointByName(eps []endpointInfo, name string) (endpointInfo, bool) {
	for _, ep := range eps {
		if ep.Name == name {
			return ep, true
		}
	}
	return endpointInfo{}, false
}

// TestResolveEndpoints_InheritsFromAbstractBase is the case the Endpoints table
// exists for: abstract contexts get no page, so a row declared on one is only
// ever seen because it resolves onto the concrete variants that compose it.
func TestResolveEndpoints_InheritsFromAbstractBase(t *testing.T) {
	registryDir, configs := loadRegistry(t)

	eps := resolveEndpoints("grafana/standalone", configs, registryDir)

	grafana, ok := endpointByName(eps, "Grafana")
	if !ok {
		t.Fatalf("expected grafana/standalone to inherit the Grafana row, got %+v", eps)
	}
	if grafana.URL != "http://localhost:30300" {
		t.Errorf("Grafana url = %q, want http://localhost:30300", grafana.URL)
	}
	if grafana.Origin != "grafana/base" {
		t.Errorf("Grafana origin = %q, want grafana/base", grafana.Origin)
	}

	// Its own rows carry their own origin.
	if otlp, ok := endpointByName(eps, "OTLP gRPC"); !ok || otlp.Origin != "grafana/standalone" {
		t.Errorf("OTLP gRPC = %+v, want origin grafana/standalone", otlp)
	}
}

// TestResolveEndpoints_HonoursWhenGuards checks both guard directions against
// the real registry: flag-gated rows are absent from the default view, and a
// variant that stops exposing an inherited service suppresses its row.
func TestResolveEndpoints_HonoursWhenGuards(t *testing.T) {
	registryDir, configs := loadRegistry(t)

	// A flag-gated row is still listed, but marked with the flag that reveals
	// it, and it sorts after the rows a plain "gck create" exposes.
	eps := resolveEndpoints("grafana/standalone", configs, registryDir)
	route, ok := endpointByName(eps, "Grafana (route)")
	if !ok {
		t.Fatalf("expected the flag-gated route row to be listed, got %+v", eps)
	}
	if len(route.Requires) != 1 || route.Requires[0] != "enable-route" {
		t.Errorf("Grafana (route) requires = %v, want [enable-route]", route.Requires)
	}
	if eps[len(eps)-1].Name != "Grafana (route)" {
		t.Errorf("expected gated rows last, got %+v", eps)
	}
	// Unconditional rows carry no flag.
	if grafana, _ := endpointByName(eps, "Grafana"); len(grafana.Requires) != 0 {
		t.Errorf("Grafana requires = %v, want none", grafana.Requires)
	}

	// dbless runs the gateway alone and gates off everything else apim/base
	// declares, so it exposes exactly one endpoint out of the box. It still
	// inherits apim/base's flags, hence the check on unconditional rows only.
	var dblessDefault []string
	for _, ep := range resolveEndpoints("gravitee-io/oss/apim/dbless", configs, registryDir) {
		if len(ep.Requires) == 0 {
			dblessDefault = append(dblessDefault, ep.Name)
		}
	}
	if len(dblessDefault) != 1 || dblessDefault[0] != "APIM Gateway" {
		t.Errorf("dbless default endpoints = %v, want only APIM Gateway", dblessDefault)
	}

	// The EE stack fronts Kafka with the gateway, so the broker row is gated off.
	ee := resolveEndpoints("gravitee-io/ee/apim/jdbc/postgres", configs, registryDir)
	if _, ok := endpointByName(ee, "Kafka"); ok {
		t.Error("expected the inherited Kafka row to be suppressed in the EE stack")
	}
	if _, ok := endpointByName(ee, "Kafka Gateway"); !ok {
		t.Errorf("expected the Kafka Gateway row, got %+v", ee)
	}
}

// TestResolveEndpoints_IncludesComposedDatastores guards the drift that the
// hand-written README tables all shared: a product context binds its
// datastore's host ports, so those endpoints belong on its page.
func TestResolveEndpoints_IncludesComposedDatastores(t *testing.T) {
	registryDir, configs := loadRegistry(t)

	eps := resolveEndpoints("gravitee-io/oss/apim/jdbc/postgres", configs, registryDir)
	for name, wantOrigin := range map[string]string{
		"PostgreSQL":    "postgresql/standalone",
		"Elasticsearch": "elastic/elasticsearch/standalone",
		"APIM Gateway":  "gravitee-io/oss/apim/base",
	} {
		ep, ok := endpointByName(eps, name)
		if !ok {
			t.Errorf("expected a %q row, got %+v", name, eps)
			continue
		}
		if ep.Origin != wantOrigin {
			t.Errorf("%s origin = %q, want %q", name, ep.Origin, wantOrigin)
		}
	}
}
