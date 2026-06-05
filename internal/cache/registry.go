package cache

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gravitee-io-labs/gck/internal/config"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

const (
	containerPrefix = "gck-mirror-"
	registryImage   = "registry:2"
	basePort        = 5000
	internalPort    = nat.Port("5000/tcp")
)

// SanitizeHost converts a registry hostname into a string safe for use in
// Docker container names (e.g. "docker.io" -> "docker-io").
func SanitizeHost(host string) string {
	return strings.NewReplacer(".", "-", ":", "-").Replace(host)
}

// ContainerName returns the Docker container name for a cache proxy targeting
// the given upstream registry.
func ContainerName(upstream string) string {
	return containerPrefix + SanitizeHost(upstream)
}

// AllUpstreams returns the full list of upstream registries to proxy, always
// including docker.io as the first entry.
func AllUpstreams(cfg *config.MirrorsConfig) []string {
	upstreams := []string{"docker.io"}
	seen := map[string]bool{"docker.io": true}
	for _, u := range cfg.Upstreams {
		if !seen[u] {
			upstreams = append(upstreams, u)
			seen[u] = true
		}
	}
	return upstreams
}

// ResolveDir returns the mirror storage directory, falling back to
// $gckHome/mirrors when the config does not specify one.
func ResolveDir(cfg *config.MirrorsConfig, gckHome string) string {
	if cfg.Data != "" {
		return cfg.Data
	}
	return filepath.Join(gckHome, "mirrors")
}

// EnsureProxies creates and starts a registry:2 pull-through proxy container
// for each configured upstream registry. Containers that are already running
// are left untouched. Stale (stopped) containers are replaced.
func EnsureProxies(ctx context.Context, cfg *config.MirrorsConfig, gckHome string) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("creating docker client: %w", err)
	}
	defer cli.Close()

	rc, err := cli.ImagePull(ctx, registryImage, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pulling %s: %w", registryImage, err)
	}
	_, _ = io.Copy(io.Discard, rc)
	rc.Close()

	dir := ResolveDir(cfg, gckHome)

	for i, upstream := range AllUpstreams(cfg) {
		name := ContainerName(upstream)

		running, err := isContainerRunning(ctx, cli, name)
		if err != nil {
			return fmt.Errorf("inspecting container %s: %w", name, err)
		}
		if running {
			continue
		}

		if err := forceRemove(ctx, cli, name); err != nil {
			return fmt.Errorf("removing stale container %s: %w", name, err)
		}

		dataDir := filepath.Join(dir, SanitizeHost(upstream))
		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			return fmt.Errorf("creating cache dir %s: %w", dataDir, err)
		}

		configPath := filepath.Join(dataDir, "config.yml")
		if err := writeProxyConfig(configPath, upstream); err != nil {
			return fmt.Errorf("writing registry config for %s: %w", upstream, err)
		}

		hostPort := fmt.Sprintf("%d", basePort+i)

		resp, err := cli.ContainerCreate(ctx,
			&container.Config{
				Image:        registryImage,
				ExposedPorts: nat.PortSet{internalPort: struct{}{}},
				Labels: map[string]string{
					"gck.role":     "cache-proxy",
					"gck.upstream": upstream,
				},
			},
			&container.HostConfig{
				PortBindings: nat.PortMap{
					internalPort: []nat.PortBinding{
						{HostIP: "127.0.0.1", HostPort: hostPort},
					},
				},
				Mounts: []mount.Mount{
					{
						Type:   mount.TypeBind,
						Source: dataDir,
						Target: "/var/lib/registry",
					},
					{
						Type:     mount.TypeBind,
						Source:   configPath,
						Target:  "/etc/docker/registry/config.yml",
						ReadOnly: true,
					},
				},
				RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
			},
			nil, nil, name,
		)
		if err != nil {
			return fmt.Errorf("creating container %s: %w", name, err)
		}

		if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
			return fmt.Errorf("starting container %s: %w", name, err)
		}
	}

	return nil
}

// StopProxies stops and removes all gck-mirror-* proxy containers for the
// configured upstreams.
func StopProxies(ctx context.Context, cfg *config.MirrorsConfig) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("creating docker client: %w", err)
	}
	defer cli.Close()

	for _, upstream := range AllUpstreams(cfg) {
		if err := forceRemove(ctx, cli, ContainerName(upstream)); err != nil {
			return fmt.Errorf("stopping container %s: %w", ContainerName(upstream), err)
		}
	}
	return nil
}

// isContainerRunning returns true if a container with the given name exists and
// its state is Running.
func isContainerRunning(ctx context.Context, cli *client.Client, name string) (bool, error) {
	info, err := cli.ContainerInspect(ctx, name)
	if err != nil {
		if client.IsErrNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return info.State != nil && info.State.Running, nil
}

// forceRemove removes a container by name, doing nothing if it doesn't exist.
func forceRemove(ctx context.Context, cli *client.Client, name string) error {
	err := cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})
	if err != nil && !client.IsErrNotFound(err) {
		return err
	}
	return nil
}

