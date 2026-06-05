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

func runCPKServe(_ *cobra.Command, _ []string) error {
	if runtime.GOOS != "linux" {
		config.DefaultConfig.LoadBalancerConnectivity = config.Tunnel
		config.DefaultConfig.ControlPlaneConnectivity = config.Portmap
	}

	if enableGateway {
		config.DefaultConfig.GatewayReleaseChannel = config.Standard
	} else {
		config.DefaultConfig.GatewayReleaseChannel = config.Disabled
	}

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
