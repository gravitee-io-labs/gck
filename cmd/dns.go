package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gravitee-io-labs/gck/internal/config"
	"github.com/gravitee-io-labs/gck/internal/dns"
	"github.com/spf13/cobra"
)

var dnsCmd = &cobra.Command{
	Use:    "dns",
	Short:  "Manage the local DNS server",
	Hidden: true,
}

var (
	dnsDir      string
	dnsDomain   string
	dnsAddr     string
	dnsUpstream string
)

var dnsServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the local DNS server",
	Long: `Start the local DNS server that resolves hostnames from per-cluster
record files. The server watches the record directory for changes, hot-reloads
records, and shuts itself down automatically when all record files are removed.

This command is typically started as a background process by "gck create" and does
not need to be invoked directly.`,
	RunE: runDNSServe,
}

func init() {
	dnsServeCmd.Flags().StringVar(&dnsDir, "dir", "", "path to DNS record directory (default: $GCK_HOME/dns)")
	dnsServeCmd.Flags().StringVar(&dnsDomain, "domain", config.DNSDefaultDomain, "DNS domain to serve")
	dnsServeCmd.Flags().StringVar(&dnsAddr, "addr", "", "UDP listen address (default: 127.0.0.1:<port>)")
	dnsServeCmd.Flags().StringVar(&dnsUpstream, "upstream", "8.8.8.8:53", "upstream DNS server for non-matching queries")

	dnsCmd.AddCommand(dnsServeCmd)
	rootCmd.AddCommand(dnsCmd)
}

func runDNSServe(cmd *cobra.Command, _ []string) error {
	if dnsDir == "" {
		dnsDir = filepath.Join(gckHome, "dns")
	}
	if err := os.MkdirAll(dnsDir, 0o755); err != nil {
		return fmt.Errorf("creating DNS record directory: %w", err)
	}

	if !cmd.Flags().Changed("domain") && cfg.Features.DNS != nil && cfg.Features.DNS.Domain != "" {
		dnsDomain = cfg.Features.DNS.Domain
	}

	if dnsAddr == "" {
		port := config.DNSDefaultPort
		if cfg.Features.DNS != nil && cfg.Features.DNS.Port != 0 {
			port = cfg.Features.DNS.Port
		}
		dnsAddr = fmt.Sprintf("127.0.0.1:%d", port)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return dns.Run(ctx, dns.Config{
		Dir:      dnsDir,
		Domain:   dnsDomain,
		Addr:     dnsAddr,
		Upstream: dnsUpstream,
	})
}
