package dns

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gravitee-io-labs/gck/internal/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"sigs.k8s.io/kind/pkg/cluster"
)

var (
	gatewayGVR = schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "gateways",
	}
	httpRouteGVR = schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "httproutes",
	}
)

const defaultPollInterval = 2 * time.Second

// IntrospectCluster queries the named Kind cluster for DNS-relevant services
// and writes the result as a record file to recordDir/<clusterName>.json.
//
// When introspectGateway is true, Gateway API resources (Gateways + HTTPRoutes)
// are polled and their hostname→IP mappings are included.
//
// Explicit dnsRecords (from features.dns.records) are resolved by looking up
// each declared service's LoadBalancer ingress IP.
// Collecting nothing is an error when the cluster declared something to
// collect, and only then. Every failure below used to be a warning that left
// `gck create` exiting 0 with no record file, so the DNS server came up and
// served NXDOMAIN forever — a broken cluster reported as a healthy one. The
// predicate is what was actually asked for (route hostnames, or declared
// records), NOT features.dns.enabled: a context can enable DNS while declaring
// neither gateway nor records (registry/gravitee-io/oss/apim/gateway does), and
// collecting nothing there is correct.
//
// It returns the records it wrote, so the caller can verify they are servable.
func IntrospectCluster(ctx context.Context, clusterName, recordDir string, pollTimeout time.Duration, introspectGateway bool, dnsRecords []config.DNSRecord) (map[string]string, error) {
	restCfg, err := introspectRESTConfig(clusterName)
	if err != nil {
		return nil, err
	}

	records := make(map[string]string)
	sawRouteHostnames := false

	if introspectGateway {
		dynClient, err := dynamic.NewForConfig(restCfg)
		if err != nil {
			return nil, fmt.Errorf("creating dynamic client: %w", err)
		}

		gwRecords, sawRoutes, err := pollGatewayRecords(ctx, dynClient, pollTimeout)
		if err != nil {
			return nil, err
		}
		sawRouteHostnames = sawRoutes
		for k, v := range gwRecords {
			records[k] = v
		}
	}

	if len(dnsRecords) > 0 {
		clientset, err := kubernetes.NewForConfig(restCfg)
		if err != nil {
			return nil, fmt.Errorf("creating kubernetes client: %w", err)
		}
		svcRecords, err := resolveServiceRecords(ctx, clientset, dnsRecords, pollTimeout)
		if err != nil {
			return nil, err
		}
		for k, v := range svcRecords {
			records[k] = v
		}
	}

	if len(records) == 0 {
		if sawRouteHostnames || len(dnsRecords) > 0 {
			return nil, fmt.Errorf(
				"collected no DNS records for cluster %q although the context declares them",
				clusterName)
		}
		klog.Info("context declares no DNS sources (no HTTPRoute hostnames, no features.dns.records); skipping record file")
		return nil, nil
	}

	klog.Infof("writing %d DNS record(s) for cluster %q", len(records), clusterName)
	if err := WriteRecordFile(recordDir, clusterName, records); err != nil {
		return nil, err
	}
	return records, nil
}

type gatewayKey struct {
	Namespace string
	Name      string
}

// pollGatewayRecords polls Gateways and HTTPRoutes until every route hostname
// maps to a Gateway address, or the timeout expires.
//
// sawRoutes reports whether the cluster declares any HTTPRoute hostnames at
// all. A cluster with none is legitimate (the caller decides); a route whose
// Gateway never gets an address is not, and is an error here.
//
// Route mapping is folded into the poll rather than run once afterwards so a
// Gateway that gets its address late still gets its routes resolved, and so a
// route pointing at an addressless Gateway keeps the loop going instead of
// being dropped with a V(2) line nobody reads.
func pollGatewayRecords(ctx context.Context, client dynamic.Interface, timeout time.Duration) (records map[string]string, sawRoutes bool, err error) {
	deadline := time.After(timeout)
	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()

	var unresolved []string
	for {
		addrs, err := listGatewayAddresses(ctx, client)
		if err != nil {
			return nil, false, err
		}

		if len(addrs) > 0 {
			records, unresolved, err = buildRecords(ctx, client, addrs)
			if err != nil {
				return nil, false, err
			}
			if len(unresolved) == 0 {
				return records, len(records) > 0, nil
			}
		}

		klog.V(2).Info("Gateway addresses or routes not ready yet, waiting...")
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case <-deadline:
			if len(addrs) == 0 {
				return nil, false, fmt.Errorf(
					"timed out after %s waiting for any Gateway to report .status.addresses; "+
						"the cloud-provider-kind controller did not provision the Gateway", timeout)
			}
			return nil, false, fmt.Errorf(
				"timed out after %s: %d HTTPRoute hostname(s) have no Gateway address (%s)",
				timeout, len(unresolved), strings.Join(unresolved, ", "))
		case <-ticker.C:
		}
	}
}

// listGatewayAddresses returns a map from Gateway namespace/name to the first
// IPAddress-type value from .status.addresses.
func listGatewayAddresses(ctx context.Context, client dynamic.Interface) (map[gatewayKey]string, error) {
	list, err := client.Resource(gatewayGVR).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing Gateways: %w", err)
	}

	addrs := make(map[gatewayKey]string)
	for i := range list.Items {
		gw := &list.Items[i]
		ip := extractGatewayIP(gw)
		if ip == "" {
			continue
		}
		key := gatewayKey{
			Namespace: gw.GetNamespace(),
			Name:      gw.GetName(),
		}
		addrs[key] = ip
		klog.V(2).Infof("Gateway %s/%s → %s", key.Namespace, key.Name, ip)
	}
	return addrs, nil
}

// extractGatewayIP returns the first IPAddress-type value from
// .status.addresses, or the first address value if no type is specified.
func extractGatewayIP(gw *unstructured.Unstructured) string {
	addresses, found, err := unstructured.NestedSlice(gw.Object, "status", "addresses")
	if err != nil || !found || len(addresses) == 0 {
		return ""
	}
	for _, addr := range addresses {
		m, ok := addr.(map[string]interface{})
		if !ok {
			continue
		}
		addrType, _, _ := unstructured.NestedString(m, "type")
		if addrType != "" && addrType != "IPAddress" {
			continue
		}
		val, _, _ := unstructured.NestedString(m, "value")
		if val != "" {
			return val
		}
	}
	return ""
}

// resolveServiceRecords polls each declared DNS record's LoadBalancer service
// until all have ingress IPs or the timeout expires. This handles the race
// where CPK hasn't assigned LB IPs yet when DNS setup runs right after
// component install.
func resolveServiceRecords(ctx context.Context, client kubernetes.Interface, dnsRecords []config.DNSRecord, timeout time.Duration) (map[string]string, error) {
	deadline := time.After(timeout)
	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()

	for {
		records := make(map[string]string)
		var pendingHosts []string
		for _, r := range dnsRecords {
			svc, err := client.CoreV1().Services(r.Namespace).Get(ctx, r.Service, metav1.GetOptions{})
			if err != nil {
				klog.V(2).Infof("DNS record %q: service %s/%s not found yet: %v", r.Hostname, r.Namespace, r.Service, err)
				pendingHosts = append(pendingHosts, r.Hostname)
				continue
			}
			if len(svc.Status.LoadBalancer.Ingress) == 0 || svc.Status.LoadBalancer.Ingress[0].IP == "" {
				klog.V(2).Infof("DNS record %q: service %s/%s has no LoadBalancer IP yet", r.Hostname, r.Namespace, r.Service)
				pendingHosts = append(pendingHosts, r.Hostname)
				continue
			}
			ip := svc.Status.LoadBalancer.Ingress[0].IP
			records[strings.ToLower(r.Hostname)] = ip
			klog.V(2).Infof("DNS record %s → %s (service %s/%s)", r.Hostname, ip, r.Namespace, r.Service)
		}

		if len(pendingHosts) == 0 {
			return records, nil
		}

		klog.V(2).Infof("%d/%d LoadBalancer service(s) still pending", len(pendingHosts), len(dnsRecords))
		select {
		case <-ctx.Done():
			return records, ctx.Err()
		case <-deadline:
			// Returning the partial set with a nil error silently dropped
			// whatever was still pending — for ee/apim that is the whole
			// *.kafka.gck.local wildcard, absent with no failure anywhere.
			return records, fmt.Errorf(
				"timed out after %s waiting for LoadBalancer IPs: %d/%d record(s) unresolved (%s)",
				timeout, len(pendingHosts), len(dnsRecords), strings.Join(pendingHosts, ", "))
		case <-ticker.C:
		}
	}
}

// buildRecords lists all HTTPRoutes and maps their hostnames to Gateway IPs
// using parentRefs.
//
// unresolved holds the hostnames whose parentRefs matched no addressed Gateway.
// They are returned rather than only logged so the caller can keep polling and,
// on timeout, name them: silently dropping one produced a cluster that resolved
// most of its hostnames and failed on the rest with no indication why.
func buildRecords(ctx context.Context, client dynamic.Interface, gwAddrs map[gatewayKey]string) (records map[string]string, unresolved []string, err error) {
	list, err := client.Resource(httpRouteGVR).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("listing HTTPRoutes: %w", err)
	}

	records = make(map[string]string)
	for i := range list.Items {
		route := &list.Items[i]
		hostnames := extractHostnames(route)
		if len(hostnames) == 0 {
			continue
		}

		parentIPs := resolveParentIPs(route, gwAddrs)
		if len(parentIPs) == 0 {
			klog.V(2).Infof("HTTPRoute %s/%s: no matching Gateway addresses for parentRefs",
				route.GetNamespace(), route.GetName())
			unresolved = append(unresolved, hostnames...)
			continue
		}

		ip := parentIPs[0]
		for _, h := range hostnames {
			records[strings.ToLower(h)] = ip
			klog.V(2).Infof("  %s → %s", h, ip)
		}
	}
	return records, unresolved, nil
}

// extractHostnames returns .spec.hostnames from an HTTPRoute.
func extractHostnames(route *unstructured.Unstructured) []string {
	vals, found, err := unstructured.NestedStringSlice(route.Object, "spec", "hostnames")
	if err != nil || !found {
		return nil
	}
	return vals
}

// resolveParentIPs looks up each parentRef of an HTTPRoute in the Gateway
// address map and returns the corresponding IPs.
func resolveParentIPs(route *unstructured.Unstructured, gwAddrs map[gatewayKey]string) []string {
	parentRefs, found, err := unstructured.NestedSlice(route.Object, "spec", "parentRefs")
	if err != nil || !found {
		return nil
	}

	routeNS := route.GetNamespace()
	var ips []string
	seen := make(map[string]bool)
	for _, ref := range parentRefs {
		m, ok := ref.(map[string]interface{})
		if !ok {
			continue
		}

		group, _, _ := unstructured.NestedString(m, "group")
		kind, _, _ := unstructured.NestedString(m, "kind")
		if group != "" && group != "gateway.networking.k8s.io" {
			continue
		}
		if kind != "" && kind != "Gateway" {
			continue
		}

		name, _, _ := unstructured.NestedString(m, "name")
		if name == "" {
			continue
		}
		ns, _, _ := unstructured.NestedString(m, "namespace")
		if ns == "" {
			ns = routeNS
		}

		key := gatewayKey{Namespace: ns, Name: name}
		if ip, ok := gwAddrs[key]; ok && !seen[ip] {
			ips = append(ips, ip)
			seen[ip] = true
		}
	}
	return ips
}

func introspectRESTConfig(clusterName string) (*rest.Config, error) {
	provider := cluster.NewProvider()
	kubeConfig, err := provider.KubeConfig(clusterName, false)
	if err != nil {
		return nil, fmt.Errorf("getting kubeconfig for %q: %w", clusterName, err)
	}
	restCfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeConfig))
	if err != nil {
		return nil, fmt.Errorf("parsing kubeconfig: %w", err)
	}
	return restCfg, nil
}
