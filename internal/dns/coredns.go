package dns

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/gravitee-io-labs/gck/internal/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

const (
	coreDNSConfigMap = "coredns"
	coreDNSNamespace = "kube-system"
	coreDNSDataKey   = "Corefile"

	markerBegin = "# BEGIN gck.local"
	markerEnd   = "# END gck.local"
)

// SyncCoreDNS patches the in-cluster CoreDNS Corefile so pods can resolve
// gck.local hostnames. This allows in-cluster services to use the same
// hostnames as the host machine (e.g. for OAuth token introspection).
//
// When gatewayEnabled is true, the Gateway LB IP is discovered and all
// HTTPRoute hostnames are pointed at it. The Gateway LB IP is reachable
// from inside the cluster (Kind nodes share the Docker bridge network
// with the Envoy proxy container), so pods hit port 80 on the same
// Envoy that serves host traffic — no port mismatch.
//
// When gatewayEnabled is false, explicit dnsRecords are resolved to their
// backing services' ClusterIPs.
func SyncCoreDNS(ctx context.Context, clusterName string, domain string, dnsRecords []config.DNSRecord, gatewayEnabled bool) error {
	restCfg, err := introspectRESTConfig(clusterName)
	if err != nil {
		return err
	}
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("creating kubernetes client: %w", err)
	}

	var records map[string]string

	if gatewayEnabled {
		dynClient, err := dynamic.NewForConfig(restCfg)
		if err != nil {
			return fmt.Errorf("creating dynamic client: %w", err)
		}
		records, err = resolveGatewayRecords(ctx, dynClient)
		if err != nil {
			return err
		}
	} else {
		records, err = resolveClusterIPs(ctx, clientset, dnsRecords)
		if err != nil {
			return err
		}
	}

	if len(records) == 0 {
		klog.Info("no DNS records to sync to CoreDNS")
		return nil
	}

	block := buildCorefileBlock(domain, records)
	return patchCoreDNSConfigMap(ctx, clientset, block)
}

// CleanupCoreDNS removes the gck.local block from the CoreDNS Corefile,
// restoring it to its original state. Safe to call even if the block was
// never added (no-op in that case).
func CleanupCoreDNS(ctx context.Context, clusterName string) error {
	restCfg, err := introspectRESTConfig(clusterName)
	if err != nil {
		return err
	}
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("creating kubernetes client: %w", err)
	}

	cm, err := clientset.CoreV1().ConfigMaps(coreDNSNamespace).Get(ctx, coreDNSConfigMap, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("reading CoreDNS ConfigMap: %w", err)
	}

	corefile, ok := cm.Data[coreDNSDataKey]
	if !ok {
		return nil
	}

	cleaned := stripMarkerBlock(corefile)
	if cleaned == corefile {
		return nil
	}

	cm.Data[coreDNSDataKey] = cleaned
	if _, err := clientset.CoreV1().ConfigMaps(coreDNSNamespace).Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("updating CoreDNS ConfigMap: %w", err)
	}
	klog.Info("removed gck.local block from CoreDNS Corefile")
	return nil
}

// CoreDNSSynced reports whether the CoreDNS Corefile in the given cluster
// contains a gck.local block.
func CoreDNSSynced(ctx context.Context, clusterName string) (bool, error) {
	restCfg, err := introspectRESTConfig(clusterName)
	if err != nil {
		return false, err
	}
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return false, err
	}

	cm, err := clientset.CoreV1().ConfigMaps(coreDNSNamespace).Get(ctx, coreDNSConfigMap, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	corefile := cm.Data[coreDNSDataKey]
	return strings.Contains(corefile, markerBegin), nil
}

// resolveGatewayRecords lists Gateways and HTTPRoutes, returning a
// hostname→IP map where each HTTPRoute hostname points to its parent
// Gateway's LB IP. The LB IP is reachable from inside the cluster.
func resolveGatewayRecords(ctx context.Context, client dynamic.Interface) (map[string]string, error) {
	gwAddrs, err := listGatewayAddresses(ctx, client)
	if err != nil {
		return nil, err
	}
	if len(gwAddrs) == 0 {
		klog.Info("CoreDNS sync: no Gateway addresses found")
		return nil, nil
	}
	records, err := buildRecords(ctx, client, gwAddrs)
	if err != nil {
		return nil, err
	}
	for k, v := range records {
		klog.V(2).Infof("CoreDNS sync (gateway): %s → %s", k, v)
	}
	return records, nil
}

// resolveClusterIPs looks up each DNS record's service and returns a
// hostname→ClusterIP map. Unlike resolveServiceRecords (which uses LB IPs),
// this uses the stable ClusterIP for in-cluster routing.
func resolveClusterIPs(ctx context.Context, client kubernetes.Interface, dnsRecords []config.DNSRecord) (map[string]string, error) {
	records := make(map[string]string)
	for _, r := range dnsRecords {
		svc, err := client.CoreV1().Services(r.Namespace).Get(ctx, r.Service, metav1.GetOptions{})
		if err != nil {
			klog.Warningf("CoreDNS sync: service %s/%s not found: %v", r.Namespace, r.Service, err)
			continue
		}
		if svc.Spec.ClusterIP == "" || svc.Spec.ClusterIP == "None" {
			klog.Warningf("CoreDNS sync: service %s/%s has no ClusterIP (headless)", r.Namespace, r.Service)
			continue
		}
		records[strings.ToLower(r.Hostname)] = svc.Spec.ClusterIP
		klog.V(2).Infof("CoreDNS sync: %s → %s (service %s/%s)", r.Hostname, svc.Spec.ClusterIP, r.Namespace, r.Service)
	}
	return records, nil
}

// buildCorefileBlock generates a CoreDNS server block for the given domain
// with hosts entries for exact records and template blocks for wildcards.
func buildCorefileBlock(domain string, records map[string]string) string {
	var wildcards, exact []string
	for hostname := range records {
		if strings.HasPrefix(hostname, "*.") {
			wildcards = append(wildcards, hostname)
		} else {
			exact = append(exact, hostname)
		}
	}
	sort.Strings(wildcards)
	sort.Strings(exact)

	var b strings.Builder
	b.WriteString(markerBegin + "\n")
	b.WriteString(domain + ":53 {\n")

	for _, wc := range wildcards {
		ip := records[wc]
		suffix := strings.TrimPrefix(wc, "*")
		escaped := regexp.QuoteMeta(suffix + ".")
		b.WriteString("    template IN A {\n")
		fmt.Fprintf(&b, "        match \"^[^.]+%s$\"\n", escaped)
		fmt.Fprintf(&b, "        answer \"{{.Name}} 5 IN A %s\"\n", ip)
		b.WriteString("        fallthrough\n")
		b.WriteString("    }\n")
	}

	if len(exact) > 0 {
		b.WriteString("    hosts {\n")
		for _, h := range exact {
			fmt.Fprintf(&b, "        %s %s\n", records[h], h)
		}
		b.WriteString("        fallthrough\n")
		b.WriteString("    }\n")
	}

	b.WriteString("    cache 5\n")
	b.WriteString("}\n")
	b.WriteString(markerEnd + "\n")
	return b.String()
}

// patchCoreDNSConfigMap reads the CoreDNS ConfigMap, replaces or appends the
// gck.local block, and writes it back.
func patchCoreDNSConfigMap(ctx context.Context, client kubernetes.Interface, block string) error {
	cm, err := client.CoreV1().ConfigMaps(coreDNSNamespace).Get(ctx, coreDNSConfigMap, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("reading CoreDNS ConfigMap: %w", err)
	}

	corefile, ok := cm.Data[coreDNSDataKey]
	if !ok {
		return fmt.Errorf("CoreDNS ConfigMap has no %q key", coreDNSDataKey)
	}

	cleaned := stripMarkerBlock(corefile)
	cm.Data[coreDNSDataKey] = cleaned + block

	if _, err := client.CoreV1().ConfigMaps(coreDNSNamespace).Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("updating CoreDNS ConfigMap: %w", err)
	}

	klog.Info("patched CoreDNS Corefile with gck.local records")
	return nil
}

// stripMarkerBlock removes the text between BEGIN and END markers (inclusive)
// from the Corefile. Returns the original string if no markers are found.
func stripMarkerBlock(corefile string) string {
	beginIdx := strings.Index(corefile, markerBegin)
	if beginIdx < 0 {
		return corefile
	}
	endIdx := strings.Index(corefile, markerEnd)
	if endIdx < 0 {
		return corefile
	}
	endIdx += len(markerEnd)
	if endIdx < len(corefile) && corefile[endIdx] == '\n' {
		endIdx++
	}
	return corefile[:beginIdx] + corefile[endIdx:]
}
