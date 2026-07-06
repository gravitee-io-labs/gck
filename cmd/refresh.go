package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gravitee-io-labs/gck/internal/config"
	"github.com/gravitee-io-labs/gck/internal/dns"
	"github.com/gravitee-io-labs/gck/internal/logger"
	"github.com/spf13/cobra"
)

var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Re-collect external resources",
}

var refreshDNSCmd = &cobra.Command{
	Use:   "dns",
	Short: "Re-collect DNS records from the cluster",
	Long: `Re-run DNS introspection against the current Kind cluster. This picks up
Gateways and LoadBalancer services that were created after "gck create" finished.
The running DNS server hot-reloads the updated record files automatically.`,
	RunE: runDNSRefresh,
}

func init() {
	refreshCmd.AddCommand(refreshDNSCmd)
	rootCmd.AddCommand(refreshCmd)
}

const refreshPollTimeout = 30 * time.Second

func runDNSRefresh(_ *cobra.Command, _ []string) error {
	if _, err := resolveContextConfig(); err != nil {
		return err
	}

	dnsDir := filepath.Join(gckHome, "dns")
	if err := os.MkdirAll(dnsDir, 0o755); err != nil {
		return fmt.Errorf("creating DNS record directory: %w", err)
	}

	clusterName := cfg.Kind.Name
	var dnsRecords []config.DNSRecord
	if cfg.Features.DNS != nil && cfg.Features.DNS.Records != nil {
		dnsRecords = cfg.Features.DNS.Records
	}

	introspectGateway := cfg.Features.Gateway != nil && cfg.Features.Gateway.Enabled
	ctx := context.Background()
	if err := dns.IntrospectCluster(ctx, clusterName, dnsDir, refreshPollTimeout, introspectGateway, dnsRecords); err != nil {
		return fmt.Errorf("introspecting cluster %q: %w", clusterName, err)
	}

	logger.Success("DNS records refreshed for cluster %q", clusterName)

	if err := ensureDNSServerRunning(cfg); err != nil {
		logger.Warn("failed to start DNS server: %v", err)
	}

	domain := config.DNSDefaultDomain
	if cfg.Features.DNS != nil && cfg.Features.DNS.Domain != "" {
		domain = cfg.Features.DNS.Domain
	}
	if err := dns.SyncCoreDNS(ctx, clusterName, domain, dnsRecords, introspectGateway); err != nil {
		logger.Warn("failed to sync in-cluster DNS: %v", err)
	} else {
		logger.Success("In-cluster DNS synced for cluster %q", clusterName)
	}

	return nil
}
