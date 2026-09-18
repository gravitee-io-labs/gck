package cmd

import (
	"context"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/spf13/cobra"
	"sigs.k8s.io/cloud-provider-kind/pkg/config"
	"sigs.k8s.io/cloud-provider-kind/pkg/controller"
	"sigs.k8s.io/kind/pkg/cluster"
)

var enableGateway bool

var cpkCmd = &cobra.Command{
	Use:    "cpk",
	Short:  "Manage the cloud-provider-kind controller",
	Hidden: true,
}

var cpkServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the cloud-provider-kind controller",
	Long: `Run CPK's controller manager as a background process. It watches for
Kind clusters, reconciles LoadBalancer services and Gateway API resources
(creating Docker proxy/envoy containers as needed).

This command is started automatically by "gck create" when the lb feature
is enabled. It does not need to be invoked directly.`,
	RunE: runCPKServe,
}

func init() {
	cpkServeCmd.Flags().BoolVar(&enableGateway, "enable-gateway", false,
		"register the cloud-provider-kind GatewayClass and handle Gateway API resources")
	cpkCmd.AddCommand(cpkServeCmd)
	rootCmd.AddCommand(cpkCmd)
}

// applyCPKConfig sets the CPK globals this command runs with.
//
// IngressDefault is the one that is not obvious. CPK ships an Ingress→Gateway
// API translation controller alongside its gateway controller, and by default
// registers its own IngressClass as the cluster default — which makes it adopt
// every Ingress that names no class, including the placeholder Ingresses Helm
// charts ship (graviteeio/apim renders apim.example.com). Each adopted Ingress
// becomes an HTTPRoute named <ingress>-<sha256(host)[:10]>, and gck collects
// HTTPRoute hostnames as DNS records — so a chart's placeholder host became a
// record the DNS server cannot serve and a create that failed on a hostname
// appearing in no context. Contexts declare the HTTPRoutes they want, so an
// Ingress here has to ask for this controller by name
// (ingressClassName: cloud-provider-kind) rather than being adopted silently.
func applyCPKConfig(enableGateway bool) {
	if runtime.GOOS != "linux" {
		config.DefaultConfig.LoadBalancerConnectivity = config.Tunnel
		config.DefaultConfig.ControlPlaneConnectivity = config.Portmap
	}

	config.DefaultConfig.IngressDefault = false

	if enableGateway {
		config.DefaultConfig.GatewayReleaseChannel = config.Standard
	} else {
		config.DefaultConfig.GatewayReleaseChannel = config.Disabled
	}
}

func runCPKServe(_ *cobra.Command, _ []string) error {
	applyCPKConfig(enableGateway)

	option, err := cluster.DetectNodeProvider()
	if err != nil {
		return err
	}
	kindProvider := cluster.NewProvider(option)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	os.Stdout = nil
	os.Stderr = nil

	controller.New(kindProvider).Run(ctx)
	return nil
}
