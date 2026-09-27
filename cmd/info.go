package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/gravitee-io-labs/gck/internal/config"
	"github.com/gravitee-io-labs/gck/internal/registry"
	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "info [context...]",
	Short: "Show context details including available flags",
	Long: `Show information about a context without creating a cluster.

Name the contexts to compose as arguments, or with --from, or leave both out
to use the config file's from. Displays the composed context paths, the
component list, the available alternatives (--use-* flags, default marked),
the context flags, and the enabled features. Use this to discover what flags
a context supports before running "gck create". Pass --use-* flags to
preview the components of another alternative.`,
	Example: `  gck info my-org/product
  gck info my-org/product --use-postgres
  gck info my-org/product my-org/tool`,
	FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
	RunE:               runInfo,
}

func init() {
	rootCmd.AddCommand(infoCmd)
}

func runInfo(cmd *cobra.Command, _ []string) error {
	// Not cobra's args: see positionalArgs.
	if args := positionalArgs(os.Args, cmd.Name(), cmd.InheritedFlags(), cmd.LocalFlags()); len(args) > 0 {
		if len(fromPaths) > 0 {
			return fmt.Errorf("pass contexts as arguments or with --from, not both")
		}
		overrideFrom(args)
	}
	resolved, err := resolveContextConfig()
	if err != nil {
		return err
	}
	if resolved == nil {
		return fmt.Errorf("no context to show: name one, as in \"gck info <context>\", or set from in gck.yaml")
	}
	// Alternatives were applied at resolution; apply the plain flags too, as
	// gck create does, so the components and any requires/conflicts error
	// are the ones create would see.
	active, err := extractActiveFlags(os.Args, cmd.InheritedFlags(), cmd.LocalFlags(), resolved.Flags)
	if err != nil {
		return err
	}
	if err := registry.ApplyFlags(resolved, active, setOverrides); err != nil {
		return err
	}
	inForce := make(map[string]bool)
	for _, name := range registry.EffectiveFlags(resolved, active) {
		inForce[name] = true
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
	printInfoAlternatives(bold, resolved, inForce)
	printInfoFlags(bold, resolved, inForce)
	printInfoFeatures(bold, cfg.Features)
	printInfoUsage(bold, active)

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

// printInfoAlternatives lists the alternative groups users can choose from,
// in name order, one member per line with the default and the current
// selection marked. A group switched off by a flag in force names that flag.
// Groups pinned by a composing context are not offered and are left out.
func printInfoAlternatives(bold *color.Color, resolved *config.ResolvedContext, inForce map[string]bool) {
	var groups []string
	byGroup := make(map[string][]config.ContextFlag)
	offBy := make(map[string]string)
	nameW := 0
	for _, f := range resolved.Flags {
		if !f.IsAlternative() {
			for _, g := range f.Disables {
				if inForce[f.Name] {
					offBy[g] = f.Name
				}
			}
			continue
		}
		if f.Pinned {
			continue
		}
		if _, ok := byGroup[f.Group]; !ok {
			groups = append(groups, f.Group)
		}
		byGroup[f.Group] = append(byGroup[f.Group], f)
		if w := len(f.Name) + 2; w > nameW {
			nameW = w
		}
	}
	if len(groups) == 0 {
		return
	}
	sort.Strings(groups)

	bold.Println("Alternatives")
	fmtStr := fmt.Sprintf("  %%s %%-%ds  %%s%%s\n", nameW)
	for _, g := range groups {
		if by, off := offBy[g]; off && resolved.Selected[g] == "" {
			fmt.Printf("  %s (off: --%s)\n", g, by)
		} else {
			fmt.Printf("  %s\n", g)
		}
		for _, f := range byGroup[g] {
			mark := " "
			if resolved.Selected[g] == f.Name {
				mark = "*"
			}
			def := ""
			if f.Default {
				def = " (default)"
			}
			fmt.Printf(fmtStr, mark, "--"+f.Name, f.Description, def)
		}
	}
	fmt.Println()
}

// printInfoFlags lists the plain flags, marking the ones the selected
// alternatives already turn on, the ones they rule out, and the ones whose
// requirements are not in force.
func printInfoFlags(bold *color.Color, resolved *config.ResolvedContext, inForce map[string]bool) {
	var flags []config.ContextFlag
	impliedBy := make(map[string]string)
	for _, f := range resolved.Flags {
		if !f.IsAlternative() {
			flags = append(flags, f)
			continue
		}
		if resolved.Selected[f.Group] != f.Name {
			continue
		}
		for _, implied := range f.Implies {
			impliedBy[implied] = f.Name
		}
	}
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

	fmtStr := fmt.Sprintf("  %%-%ds  %%s%%s\n", nameW)
	for _, f := range flags {
		note := ""
		if by, ok := impliedBy[f.Name]; ok {
			note = fmt.Sprintf(" (implied by --%s)", by)
		}
		for _, r := range f.Requires {
			if !inForce[r] {
				note = fmt.Sprintf(" (needs --%s)", r)
			}
		}
		for _, c := range f.Conflicts {
			if inForce[c] {
				note = fmt.Sprintf(" (not with --%s)", c)
			}
		}
		fmt.Printf(fmtStr, "--"+f.Name, f.Description, note)
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
}

// printInfoUsage prints the gck create command for what info previewed: the
// same contexts and the context flags info was given.
func printInfoUsage(bold *color.Color, active []string) {
	if len(cfg.From) == 0 {
		return
	}
	fmt.Println()
	bold.Println("Usage")
	line := "  gck create --from " + strings.Join(cfg.From, " --from ")
	for _, f := range active {
		line += " --" + f
	}
	fmt.Println(line)
}
