package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gravitee-io-labs/gck/internal/cache"
	"github.com/gravitee-io-labs/gck/internal/config"
	"github.com/gravitee-io-labs/gck/internal/installer"
	"github.com/gravitee-io-labs/gck/internal/kind"
	"github.com/gravitee-io-labs/gck/internal/logger"
	"github.com/gravitee-io-labs/gck/internal/registry"
	"github.com/gravitee-io-labs/gck/internal/state"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
)

var patchClusterName string
var patchDryRun bool
var patchSkipPreload bool

var patchCmd = &cobra.Command{
	Use:   "patch [patch-file]",
	Short: "Patch a running cluster by upgrading components with overrides",
	Long: `Patch upgrades components on a running Kind cluster.

There are two ways to use patch:

  1. With a patch file — merges the file into the resolved context and upgrades
     only the components listed in the file:

       gck patch upgrade.yaml

  2. With --set only — re-renders the resolved context with new template variable
     values and upgrades all components:

       gck patch --set imageTag=4.11.0

Both modes can be combined: a patch file with --set overrides.`,
	FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
	Args:               cobra.MaximumNArgs(1),
	RunE:               runPatch,
}

func init() {
	patchCmd.Flags().StringVar(&patchClusterName, "name", "", "name of the cluster to patch (default: from config)")
	patchCmd.Flags().BoolVar(&patchDryRun, "dry-run", false, "show what would change without applying")
	patchCmd.Flags().BoolVar(&patchSkipPreload, "skip-preload", false, "skip image preloading even when images.preload is configured")
	rootCmd.AddCommand(patchCmd)
}

func runPatch(cmd *cobra.Command, args []string) error {
	start := time.Now()

	hasPatchFile := len(args) == 1
	if !hasPatchFile && len(setOverrides) == 0 {
		return fmt.Errorf("patch requires either a patch file or --set overrides (or both)")
	}

	// Inherit the cluster's create-time context (from, registry, --set vars and
	// flags) so a patch only needs to express the deltas. Patch-time inputs win,
	// and a cluster with no saved state (e.g. created by an older gck) behaves as
	// before.
	inheritName := patchClusterName
	if inheritName == "" {
		inheritName = cfg.Kind.Name
	}
	inherited := inheritClusterState(inheritName)

	resolved, err := resolveContextConfig()
	if err != nil {
		return err
	}
	if resolved == nil {
		return fmt.Errorf("no registry context configured; patch requires a resolved context (set registry and from in gck.yaml or via flags)")
	}

	if err := applyPatchFlags(cmd, resolved, inherited); err != nil {
		return err
	}

	clusterName := patchClusterName
	if clusterName == "" {
		clusterName = cfg.Kind.Name
	}
	if clusterName == "" {
		return fmt.Errorf("cannot determine cluster name; use --name or set kind.name in config")
	}

	exists, err := kind.Exists(clusterName)
	if err != nil {
		return fmt.Errorf("checking cluster %q: %w", clusterName, err)
	}
	if !exists {
		return fmt.Errorf("cluster %q not found; create it first with \"gck create\"", clusterName)
	}

	logDir := filepath.Join(gckHome, "logs")
	if len(cfg.From) > 0 {
		logDir = filepath.Join(logDir, strings.Join(cfg.From, "_"))
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("creating log directory %s: %w", logDir, err)
	}
	logFile, err := os.OpenFile(filepath.Join(logDir, "patch.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}
	defer logFile.Close()
	klog.LogToStderr(false)
	klog.SetOutput(logFile)

	registry.MergeComponents(resolved, cfg.Components, cfg.Dir)
	resolved.Repos = registry.MergeRepos(resolved.Repos, cfg.Helm.Repos)

	var filter func(config.Component) bool
	preloadSource := cfg

	if hasPatchFile {
		patchFile := args[0]
		patch, err := config.Load(patchFile, setOverrides)
		if err != nil {
			return fmt.Errorf("loading patch file %s: %w", patchFile, err)
		}

		patchedNames := make(map[string]bool, len(patch.Components))
		for _, c := range patch.Components {
			patchedNames[c.Name] = true
		}
		if len(patchedNames) == 0 {
			logger.Warn("patch file contains no components; nothing to do")
			return nil
		}

		registry.MergeComponents(resolved, patch.Components, patch.Dir)
		resolved.Repos = registry.MergeRepos(resolved.Repos, patch.Helm.Repos)
		preloadSource = patch

		filter = func(c config.Component) bool {
			return patchedNames[c.Name]
		}
	}

	ctx := context.Background()

	if !patchDryRun && !patchSkipPreload {
		preloadRefs := getPreloadRefs(preloadSource)
		if len(preloadRefs) > 0 {
			running, _ := cache.IsPreloadRunning(ctx)
			if running {
				if err := logger.WithSpinner("Pulling images for preload", func() error {
					return cache.PullImages(ctx, preloadRefs)
				}); err != nil {
					return err
				}
				if err := logger.WithSpinner("Pushing images to preload registry", func() error {
					return cache.PushImages(ctx, preloadRefs)
				}); err != nil {
					return err
				}
			} else {
				color.Yellow("  preload refs specified but no preload registry running; images will be pulled on demand")
			}
		}
	}

	if err := installComponents(ctx, resolved, filter, installer.InstallOpts{DryRun: patchDryRun}); err != nil {
		return err
	}

	fmt.Println()
	if patchDryRun {
		color.Blue("  Dry-run completed in %s", time.Since(start).Round(time.Millisecond))
	} else {
		color.Blue("  Patch applied in %s", time.Since(start).Round(time.Millisecond))
	}

	return nil
}

// inheritClusterState loads the saved state for clusterName and folds its
// create-time inputs into the global cfg/setOverrides so a patch reuses them.
// Patch-time inputs keep priority: explicit --from / --registry (or a from set
// in the config) are left untouched, and per-key patch --set overrides win over
// the stored ones. Returns the loaded state, or nil when the cluster has no
// state file (e.g. created by an older gck) so patch falls back to its previous
// behaviour.
func inheritClusterState(clusterName string) *state.ClusterState {
	if clusterName == "" {
		return nil
	}
	st, err := state.Load(filepath.Join(gckHome, "clusters"), clusterName)
	if err != nil {
		return nil
	}
	if len(cfg.From) == 0 && len(st.From) > 0 {
		cfg.From = st.From
	}
	if cfg.Registry == "" && st.Registry != "" {
		cfg.Registry = st.Registry
	}
	setOverrides = mergeSet(st.Set, setOverrides)
	return st
}

// mergeSet overlays overrides on top of inherited --set values, with overrides
// winning per key.
func mergeSet(inherited, overrides map[string]string) map[string]string {
	merged := make(map[string]string, len(inherited)+len(overrides))
	for k, v := range inherited {
		merged[k] = v
	}
	for k, v := range overrides {
		merged[k] = v
	}
	return merged
}

// applyPatchFlags applies context flags to the resolved context, combining the
// flags inherited from the cluster's create-time state with any passed on the
// patch command line (deduplicated). Patch-time flags are additive.
func applyPatchFlags(cmd *cobra.Command, resolved *config.ResolvedContext, inherited *state.ClusterState) error {
	if resolved == nil || len(resolved.Flags) == 0 {
		return nil
	}
	active, err := extractActiveFlags(os.Args, cmd.InheritedFlags(), cmd.LocalFlags(), resolved.Flags)
	if err != nil {
		return err
	}
	var inheritedFlags []string
	if inherited != nil {
		inheritedFlags = inherited.Flags
	}
	return registry.ApplyFlags(resolved, unionFlags(inheritedFlags, active), setOverrides)
}

// unionFlags concatenates two flag-name lists, preserving order and dropping
// duplicates.
func unionFlags(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, list := range [][]string{a, b} {
		for _, f := range list {
			if !seen[f] {
				seen[f] = true
				out = append(out, f)
			}
		}
	}
	return out
}
