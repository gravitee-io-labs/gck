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

	resolved, err := resolveContextConfig()
	if err != nil {
		return err
	}
	if resolved == nil {
		return fmt.Errorf("no registry context configured; patch requires a resolved context (set registry and from in gck.yaml or via flags)")
	}

	if _, err := applyContextFlags(cmd, resolved); err != nil {
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
