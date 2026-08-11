package notes

import (
	"strings"
	"testing"
)

func layer(source, raw string) Layer { return Layer{Source: source, Raw: raw} }

func compose(t *testing.T, layers []Layer, flags []string) string {
	t.Helper()
	out, err := Compose(layers, nil, flags)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	return out
}

func TestCompose_MergesEndpointsFromEveryLayer(t *testing.T) {
	out := compose(t, []Layer{
		layer("postgresql/standalone", "---\ntitle: PostgreSQL\nendpoints:\n  - name: PostgreSQL\n    url: localhost:30432\n---\n"),
		layer("apim/base", "---\ntitle: APIM\nendpoints:\n  - name: APIM Gateway\n    url: http://localhost:30082\n---\n"),
		layer("grafana/base", "---\ntitle: Grafana\nendpoints:\n  - name: Grafana\n    url: http://localhost:30300\n---\n"),
	}, nil)

	for _, want := range []string{"PostgreSQL", "APIM Gateway", "Grafana"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected endpoint %q in output, got:\n%s", want, out)
		}
	}
	// One table, not three.
	if n := strings.Count(out, "Endpoints"); n != 1 {
		t.Errorf("expected a single endpoints table, got %d headings:\n%s", n, out)
	}
	if i, j := strings.Index(out, "PostgreSQL"), strings.Index(out, "Grafana"); i > j {
		t.Errorf("expected composition order to be preserved, got:\n%s", out)
	}
}

func TestCompose_LaterLayerReplacesRowInPlace(t *testing.T) {
	out := compose(t, []Layer{
		layer("am/base", "---\nendpoints:\n  - name: AM Console\n    url: http://localhost:30090\n  - name: AM API\n    url: http://localhost:30093\n---\n"),
		layer("ee/gamma", "---\nendpoints:\n  - name: AM Console\n    url: http://am-console.gck.local\n---\n"),
	}, nil)

	if strings.Contains(out, "localhost:30090") {
		t.Errorf("expected the later layer's URL to win, got:\n%s", out)
	}
	if !strings.Contains(out, "http://am-console.gck.local") {
		t.Errorf("expected the override URL, got:\n%s", out)
	}
	if i, j := strings.Index(out, "AM Console"), strings.Index(out, "AM API"); i > j {
		t.Errorf("expected the replaced row to keep its position, got:\n%s", out)
	}
}

func TestCompose_LaterLayerHidesInheritedRow(t *testing.T) {
	out := compose(t, []Layer{
		layer("kafka/standalone", "---\nendpoints:\n  - name: Kafka\n    url: localhost:30092\n---\n"),
		layer("ee/apim/base", "---\nendpoints:\n  - name: Kafka\n    when: false\n---\n"),
	}, nil)

	if strings.Contains(out, "Kafka") {
		t.Errorf("expected the inherited row to be hidden, got:\n%s", out)
	}
}

func TestCompose_EndpointWhenFollowsFlags(t *testing.T) {
	raw := "---\nendpoints:\n  - name: APIM Console\n    url: http://localhost:30080\n    when: '{{ not (hasFlag \"disable-ui\") }}'\n  - name: APIM Gateway\n    url: http://localhost:30082\n---\n"

	if out := compose(t, []Layer{layer("apim/base", raw)}, nil); !strings.Contains(out, "APIM Console") {
		t.Errorf("expected the console without the flag, got:\n%s", out)
	}
	out := compose(t, []Layer{layer("apim/base", raw)}, []string{"disable-ui"})
	if strings.Contains(out, "APIM Console") {
		t.Errorf("expected the console hidden by --disable-ui, got:\n%s", out)
	}
	if !strings.Contains(out, "APIM Gateway") {
		t.Errorf("expected unrelated rows untouched, got:\n%s", out)
	}
}

func TestMerge_RowsCarryTheirOrigin(t *testing.T) {
	merged, err := Merge([]Layer{
		layer("grafana/base", "---\nendpoints:\n  - name: Grafana\n    url: http://localhost:30300\n---\n"),
		layer("grafana/standalone", "---\nendpoints:\n  - name: OTLP gRPC\n    url: localhost:30317\n---\n"),
	}, nil, nil)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	want := map[string]string{"Grafana": "grafana/base", "OTLP gRPC": "grafana/standalone"}
	for _, ep := range merged.Endpoints {
		if ep.Origin != want[ep.Name] {
			t.Errorf("%s origin = %q, want %q", ep.Name, ep.Origin, want[ep.Name])
		}
	}
}

func TestMerge_ReplacedRowTakesTheReplacingOrigin(t *testing.T) {
	merged, err := Merge([]Layer{
		layer("am/base", "---\nendpoints:\n  - name: AM Console\n    url: http://localhost:30090\n---\n"),
		layer("ee/gamma", "---\nendpoints:\n  - name: AM Console\n    url: http://am-console.gck.local\n---\n"),
	}, nil, nil)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	if len(merged.Endpoints) != 1 {
		t.Fatalf("expected one row, got %+v", merged.Endpoints)
	}
	if got := merged.Endpoints[0].Origin; got != "ee/gamma" {
		t.Errorf("origin = %q, want ee/gamma -- the layer that supplied the content", got)
	}
}

func TestMerge_VisibleEndpointsAppliesGuards(t *testing.T) {
	merged, err := Merge([]Layer{
		layer("ctx", "---\nendpoints:\n  - name: Shown\n    url: http://localhost:1\n  - name: Gated\n    url: http://localhost:2\n    when: false\n---\n"),
	}, nil, nil)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	// Merge keeps gated rows so callers can audit what a context documents.
	if len(merged.Endpoints) != 2 {
		t.Errorf("expected Merge to keep gated rows, got %+v", merged.Endpoints)
	}
	visible := merged.VisibleEndpoints()
	if len(visible) != 1 || visible[0].Name != "Shown" {
		t.Errorf("VisibleEndpoints() = %+v, want only Shown", visible)
	}
}

func TestCompose_BodyReplacedBySameTitle(t *testing.T) {
	out := compose(t, []Layer{
		layer("kafka/standalone", "---\ntitle: Kafka\n---\nconnect with kcat"),
		layer("ee/apim/base", "---\ntitle: Kafka\n---\nconnect through the gateway"),
	}, nil)

	if strings.Contains(out, "kcat") {
		t.Errorf("expected the same-title body to be replaced, got:\n%s", out)
	}
	if !strings.Contains(out, "connect through the gateway") {
		t.Errorf("expected the replacement body, got:\n%s", out)
	}
	if n := strings.Count(out, "Kafka"); n != 1 {
		t.Errorf("expected one Kafka heading, got %d:\n%s", n, out)
	}
}

func TestCompose_DocWhenGatesWholeLayer(t *testing.T) {
	out := compose(t, []Layer{
		layer("ctx", "---\ntitle: Optional\nwhen: false\nendpoints:\n  - name: Hidden\n    url: http://localhost:1\n---\nnot shown"),
	}, nil)

	if out != "" {
		t.Errorf("expected a gated layer to contribute nothing, got:\n%s", out)
	}
}

func TestCompose_BodyKeepsCommandIndentation(t *testing.T) {
	out := compose(t, []Layer{layer("ctx", "---\ntitle: Redis\n---\n  redis-cli -h localhost -p 30379\n")}, nil)

	if !strings.Contains(out, "      redis-cli -h localhost -p 30379") {
		t.Errorf("expected the authored indent to survive under the heading, got:\n%q", out)
	}
}

func TestCompose_NoFrontMatterIsAllProse(t *testing.T) {
	out := compose(t, []Layer{layer("ctx", "just prose")}, nil)
	if !strings.Contains(out, "just prose") {
		t.Errorf("expected prose-only notes to render, got:\n%s", out)
	}
}

func TestCompose_RejectsNamelessEndpoint(t *testing.T) {
	_, err := Compose([]Layer{layer("ctx", "---\nendpoints:\n  - url: http://localhost:1\n---\n")}, nil, nil)
	if err == nil {
		t.Fatal("expected an error for an endpoint without a name")
	}
	if !strings.Contains(err.Error(), "ctx") {
		t.Errorf("expected the source in the error, got: %v", err)
	}
}

func TestCompose_ReportsMalformedFrontMatter(t *testing.T) {
	_, err := Compose([]Layer{layer("broken/ctx", "---\nendpoints: [oops\n---\n")}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "broken/ctx") {
		t.Fatalf("expected a parse error naming the source, got: %v", err)
	}
}

func TestAppend_SkipsContextReachedTwice(t *testing.T) {
	base := []Layer{layer("otel-collector/base", "a")}
	got := Append(base, []Layer{layer("otel-collector/base", "a"), layer("grafana/base", "b")})

	if len(got) != 2 {
		t.Fatalf("expected 2 layers, got %d: %v", len(got), got)
	}
	if got[1].Source != "grafana/base" {
		t.Errorf("expected the new layer appended, got %v", got)
	}
	if len(base) != 1 {
		t.Errorf("expected the base slice untouched, got %v", base)
	}
}

func TestSplitFrontMatter(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantFront string
		wantBody  string
	}{
		{"no front matter", "hello", "", "hello"},
		{"front matter and body", "---\ntitle: A\n---\nbody", "title: A", "body"},
		{"trailing space on delimiter", "--- \ntitle: A\n---\nbody", "title: A", "body"},
		{"unterminated is all body", "---\ntitle: A\nbody", "", "---\ntitle: A\nbody"},
		{"empty body", "---\ntitle: A\n---\n", "title: A", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			front, body := splitFrontMatter(tt.in)
			if front != tt.wantFront {
				t.Errorf("front = %q, want %q", front, tt.wantFront)
			}
			if strings.TrimSpace(body) != strings.TrimSpace(tt.wantBody) {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}
