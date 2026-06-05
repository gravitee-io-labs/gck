package registry

import (
	"path/filepath"

	"github.com/gravitee-io-labs/gck/internal/config"
)

// MergeComponents merges user-level component customizations into the resolved
// context. Components are matched by name. For each match the merge applies:
//   - namespace: user wins if non-empty
//   - conditions: user wins if conditions.ready is true
//   - selector: user wins if non-nil
//   - timeout: user wins if non-empty
//   - requires: user requirements are appended (deduplicated by component name)
//   - helm.chart: user wins if non-empty
//   - helm.version: user wins if non-empty
//   - helm.valueFiles: user files are appended (higher precedence in Helm)
//   - helm.values: deep-merged on top of context values (user wins per leaf key)
//
// Value file paths from the user config are resolved relative to configDir.
func MergeComponents(resolved *config.ResolvedContext, components []config.Component, configDir string) {
	if len(components) == 0 {
		return
	}
	byName := make(map[string]*config.Component, len(resolved.Components))
	for i := range resolved.Components {
		byName[resolved.Components[i].Name] = &resolved.Components[i]
	}
	// Two-pass merge: process all merges first (via byName pointers), then
	// append new components. Appending during the merge loop would invalidate
	// byName pointers when the slice's backing array is reallocated.
	var newComponents []config.Component
	for _, patch := range components {
		comp, ok := byName[patch.Name]
		if !ok {
			resolveValueFilePaths(&patch, configDir)
			resolveManifestFilePaths(&patch, configDir)
			resolveLocalResourcePaths(&patch, configDir)
			newComponents = append(newComponents, patch)
			continue
		}
		if len(patch.Requires) > 0 {
			existing := make(map[string]bool, len(comp.Requires))
			for _, r := range comp.Requires {
				existing[r.Component] = true
			}
			for _, r := range patch.Requires {
				if !existing[r.Component] {
					comp.Requires = append(comp.Requires, r)
				}
			}
		}
		if patch.Enabled != nil {
			comp.Enabled = patch.Enabled
		}
		if patch.Namespace != "" {
			comp.Namespace = patch.Namespace
		}
		if patch.Conditions.Ready {
			comp.Conditions = patch.Conditions
		}
		if patch.Selector != nil {
			comp.Selector = patch.Selector
		}
		if patch.Timeout != "" {
			comp.Timeout = patch.Timeout
		}
		if patch.Helm != nil && comp.Helm != nil {
			if patch.Helm.Chart != "" {
				comp.Helm.Chart = patch.Helm.Chart
			}
			if patch.Helm.Version != "" {
				comp.Helm.Version = patch.Helm.Version
			}
			resolveValueFilePaths(&patch, configDir)
			comp.Helm.ValueFiles = append(comp.Helm.ValueFiles, patch.Helm.ValueFiles...)
			if len(patch.Helm.Values) > 0 {
				if comp.Helm.Values == nil {
					comp.Helm.Values = make(map[string]interface{})
				}
				deepMergeValues(comp.Helm.Values, patch.Helm.Values)
			}
		}
		if patch.K8s != nil {
			if comp.K8s == nil {
				comp.K8s = &config.K8sSpec{}
			}
			resolveManifestFilePaths(&patch, configDir)
			resolveLocalResourcePaths(&patch, configDir)
			comp.K8s.ManifestFiles = append(comp.K8s.ManifestFiles, patch.K8s.ManifestFiles...)
			comp.K8s.Manifests = mergeManifests(comp.K8s.Manifests, patch.K8s.Manifests)
			comp.K8s.Secrets = append(comp.K8s.Secrets, patch.K8s.Secrets...)
			comp.K8s.ConfigMaps = append(comp.K8s.ConfigMaps, patch.K8s.ConfigMaps...)
		}
	}
	resolved.Components = append(resolved.Components, newComponents...)
}

// manifestKey identifies a Kubernetes resource by its API coordinates and name.
type manifestKey struct {
	apiVersion string
	kind       string
	name       string
	namespace  string
}

// manifestKeyOf extracts the resource identity from an unstructured manifest.
func manifestKeyOf(m map[string]interface{}) manifestKey {
	str := func(v interface{}) string {
		s, _ := v.(string)
		return s
	}
	key := manifestKey{
		apiVersion: str(m["apiVersion"]),
		kind:       str(m["kind"]),
	}
	if meta, ok := m["metadata"].(map[string]interface{}); ok {
		key.name = str(meta["name"])
		key.namespace = str(meta["namespace"])
	}
	return key
}

// mergeManifests returns the union of base and override manifests,
// deduplicated by (apiVersion, kind, name, namespace). When both sides
// define the same resource, the override entry wins.
func mergeManifests(base, override []map[string]interface{}) []map[string]interface{} {
	if len(override) == 0 {
		return base
	}
	if len(base) == 0 {
		return override
	}
	seen := make(map[manifestKey]int, len(base))
	result := make([]map[string]interface{}, len(base))
	copy(result, base)
	for i, m := range result {
		seen[manifestKeyOf(m)] = i
	}
	for _, m := range override {
		key := manifestKeyOf(m)
		if idx, ok := seen[key]; ok {
			result[idx] = m
		} else {
			seen[key] = len(result)
			result = append(result, m)
		}
	}
	return result
}

// deepMergeValues recursively merges src into dst. When both dst[k] and src[k]
// are maps, the merge recurses. When both are named lists (every element is a
// map with a "name" key), the lists are merged by name. Otherwise src[k] wins.
func deepMergeValues(dst, src map[string]interface{}) {
	for k, srcVal := range src {
		dstVal, exists := dst[k]
		if !exists {
			dst[k] = srcVal
			continue
		}
		dstMap, dstOk := dstVal.(map[string]interface{})
		srcMap, srcOk := srcVal.(map[string]interface{})
		if dstOk && srcOk {
			deepMergeValues(dstMap, srcMap)
			continue
		}
		dstList, dstOk := dstVal.([]interface{})
		srcList, srcOk := srcVal.([]interface{})
		if dstOk && srcOk && isNamedList(dstList) && isNamedList(srcList) {
			dst[k] = mergeNamedList(dstList, srcList)
			continue
		}
		dst[k] = srcVal
	}
}

// isNamedList returns true if list is non-empty and every element is a
// map[string]interface{} containing a "name" key. This matches the Kubernetes
// convention for env, ports, volumeMounts, etc.
func isNamedList(list []interface{}) bool {
	if len(list) == 0 {
		return false
	}
	for _, item := range list {
		m, ok := item.(map[string]interface{})
		if !ok {
			return false
		}
		if _, has := m["name"]; !has {
			return false
		}
	}
	return true
}

// mergeNamedList merges two named lists by the "name" field. Entries from src
// with the same name as an entry in dst override it in place; new entries are
// appended.
func mergeNamedList(dst, src []interface{}) []interface{} {
	byName := make(map[string]int, len(dst))
	result := make([]interface{}, len(dst))
	copy(result, dst)
	for i, item := range result {
		byName[item.(map[string]interface{})["name"].(string)] = i
	}
	for _, item := range src {
		name := item.(map[string]interface{})["name"].(string)
		if idx, ok := byName[name]; ok {
			result[idx] = item
		} else {
			byName[name] = len(result)
			result = append(result, item)
		}
	}
	return result
}

// resolveValueFilePaths resolves relative value file paths in a component's
// HelmSpec to absolute paths based on configDir.
func resolveValueFilePaths(c *config.Component, configDir string) {
	if c.Helm == nil || configDir == "" {
		return
	}
	for i, v := range c.Helm.ValueFiles {
		if !filepath.IsAbs(v) {
			c.Helm.ValueFiles[i] = filepath.Join(configDir, v)
		}
	}
}

// resolveManifestFilePaths resolves relative manifest file paths to absolute
// paths based on configDir. This allows user configs to reference manifest
// files relative to their own location rather than the registry context dir.
func resolveManifestFilePaths(c *config.Component, configDir string) {
	if c.K8s == nil || configDir == "" {
		return
	}
	for i, f := range c.K8s.ManifestFiles {
		if !filepath.IsAbs(f) {
			c.K8s.ManifestFiles[i] = filepath.Join(configDir, f)
		}
	}
}

// resolveLocalResourcePaths resolves relative fromFile paths in Secrets and
// ConfigMaps to absolute paths based on configDir. Both the shorthand
// top-level FromFile and per-entry FromFile fields are resolved.
func resolveLocalResourcePaths(c *config.Component, configDir string) {
	if c.K8s == nil || configDir == "" {
		return
	}
	resolveResources := func(resources []config.LocalResource) {
		for i := range resources {
			if resources[i].FromFile != "" && !filepath.IsAbs(resources[i].FromFile) {
				resources[i].FromFile = filepath.Join(configDir, resources[i].FromFile)
			}
			for j := range resources[i].Entries {
				if resources[i].Entries[j].FromFile != "" && !filepath.IsAbs(resources[i].Entries[j].FromFile) {
					resources[i].Entries[j].FromFile = filepath.Join(configDir, resources[i].Entries[j].FromFile)
				}
			}
		}
	}
	resolveResources(c.K8s.Secrets)
	resolveResources(c.K8s.ConfigMaps)
}

// absolutizeComponentPaths makes all relative component file paths in rc
// absolute using rc.Dir. This is necessary before merging multiple parents
// whose files live in different directories.
func absolutizeComponentPaths(rc *config.ResolvedContext) {
	for i := range rc.Components {
		resolveValueFilePaths(&rc.Components[i], rc.Dir)
		resolveManifestFilePaths(&rc.Components[i], rc.Dir)
		resolveLocalResourcePaths(&rc.Components[i], rc.Dir)
	}
}

// MergeRepos merges local repos into context repos, deduplicating by name.
// When both lists contain a repo with the same name, the local entry wins.
func MergeRepos(contextRepos, localRepos []config.Repo) []config.Repo {
	if len(localRepos) == 0 {
		return contextRepos
	}
	localByName := make(map[string]config.Repo, len(localRepos))
	for _, r := range localRepos {
		localByName[r.Name] = r
	}
	out := make([]config.Repo, 0, len(contextRepos)+len(localRepos))
	for _, r := range contextRepos {
		if local, ok := localByName[r.Name]; ok {
			out = append(out, local)
			delete(localByName, r.Name)
		} else {
			out = append(out, r)
		}
	}
	for _, r := range localRepos {
		if _, ok := localByName[r.Name]; ok {
			out = append(out, r)
		}
	}
	return out
}
