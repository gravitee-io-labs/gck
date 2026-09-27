package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gravitee-io-labs/gck/internal/config"
	"github.com/gravitee-io-labs/gck/internal/registry"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Version is set by main from the build-time ldflags value.
var Version string

// DefaultConfigData holds the embedded gck.yaml from the project root,
// set by main before Execute().
var DefaultConfigData []byte

// SchemaData holds the embedded JSON Schema for gck.yaml,
// set by main before Execute().
var SchemaData []byte

var (
	cfgFile      string
	registryURL  string
	fromPaths    []string
	setValues    []string
	setOverrides map[string]string
	cfg          *config.Config
	gckHome      string
)

// parseSetValues converts the raw --set flag values (each "key=value") into a
// map. It returns an error if any entry is missing the '=' separator.
func parseSetValues(raw []string) (map[string]string, error) {
	m := make(map[string]string, len(raw))
	for _, entry := range raw {
		k, v, ok := strings.Cut(entry, "=")
		if !ok {
			return nil, fmt.Errorf("invalid --set value %q: expected key=value", entry)
		}
		m[k] = v
	}
	return m, nil
}

var rootCmd = &cobra.Command{
	Use:   "gck",
	Short: "Provisions ready-made stacks on local Kubernetes clusters in one command",
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		gckHome = os.Getenv("GCK_HOME")
		if gckHome == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("determining user home directory: %w", err)
			}
			gckHome = filepath.Join(home, ".gck")
		}

		parsed, err := parseSetValues(setValues)
		if err != nil {
			return err
		}
		setOverrides = parsed

		if cmd.Annotations["gck_skip_config"] == "true" {
			return nil
		}
		cfg, err = resolveConfig(cfgFile)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		if registryURL != "" {
			cfg.Registry = registryURL
		}
		if len(fromPaths) > 0 {
			overrideFrom(fromPaths)
		}
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "path to config file (default: ./gck.yaml or ~/.gck/gck.yaml)")
	rootCmd.PersistentFlags().StringVar(&registryURL, "registry", "", "registry URL to use (overrides config file)")
	rootCmd.PersistentFlags().StringSliceVar(&fromPaths, "from", nil, "context paths to compose (repeatable, overrides config file)")
	rootCmd.PersistentFlags().StringSliceVar(&setValues, "set", nil, "set template variables (key=value, repeatable)")
}

// overrideFrom replaces the config file's from with contexts named on the
// command line. The file's use: goes with it: it selects members of the
// file's contexts, which the command line's may not have.
func overrideFrom(paths []string) {
	cfg.From = paths
	cfg.Use = nil
}

// Execute runs the root command.
func Execute() error {
	rootCmd.Version = Version
	return rootCmd.Execute()
}

// resolveConfig loads the configuration using layered merging:
//  1. Load $gckHome/gck.yaml as the base config (if it exists).
//  2. If --config is given, load and merge on top; otherwise if ./gck.yaml
//     exists, load and merge on top.
//  3. Apply embedded defaults to fill any remaining gaps.
func resolveConfig(explicit string) (*config.Config, error) {
	basePath := filepath.Join(gckHome, "gck.yaml")
	var base *config.Config
	if fileExists(basePath) {
		var err error
		base, err = config.Load(basePath, setOverrides)
		if err != nil {
			return nil, fmt.Errorf("loading base config %s: %w", basePath, err)
		}
	} else {
		base = &config.Config{}
		base.Kind.ApplyDefaults()
	}

	var projectCfg *config.Config
	switch {
	case explicit != "":
		var err error
		projectCfg, err = config.Load(explicit, setOverrides)
		if err != nil {
			return nil, err
		}
	case fileExists("gck.yaml"):
		var err error
		projectCfg, err = config.Load("gck.yaml", setOverrides)
		if err != nil {
			return nil, err
		}
	}

	if projectCfg != nil {
		config.Merge(base, projectCfg)
	}

	config.ApplyDefaults(base, DefaultConfigData)
	return base, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

var (
	ctxResolved   *config.ResolvedContext
	ctxResolvedOK bool
)

func resetContextConfigCache() {
	ctxResolved = nil
	ctxResolvedOK = false
}

// resolveContextConfig resolves the registry contexts (if configured) and
// merges their Kind, Features and Images settings into the global cfg.
// When multiple from entries are provided they are resolved left-to-right
// and merged using the same accumulator pattern as registry-level
// composition.  The returned ResolvedContext is nil when no
// registry/context is set.
//
// The result is memoized so that multiple commands can call this safely
// without double-merging into cfg.
func resolveContextConfig() (*config.ResolvedContext, error) {
	if ctxResolvedOK {
		return ctxResolved, nil
	}
	ctxResolvedOK = true

	if cfg.Registry == "" || len(cfg.From) == 0 {
		return nil, nil
	}
	regURL := cfg.Registry
	if strings.HasPrefix(regURL, "file://") {
		path := strings.TrimPrefix(regURL, "file://")
		if abs, err := filepath.Abs(path); err == nil {
			regURL = "file://" + abs
		}
	}

	cliUse, cliFlags := extractCLIFlags(os.Args)
	use := append(append([]string{}, cfg.Use...), inheritedUse...)
	ctx := registry.WithUse(registry.WithConfiguredUse(registry.WithSavedUse(context.Background(), inheritedSelected), use), cliUse)
	ctx = registry.WithFlags(ctx, append(append([]string{}, inheritedFlags...), cliFlags...))

	acc := &config.ResolvedContext{}
	for _, ref := range cfg.From {
		resolver := registry.NewResolver(regURL, gckHome, setOverrides)
		resolved, err := resolver.Resolve(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("resolving context %q: %w", ref, err)
		}
		if len(cfg.From) == 1 && resolved.Abstract {
			return nil, fmt.Errorf("context %q is abstract and cannot be deployed directly; compose it via 'from' in another context", ref)
		}
		registry.MergeInto(acc, resolved)
	}
	if err := checkConfigUse(cfg.Use, acc); err != nil {
		return nil, err
	}

	cfg.Kind.MergeWithContext(&acc.Kind)
	cfg.Features = config.MergeFeatures(acc.Features, cfg.Features)
	cfg.Images = config.MergeImages(acc.Images, cfg.Images)
	ctxResolved = acc
	return acc, nil
}

// inheritedUse and inheritedFlags hold the alternatives and plain flags a
// patched cluster was created with, so the context re-resolves to the same
// composition. inheritedSelected holds the member of each group, when the
// state records it; inheritedUse is then empty.
var (
	inheritedUse      []string
	inheritedFlags    []string
	inheritedSelected map[string]string
)

// extractCLIFlags returns the --name tokens passed on the command line, split
// into alternatives (--use-*) and everything else. Both must be known before
// the context is resolved: alternatives are applied during resolution, and a
// plain flag can switch an alternative group off (--disable-metrics).
// Resolvers only act on names that match a flag of the context, so Cobra's own
// flags in the second list are harmless; unknown names are rejected later by
// extractActiveFlags.
func extractCLIFlags(args []string) (use, plain []string) {
	for _, arg := range args {
		if arg == "--" {
			break
		}
		name, ok := strings.CutPrefix(arg, "--")
		if !ok || name == "" {
			continue
		}
		if idx := strings.IndexByte(name, '='); idx >= 0 {
			name = name[:idx]
		}
		if strings.HasPrefix(name, registry.AlternativePrefix) {
			use = append(use, name)
		} else {
			plain = append(plain, name)
		}
	}
	return use, plain
}

// checkConfigUse verifies that the alternatives listed in the user's
// gck.yaml use: block exist in the resolved composition.
func checkConfigUse(use []string, resolved *config.ResolvedContext) error {
	for _, n := range use {
		name := registry.AlternativeFlagName(n)
		found := false
		for _, f := range resolved.Flags {
			if f.IsAlternative() && f.Name == name {
				found = true
				break
			}
		}
		if !found {
			var members []string
			for _, f := range resolved.Flags {
				if f.IsAlternative() {
					if m := strings.TrimPrefix(f.Name, "use-"); !slices.Contains(members, m) {
						members = append(members, m)
					}
				}
			}
			if len(members) == 0 {
				return fmt.Errorf("use: %s: the context has no alternatives", n)
			}
			return fmt.Errorf("use: %s matches no alternative of the context (available: %s)", n, strings.Join(members, ", "))
		}
	}
	return nil
}

// applyContextFlags extracts context-specific flags from the CLI arguments
// and applies their patch files to the resolved context. It returns the flags
// in force -- the selected alternatives, defaults included, followed by the
// active plain flags -- for notes rendering and cluster state. Returns
// nil, nil when no context flags are relevant.
func applyContextFlags(cmd *cobra.Command, resolved *config.ResolvedContext) ([]string, error) {
	if resolved == nil || len(resolved.Flags) == 0 {
		return nil, nil
	}
	active, err := extractActiveFlags(os.Args, cmd.InheritedFlags(), cmd.LocalFlags(), resolved.Flags)
	if err != nil {
		return nil, err
	}
	if err := registry.ApplyFlags(resolved, active, setOverrides); err != nil {
		return nil, err
	}
	return registry.EffectiveFlags(resolved, active), nil
}

// extractActiveFlags walks args looking for --flag-name tokens that are not
// known Cobra flags and match one of the available context flags. It returns
// the list of active flag names or an error if an unrecognized flag is found.
// positionalArgs returns the non-flag arguments that follow the subcommand
// in argv. Commands that accept context flags whitelist unknown flags, and
// pflag then takes the word after an unknown flag as its value, so
// "--use-x ctx" would lose ctx. Context flags never take a value: only a known
// flag that does (--from, --set, ...) consumes the next word.
func positionalArgs(argv []string, subcommand string, inherited, local *pflag.FlagSet) []string {
	start := -1
	for i := 1; i < len(argv); i++ {
		if argv[i] == subcommand {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return nil
	}
	var out []string
	for i := start; i < len(argv); i++ {
		arg := argv[i]
		if arg == "--" {
			return append(out, argv[i+1:]...)
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			out = append(out, arg)
			continue
		}
		name := strings.TrimLeft(arg, "-")
		if strings.Contains(name, "=") {
			continue
		}
		var f *pflag.Flag
		if strings.HasPrefix(arg, "--") {
			f = lookupFlag(inherited, local, name)
		} else {
			f = lookupShorthand(inherited, local, name)
		}
		if f != nil && f.NoOptDefVal == "" && f.Value.Type() != "bool" {
			i++ // the flag's value
		}
	}
	return out
}

func lookupFlag(inherited, local *pflag.FlagSet, name string) *pflag.Flag {
	if f := local.Lookup(name); f != nil {
		return f
	}
	return inherited.Lookup(name)
}

func lookupShorthand(inherited, local *pflag.FlagSet, name string) *pflag.Flag {
	if len(name) != 1 {
		return nil
	}
	if f := local.ShorthandLookup(name); f != nil {
		return f
	}
	return inherited.ShorthandLookup(name)
}

func extractActiveFlags(args []string, inherited, local *pflag.FlagSet, available []config.ContextFlag) ([]string, error) {
	availableByName := make(map[string]bool, len(available))
	for _, f := range available {
		availableByName[f.Name] = true
	}

	var active []string
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		name := strings.TrimPrefix(arg, "--")
		if idx := strings.IndexByte(name, '='); idx >= 0 {
			name = name[:idx]
		}
		if name == "" {
			continue
		}
		if inherited.Lookup(name) != nil || local.Lookup(name) != nil {
			continue
		}
		if name == "help" || name == "version" {
			continue
		}
		if !availableByName[name] {
			var known []string
			for _, f := range available {
				if !slices.Contains(known, "--"+f.Name) {
					known = append(known, "--"+f.Name)
				}
			}
			return nil, fmt.Errorf("unknown context flag --%s (available: %s)", name, strings.Join(known, ", "))
		}
		active = append(active, name)
	}
	return active, nil
}
