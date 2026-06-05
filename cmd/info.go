package cmd

import (
	"fmt"
	"strings"

	"github.com/gravitee-io-labs/gck/internal/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show context details including available flags",
	Long: `Show information about the resolved context without creating a cluster.

Displays the composition chain, component list, available context flags,
and enabled features. Use this to discover what flags a context supports
before running "gck create".`,
	RunE: runInfo,
}

func init() {
	rootCmd.AddCommand(infoCmd)
}

func runInfo(_ *cobra.Command, _ []string) error {
	resolved, err := resolveContextConfig()
	if err != nil {
		return err
	}
	if resolved == nil {
		return fmt.Errorf("no context configured; set registry and from in gck.yaml or use --registry and --from")
	}

	bold := color.New(color.Bold)

	bold.Println("Context")
	if len(cfg.From) == 1 {
		fmt.Printf("  Path: %s\n", cfg.From[0])
	} else if len(cfg.From) > 1 {
		fmt.Println("  Paths:")
		for _, f := range cfg.From {
			fmt.Printf("    - %s\n", f)
		}
	}
	fmt.Println()

	printInfoComponents(bold, resolved.Components)
	printInfoFlags(bold, resolved.Flags)
	printInfoFeatures(bold, cfg.Features)

	return nil
}

func printInfoComponents(bold *color.Color, components []config.Component) {
	var enabled []string
	for _, c := range components {
		if c.IsEnabled() {
			enabled = append(enabled, c.Name)
		}
	}
	if len(enabled) == 0 {
		return
	}
	bold.Println("Components")
	for _, name := range enabled {
		fmt.Printf("  - %s\n", name)
	}
	fmt.Println()
}

func printInfoFlags(bold *color.Color, flags []config.ContextFlag) {
	if len(flags) == 0 {
		return
	}

	bold.Println("Flags")

	nameW := 0
	for _, f := range flags {
		w := len(f.Name) + 2 // +2 for the "--" prefix
		if w > nameW {
			nameW = w
		}
	}

	fmtStr := fmt.Sprintf("  %%-%ds  %%s\n", nameW)
	for _, f := range flags {
		fmt.Printf(fmtStr, "--"+f.Name, f.Description)
	}
	fmt.Println()
}

func printInfoFeatures(bold *color.Color, features config.FeaturesConfig) {
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

	if len(cfg.From) > 0 {
		fmt.Println()
		bold.Println("Usage")
		example := fmt.Sprintf("  gck create --from %s", strings.Join(cfg.From, " --from "))
		fmt.Println(example)
	}
}
