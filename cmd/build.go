package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gravitee-io-labs/gck/internal/build"
	"github.com/gravitee-io-labs/gck/internal/config"
	"github.com/gravitee-io-labs/gck/internal/kind"
	"github.com/gravitee-io-labs/gck/internal/logger"
	"github.com/spf13/cobra"
)

var (
	buildSkipPre   bool
	buildNoRestart bool
	buildCluster   string
	buildCreate    bool
)

var buildCmd = &cobra.Command{
	Use:   "build [names...]",
	Short: "Build local Docker images, push them to the cluster, and restart workloads",
	Long: `Build one or more locally-defined Docker images, push them to the Kind
cluster's preload registry, and restart any Deployments or StatefulSets
that reference them.

Build entries are defined in the "builds" section of gck.yaml.
When called without arguments, all entries are built. Pass one or more
names to build a subset.

Use --create to automatically create the cluster if it does not exist.
Context flags (e.g. --disable-es) are forwarded to the creation flow.`,
	FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
	RunE:               runBuild,
}

func init() {
	buildCmd.Flags().BoolVar(&buildSkipPre, "skip-pre", false, "skip pre-build commands (docker build + load only)")
	buildCmd.Flags().BoolVar(&buildNoRestart, "no-restart", false, "build and load but don't restart workloads")
	buildCmd.Flags().StringVar(&buildCluster, "name", "", "target a specific cluster (default: from config)")
	buildCmd.Flags().BoolVar(&buildCreate, "create", false, "create the cluster if it does not exist")
	rootCmd.AddCommand(buildCmd)
}

func runBuild(cmd *cobra.Command, args []string) error {
	if err := requireDocker(); err != nil {
		return err
	}

	resolved, err := resolveContextConfig()
	if err != nil {
		return err
	}

	if len(cfg.Builds) == 0 {
		return fmt.Errorf("no builds defined in config; add a \"builds\" section to gck.yaml")
	}

	clusterName := buildCluster
	if clusterName == "" {
		clusterName = cfg.Kind.Name
	}

	exists, err := kind.Exists(clusterName)
	if err != nil {
		return err
	}
	if !exists {
		if !buildCreate {
			return fmt.Errorf("cluster %q not found — create it first with \"gck create\" or pass --create", clusterName)
		}
		activeFlags, err := applyContextFlags(cmd, resolved)
		if err != nil {
			return err
		}
		if resolved != nil {
			cfg.Images = config.MergeImages(cfg.Images, resolved.Images)
			cfg.Kind.MergeWithContext(&resolved.Kind)
		}
		if err := createCluster(resolved, activeFlags); err != nil {
			return err
		}
	}

	builds, err := selectBuilds(cfg.Builds, args)
	if err != nil {
		return err
	}

	logDir := filepath.Join(gckHome, "logs", "build")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("creating build log directory: %w", err)
	}
	logPath := filepath.Join(logDir, "build.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("opening build log file: %w", err)
	}
	defer logFile.Close()
	logger.SetLogFile(logPath)
	defer logger.SetLogFile("")

	ctx := context.Background()
	opts := build.Options{
		ClusterName: clusterName,
		GckHome:     gckHome,
		SkipPre:     buildSkipPre,
		NoRestart:   buildNoRestart,
		LogWriter:   logFile,
	}

	for _, b := range builds {
		fmt.Println()
		logger.Success("Building %s (%s)", b.Name, b.Image)
		if err := build.Run(ctx, b, opts); err != nil {
			return fmt.Errorf("build %q failed: %w", b.Name, err)
		}
	}

	fmt.Println()
	logger.Success("All builds completed")
	return nil
}

func selectBuilds(all []config.Build, names []string) ([]config.Build, error) {
	if len(names) == 0 {
		return all, nil
	}

	byName := make(map[string]config.Build, len(all))
	for _, b := range all {
		byName[b.Name] = b
	}

	selected := make([]config.Build, 0, len(names))
	for _, name := range names {
		b, ok := byName[name]
		if !ok {
			known := make([]string, len(all))
			for i, b := range all {
				known[i] = b.Name
			}
			return nil, fmt.Errorf("unknown build %q (available: %s)", name, joinNames(known))
		}
		selected = append(selected, b)
	}
	return selected, nil
}

func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	default:
		result := names[0]
		for _, n := range names[1:] {
			result += ", " + n
		}
		return result
	}
}
