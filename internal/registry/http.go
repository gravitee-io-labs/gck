package registry

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gravitee-io-labs/gck/internal/config"
	gcktmpl "github.com/gravitee-io-labs/gck/internal/template"
	"gopkg.in/yaml.v3"
)

// HTTPResolver resolves contexts by fetching files from an HTTP registry.
type HTTPResolver struct {
	BaseURL      string
	CacheRoot    string
	GckHome      string
	HTTPClient   *http.Client
	SetOverrides map[string]string
}

// Resolve fetches {BaseURL}/{contextPath}/gck.yaml, downloads referenced
// values files to a local cache directory, and returns a ResolvedContext
// whose Dir points to that cache.
//
// Resolution uses a two-pass approach (same as FSResolver):
//  1. Read raw gck.yaml, extract own var defaults and path-scoped overrides
//  2. Recurse into from entries, collecting var defaults from each parent
//  3. Compute effective vars (own defaults + child overrides + --set)
//  4. Render gck.yaml with effective vars, unmarshal, and merge
//
// If gck.yaml returns 404, Resolve tries the .default variant lookup.
func (r *HTTPResolver) Resolve(ctx context.Context, contextPath string) (*config.ResolvedContext, error) {
	set := SplitSetOverrides(r.SetOverrides)
	return r.resolveWithVars(ctx, contextPath, nil, set)
}

// resolveWithVars is the internal two-pass resolver. childOverrides are
// path-scoped var overrides collected from a child context's vars block
// that target this or deeper contexts.
func (r *HTTPResolver) resolveWithVars(ctx context.Context, contextPath string, childOverrides map[string]map[string]string, set SetOverrides) (*config.ResolvedContext, error) {
	ctx, err := withVisited(ctx, contextRef{Registry: r.BaseURL, Context: contextPath})
	if err != nil {
		return nil, err
	}

	baseURL := strings.TrimSuffix(r.BaseURL, "/")
	client := r.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	data, status, err := r.fetchContextFile(ctx, client, baseURL, contextPath)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		variant, defaultErr := r.fetchDefault(ctx, client, baseURL, contextPath)
		if defaultErr != nil {
			return nil, fmt.Errorf("fetching context: 404 Not Found")
		}
		return r.resolveWithVars(ctx, contextPath+"/"+variant, childOverrides, set)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("fetching context: %d", status)
	}

	tree, err := gcktmpl.ExtractVarsTree(data)
	if err != nil {
		return nil, fmt.Errorf("extracting vars from %s: %w", contextPath, err)
	}

	ownDefaults := make(map[string]string, len(tree.Defs))
	for _, d := range tree.Defs {
		ownDefaults[d.Name] = d.Default
	}

	myOverrides := make(map[string]string)
	if childOverrides != nil {
		if overrides, ok := childOverrides[contextPath]; ok {
			for k, v := range overrides {
				myOverrides[k] = v
			}
		}
	}

	parentOverrides := mergeOverrideMaps(childOverrides, tree.Overrides)

	scopedForMe := make(map[string]string)
	knownPaths := map[string]bool{contextPath: true}
	for dottedKey, val := range set.Scoped {
		ctxPath, varName := resolveScopedOverride(dottedKey, knownPaths)
		if ctxPath == contextPath {
			scopedForMe[varName] = val
		}
	}

	effectiveVars := computeEffectiveVars(ownDefaults, myOverrides, set)
	for k, v := range scopedForMe {
		effectiveVars[k] = v
	}

	rendered, err := gcktmpl.RenderWithVars(data, effectiveVars)
	if err != nil {
		return nil, fmt.Errorf("templating context file %s: %w", contextPath, err)
	}

	var parsed config.Config
	if err := yaml.Unmarshal(rendered, &parsed); err != nil {
		return nil, fmt.Errorf("parsing context file %s: %w", contextPath, err)
	}

	cacheDir := filepath.Join(r.CacheRoot, contextPath)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating cache dir: %w", err)
	}

	var filesToFetch []string
	seen := make(map[string]bool)
	for _, comp := range parsed.Components {
		if comp.Helm != nil {
			for _, v := range comp.Helm.ValueFiles {
				if !seen[v] {
					seen[v] = true
					filesToFetch = append(filesToFetch, v)
				}
			}
		}
		if comp.K8s != nil {
			for _, f := range comp.K8s.ManifestFiles {
				if !seen[f] {
					seen[f] = true
					filesToFetch = append(filesToFetch, f)
				}
			}
		}
	}

	for _, f := range filesToFetch {
		if err := r.fetchAndCache(ctx, client, baseURL, contextPath, cacheDir, f, false); err != nil {
			return nil, err
		}
	}

	for _, notesFile := range []string{"notes.create", "notes.delete"} {
		if err := r.fetchAndCache(ctx, client, baseURL, contextPath, cacheDir, notesFile, true); err != nil {
			return nil, err
		}
	}

	flags, err := r.fetchFlags(ctx, client, baseURL, contextPath, cacheDir)
	if err != nil {
		return nil, err
	}

	if len(parsed.From) > 0 {
		resolved, err := r.resolveFromWithVars(ctx, parsed, cacheDir, contextPath, parentOverrides, set)
		if err != nil {
			return nil, err
		}
		resolved.EffectiveVars = mergeVarMaps(resolved.EffectiveVars, effectiveVars)
		return resolved, nil
	}

	return &config.ResolvedContext{
		Repos:         parsed.Helm.Repos,
		Components:    parsed.Components,
		Dir:           cacheDir,
		Kind:          parsed.Kind,
		Features:      parsed.Features,
		Images:        parsed.Images,
		Notes:         readNotes(cacheDir, contextPath),
		Abstract:      parsed.Abstract,
		Flags:         flags,
		EffectiveVars: effectiveVars,
	}, nil
}

// resolveFromWithVars resolves all from entries with two-pass var resolution.
func (r *HTTPResolver) resolveFromWithVars(ctx context.Context, childCfg config.Config, childDir, childPath string, overrides map[string]map[string]string, set SetOverrides) (*config.ResolvedContext, error) {
	registryURL := r.BaseURL
	if childCfg.Registry != "" {
		registryURL = resolveRegistryURL(childCfg.Registry, childDir)
	}

	acc := &config.ResolvedContext{}
	for _, ref := range childCfg.From {
		if strings.HasPrefix(registryURL, "file://") {
			fsResolver := &FSResolver{
				Root:         strings.TrimPrefix(registryURL, "file://"),
				GckHome:      r.GckHome,
				SetOverrides: r.SetOverrides,
			}
			parent, err := fsResolver.resolveWithVars(ctx, ref, overrides, set)
			if err != nil {
				return nil, fmt.Errorf("resolving from %q: %w", ref, err)
			}
			absolutizeComponentPaths(parent)
			MergeInto(acc, parent)
		} else {
			httpResolver := &HTTPResolver{
				BaseURL:      registryURL,
				CacheRoot:    r.CacheRoot,
				GckHome:      r.GckHome,
				HTTPClient:   newAuthenticatedClient(registryURL),
				SetOverrides: r.SetOverrides,
			}
			parent, err := httpResolver.resolveWithVars(ctx, ref, overrides, set)
			if err != nil {
				return nil, fmt.Errorf("resolving from %q: %w", ref, err)
			}
			absolutizeComponentPaths(parent)
			MergeInto(acc, parent)
		}
	}

	MergeComponents(acc, childCfg.Components, childDir)
	acc.Repos = MergeRepos(acc.Repos, childCfg.Helm.Repos)
	acc.Features = config.MergeFeatures(acc.Features, childCfg.Features)
	acc.Kind = mergeKind(acc.Kind, childCfg.Kind)
	acc.Images = config.MergeImages(acc.Images, childCfg.Images)
	acc.Notes = appendNotes(acc.Notes, readNotes(childDir, childPath))
	acc.Abstract = childCfg.Abstract

	childFlags, err := DiscoverFlags(childDir)
	if err != nil {
		return nil, fmt.Errorf("discovering flags: %w", err)
	}
	acc.Flags = MergeFlags(acc.Flags, childFlags)

	return acc, nil
}

// fetchDefault fetches {baseURL}/{contextPath}/.default and returns the
// trimmed variant name. It returns an error if the file does not exist,
// the server returns a non-200 status, or the file content is empty.
func (r *HTTPResolver) fetchDefault(ctx context.Context, client *http.Client, baseURL, contextPath string) (string, error) {
	u := baseURL + "/" + contextPath + "/.default"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("building request for .default: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching .default: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching .default: %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading .default: %w", err)
	}
	name := strings.TrimSpace(string(data))
	if name == "" {
		return "", fmt.Errorf("empty .default file at %s", u)
	}
	return name, nil
}

// fetchAndCache fetches a single file from the remote registry and writes it
// to the local cache directory. When ignoreNotFound is true, a 404 response
// is silently skipped instead of treated as an error.
func (r *HTTPResolver) fetchAndCache(ctx context.Context, client *http.Client, baseURL, contextPath, cacheDir, filename string, ignoreNotFound bool) error {
	u := baseURL + "/" + contextPath + "/" + filename
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("building request for %s: %w", filename, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", filename, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound && ignoreNotFound {
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching %s: %s", filename, resp.Status)
	}
	outPath := filepath.Join(cacheDir, filename)
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		return fmt.Errorf("writing %s: %w", filename, err)
	}
	return nil
}

type flagsManifest struct {
	Flags []flagEntry `yaml:"flags"`
}

type flagEntry struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// fetchFlags fetches the gck.flags.yaml manifest from the remote registry,
// downloads each referenced flag patch file to cacheDir, and returns the
// corresponding ContextFlag entries. A 404 on the manifest means no flags.
func (r *HTTPResolver) fetchFlags(ctx context.Context, client *http.Client, baseURL, contextPath, cacheDir string) ([]config.ContextFlag, error) {
	u := baseURL + "/" + contextPath + "/gck.flags.yaml"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("building request for flags manifest: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching flags manifest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching flags manifest: %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading flags manifest: %w", err)
	}

	var manifest flagsManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parsing flags manifest: %w", err)
	}

	var flags []config.ContextFlag
	for _, entry := range manifest.Flags {
		flagFile := flagFilePrefix + entry.Name + ".yaml"
		if err := r.fetchAndCache(ctx, client, baseURL, contextPath, cacheDir, flagFile, true); err != nil {
			return nil, err
		}
		flags = append(flags, config.ContextFlag{
			Name:        entry.Name,
			Description: entry.Description,
			Dir:         cacheDir,
		})
	}
	return flags, nil
}

// fetchContextFile fetches gck.yaml from the registry. It returns the body
// bytes, the HTTP status code, and any transport-level error. A 404 is
// returned as status (not as an error) so the caller can attempt the
// .default fallback.
func (r *HTTPResolver) fetchContextFile(ctx context.Context, client *http.Client, baseURL, contextPath string) ([]byte, int, error) {
	u := baseURL + "/" + contextPath + "/gck.yaml"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("building request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("fetching context: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, nil
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("reading context: %w", err)
	}
	return data, http.StatusOK, nil
}




