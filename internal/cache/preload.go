package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/distribution/reference"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

const (
	pullWorkers          = 4
	preloadContainerName = "gck-preload"
	preloadPort          = "5100"
)

// PreloadRegistryHost returns the hostname:port of the preload registry as
// seen from within the Kind Docker network (container name based).
func PreloadRegistryHost() string {
	return preloadContainerName + ":5000"
}

// IsPreloadRunning reports whether the gck-preload registry container is
// currently running.
func IsPreloadRunning(ctx context.Context) (bool, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return false, err
	}
	defer cli.Close()
	return isContainerRunning(ctx, cli, preloadContainerName)
}

// PullImages pulls all images in parallel using the Docker daemon on the host.
// When running on CI with Docker Layer Caching (DLC), already-cached layers
// make subsequent pulls effectively free.
func PullImages(ctx context.Context, images []string) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("creating docker client: %w", err)
	}
	defer cli.Close()

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
		sem  = make(chan struct{}, pullWorkers)
	)

	for _, img := range images {
		wg.Add(1)
		go func(ref string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			canonical := normalizeRef(ref)
			rc, err := cli.ImagePull(ctx, canonical, image.PullOptions{})
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("pulling %s: %w", canonical, err))
				mu.Unlock()
				return
			}
			if err := drainPullStream(rc); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("pulling %s: %w", canonical, err))
				mu.Unlock()
				return
			}
		}(img)
	}

	wg.Wait()

	if len(errs) > 0 {
		return fmt.Errorf("failed to pull %d image(s): %v", len(errs), errs[0])
	}
	return nil
}

// EnsurePreloadRegistry starts a plain registry:2 container for receiving
// pre-pushed images. Unlike mirror proxies, this registry has no
// proxy.remoteurl -- it only serves images that have been explicitly pushed.
// Registry data is persisted to $GCK_HOME/preload so that cached layers
// survive container removal across cluster lifecycles.
func EnsurePreloadRegistry(ctx context.Context, gckHome string) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("creating docker client: %w", err)
	}
	defer cli.Close()

	running, err := isContainerRunning(ctx, cli, preloadContainerName)
	if err != nil {
		return fmt.Errorf("inspecting preload registry: %w", err)
	}
	if running {
		return nil
	}

	if err := forceRemove(ctx, cli, preloadContainerName); err != nil {
		return fmt.Errorf("removing stale preload registry: %w", err)
	}

	rc, err := cli.ImagePull(ctx, registryImage, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pulling %s: %w", registryImage, err)
	}
	_, _ = io.Copy(io.Discard, rc)
	rc.Close()

	dataDir := filepath.Join(gckHome, "preload")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("creating preload data directory %s: %w", dataDir, err)
	}

	resp, err := cli.ContainerCreate(ctx,
		&container.Config{
			Image:        registryImage,
			ExposedPorts: nat.PortSet{internalPort: struct{}{}},
			Labels: map[string]string{
				"gck.role": "preload-registry",
			},
		},
		&container.HostConfig{
			PortBindings: nat.PortMap{
				internalPort: []nat.PortBinding{
					{HostIP: "127.0.0.1", HostPort: preloadPort},
				},
			},
			Mounts: []mount.Mount{
				{
					Type:   mount.TypeBind,
					Source: dataDir,
					Target: "/var/lib/registry",
				},
			},
			RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
		},
		nil, nil, preloadContainerName,
	)
	if err != nil {
		return fmt.Errorf("creating preload registry: %w", err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("starting preload registry: %w", err)
	}
	return nil
}

// PushImages re-tags each pre-pulled image and pushes it to the local preload
// registry. The re-tagging strips the source registry host so that containerd
// mirror resolution finds the image at the expected path.
func PushImages(ctx context.Context, images []string) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("creating docker client: %w", err)
	}
	defer cli.Close()

	localReg := "localhost:" + preloadPort

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
		sem  = make(chan struct{}, pullWorkers)
	)

	for _, img := range images {
		wg.Add(1)
		go func(ref string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			canonical := normalizeRef(ref)
			localRef := localReg + "/" + stripRegistryHost(ref)

			if err := cli.ImageTag(ctx, canonical, localRef); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("tagging %s as %s: %w", ref, localRef, err))
				mu.Unlock()
				return
			}

			rc, err := cli.ImagePush(ctx, localRef, image.PushOptions{RegistryAuth: "e30="})
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("pushing %s: %w", localRef, err))
				mu.Unlock()
				return
			}
			_, _ = io.Copy(io.Discard, rc)
			rc.Close()
		}(img)
	}

	wg.Wait()

	if len(errs) > 0 {
		return fmt.Errorf("failed to push %d image(s): %v", len(errs), errs[0])
	}
	return nil
}

// PreloadUpstreams returns the deduplicated set of registry hosts referenced by
// the given image list. Used to generate containerd hosts.toml entries.
func PreloadUpstreams(images []string) []string {
	seen := make(map[string]bool)
	var upstreams []string
	for _, img := range images {
		host := registryHost(img)
		if !seen[host] {
			seen[host] = true
			upstreams = append(upstreams, host)
		}
	}
	return upstreams
}

// ConnectPreloadToKindNetwork connects the preload registry container to the
// Kind Docker network so that Kind nodes can resolve it by container name.
func ConnectPreloadToKindNetwork(ctx context.Context) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("creating docker client: %w", err)
	}
	defer cli.Close()

	netID, err := findKindNetwork(ctx, cli)
	if err != nil {
		return err
	}

	inspect, err := cli.NetworkInspect(ctx, netID, network.InspectOptions{})
	if err != nil {
		return fmt.Errorf("inspecting kind network: %w", err)
	}
	for _, ep := range inspect.Containers {
		if ep.Name == preloadContainerName {
			return nil
		}
	}

	return cli.NetworkConnect(ctx, netID, preloadContainerName, nil)
}

// StopPreloadRegistry stops and removes the preload registry container.
func StopPreloadRegistry(ctx context.Context) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("creating docker client: %w", err)
	}
	defer cli.Close()
	return forceRemove(ctx, cli, preloadContainerName)
}

// normalizeRef expands a short Docker image reference into its canonical form
// so that it matches what the Docker daemon stores after a pull.
// E.g. "postgres:17" -> "docker.io/library/postgres:17".
func normalizeRef(ref string) string {
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return ref
	}
	named = reference.TagNameOnly(named)
	if tagged, ok := named.(reference.Tagged); ok {
		return reference.Domain(named) + "/" + reference.Path(named) + ":" + tagged.Tag()
	}
	return reference.Domain(named) + "/" + reference.Path(named)
}

// stripRegistryHost removes the registry hostname from an image reference,
// returning just the path and tag (e.g. "graviteeio/apim-gateway:latest").
// Docker Hub images (implicit or explicit docker.io) have just their path
// returned. For other registries the first component is stripped.
func stripRegistryHost(ref string) string {
	path, tag := ref, ""
	if i := strings.LastIndex(path, ":"); i > strings.LastIndex(path, "/") {
		tag = path[i:]
		path = path[:i]
	}

	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 1 {
		return "library/" + parts[0] + tag
	}
	if isRegistryHost(parts[0]) {
		return parts[1] + tag
	}
	return path + tag
}

// registryHost extracts the registry hostname from an image reference.
// Returns "docker.io" for Docker Hub images.
func registryHost(ref string) string {
	path := ref
	if i := strings.LastIndex(path, ":"); i > strings.LastIndex(path, "/") {
		path = path[:i]
	}

	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 1 {
		return "docker.io"
	}
	if isRegistryHost(parts[0]) {
		return parts[0]
	}
	return "docker.io"
}

func isRegistryHost(s string) bool {
	return strings.Contains(s, ".") || strings.Contains(s, ":") || s == "localhost"
}

// drainPullStream reads the Docker pull JSON stream to completion and
// returns the first error reported in the stream, if any.
func drainPullStream(rc io.ReadCloser) error {
	defer rc.Close()
	dec := json.NewDecoder(rc)
	var msg struct {
		Error string `json:"error"`
	}
	for dec.More() {
		if err := dec.Decode(&msg); err != nil {
			return err
		}
		if msg.Error != "" {
			return fmt.Errorf("%s", msg.Error)
		}
	}
	return nil
}
