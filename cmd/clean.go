package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/docker/go-units"
	"github.com/gravitee-io-labs/gck/internal/cache"
	"github.com/gravitee-io-labs/gck/internal/logger"
	"github.com/spf13/cobra"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Reclaim disk space used by gck caches",
}

var cleanPreloadCmd = &cobra.Command{
	Use:   "preload",
	Short: "Remove superseded images from the preload registry",
	Long: `Remove the image manifests no tag points at from the preload registry, and
every layer only those manifests used.

Pushing a mutable tag (latest, nightly builds) again moves the tag to the new
image and leaves the previous one in $GCK_HOME/preload. "gck create" collects
this garbage after each push; run this command to reclaim the space left by
"gck build" and "gck patch", or to see what would be removed.

The preload registry is stopped during the collection and restarted afterwards.
Images still tagged in the registry are never removed.`,
	Annotations: map[string]string{"gck_skip_config": "true"},
	RunE:        runCleanPreload,
}

var cleanDryRun bool

func init() {
	cleanPreloadCmd.Flags().BoolVar(&cleanDryRun, "dry-run", false, "Report what would be removed without removing it")
	cleanCmd.AddCommand(cleanPreloadCmd)
	rootCmd.AddCommand(cleanCmd)
}

func runCleanPreload(_ *cobra.Command, _ []string) error {
	report, err := cache.ScanPreloadGarbage(gckHome)
	if err != nil {
		return err
	}
	if report.Empty() {
		logger.Success("Nothing to collect in the preload registry")
		return nil
	}

	if cleanDryRun {
		for _, m := range report.Untagged {
			fmt.Printf("  %s@%s\n", m.Repo, m.Digest)
		}
		logger.Success("Would reclaim %s from %d superseded manifest(s)",
			units.HumanSize(float64(report.ReclaimableBytes)), len(report.Untagged))
		return nil
	}

	if err := requireDocker(); err != nil {
		return err
	}

	lock, err := cache.LockPreload(gckHome, cache.Exclusive, "Waiting for other gck commands to finish with the preload registry")
	if err != nil {
		return err
	}
	defer lock.Unlock()

	var res cache.GCResult
	if err := logger.WithSpinner("Collecting preload registry garbage", func() error {
		res, err = cache.CollectPreloadGarbage(context.Background(), gckHome)
		return err
	}); err != nil {
		return err
	}
	logger.Success("Reclaimed %s from %d superseded manifest(s)", units.HumanSize(float64(res.Bytes)), res.Manifests)
	return nil
}

// collectPreloadGarbage removes the images superseded by the push that just
// ran. It needs the exclusive lock, so it releases the caller's shared lock
// first and returns a new one. When another command holds the lock the
// collection is skipped: a later create will do it. A failed collection only
// warns, since the registry is still usable, just larger.
func collectPreloadGarbage(ctx context.Context, shared *cache.PreloadLock) (*cache.PreloadLock, error) {
	report, err := cache.ScanPreloadGarbage(gckHome)
	if err != nil {
		logger.Warn("skipping preload registry garbage collection: %v", err)
		return shared, nil
	}
	if report.Empty() {
		return shared, nil
	}

	shared.Unlock()
	exclusive, err := cache.TryLockPreload(gckHome, cache.Exclusive)
	switch {
	case errors.Is(err, cache.ErrPreloadBusy):
	case err != nil:
		logger.Warn("skipping preload registry garbage collection: %v", err)
	default:
		var res cache.GCResult
		if err := logger.WithSpinner("Collecting preload registry garbage", func() error {
			res, err = cache.CollectPreloadGarbage(ctx, gckHome)
			return err
		}); err != nil {
			logger.Warn("preload registry garbage collection failed: %v", err)
		} else {
			logger.Success("Reclaimed %s from %d superseded manifest(s)", units.HumanSize(float64(res.Bytes)), res.Manifests)
		}
		exclusive.Unlock()
	}

	shared, err = cache.LockPreload(gckHome, cache.Shared, "Waiting for preload registry")
	if err != nil {
		return nil, err
	}
	// Another command may have stopped the registry while this one held no
	// lock; the pushed images are still on disk, so restarting it is enough.
	if err := cache.EnsurePreloadRegistry(ctx, gckHome); err != nil {
		return shared, fmt.Errorf("restarting preload registry: %w", err)
	}
	return shared, nil
}
