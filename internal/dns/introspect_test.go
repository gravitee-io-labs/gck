package dns

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gravitee-io-labs/gck/internal/config"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

func gateway(namespace, name string, addresses ...string) *unstructured.Unstructured {
	addrs := make([]interface{}, 0, len(addresses))
	for _, a := range addresses {
		addrs = append(addrs, map[string]interface{}{"type": "IPAddress", "value": a})
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "Gateway",
		"metadata":   map[string]interface{}{"namespace": namespace, "name": name},
		"status":     map[string]interface{}{"addresses": addrs},
	}}
}

func httpRoute(namespace, name, parentName string, hostnames ...string) *unstructured.Unstructured {
	hosts := make([]interface{}, 0, len(hostnames))
	for _, h := range hostnames {
		hosts = append(hosts, h)
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "HTTPRoute",
		"metadata":   map[string]interface{}{"namespace": namespace, "name": name},
		"spec": map[string]interface{}{
			"hostnames": hosts,
			// Namespace is explicit, as in the real contexts: a parentRef
			// without one defaults to the route's own namespace, and the
			// shared gck-gateway lives in default.
			"parentRefs": []interface{}{
				map[string]interface{}{"name": parentName, "namespace": "default"},
			},
		},
	}}
}

// Objects go in through the tracker with an explicit GVR rather than as
// constructor arguments. Passing them to the constructor makes the tracker
// derive the GVR with meta.UnsafeGuessKindToResource, whose pluralisation turns
// "Gateway" into "gatewaies" — so the objects land under a resource nothing
// ever lists, and every List silently returns zero items.
func fakeDynamic(objs ...*unstructured.Unstructured) *fake.FakeDynamicClient {
	client := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			gatewayGVR:   "GatewayList",
			httpRouteGVR: "HTTPRouteList",
		},
	)

	for _, obj := range objs {
		gvr := gatewayGVR
		if obj.GetKind() == "HTTPRoute" {
			gvr = httpRouteGVR
		}
		if err := client.Tracker().Create(gvr, obj, obj.GetNamespace()); err != nil {
			panic(err)
		}
	}
	return client
}

// THE regression test. pollGatewayAddresses used to return (nil, nil) here, so
// IntrospectCluster collected nothing, wrote no record file and reported
// success — `gck create` exited 0 with a DNS server that could only ever serve
// NXDOMAIN. Three gravitee-platform-e2e nightlies died of this in Sept 2026.
func TestPollGatewayRecords_TimeoutIsAnError(t *testing.T) {
	client := fakeDynamic(gateway("default", "gck-gateway")) // no .status.addresses

	_, _, err := pollGatewayRecords(context.Background(), client, 100*time.Millisecond, "gck.local")
	if err == nil {
		t.Fatal("a Gateway that never gets an address must be an error, not a silent success")
	}
	if !strings.Contains(err.Error(), "status.addresses") {
		t.Fatalf("error should name what it waited for, got: %v", err)
	}
}

// A route whose parentRefs match no addressed Gateway used to be dropped with
// only a V(2) log line, producing a cluster that resolved some hostnames and
// silently not others.
func TestPollGatewayRecords_UnresolvedRouteIsAnError(t *testing.T) {
	client := fakeDynamic(
		gateway("default", "gck-gateway", "172.18.0.5"),
		httpRoute("gravitee", "apim-api", "other-gateway", "apim-api.gravitee.gck.local"),
	)

	_, _, err := pollGatewayRecords(context.Background(), client, 100*time.Millisecond, "gck.local")
	if err == nil {
		t.Fatal("a route with no addressed parent Gateway must be an error")
	}
	if !strings.Contains(err.Error(), "apim-api.gravitee.gck.local") {
		t.Fatalf("error should name the unresolved hostname, got: %v", err)
	}
}

// registry/gravitee-io/oss/apim/gateway enables features.dns while declaring
// neither routes nor records. Collecting nothing there is correct, which is why
// the predicate is route/record presence and not features.dns.enabled.
func TestPollGatewayRecords_NoRouteHostnamesIsNotAnError(t *testing.T) {
	client := fakeDynamic(gateway("default", "gck-gateway", "172.18.0.5"))

	records, sawRoutes, err := pollGatewayRecords(context.Background(), client, 2*time.Second, "gck.local")
	if err != nil {
		t.Fatalf("a cluster with no routes is legitimate: %v", err)
	}
	if sawRoutes {
		t.Fatal("sawRoutes must be false when no HTTPRoute declares hostnames")
	}
	if len(records) != 0 {
		t.Fatalf("expected no records, got %v", records)
	}
}

func TestPollGatewayRecords_MapsHostnamesToGatewayAddress(t *testing.T) {
	client := fakeDynamic(
		gateway("default", "gck-gateway", "172.18.0.5"),
		httpRoute("gravitee", "apim-api", "gck-gateway", "apim-api.gravitee.gck.local"),
	)

	records, sawRoutes, err := pollGatewayRecords(context.Background(), client, 2*time.Second, "gck.local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawRoutes {
		t.Fatal("sawRoutes must be true when a route declares hostnames")
	}
	if got := records["apim-api.gravitee.gck.local"]; got != "172.18.0.5" {
		t.Fatalf("expected 172.18.0.5, got %q", got)
	}
}

// The bug this filter exists for. graviteeio/apim renders apim.example.com as
// its placeholder host, so a chart — not a registry context — puts a hostname
// in the cluster that gck's server can never answer for: ServeDNS forwards
// anything outside the zone upstream. Collected, it went into the record file
// and then burned the whole startup probe budget before failing with "the
// server may have failed to bind", which was never the problem.
func TestPollGatewayRecords_SkipsHostnamesOutsideDomain(t *testing.T) {
	client := fakeDynamic(
		gateway("default", "gck-gateway", "172.18.0.5"),
		httpRoute("gravitee", "apim-console", "gck-gateway", "apim-console.gravitee.gck.local"),
		httpRoute("gravitee", "apim-ui", "gck-gateway", "apim.example.com"),
	)

	records, sawRoutes, err := pollGatewayRecords(context.Background(), client, 2*time.Second, "gck.local")
	if err != nil {
		t.Fatalf("a foreign hostname is not a failure: %v", err)
	}
	if !sawRoutes {
		t.Fatal("the in-domain route must still count")
	}
	if _, ok := records["apim.example.com"]; ok {
		t.Fatalf("apim.example.com must never be collected, got %v", records)
	}
	if got := records["apim-console.gravitee.gck.local"]; got != "172.18.0.5" {
		t.Fatalf("in-domain hostname lost: %v", records)
	}
}

// A route gck does not serve is also a route gck does not wait for. Before the
// filter this cost 90 seconds and then failed the create, naming a hostname
// that appears in no registry file.
func TestPollGatewayRecords_ForeignHostnameDoesNotBlockOnMissingGateway(t *testing.T) {
	client := fakeDynamic(
		gateway("default", "gck-gateway", "172.18.0.5"),
		httpRoute("gravitee", "apim-ui", "traefik-gateway", "apim.example.com"),
	)

	records, sawRoutes, err := pollGatewayRecords(context.Background(), client, 100*time.Millisecond, "gck.local")
	if err != nil {
		t.Fatalf("an out-of-zone route with no addressed parent must not block: %v", err)
	}
	if sawRoutes || len(records) != 0 {
		t.Fatalf("nothing should have been collected, got %v (sawRoutes=%v)", records, sawRoutes)
	}
}

func TestInDomain(t *testing.T) {
	cases := []struct {
		hostname string
		want     bool
	}{
		{"apim-api.gravitee.gck.local", true},
		{"*.kafka.gck.local", true},
		{"gck.local", true},
		{"APIM-API.GRAVITEE.GCK.LOCAL", true},
		{"apim-api.gravitee.gck.local.", true},
		{"apim.example.com", false},
		{"notgck.local", false},
		{"gck.local.evil.com", false},
	}
	for _, c := range cases {
		if got := inDomain(c.hostname, "gck.local"); got != c.want {
			t.Errorf("inDomain(%q) = %v, want %v", c.hostname, got, c.want)
		}
	}
}

// A declared record is authored intent, so it fails loudly instead of being
// dropped into a V(2) line and rediscovered later as a probe timeout.
func TestValidateRecordDomains(t *testing.T) {
	err := validateRecordDomains([]config.DNSRecord{
		{Hostname: "*.kafka.gck.local", Service: "kafka", Namespace: "gravitee"},
		{Hostname: "apim.example.com", Service: "apim", Namespace: "gravitee"},
	}, "gck.local")
	if err == nil {
		t.Fatal("a record outside the served domain must be a config error")
	}
	if !strings.Contains(err.Error(), "apim.example.com") {
		t.Fatalf("error must name the offending hostname, got: %v", err)
	}
	if strings.Contains(err.Error(), "kafka") {
		t.Fatalf("error must not implicate the valid record, got: %v", err)
	}

	if err := validateRecordDomains([]config.DNSRecord{
		{Hostname: "*.kafka.gck.local"},
	}, "gck.local"); err != nil {
		t.Fatalf("an in-domain record must pass: %v", err)
	}
}
