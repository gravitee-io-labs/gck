package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gravitee-io-labs/gck/internal/config"
)

type MirrorConfig struct {
	Patch  string
	Mounts []config.Mount
}

// PrepareContainerdHosts writes containerd 2.x file-based registry mirror
// configuration (hosts.toml per upstream) and returns a containerd config patch
// plus the extra mount needed to expose the config inside Kind nodes.
//
// When both mirrors and preload are configured, each upstream's hosts.toml
// lists the preload registry first (for pre-pushed images) and the mirror
// proxy second (for transparent caching of non-preloaded images).
func PrepareContainerdHosts(cfg *config.MirrorsConfig, preloadUpstreams []string, gckHome string) (*MirrorConfig, error) {
	hostsDir := filepath.Join(gckHome, "mirrors", "containerd-hosts")

	// Collect all upstreams that need a hosts.toml entry.
	seen := make(map[string]bool)
	var allUpstreams []string

	if cfg != nil {
		for _, u := range AllUpstreams(cfg) {
			if !seen[u] {
				seen[u] = true
				allUpstreams = append(allUpstreams, u)
			}
		}
	}
	for _, u := range preloadUpstreams {
		if !seen[u] {
			seen[u] = true
			allUpstreams = append(allUpstreams, u)
		}
	}

	preloadSet := make(map[string]bool, len(preloadUpstreams))
	for _, u := range preloadUpstreams {
		preloadSet[u] = true
	}

	mirrorSet := make(map[string]bool)
	if cfg != nil {
		for _, u := range AllUpstreams(cfg) {
			mirrorSet[u] = true
		}
	}

	for _, upstream := range allUpstreams {
		dir := filepath.Join(hostsDir, upstream)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating hosts dir for %s: %w", upstream, err)
		}

		var b strings.Builder
		fmt.Fprintf(&b, "server = %q\n", remoteURL(upstream))

		if preloadSet[upstream] {
			fmt.Fprintf(&b, "\n[host.\"http://%s\"]\n", PreloadRegistryHost())
			b.WriteString("  capabilities = [\"pull\", \"resolve\"]\n")
		}

		if mirrorSet[upstream] {
			name := ContainerName(upstream)
			fmt.Fprintf(&b, "\n[host.\"http://%s:5000\"]\n", name)
			b.WriteString("  capabilities = [\"pull\", \"resolve\"]\n")
		}

		hostsFile := filepath.Join(dir, "hosts.toml")
		if err := os.WriteFile(hostsFile, []byte(b.String()), 0o644); err != nil {
			return nil, fmt.Errorf("writing hosts.toml for %s: %w", upstream, err)
		}
	}

	return &MirrorConfig{
		Patch: "[plugins.\"io.containerd.grpc.v1.cri\".registry]\n  config_path = \"/etc/containerd/certs.d\"",
		Mounts: []config.Mount{{
			HostPath:      hostsDir,
			ContainerPath: "/etc/containerd/certs.d",
		}},
	}, nil
}

// PrepareMirrors is a convenience wrapper for PrepareContainerdHosts when only
// mirrors are configured (no preloading).
func PrepareMirrors(cfg *config.MirrorsConfig, gckHome string) (*MirrorConfig, error) {
	return PrepareContainerdHosts(cfg, nil, gckHome)
}
