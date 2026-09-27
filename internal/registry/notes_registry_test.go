package registry

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/gravitee-io-labs/gck/internal/config"
	"github.com/gravitee-io-labs/gck/internal/notes"
)

// registryRoot is the on-disk registry shipped with gck.
func registryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "registry"))
	if err != nil {
		t.Fatalf("resolving registry root: %v", err)
	}
	return root
}

// registryContexts returns the path of every context in the registry,
// relative to the registry root.
func registryContexts(t *testing.T) []string {
	t.Helper()
	root := registryRoot(t)
	var paths []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || info.Name() != "gck.yaml" {
			return nil
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		t.Fatalf("walking registry: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no contexts found in registry/")
	}
	sort.Strings(paths)
	return paths
}

// flagsIn returns the plain context flag names declared in a context
// directory. Alternatives are left out: they are selected, not toggled.
func flagsIn(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, flagFilePrefix+"*.yaml"))
	if err != nil {
		t.Fatalf("globbing flag files: %v", err)
	}
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		name, err := FlagNameFromFile(filepath.Base(m))
		if err != nil {
			t.Fatalf("%v", err)
		}
		if strings.HasPrefix(name, AlternativePrefix) {
			continue
		}
		names = append(names, name)
	}
	return names
}

// resolvedCombination is a context resolved with one member of each of its
// alternative groups.
type resolvedCombination struct {
	name     string
	resolved *config.ResolvedContext
}

// alternativeCombinations resolves ctxPath once per combination of its
// user-selectable alternatives (the cartesian product of its groups), so
// registry-wide checks cover every stack a user can create, not just the
// defaults.
func alternativeCombinations(t *testing.T, root, ctxPath string) []resolvedCombination {
	t.Helper()
	r := &FSResolver{Root: root}
	base, err := r.Resolve(context.Background(), ctxPath)
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}

	var groups []string
	members := make(map[string][]string)
	for _, f := range base.Flags {
		if !f.IsAlternative() || f.Pinned {
			continue
		}
		if _, ok := members[f.Group]; !ok {
			groups = append(groups, f.Group)
		}
		members[f.Group] = append(members[f.Group], f.Name)
	}
	if len(groups) == 0 {
		return []resolvedCombination{{name: "defaults", resolved: base}}
	}

	combos := [][]string{nil}
	for _, g := range groups {
		var next [][]string
		for _, c := range combos {
			for _, m := range members[g] {
				next = append(next, append(append([]string{}, c...), m))
			}
		}
		combos = next
	}

	out := make([]resolvedCombination, 0, len(combos))
	for _, c := range combos {
		resolved, err := r.Resolve(WithUse(context.Background(), c), ctxPath)
		if err != nil {
			t.Fatalf("resolving with %v: %v", c, err)
		}
		out = append(out, resolvedCombination{name: strings.Join(c, "+"), resolved: resolved})
	}
	return out
}

// TestRegistryNotesCompose resolves every context and composes its notes with
// no flags and with all of them, proving the front matter parses and renders.
// Notes are printed only at the very end of a real deployment, so a malformed
// file would otherwise surface after a cluster has already been built.
func TestRegistryNotesCompose(t *testing.T) {
	root := registryRoot(t)

	for _, ctxPath := range registryContexts(t) {
		for _, combo := range alternativeCombinations(t, root, ctxPath) {
			t.Run(ctxPath+"/"+combo.name, func(t *testing.T) {
				resolved := combo.resolved
				cfg := &config.Config{Kind: resolved.Kind}
				for _, flags := range [][]string{nil, flagsIn(t, filepath.Join(root, ctxPath))} {
					flags = EffectiveFlags(resolved, flags)
					if _, err := notes.Compose(resolved.Notes.Create, cfg, flags); err != nil {
						t.Errorf("flags=%v: %v", flags, err)
					}
					if _, err := notes.Compose(resolved.Notes.Delete, cfg, flags); err != nil {
						t.Errorf("flags=%v: %v", flags, err)
					}
				}
			})
		}
	}
}

// localhostPort matches the port of an endpoint URL that points at the host,
// with or without a scheme: "localhost:30432", "http://127.0.0.1:30080".
var localhostPort = regexp.MustCompile(`^(?:[a-z][a-z0-9+.-]*://)?(?:localhost|127\.0\.0\.1):(\d+)`)

// hostPortDecl matches a hostPort mapping in a raw gck.yaml. The raw file is
// scanned rather than parsed because it is a Go template before rendering.
var hostPortDecl = regexp.MustCompile(`(?m)^\s*hostPort:\s*(\d+)\s*$`)

// TestRegistryNotesMatchPortMappings keeps notes and config in sync in both
// directions:
//
//   - every localhost endpoint a context documents must correspond to a host
//     port its composition actually maps, so a note cannot go stale when a
//     port moves;
//   - every host port a context maps itself must be documented somewhere in
//     its composition, so adding a port and forgetting the note fails here.
//
// The second rule only applies to contexts that document at least one
// localhost endpoint of their own. Contexts that deliberately route through
// hostnames or a load balancer instead (gamma, gateway-api) map ports without
// presenting them as localhost URLs, and are not forced to.
func TestRegistryNotesMatchPortMappings(t *testing.T) {
	root := registryRoot(t)

	for _, ctxPath := range registryContexts(t) {
		for _, combo := range alternativeCombinations(t, root, ctxPath) {
			t.Run(ctxPath+"/"+combo.name, func(t *testing.T) {
				checkNotesMatchPortMappings(t, root, ctxPath, combo.resolved)
			})
		}
	}
}

func checkNotesMatchPortMappings(t *testing.T, root, ctxPath string, resolved *config.ResolvedContext) {
	t.Helper()
	dir := filepath.Join(root, ctxPath)
	if len(resolved.Notes.Create) == 0 {
		return
	}
	flags := EffectiveFlags(resolved, flagsIn(t, dir))

	cfg := &config.Config{Kind: resolved.Kind}
	merged, err := notes.Merge(resolved.Notes.Create, cfg, flags)
	if err != nil {
		t.Fatalf("merging notes: %v", err)
	}

	documented := make(map[int]string)
	for _, ep := range merged.Endpoints {
		m := localhostPort.FindStringSubmatch(ep.URL)
		if m == nil {
			continue
		}
		port, _ := strconv.Atoi(m[1])
		documented[port] = ep.Name
	}

	ownMerged, err := notes.Merge(ownNotes(resolved.Notes.Create, ctxPath), cfg, flags)
	if err != nil {
		t.Fatalf("merging own notes: %v", err)
	}
	ownDocumentsLocalhost := false
	for _, ep := range ownMerged.Endpoints {
		if localhostPort.MatchString(ep.URL) {
			ownDocumentsLocalhost = true
			break
		}
	}

	mapped := make(map[int]bool)
	for _, node := range resolved.Kind.Nodes {
		for _, pm := range node.ExtraPortMappings {
			mapped[int(pm.HostPort)] = true
		}
	}
	// Flag patches are not applied during resolution, so their port
	// mappings are not in resolved.Kind -- but a guarded row pointing
	// at one is still legitimate.
	for _, f := range resolved.Flags {
		if f.IsAlternative() {
			continue // applied during resolution: already in resolved.Kind
		}
		for _, port := range hostPortsIn(t, filepath.Join(f.Dir, flagFilePrefix+f.Name+".yaml")) {
			mapped[port] = true
		}
	}

	for port, name := range documented {
		if !mapped[port] {
			t.Errorf("endpoint %q documents localhost:%d but no context in the composition maps that host port", name, port)
		}
	}

	if !ownDocumentsLocalhost {
		return
	}
	for _, port := range ownHostPorts(t, dir) {
		if _, ok := documented[port]; !ok {
			t.Errorf("gck.yaml maps host port %d but no notes.create in the composition documents it — add an endpoint row (use a `when` guard if it is conditional)", port)
		}
	}
}

// ownNotes returns just the layer a context contributed itself, dropping the
// ones it inherited through from.
func ownNotes(layers []notes.Layer, ctxPath string) []notes.Layer {
	for _, l := range layers {
		if l.Source == ctxPath {
			return []notes.Layer{l}
		}
	}
	return nil
}

// ownHostPorts returns the host ports a context maps itself, from its gck.yaml
// and from its flag patches, ignoring the ones it inherits. Flag files count
// because a port a flag maps still needs a (guarded) endpoint row.
func ownHostPorts(t *testing.T, dir string) []int {
	t.Helper()
	files := []string{filepath.Join(dir, "gck.yaml")}
	flagFiles, err := filepath.Glob(filepath.Join(dir, flagFilePrefix+"*.yaml"))
	if err != nil {
		t.Fatalf("globbing flag files: %v", err)
	}
	files = append(files, flagFiles...)

	var ports []int
	for _, file := range files {
		ports = append(ports, hostPortsIn(t, file)...)
	}
	sort.Ints(ports)
	return ports
}

// hostPortsIn scans one raw YAML file for hostPort mappings. The file is
// scanned rather than parsed because it is a Go template before rendering.
func hostPortsIn(t *testing.T, path string) []int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("reading %s: %v", path, err)
	}
	var ports []int
	for _, m := range hostPortDecl.FindAllStringSubmatch(string(data), -1) {
		port, err := strconv.Atoi(strings.TrimSpace(m[1]))
		if err != nil {
			continue
		}
		ports = append(ports, port)
	}
	return ports
}
