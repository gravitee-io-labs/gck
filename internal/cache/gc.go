package cache

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

const preloadGCContainerName = "gck-preload-gc"

// registry:2 keeps its data under registryMount inside the container, in the
// registryV2 subdirectory of the bind-mounted preload directory.
const (
	registryMount = "/var/lib/registry"
	registryV2    = "docker/registry/v2"
)

// Manifest identifies one manifest revision stored in a repository of the
// preload registry.
type Manifest struct {
	Repo   string
	Digest string

	// links are the storage paths, relative to registryV2, that link the
	// repository to this manifest: its revision and any tag history entries.
	links []string
}

// GarbageReport describes what a garbage collection of the preload registry
// would remove.
type GarbageReport struct {
	// Untagged lists the manifests no tag points at, directly or through a
	// tagged index.
	Untagged []Manifest
	// ReclaimableBytes is the size of the blobs no remaining manifest uses.
	ReclaimableBytes int64
}

// Empty reports whether a collection would remove nothing.
func (r GarbageReport) Empty() bool {
	return len(r.Untagged) == 0 && r.ReclaimableBytes == 0
}

// GCResult describes what a garbage collection of the preload registry removed.
type GCResult struct {
	Manifests int
	Bytes     int64
}

func preloadDataDir(gckHome string) string {
	return filepath.Join(gckHome, "preload")
}

// ScanPreloadGarbage inspects the preload registry storage and reports the
// manifests no tag points at and the space their exclusive blobs use. It only
// reads the storage, so it is safe to call while the registry is running.
//
// Every push of a mutable tag (latest, nightly builds) moves the tag to a new
// manifest and leaves the previous one in storage; this is what accumulates.
func ScanPreloadGarbage(gckHome string) (GarbageReport, error) {
	root := filepath.Join(preloadDataDir(gckHome), filepath.FromSlash(registryV2))
	reposDir := filepath.Join(root, "repositories")

	var report GarbageReport
	marked := make(map[string]bool)

	err := filepath.WalkDir(reposDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !d.IsDir() {
			return nil
		}
		switch d.Name() {
		case "_layers", "_uploads":
			return filepath.SkipDir
		case "_manifests":
			rel, err := filepath.Rel(reposDir, filepath.Dir(p))
			if err != nil {
				return err
			}
			untagged, err := scanRepository(root, p, filepath.ToSlash(rel), marked)
			if err != nil {
				return err
			}
			report.Untagged = append(report.Untagged, untagged...)
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return GarbageReport{}, fmt.Errorf("scanning preload registry: %w", err)
	}

	err = walkBlobs(root, func(digest string, size int64) {
		if !marked[digest] {
			report.ReclaimableBytes += size
		}
	})
	if err != nil {
		return GarbageReport{}, err
	}
	return report, nil
}

// scanRepository marks every blob reachable from the repository's tags and
// returns the manifest revisions that are not reachable.
func scanRepository(root, manifestsDir, repo string, marked map[string]bool) ([]Manifest, error) {
	revisions, err := listDigests(filepath.Join(manifestsDir, "revisions"))
	if err != nil {
		return nil, err
	}

	tagsDir := filepath.Join(manifestsDir, "tags")
	tags, err := os.ReadDir(tagsDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	keep := make(map[string]bool)
	var queue []string
	for _, t := range tags {
		link, err := os.ReadFile(filepath.Join(tagsDir, t.Name(), "current", "link"))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		digest := strings.TrimSpace(string(link))
		if !keep[digest] {
			keep[digest] = true
			queue = append(queue, digest)
		}
	}

	for len(queue) > 0 {
		digest := queue[0]
		queue = queue[1:]
		marked[digest] = true

		m, err := readManifest(root, digest)
		if err != nil {
			return nil, err
		}
		if m == nil {
			continue
		}
		if m.Config.Digest != "" {
			marked[m.Config.Digest] = true
		}
		for _, l := range m.Layers {
			marked[l.Digest] = true
		}
		// Children of an index or manifest list have no tag of their own but
		// must survive as long as the index does.
		for _, c := range m.Manifests {
			if !keep[c.Digest] {
				keep[c.Digest] = true
				queue = append(queue, c.Digest)
			}
		}
	}

	var untagged []Manifest
	for _, digest := range revisions {
		if keep[digest] {
			continue
		}
		algo, hex, _ := strings.Cut(digest, ":")
		base := path.Join("repositories", repo, "_manifests")
		m := Manifest{Repo: repo, Digest: digest, links: []string{path.Join(base, "revisions", algo, hex)}}
		for _, t := range tags {
			if _, err := os.Stat(filepath.Join(tagsDir, t.Name(), "index", algo, hex)); err == nil {
				m.links = append(m.links, path.Join(base, "tags", t.Name(), "index", algo, hex))
			}
		}
		untagged = append(untagged, m)
	}
	return untagged, nil
}

type manifestRef struct {
	Digest string `json:"digest"`
}

type manifestContent struct {
	Config    manifestRef   `json:"config"`
	Layers    []manifestRef `json:"layers"`
	Manifests []manifestRef `json:"manifests"`
}

// readManifest parses the manifest stored under digest. It returns nil when
// the blob is missing, which leaves the registry to report the broken tag.
func readManifest(root, digest string) (*manifestContent, error) {
	p, ok := blobPath(root, digest)
	if !ok {
		return nil, nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var m manifestContent
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest %s: %w", digest, err)
	}
	return &m, nil
}

func blobPath(root, digest string) (string, bool) {
	algo, hex, ok := strings.Cut(digest, ":")
	if !ok || len(hex) < 2 {
		return "", false
	}
	return filepath.Join(root, "blobs", algo, hex[:2], hex, "data"), true
}

// listDigests returns the digests linked under a revisions directory laid out
// as <dir>/<algorithm>/<hex>.
func listDigests(dir string) ([]string, error) {
	algos, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var digests []string
	for _, a := range algos {
		entries, err := os.ReadDir(filepath.Join(dir, a.Name()))
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			digests = append(digests, a.Name()+":"+e.Name())
		}
	}
	return digests, nil
}

// walkBlobs calls fn with the digest and size of every blob in storage.
func walkBlobs(root string, fn func(digest string, size int64)) error {
	blobsDir := filepath.Join(root, "blobs")
	err := filepath.WalkDir(blobsDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() || d.Name() != "data" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		// <blobs>/<algorithm>/<xx>/<hex>/data
		hexDir := filepath.Dir(p)
		algo := filepath.Base(filepath.Dir(filepath.Dir(hexDir)))
		fn(algo+":"+filepath.Base(hexDir), info.Size())
		return nil
	})
	if err != nil {
		return fmt.Errorf("scanning preload blobs: %w", err)
	}
	return nil
}

func blobsSize(gckHome string) (int64, error) {
	root := filepath.Join(preloadDataDir(gckHome), filepath.FromSlash(registryV2))
	var total int64
	err := walkBlobs(root, func(_ string, size int64) { total += size })
	return total, err
}

// CollectPreloadGarbage removes the manifests no tag points at and every blob
// no remaining manifest uses. The preload registry is stopped for the duration
// and restarted afterwards if it was running: the registry must not accept
// writes during a collection, and its blob cache must be dropped after one.
//
// Untagged manifests are unlinked here rather than with garbage-collect
// --delete-untagged, which also deletes the untagged children of a tagged
// multi-platform index.
func CollectPreloadGarbage(ctx context.Context, gckHome string) (GCResult, error) {
	report, err := ScanPreloadGarbage(gckHome)
	if err != nil {
		return GCResult{}, err
	}
	if report.Empty() {
		return GCResult{}, nil
	}

	before, err := blobsSize(gckHome)
	if err != nil {
		return GCResult{}, err
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return GCResult{}, fmt.Errorf("creating docker client: %w", err)
	}
	defer cli.Close()

	running, networks, err := preloadRegistryState(ctx, cli)
	if err != nil {
		return GCResult{}, fmt.Errorf("inspecting preload registry: %w", err)
	}
	if err := forceRemove(ctx, cli, preloadContainerName); err != nil {
		return GCResult{}, fmt.Errorf("stopping preload registry: %w", err)
	}

	gcErr := runGarbageCollect(ctx, cli, gckHome, report.Untagged)

	if running {
		if err := restartPreloadRegistry(ctx, cli, gckHome, networks); err != nil {
			return GCResult{}, errors.Join(gcErr, err)
		}
	}
	if gcErr != nil {
		return GCResult{}, gcErr
	}

	after, err := blobsSize(gckHome)
	if err != nil {
		return GCResult{}, err
	}
	return GCResult{Manifests: len(report.Untagged), Bytes: before - after}, nil
}

// preloadRegistryState reports whether the preload registry runs and the
// networks it is attached to besides the default bridge. Running clusters
// reach it by name on their network, typically "kind".
func preloadRegistryState(ctx context.Context, cli *client.Client) (bool, []string, error) {
	info, err := cli.ContainerInspect(ctx, preloadContainerName)
	if err != nil {
		if client.IsErrNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	var networks []string
	if info.NetworkSettings != nil {
		for name := range info.NetworkSettings.Networks {
			if name != "bridge" {
				networks = append(networks, name)
			}
		}
	}
	return info.State != nil && info.State.Running, networks, nil
}

// restartPreloadRegistry starts the preload registry again and reattaches it
// to the networks it was on, so running clusters can pull from it again.
func restartPreloadRegistry(ctx context.Context, cli *client.Client, gckHome string, networks []string) error {
	if err := EnsurePreloadRegistry(ctx, gckHome); err != nil {
		return fmt.Errorf("restarting preload registry: %w", err)
	}
	for _, n := range networks {
		if err := cli.NetworkConnect(ctx, n, preloadContainerName, nil); err != nil {
			return fmt.Errorf("reconnecting preload registry to network %s: %w", n, err)
		}
	}
	return nil
}

// runGarbageCollect unlinks the untagged manifests and runs the registry's
// garbage collector in a one-shot container. Both run inside the container
// because registry:2 writes its storage as root, which the host user may not
// be allowed to delete on Linux.
func runGarbageCollect(ctx context.Context, cli *client.Client, gckHome string, untagged []Manifest) error {
	if err := ensureImage(ctx, cli, registryImage); err != nil {
		return err
	}
	if err := forceRemove(ctx, cli, preloadGCContainerName); err != nil {
		return fmt.Errorf("removing stale %s container: %w", preloadGCContainerName, err)
	}

	// Paths are passed as positional arguments, never interpolated into the
	// script.
	cmd := []string{`{ [ $# -eq 0 ] || rm -rf "$@"; } && registry garbage-collect /etc/docker/registry/config.yml`, "gc"}
	for _, m := range untagged {
		for _, l := range m.links {
			cmd = append(cmd, path.Join(registryMount, registryV2, l))
		}
	}

	resp, err := cli.ContainerCreate(ctx,
		&container.Config{
			Image:      registryImage,
			Entrypoint: []string{"/bin/sh", "-c"},
			Cmd:        cmd,
			Labels:     map[string]string{"gck.role": "preload-gc"},
		},
		&container.HostConfig{
			Mounts: []mount.Mount{
				{
					Type:   mount.TypeBind,
					Source: preloadDataDir(gckHome),
					Target: registryMount,
				},
			},
		},
		nil, nil, preloadGCContainerName,
	)
	if err != nil {
		return fmt.Errorf("creating %s container: %w", preloadGCContainerName, err)
	}
	defer func() { _ = forceRemove(context.WithoutCancel(ctx), cli, preloadGCContainerName) }()

	waitCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNextExit)
	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("starting %s container: %w", preloadGCContainerName, err)
	}

	select {
	case err := <-errCh:
		return fmt.Errorf("waiting for %s container: %w", preloadGCContainerName, err)
	case res := <-waitCh:
		if res.StatusCode == 0 {
			return nil
		}
		return fmt.Errorf("garbage collection exited with status %d: %s",
			res.StatusCode, containerOutput(ctx, cli, resp.ID))
	}
}

// containerOutput returns the last lines a container wrote, for error reports.
func containerOutput(ctx context.Context, cli *client.Client, id string) string {
	rc, err := cli.ContainerLogs(ctx, id, container.LogsOptions{ShowStdout: true, ShowStderr: true, Tail: "20"})
	if err != nil {
		return "(no logs)"
	}
	defer rc.Close()
	var out bytes.Buffer
	if _, err := stdcopy.StdCopy(&out, &out, rc); err != nil {
		return "(no logs)"
	}
	return strings.TrimSpace(out.String())
}
