package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
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

	// --use-dbless runs the gateway alone: it implies --disable-ui and
	// --disable-analytics and hides the management API, so those rows are
	// marked as absent under it while the gateway row stays unconditional.
	apim := resolveEndpoints("gravitee-io/apim", configs, registryDir)
	for _, name := range []string{"APIM Console", "APIM Portal", "APIM API", "APIM Gateway (TLS)"} {
		ep, ok := endpointByName(apim, name)
		if !ok || !slices.Equal(ep.Unless, []string{"use-dbless"}) {
			t.Errorf("%s = %+v, want unless [use-dbless]", name, ep)
		}
	}
	if gw, _ := endpointByName(apim, "APIM Gateway"); len(gw.Use)+len(gw.Unless)+len(gw.Requires) != 0 {
		t.Errorf("APIM Gateway = %+v, want unconditional", gw)
	}
	// dbless disables the analytics group altogether: no Elasticsearch.
	if es, _ := endpointByName(apim, "Elasticsearch"); !slices.Equal(es.Use, []string{"use-elasticsearch"}) || !slices.Equal(es.Unless, []string{"use-dbless"}) {
		t.Errorf("Elasticsearch = %+v, want use [use-elasticsearch] unless [use-dbless]", es)
	}

	// --enable-kafka-gateway composes a broker and fronts it with the gateway,
	// so the broker's own row is suppressed and the gateway's is flag-gated.
	if _, ok := endpointByName(apim, "Kafka"); ok {
		t.Error("expected the composed Kafka row to be suppressed")
	}
	gw, ok := endpointByName(apim, "Kafka Gateway")
	if !ok || !slices.Equal(gw.Requires, []string{"enable-kafka-gateway"}) {
		t.Errorf("expected the Kafka Gateway row behind --enable-kafka-gateway, got %+v", gw)
	}
}

// TestResolveEndpoints_IncludesComposedDatastores guards the drift that the
// hand-written README tables all shared: a product context binds its
// datastore's host ports, so those endpoints belong on its page -- marked with
// the alternative that brings each datastore in.
func TestResolveEndpoints_IncludesComposedDatastores(t *testing.T) {
	registryDir, configs := loadRegistry(t)

	eps := resolveEndpoints("gravitee-io/apim", configs, registryDir)
	for name, want := range map[string]struct {
		origin string
		use    []string
	}{
		"PostgreSQL":    {"postgresql/standalone", []string{"use-jdbc-postgres"}},
		"MySQL":         {"mysql/standalone", []string{"use-jdbc-mysql"}},
		"MongoDB":       {"mongodb/standalone", []string{"use-mongodb"}},
		"Elasticsearch": {"elastic/elasticsearch/standalone", []string{"use-elasticsearch"}},
		"OpenSearch":    {"opensearch/standalone", []string{"use-opensearch"}},
		"APIM Gateway":  {"gravitee-io/apim/base", nil},
	} {
		ep, ok := endpointByName(eps, name)
		if !ok {
			t.Errorf("expected a %q row, got %+v", name, eps)
			continue
		}
		if ep.Origin != want.origin {
			t.Errorf("%s origin = %q, want %q", name, ep.Origin, want.origin)
		}
		if !slices.Equal(ep.Use, want.use) {
			t.Errorf("%s use = %v, want %v", name, ep.Use, want.use)
		}
	}
}

// TestResolveAlternatives lists the groups a user can pick from, and hides
// the ones a composing context pins.
func TestResolveAlternatives(t *testing.T) {
	registryDir, configs := loadRegistry(t)

	groups := resolveAlternatives("gravitee-io/apim", configs, registryDir)
	var names []string
	defaults := map[string]string{}
	for _, g := range groups {
		names = append(names, g.Name)
		for _, o := range g.Options {
			if o.Default {
				defaults[g.Name] = o.Name
			}
			if o.Source != "gravitee-io/apim" {
				t.Errorf("--%s source = %q, want the declaring context gravitee-io/apim", o.Name, o.Source)
			}
		}
	}
	if !slices.Equal(names, []string{"analytics", "datasource"}) {
		t.Errorf("groups = %v, want [analytics datasource]", names)
	}
	if defaults["datasource"] != "use-jdbc-postgres" || defaults["analytics"] != "use-elasticsearch" {
		t.Errorf("defaults = %v", defaults)
	}

	if g := resolveAlternatives("gravitee-io/gamma", configs, registryDir); len(g) != 0 {
		t.Errorf("gamma pins its datasource; expected no selectable groups, got %+v", g)
	}
}

// TestResolveVars_GatesAlternativeVars marks the variables only some
// alternatives declare, and keeps the rest unconditional.
func TestResolveVars_GatesAlternativeVars(t *testing.T) {
	registryDir, configs := loadRegistry(t)

	var pgURL, mysqlURL, imagePrefix *varInfo
	vars := resolveVars("gravitee-io/apim", configs, registryDir)
	for i, v := range vars {
		switch {
		case v.Name == "jdbcUrl" && strings.Contains(v.Default, "postgresql"):
			pgURL = &vars[i]
		case v.Name == "jdbcUrl" && strings.Contains(v.Default, "mysql"):
			mysqlURL = &vars[i]
		case v.Name == "imagePrefix":
			imagePrefix = &vars[i]
		}
	}
	if pgURL == nil || !slices.Equal(pgURL.Use, []string{"use-jdbc-postgres"}) {
		t.Errorf("postgres jdbcUrl = %+v, want gated by use-jdbc-postgres", pgURL)
	}
	if mysqlURL == nil || !slices.Equal(mysqlURL.Use, []string{"use-jdbc-mysql"}) {
		t.Errorf("mysql jdbcUrl = %+v, want gated by use-jdbc-mysql", mysqlURL)
	}
	if imagePrefix == nil || len(imagePrefix.Use) != 0 {
		t.Errorf("imagePrefix = %+v, want unconditional", imagePrefix)
	}
}
