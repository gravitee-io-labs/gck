package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
			cfg.From = fromPaths
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

	acc := &config.ResolvedContext{}
	for _, ref := range cfg.From {
		resolver := registry.NewResolver(regURL, gckHome, setOverrides)
		resolved, err := resolver.Resolve(context.Background(), ref)
		if err != nil {
			return nil, fmt.Errorf("resolving context %q: %w", ref, err)
		}
		if len(cfg.From) == 1 && resolved.Abstract {
			return nil, fmt.Errorf("context %q is abstract and cannot be deployed directly; compose it via 'from' in another context", ref)
		}
		registry.MergeInto(acc, resolved)
	}

	cfg.Kind.MergeWithContext(&acc.Kind)
	cfg.Features = config.MergeFeatures(acc.Features, cfg.Features)
	cfg.Images = config.MergeImages(acc.Images, cfg.Images)
	ctxResolved = acc
	return acc, nil
}

// applyContextFlags extracts context-specific flags from the CLI arguments
// and applies their patch files to the resolved context. It returns the list
// of active flag names (for use in notes rendering) and any error. Returns
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
	return active, nil
}

// extractActiveFlags walks args looking for --flag-name tokens that are not
// known Cobra flags and match one of the available context flags. It returns
// the list of active flag names or an error if an unrecognized flag is found.
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
				known = append(known, "--"+f.Name)
			}
			return nil, fmt.Errorf("unknown context flag --%s (available: %s)", name, strings.Join(known, ", "))
		}
		active = append(active, name)
	}
	return active, nil
}
