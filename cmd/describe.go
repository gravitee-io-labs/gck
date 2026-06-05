package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/gravitee-io-labs/gck/internal/cloudprovider"
	"github.com/gravitee-io-labs/gck/internal/config"
	"github.com/gravitee-io-labs/gck/internal/dns"
	"github.com/gravitee-io-labs/gck/internal/state"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var describeCmd = &cobra.Command{
	Use:         "describe [name]",
	Short:       "Show detailed information about a cluster",
	Annotations: map[string]string{"gck_skip_config": "true"},
	Args:        cobra.MaximumNArgs(1),
	RunE:        runDescribe,
}

func init() {
	rootCmd.AddCommand(describeCmd)
}

func runDescribe(_ *cobra.Command, args []string) error {
	stateDir := filepath.Join(gckHome, "clusters")

	cs, err := resolveDescribeTarget(stateDir, args)
	if err != nil {
		return err
	}

	bold := color.New(color.Bold)

	bold.Println("Cluster")
	fmt.Printf("  Name:    %s\n", cs.Name)
	fmt.Printf("  Created: %s\n", cs.CreatedAt.Format("2006-01-02 15:04"))
	if len(cs.From) == 1 {
		fmt.Printf("  From:    %s\n", cs.From[0])
	} else if len(cs.From) > 1 {
		fmt.Println("  From:")
		for _, f := range cs.From {
			fmt.Printf("    - %s\n", f)
		}
	} else {
		fmt.Println("  From:    -")
	}
	if len(cs.Flags) > 0 {
		flagStrs := make([]string, len(cs.Flags))
		for i, f := range cs.Flags {
			flagStrs[i] = "--" + f
		}
		fmt.Printf("  Flags:   %s\n", strings.Join(flagStrs, ", "))
	}
	fmt.Println()

	printDescribeFeatures(bold, cs.Features)
	printDescribeLBs(bold, cs.Name)
	printDescribeDNS(bold, cs.Features)
	return nil
}

func resolveDescribeTarget(stateDir string, args []string) (*state.ClusterState, error) {
	if len(args) > 0 {
		return state.Load(stateDir, args[0])
	}

	names, err := state.List(stateDir)
	if err != nil {
		return nil, fmt.Errorf("listing cluster states: %w", err)
	}

	switch len(names) {
	case 0:
		return nil, fmt.Errorf("no clusters found; create one with \"gck create\"")
	case 1:
		return state.Load(stateDir, names[0])
	default:
		return nil, fmt.Errorf("multiple clusters found; specify a name or run \"gck list\" to see them")
	}
}

func printDescribeFeatures(bold *color.Color, features config.FeaturesConfig) {
	bold.Println("Features")

	lbEnabled := features.LB != nil && features.LB.Enabled
	gwEnabled := features.Gateway != nil && features.Gateway.Enabled
	dnsEnabled := features.DNS != nil && features.DNS.Enabled

	fmt.Printf("  lb:      %s\n", enabledStr(lbEnabled))
	gwLine := enabledStr(gwEnabled)
	if gwEnabled && features.Gateway.Channel != "" {
		gwLine += fmt.Sprintf(" (channel: %s)", features.Gateway.Channel)
	}
	fmt.Printf("  gateway: %s\n", gwLine)

	dnsLine := enabledStr(dnsEnabled)
	if dnsEnabled {
		domain := features.DNS.Domain
		if domain == "" {
			domain = config.DNSDefaultDomain
		}
		port := features.DNS.Port
		if port == 0 {
			port = config.DNSDefaultPort
		}
		dnsLine += fmt.Sprintf(" (domain: %s, port: %d)", domain, port)
	}
	fmt.Printf("  dns:     %s\n", dnsLine)
	fmt.Println()
}

func printDescribeLBs(bold *color.Color, clusterName string) {
	bold.Println("Load Balancers")
	ips, err := cloudprovider.ListLBIPs(clusterName)
	if err != nil {
		color.Yellow("  could not list LB containers: %v", err)
		fmt.Println()
		return
	}
	if len(ips) == 0 {
		fmt.Println("  (none)")
	} else {
		for name, ip := range ips {
			fmt.Printf("  %s → %s\n", name, ip)
		}
	}
	fmt.Println()
}

func printDescribeDNS(bold *color.Color, features config.FeaturesConfig) {
	bold.Println("DNS")

	dnsEnabled := features.DNS != nil && features.DNS.Enabled
	if !dnsEnabled {
		fmt.Println("  (disabled)")
		return
	}

	domain := features.DNS.Domain
	if domain == "" {
		domain = config.DNSDefaultDomain
	}
	port := features.DNS.Port
	if port == 0 {
		port = config.DNSDefaultPort
	}

	resolverOK := dns.ResolverConfigured(domain, port)
	if resolverOK {
		color.Blue("  resolver: configured for %s", domain)
	} else {
		color.Yellow("  resolver: not configured (run \"gck setup dns\")")
	}

	serverRunning := isDNSServerRunning()
	if serverRunning {
		color.Blue("  server:   running on 127.0.0.1:%d", port)
	} else {
		color.Yellow("  server:   not running")
	}

	dnsDir := filepath.Join(gckHome, "dns")
	printDNSRecords(dnsDir)
}

func printDNSRecords(dnsDir string) {
	entries, err := os.ReadDir(dnsDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("  records:  (none)")
			return
		}
		color.Yellow("  records:  could not read %s: %v", dnsDir, err)
		return
	}

	var totalRecords int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dnsDir, e.Name()))
		if err != nil {
			continue
		}
		var rf dns.RecordFile
		if err := json.Unmarshal(data, &rf); err != nil {
			continue
		}
		cluster := strings.TrimSuffix(e.Name(), ".json")
		for host, ip := range rf.Records {
			if totalRecords == 0 {
				fmt.Println("  records:")
			}
			fmt.Printf("    %s → %s (%s)\n", host, ip, cluster)
			totalRecords++
		}
	}
	if totalRecords == 0 {
		fmt.Println("  records:  (none)")
	}
}

func isDNSServerRunning() bool {
	pidPath := filepath.Join(gckHome, "pids", "dns.pid")
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func enabledStr(enabled bool) string {
	if enabled {
		return color.BlueString("enabled")
	}
	return "disabled"
}
