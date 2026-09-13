// Package cloudprovider wraps cloud-provider-kind (CPK) to provide one-shot
// load balancer provisioning and Gateway API CRD installation for Kind clusters.
//
// During "gck create", EnsureLBs scans the cluster for Services of type LoadBalancer
// and creates Docker proxy containers (envoy) for each. These containers persist
// independently after gck exits. CleanupLBs removes them during "gck delete".
package cloudprovider

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"github.com/gravitee-io-labs/gck/internal/config"
	"github.com/gravitee-io-labs/gck/internal/privilege"
	cpkconfig "sigs.k8s.io/cloud-provider-kind/pkg/config"
	"sigs.k8s.io/cloud-provider-kind/pkg/constants"
	"sigs.k8s.io/cloud-provider-kind/pkg/container"
	"sigs.k8s.io/cloud-provider-kind/pkg/gateway"
	"sigs.k8s.io/cloud-provider-kind/pkg/loadbalancer"
	"sigs.k8s.io/cloud-provider-kind/pkg/tunnels"
	"sigs.k8s.io/kind/pkg/cluster"
)

func init() {
	ConfigurePlatformDefaults()
}

// ConfigurePlatformDefaults sets CPK's global config based on the host OS.
// On macOS (and Windows), Docker runs in a VM so container IPs are not
// directly reachable. Portmap mode enables port publishing on LB containers
// without creating CPK's internal tunnel manager (we handle tunnels ourselves
// with proper privilege escalation). On Linux, containers share the host
// network bridge so direct access works without any of this.
func ConfigurePlatformDefaults() {
	switch runtime.GOOS {
	case "darwin", "windows":
		cpkconfig.DefaultConfig.LoadBalancerConnectivity = cpkconfig.Portmap
		cpkconfig.DefaultConfig.ControlPlaneConnectivity = cpkconfig.Portmap
	}
}

// NeedsTunnels reports whether the current platform requires userspace tunnels
// to make LB container IPs reachable from the host.
func NeedsTunnels() bool {
	return runtime.GOOS == "darwin" || runtime.GOOS == "windows"
}

// EnsureLBs connects to the named Kind cluster, lists all Services of type
// LoadBalancer, and creates Docker proxy containers for each. Containers that
// are already running are left untouched. The assigned external IPs are written
// back to each Service's status so that kubectl and other tools see them.
//
// On platforms that need tunnels (macOS/Windows), call SetupLBTunnels
// separately after this function — it requires terminal access for privilege
// escalation and must run outside a spinner.
//
// Call after components have been deployed, as it operates on existing services.
func EnsureLBs(ctx context.Context, clusterName string) error {
	restCfg, err := clusterRESTConfig(clusterName)
	if err != nil {
		return err
	}
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("creating kubernetes client: %w", err)
	}

	nodeList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing nodes: %w", err)
	}
	nodes := make([]*v1.Node, len(nodeList.Items))
	for i := range nodeList.Items {
		nodes[i] = &nodeList.Items[i]
	}

	svcList, err := clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing services: %w", err)
	}

	lb := loadbalancer.NewServer()
	for i := range svcList.Items {
		svc := &svcList.Items[i]
		if svc.Spec.Type != v1.ServiceTypeLoadBalancer {
			continue
		}
		klog.V(2).Infof("ensuring load balancer for %s/%s", svc.Namespace, svc.Name)
		status, err := lb.EnsureLoadBalancer(ctx, clusterName, svc, nodes)
		if err != nil {
			return fmt.Errorf("creating load balancer for %s/%s: %w", svc.Namespace, svc.Name, err)
		}
		svc.Status.LoadBalancer = *status
		if _, err := clientset.CoreV1().Services(svc.Namespace).UpdateStatus(ctx, svc, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("updating load balancer status for %s/%s: %w", svc.Namespace, svc.Name, err)
		}
	}

	return nil
}

// SetupLBTunnels adds LB container IPs to the host loopback interface and
// starts userspace tunnels that forward traffic from the LB IP:servicePort
// to localhost:dockerMappedPort. The ifconfig commands are batched into a
// single privilege escalation prompt (sudo with Touch ID on macOS).
//
// This function interacts with the terminal for authentication and must NOT
// be called inside a spinner.
func SetupLBTunnels(clusterName string) error {
	names, err := lbContainerNames(clusterName)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return nil
	}

	var ifconfigCmds []string
	for _, name := range names {
		ipv4, _, err := container.IPs(name)
		if err != nil {
			klog.V(2).Infof("skipping tunnel for %s: %v", name, err)
			continue
		}
		if ipv4 == "" || net.ParseIP(ipv4) == nil {
			klog.V(2).Infof("skipping tunnel for %s: invalid IP %q", name, ipv4)
			continue
		}
		ifconfigCmds = append(ifconfigCmds, fmt.Sprintf("ifconfig lo0 alias %s netmask 255.255.255.255", ipv4))
	}

	if len(ifconfigCmds) > 0 {
		batch := strings.Join(ifconfigCmds, " && ")
		if err := privilege.Elevate(batch); err != nil {
			return fmt.Errorf("adding LB IPs to loopback: %w", err)
		}
	}

	tm := tunnels.NewTunnelManager()
	for _, name := range names {
		if err := tm.SetupTunnels(name); err != nil {
			klog.Warningf("tunnel setup for %s: %v", name, err)
		}
	}

	return nil
}

// CleanupLBTunnels removes IP aliases from the host loopback interface for
// all LB containers of the given cluster. Must be called before CleanupLBs
// (which deletes the containers) because it needs the container IPs.
//
// This function interacts with the terminal for authentication and must NOT
// be called inside a spinner.
func CleanupLBTunnels(clusterName string) {
	names, err := lbContainerNames(clusterName)
	if err != nil {
		klog.Warningf("listing LB containers for tunnel cleanup: %v", err)
		return
	}
	if len(names) == 0 {
		return
	}

	var ifconfigCmds []string
	for _, name := range names {
		ipv4, _, err := container.IPs(name)
		if err != nil {
			klog.Warningf("getting IP for container %s: %v", name, err)
			continue
		}
		if ipv4 != "" {
			ifconfigCmds = append(ifconfigCmds, fmt.Sprintf("ifconfig lo0 -alias %s", ipv4))
		}
	}

	if len(ifconfigCmds) > 0 {
		batch := strings.Join(ifconfigCmds, " && ")
		if err := privilege.Elevate(batch); err != nil {
			klog.Warningf("removing LB IPs from loopback: %v", err)
		}
	}
}

// CleanupLBs removes all Docker proxy containers associated with the given
// Kind cluster. Containers are identified by CPK's cluster label.
func CleanupLBs(clusterName string) error {
	names, err := lbContainerNames(clusterName)
	if err != nil {
		return fmt.Errorf("listing LB containers: %w", err)
	}
	for _, name := range names {
		klog.V(2).Infof("removing LB container %s", name)
		if err := container.Delete(name); err != nil {
			klog.Warningf("deleting LB container %s (may already be removed): %v", name, err)
		}
	}
	return nil
}

// InstallGatewayCRDs installs the Gateway API Custom Resource Definitions into
// the named Kind cluster. The channel selects the CRD bundle ("standard" or
// "experimental").
func InstallGatewayCRDs(ctx context.Context, clusterName string, channel config.GatewayChannel) error {
	restCfg, err := clusterRESTConfig(clusterName)
	if err != nil {
		return err
	}
	mgr, err := gateway.NewCRDManager(restCfg)
	if err != nil {
		return fmt.Errorf("creating Gateway API CRD manager: %w", err)
	}
	return mgr.InstallCRDs(ctx, cpkconfig.GatewayReleaseChannel(channel))
}

// WaitForGatewayCRDs polls until the Gateway API CRDs required by gck-gateway
// are established, or until timeout. The gck-gateway manifest needs
// gateways.gateway.networking.k8s.io; httproutes is included because contexts
// that declare HTTPRoute resources are applied in the same installComponents pass.
func WaitForGatewayCRDs(ctx context.Context, clusterName string, timeout time.Duration) error {
	restCfg, err := clusterRESTConfig(clusterName)
	if err != nil {
		return err
	}
	crdClient, err := apiextensionsclient.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("creating apiextensions client: %w", err)
	}

	required := []string{
		"gateways.gateway.networking.k8s.io",
		"httproutes.gateway.networking.k8s.io",
	}

	for _, name := range required {
		if err := wait.PollUntilContextTimeout(ctx, 1*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			crd, err := crdClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				klog.V(2).Infof("waiting for CRD %s: %v", name, err)
				return false, nil
			}
			for _, c := range crd.Status.Conditions {
				if c.Type == apiextensionsv1.Established && c.Status == apiextensionsv1.ConditionTrue {
					return true, nil
				}
			}
			return false, nil
		}); err != nil {
			return fmt.Errorf("CRD %s not established within %s: %w", name, timeout, err)
		}
	}
	return nil
}

// WaitForGatewayClass polls until the named GatewayClass exists and is
// Accepted, or until timeout.
//
// This is the proof that the controller is not merely running but reconciling.
// Starting it only told us fork/exec succeeded; registering the GatewayClass is
// the first thing it does once it can watch the CRDs. A timeout here means the
// controller is alive and not working — worth saying in seconds rather than
// discovering it 90s later as a Gateway with no address and no explanation.
func WaitForGatewayClass(ctx context.Context, clusterName, className string, timeout time.Duration) error {
	restCfg, err := clusterRESTConfig(clusterName)
	if err != nil {
		return err
	}
	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("creating dynamic client: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "gatewayclasses",
	}

	lastReason := "not registered"
	if err := wait.PollUntilContextTimeout(ctx, 1*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		gc, err := dynClient.Resource(gvr).Get(ctx, className, metav1.GetOptions{})
		if err != nil {
			lastReason = fmt.Sprintf("not registered (%v)", err)
			klog.V(2).Infof("waiting for GatewayClass %s: %v", className, err)
			return false, nil
		}
		conditions, _, _ := unstructured.NestedSlice(gc.Object, "status", "conditions")
		for _, c := range conditions {
			m, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			condType, _, _ := unstructured.NestedString(m, "type")
			if condType != "Accepted" {
				continue
			}
			status, _, _ := unstructured.NestedString(m, "status")
			if status == "True" {
				return true, nil
			}
			reason, _, _ := unstructured.NestedString(m, "reason")
			message, _, _ := unstructured.NestedString(m, "message")
			lastReason = fmt.Sprintf("registered but not Accepted (reason=%s, message=%s)", reason, message)
			return false, nil
		}
		lastReason = "registered but reports no Accepted condition"
		return false, nil
	}); err != nil {
		return fmt.Errorf(
			"GatewayClass %q not accepted within %s: %s; "+
				"the cloud provider controller is not reconciling Gateway API resources",
			className, timeout, lastReason)
	}
	return nil
}

// DescribeGateways returns a human-readable status line per Gateway, for
// diagnostics when no Gateway ever reports an address. Best-effort: it only
// ever runs to explain a failure, so every error becomes part of the message.
func DescribeGateways(ctx context.Context, clusterName string) string {
	restCfg, err := clusterRESTConfig(clusterName)
	if err != nil {
		return fmt.Sprintf("(could not reach the cluster: %v)", err)
	}
	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return fmt.Sprintf("(could not build a client: %v)", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "gateways",
	}
	list, err := dynClient.Resource(gvr).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Sprintf("(could not list Gateways: %v)", err)
	}
	if len(list.Items) == 0 {
		return "  (no Gateway objects exist in the cluster)"
	}

	var b strings.Builder
	for i := range list.Items {
		gw := &list.Items[i]
		class, _, _ := unstructured.NestedString(gw.Object, "spec", "gatewayClassName")
		fmt.Fprintf(&b, "  %s/%s (class %s):", gw.GetNamespace(), gw.GetName(), class)

		conditions, _, _ := unstructured.NestedSlice(gw.Object, "status", "conditions")
		if len(conditions) == 0 {
			b.WriteString(" no status conditions — nothing has reconciled it\n")
			continue
		}
		b.WriteString("\n")
		for _, c := range conditions {
			m, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			condType, _, _ := unstructured.NestedString(m, "type")
			status, _, _ := unstructured.NestedString(m, "status")
			reason, _, _ := unstructured.NestedString(m, "reason")
			message, _, _ := unstructured.NestedString(m, "message")
			fmt.Fprintf(&b, "    %s=%s reason=%s %s\n", condType, status, reason, message)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// ListLBIPs returns a map of LB container name to its IPv4 address for the
// given cluster. Containers without an IP are omitted.
func ListLBIPs(clusterName string) (map[string]string, error) {
	names, err := lbContainerNames(clusterName)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(names))
	for _, name := range names {
		ipv4, _, err := container.IPs(name)
		if err != nil || ipv4 == "" {
			continue
		}
		result[name] = ipv4
	}
	return result, nil
}

func lbContainerNames(clusterName string) ([]string, error) {
	label := fmt.Sprintf("%s=%s", constants.NodeCCMLabelKey, clusterName)
	return container.ListByLabel(label)
}

func clusterRESTConfig(clusterName string) (*rest.Config, error) {
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
