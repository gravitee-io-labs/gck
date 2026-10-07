package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fakeRegistry writes a registry:2 storage tree under a temporary GCK_HOME.
type fakeRegistry struct {
	t       *testing.T
	gckHome string
	root    string
}

func newFakeRegistry(t *testing.T) *fakeRegistry {
	t.Helper()
	home := t.TempDir()
	return &fakeRegistry{t: t, gckHome: home, root: filepath.Join(home, "preload", "docker", "registry", "v2")}
}

func (r *fakeRegistry) write(rel string, data []byte) {
	r.t.Helper()
	p := filepath.Join(r.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		r.t.Fatal(err)
	}
}

// blob stores data and returns its digest.
func (r *fakeRegistry) blob(data []byte) string {
	sum := sha256.Sum256(data)
	h := hex.EncodeToString(sum[:])
	r.write("blobs/sha256/"+h[:2]+"/"+h+"/data", data)
	return "sha256:" + h
}

// layer stores a blob of the given size.
func (r *fakeRegistry) layer(name string, size int) string {
	data := make([]byte, size)
	copy(data, name)
	return r.blob(data)
}

// image stores a single-platform manifest in repo and returns its digest.
func (r *fakeRegistry) image(repo string, layers ...string) string {
	m := map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.docker.distribution.manifest.v2+json",
		"config":        map[string]string{"digest": r.blob([]byte("config-" + repo + strings.Join(layers, ",")))},
	}
	var ls []map[string]string
	for _, l := range layers {
		ls = append(ls, map[string]string{"digest": l})
	}
	m["layers"] = ls
	return r.manifest(repo, m)
}

// index stores a multi-platform index of children in repo.
func (r *fakeRegistry) index(repo string, children ...string) string {
	var ms []map[string]string
	for _, c := range children {
		ms = append(ms, map[string]string{"digest": c})
	}
	return r.manifest(repo, map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.oci.image.index.v1+json",
		"manifests":     ms,
	})
}

func (r *fakeRegistry) manifest(repo string, m map[string]any) string {
	data, err := json.Marshal(m)
	if err != nil {
		r.t.Fatal(err)
	}
	d := r.blob(data)
	r.write("repositories/"+repo+"/_manifests/revisions/sha256/"+d[len("sha256:"):]+"/link", []byte(d))
	return d
}

func (r *fakeRegistry) tag(repo, tag, digest string) {
	r.write("repositories/"+repo+"/_manifests/tags/"+tag+"/current/link", []byte(digest))
	r.write("repositories/"+repo+"/_manifests/tags/"+tag+"/index/sha256/"+digest[len("sha256:"):]+"/link", []byte(digest))
}

func untaggedDigests(report GarbageReport) []string {
	var ds []string
	for _, m := range report.Untagged {
		ds = append(ds, m.Repo+"@"+m.Digest)
	}
	slices.Sort(ds)
	return ds
}

func TestScanPreloadGarbage(t *testing.T) {
	t.Run("missing storage", func(t *testing.T) {
		r := newFakeRegistry(t)
		report, err := ScanPreloadGarbage(r.gckHome)
		if err != nil {
			t.Fatal(err)
		}
		if !report.Empty() {
			t.Fatalf("expected empty report, got %+v", report)
		}
	})

	t.Run("tagged images are kept", func(t *testing.T) {
		r := newFakeRegistry(t)
		base := r.layer("base", 100)
		r.tag("library/mongo", "7", r.image("library/mongo", base, r.layer("mongo", 50)))
		r.tag("graviteeio/apim-gateway", "latest", r.image("graviteeio/apim-gateway", base, r.layer("gw", 70)))

		report, err := ScanPreloadGarbage(r.gckHome)
		if err != nil {
			t.Fatal(err)
		}
		if !report.Empty() {
			t.Fatalf("expected empty report, got %+v", report)
		}
	})

	t.Run("moved tag leaves the previous manifest", func(t *testing.T) {
		r := newFakeRegistry(t)
		base := r.layer("base", 100)
		old := r.image("graviteeio/apim-gateway", base, r.layer("gw-1", 70))
		r.tag("graviteeio/apim-gateway", "latest", old)
		current := r.image("graviteeio/apim-gateway", base, r.layer("gw-2", 80))
		r.tag("graviteeio/apim-gateway", "latest", current)

		report, err := ScanPreloadGarbage(r.gckHome)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"graviteeio/apim-gateway@" + old}
		if got := untaggedDigests(report); !slices.Equal(got, want) {
			t.Fatalf("untagged = %v, want %v", got, want)
		}

		// The old manifest, its config and its own layer are reclaimable; the
		// shared base layer is not.
		oldManifest, err := os.ReadFile(filepath.Join(r.root, "blobs/sha256", old[7:9], old[7:], "data"))
		if err != nil {
			t.Fatal(err)
		}
		var m manifestContent
		if err := json.Unmarshal(oldManifest, &m); err != nil {
			t.Fatal(err)
		}
		oldConfig, err := os.Stat(filepath.Join(r.root, "blobs/sha256", m.Config.Digest[7:9], m.Config.Digest[7:], "data"))
		if err != nil {
			t.Fatal(err)
		}
		if want := int64(len(oldManifest)) + oldConfig.Size() + 70; report.ReclaimableBytes != want {
			t.Fatalf("reclaimable = %d, want %d", report.ReclaimableBytes, want)
		}

		links := report.Untagged[0].links
		wantLinks := []string{
			"repositories/graviteeio/apim-gateway/_manifests/revisions/sha256/" + old[7:],
			"repositories/graviteeio/apim-gateway/_manifests/tags/latest/index/sha256/" + old[7:],
		}
		if !slices.Equal(links, wantLinks) {
			t.Fatalf("links = %v, want %v", links, wantLinks)
		}
	})

	t.Run("children of a tagged index are kept", func(t *testing.T) {
		r := newFakeRegistry(t)
		amd := r.image("app", r.layer("amd64", 10))
		arm := r.image("app", r.layer("arm64", 10))
		r.tag("app", "v1", r.index("app", amd, arm))

		report, err := ScanPreloadGarbage(r.gckHome)
		if err != nil {
			t.Fatal(err)
		}
		if !report.Empty() {
			t.Fatalf("expected empty report, got %+v", report)
		}
	})

	t.Run("blob no manifest references is reclaimable", func(t *testing.T) {
		r := newFakeRegistry(t)
		r.tag("app", "v1", r.image("app", r.layer("kept", 10)))
		r.layer("interrupted-push", 42)

		report, err := ScanPreloadGarbage(r.gckHome)
		if err != nil {
			t.Fatal(err)
		}
		if len(report.Untagged) != 0 || report.ReclaimableBytes != 42 {
			t.Fatalf("got %+v, want 42 reclaimable bytes and no untagged manifest", report)
		}
	})
}
