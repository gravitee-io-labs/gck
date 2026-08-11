package registry

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gravitee-io-labs/gck/internal/config"
	"github.com/gravitee-io-labs/gck/internal/notes"
	gcktmpl "github.com/gravitee-io-labs/gck/internal/template"
	"gopkg.in/yaml.v3"
)

// FSResolver resolves contexts from a local filesystem directory.
type FSResolver struct {
	Root         string // absolute path to the registry root
	GckHome      string
	SetOverrides map[string]string
}

// Resolve reads {Root}/{contextPath}/gck.yaml and returns a
// ResolvedContext whose Dir points to the context directory on disk.
// Values files are already local, so no downloads are needed.
//
// Resolution uses a two-pass approach:
//  1. Read raw gck.yaml, extract own var defaults and path-scoped overrides
//  2. Recurse into from entries, collecting var defaults from each parent
//  3. Compute effective vars (own defaults + child overrides + --set)
//  4. Render gck.yaml with effective vars, unmarshal, and merge
//
// If gck.yaml does not exist, Resolve looks for a .default file
// containing the name of a default variant sub-directory. When found,
// it appends the variant to contextPath and resolves again.
func (r *FSResolver) Resolve(ctx context.Context, contextPath string) (*config.ResolvedContext, error) {
	set := SplitSetOverrides(r.SetOverrides)
	return r.resolveWithVars(ctx, contextPath, nil, set)
}

// resolveWithVars is the internal two-pass resolver. childOverrides are
// path-scoped var overrides collected from a child context's vars block
// that target this or deeper contexts.
func (r *FSResolver) resolveWithVars(ctx context.Context, contextPath string, childOverrides map[string]map[string]string, set SetOverrides) (*config.ResolvedContext, error) {
	selfRegistry := "file://" + r.Root
	ctx, err := withVisited(ctx, contextRef{Registry: selfRegistry, Context: contextPath})
	if err != nil {
		return nil, err
	}

	dir := filepath.Join(r.Root, contextPath)

	data, err := r.readContextFile(dir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("reading context file: %w", err)
		}
		variant, defaultErr := os.ReadFile(filepath.Join(dir, ".default"))
		if defaultErr != nil {
			return nil, fmt.Errorf("reading context file: %w", err)
		}
		name := strings.TrimSpace(string(variant))
		if name == "" {
			return nil, fmt.Errorf("empty .default file in %s", dir)
		}
		return r.resolveWithVars(ctx, filepath.Join(contextPath, name), childOverrides, set)
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

	// Collect path-scoped overrides declared by this context for its parents.
	parentOverrides := mergeOverrideMaps(childOverrides, tree.Overrides)

	// Apply scoped --set to this context.
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

	var ctxCfg config.Config
	if err := yaml.Unmarshal(rendered, &ctxCfg); err != nil {
		return nil, fmt.Errorf("parsing context file %s: %w", contextPath, err)
	}

	if len(ctxCfg.From) > 0 {
		resolved, err := r.resolveFromWithVars(ctx, ctxCfg, dir, contextPath, selfRegistry, parentOverrides, set)
		if err != nil {
			return nil, err
		}
		resolved.EffectiveVars = mergeVarMaps(resolved.EffectiveVars, effectiveVars)
		return resolved, nil
	}

	flags, err := DiscoverFlags(dir)
	if err != nil {
		return nil, fmt.Errorf("discovering flags: %w", err)
	}

	return &config.ResolvedContext{
		Repos:         ctxCfg.Helm.Repos,
		Components:    ctxCfg.Components,
		Dir:           dir,
		Kind:          ctxCfg.Kind,
		Features:      ctxCfg.Features,
		Images:        ctxCfg.Images,
		Notes:         readNotes(dir, contextPath),
		Abstract:      ctxCfg.Abstract,
		Flags:         flags,
		EffectiveVars: effectiveVars,
	}, nil
}

// resolveFromWithVars resolves all from entries with two-pass var resolution.
func (r *FSResolver) resolveFromWithVars(ctx context.Context, childCfg config.Config, childDir, childPath, selfRegistryURL string, overrides map[string]map[string]string, set SetOverrides) (*config.ResolvedContext, error) {
	registryURL := selfRegistryURL
	if childCfg.Registry != "" {
		registryURL = resolveRegistryURL(childCfg.Registry, childDir)
	}

	acc := &config.ResolvedContext{}
	for _, ref := range childCfg.From {
		if strings.HasPrefix(registryURL, "file://") {
			parentResolver := &FSResolver{
				Root:         strings.TrimPrefix(registryURL, "file://"),
				GckHome:      r.GckHome,
				SetOverrides: r.SetOverrides,
			}
			parent, err := parentResolver.resolveWithVars(ctx, ref, overrides, set)
			if err != nil {
				return nil, fmt.Errorf("resolving from %q: %w", ref, err)
			}
			absolutizeComponentPaths(parent)
			MergeInto(acc, parent)
		} else {
			httpResolver := &HTTPResolver{
				BaseURL:      registryURL,
				CacheRoot:    filepath.Join(r.GckHome, "cache"),
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

// mergeOverrideMaps merges VarOverride entries into the existing
// override map (contextPath -> varName -> value).
func mergeOverrideMaps(existing map[string]map[string]string, overrides []gcktmpl.VarOverride) map[string]map[string]string {
	result := make(map[string]map[string]string)
	for k, v := range existing {
		inner := make(map[string]string, len(v))
		for ik, iv := range v {
			inner[ik] = iv
		}
		result[k] = inner
	}
	for _, o := range overrides {
		if result[o.ContextPath] == nil {
			result[o.ContextPath] = make(map[string]string)
		}
		result[o.ContextPath][o.Name] = o.Default
	}
	return result
}

// readNotes loads the notes files from a context directory. contextPath
// identifies the layer so composition can deduplicate a context reached
// through more than one branch.
func readNotes(dir, contextPath string) config.ResolvedNotes {
	var resolved config.ResolvedNotes
	if data, err := os.ReadFile(filepath.Join(dir, "notes.create")); err == nil {
		resolved.Create = []notes.Layer{{Source: contextPath, Raw: string(data)}}
	}
	if data, err := os.ReadFile(filepath.Join(dir, "notes.delete")); err == nil {
		resolved.Delete = []notes.Layer{{Source: contextPath, Raw: string(data)}}
	}
	return resolved
}

func (r *FSResolver) readContextFile(dir string) ([]byte, error) {
	return os.ReadFile(filepath.Join(dir, "gck.yaml"))
}
