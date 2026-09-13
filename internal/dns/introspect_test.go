package dns

import (
	"context"
	"strings"
	"testing"
	"time"

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

	_, _, err := pollGatewayRecords(context.Background(), client, 100*time.Millisecond)
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

	_, _, err := pollGatewayRecords(context.Background(), client, 100*time.Millisecond)
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

	records, sawRoutes, err := pollGatewayRecords(context.Background(), client, 2*time.Second)
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

	records, sawRoutes, err := pollGatewayRecords(context.Background(), client, 2*time.Second)
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
