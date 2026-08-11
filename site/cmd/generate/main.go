package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gravitee-io-labs/gck/internal/notes"
	gcktmpl "github.com/gravitee-io-labs/gck/internal/template"
	"gopkg.in/yaml.v3"
)

type gckConfig struct {
	Abstract   bool           `yaml:"abstract"`
	From       []string       `yaml:"from"`
	Kind       gckKind        `yaml:"kind"`
	Components []gckComponent `yaml:"components"`
}

type gckKind struct {
	Name string `yaml:"name"`
}

type gckComponent struct {
	Name string `yaml:"name"`
}

type readmeFrontmatter struct {
	Title       string   `yaml:"title"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags"`
}

type flagInfo struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
}

type varInfo struct {
	Name        string `yaml:"name"`
	Default     string `yaml:"default"`
	Description string `yaml:"description,omitempty"`
	Origin      string `yaml:"origin,omitempty"`
}

type endpointInfo struct {
	Name   string `yaml:"name"`
	URL    string `yaml:"url"`
	Note   string `yaml:"note,omitempty"`
	Origin string `yaml:"origin,omitempty"`
	// Requires lists the context flags that must all be passed for this
	// endpoint to exist. Empty means a plain "gck create" exposes it.
	Requires []string `yaml:"requires,omitempty"`
}

type componentPage struct {
	Title       string     `yaml:"title"`
	Layout      string     `yaml:"layout"`
	Path        string     `yaml:"path"`
	Context     bool       `yaml:"context"`
	Description string     `yaml:"description,omitempty"`
	Tags        []string   `yaml:"tags,omitempty"`
	From        []string   `yaml:"from,omitempty"`
	Components  []string   `yaml:"components,omitempty"`
	Flags       []flagInfo     `yaml:"flags,omitempty"`
	Vars        []varInfo      `yaml:"vars,omitempty"`
	Endpoints   []endpointInfo `yaml:"endpoints,omitempty"`
	Icon        string         `yaml:"icon,omitempty"`
	Type        string         `yaml:"type"`
}

type sectionPage struct {
	Title          string `yaml:"title"`
	DefaultVariant string `yaml:"default_variant,omitempty"`
	Type           string `yaml:"type"`
}

func main() {
	registryDir := "registry"
	contentDir := "site/content/registry"
	staticDir := "site/static"

	if err := os.RemoveAll(contentDir); err != nil {
		fatalf("clean %s: %v", contentDir, err)
	}
	if err := os.MkdirAll(contentDir, 0755); err != nil {
		fatalf("create %s: %v", contentDir, err)
	}

	writeRegistryRoot(contentDir)

	configs := map[string]*gckConfig{}
	componentDirs := map[string]bool{}
	intermediateDirs := map[string]bool{}

	err := filepath.WalkDir(registryDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "gck.yaml" {
			return nil
		}

		dir := filepath.Dir(path)
		relDir, err := filepath.Rel(registryDir, dir)
		if err != nil {
			return fmt.Errorf("relative path for %s: %w", dir, err)
		}

		config, err := parseGckConfig(path)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}

		configs[relDir] = config

		if config.Abstract {
			fmt.Printf("skip abstract: %s\n", relDir)
			return nil
		}

		componentDirs[relDir] = true
		collectIntermediateDirs(relDir, intermediateDirs)
		return nil
	})
	if err != nil {
		fatalf("walk registry: %v", err)
	}

	for relDir := range componentDirs {
		config := configs[relDir]
		dir := filepath.Join(registryDir, relDir)

		readmeTitle, description, tags, body := parseReadme(filepath.Join(dir, "README.md"))

		title := relDir
		if readmeTitle != "" {
			title = readmeTitle
		}
		page := componentPage{
			Title:       title,
			Layout:      "detail",
			Path:        relDir,
			Context:     true,
			Description: description,
			Tags:        tags,
			From:        config.From,
			Components:  resolveComponents(relDir, configs),
			Flags:       resolveFlags(relDir, configs, registryDir),
			Vars:        resolveVars(relDir, configs, registryDir),
			Endpoints:   resolveEndpoints(relDir, configs, registryDir),
			Icon:        resolveIcon(registryDir, relDir),
			Type:        "registry",
		}

		outDir := filepath.Join(contentDir, relDir)
		if err := os.MkdirAll(outDir, 0755); err != nil {
			fatalf("mkdir %s: %v", outDir, err)
		}
		if err := writePage(filepath.Join(outDir, "_index.md"), page, body); err != nil {
			fatalf("write page %s: %v", relDir, err)
		}

		fmt.Printf("generated: %s\n", relDir)
	}

	for dir := range intermediateDirs {
		if componentDirs[dir] {
			continue
		}

		defaultVariant := readOptionalFile(filepath.Join(registryDir, dir, ".default"))

		page := sectionPage{
			Title:          dir,
			DefaultVariant: defaultVariant,
			Type:           "registry",
		}

		outDir := filepath.Join(contentDir, dir)
		if err := os.MkdirAll(outDir, 0755); err != nil {
			fatalf("mkdir intermediate %s: %v", outDir, err)
		}
		if err := writePage(filepath.Join(outDir, "_index.md"), page, ""); err != nil {
			fatalf("write intermediate %s: %v", dir, err)
		}

		fmt.Printf("generated section: %s\n", dir)
	}

	if err := cleanStaticRegistry(staticDir); err != nil {
		fatalf("clean static: %v", err)
	}

	if err := copyToStatic(registryDir, staticDir); err != nil {
		fatalf("copy to static: %v", err)
	}

	if err := generateFlagsManifests(registryDir, staticDir, configs); err != nil {
		fatalf("generate flags manifests: %v", err)
	}

	generateSchemaDoc("schema/gck.schema.yaml", "site/content/docs/reference/configuration.md")
	generateContributingDoc("CONTRIBUTING.md", "site/content/docs/reference/contributing.md")

	fmt.Println("done")
}

func resolveComponents(relDir string, configs map[string]*gckConfig) []string {
	seen := map[string]bool{}
	var result []string
	var walk func(string)
	walk = func(dir string) {
		config, ok := configs[dir]
		if !ok {
			return
		}
		for _, parent := range config.From {
			walk(parent)
		}
		for _, c := range config.Components {
			if !seen[c.Name] {
				seen[c.Name] = true
				result = append(result, c.Name)
			}
		}
	}
	walk(relDir)
	return result
}

// resolveEndpoints walks the from chain for relDir, collecting each level's
// notes.create, and folds them with the same merge the CLI uses so a row
// declared on an abstract base surfaces on every concrete context that
// composes it -- attributed, via Origin, to the context that declared it.
//
// Rows a plain "gck create" exposes come first. Rows that only exist behind a
// context flag follow, each carrying the flags that reveal it -- see
// requiredFlags.
func resolveEndpoints(relDir string, configs map[string]*gckConfig, registryDir string) []endpointInfo {
	visited := map[string]bool{}
	var layers []notes.Layer
	var walk func(string)
	walk = func(dir string) {
		config, ok := configs[dir]
		if !ok || visited[dir] {
			return
		}
		visited[dir] = true
		for _, parent := range config.From {
			walk(parent)
		}
		data, err := os.ReadFile(filepath.Join(registryDir, dir, "notes.create"))
		if err != nil {
			return
		}
		layers = append(layers, notes.Layer{Source: dir, Raw: string(data)})
	}
	walk(relDir)

	toInfo := func(ep notes.Endpoint, requires []string) endpointInfo {
		return endpointInfo{
			Name:     ep.Name,
			URL:      ep.URL,
			Note:     ep.Note,
			Origin:   ep.Origin,
			Requires: requires,
		}
	}

	var result []endpointInfo
	shown := map[string]bool{}
	for _, ep := range visibleWith(relDir, layers, nil) {
		shown[ep.Name] = true
		result = append(result, toInfo(ep, nil))
	}

	// Anything the full flag set reveals but the default view does not is
	// gated; work out which flags each of those rows actually needs.
	var flagNames []string
	for _, f := range resolveFlags(relDir, configs, registryDir) {
		flagNames = append(flagNames, f.Name)
	}
	if len(flagNames) == 0 {
		return result
	}
	for _, ep := range visibleWith(relDir, layers, flagNames) {
		if shown[ep.Name] {
			continue
		}
		shown[ep.Name] = true
		result = append(result, toInfo(ep, requiredFlags(relDir, layers, flagNames, ep.Name)))
	}
	return result
}

// visibleWith merges the layers with the given flags active and returns the
// rows whose guards pass.
func visibleWith(relDir string, layers []notes.Layer, flags []string) []notes.Endpoint {
	merged, err := notes.Merge(layers, nil, flags)
	if err != nil {
		fatalf("merge notes for %s: %v", relDir, err)
	}
	return merged.VisibleEndpoints()
}

// requiredFlags determines which of the active flags a gated endpoint actually
// depends on, by leaving each one out in turn: if dropping a flag hides the
// row, the row needs it. This reads the guard's behaviour rather than parsing
// its expression, so an `and` of two flags reports both.
func requiredFlags(relDir string, layers []notes.Layer, allFlags []string, name string) []string {
	var required []string
	for _, candidate := range allFlags {
		without := make([]string, 0, len(allFlags)-1)
		for _, f := range allFlags {
			if f != candidate {
				without = append(without, f)
			}
		}
		if !containsEndpoint(visibleWith(relDir, layers, without), name) {
			required = append(required, candidate)
		}
	}
	return required
}

func containsEndpoint(eps []notes.Endpoint, name string) bool {
	for _, ep := range eps {
		if ep.Name == name {
			return true
		}
	}
	return false
}

func resolveIcon(registryDir, relDir string) string {
	dir := relDir
	for {
		candidate := filepath.Join(registryDir, dir, "icon.svg")
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Join(dir, "icon.svg")
		}
		parent := filepath.Dir(dir)
		if parent == dir || parent == "." {
			break
		}
		dir = parent
	}
	return ""
}

func writeRegistryRoot(contentDir string) {
	page := sectionPage{
		Title: "Registry",
		Type:  "registry",
	}
	if err := writePage(filepath.Join(contentDir, "_index.md"), page, "Browse available components and application stacks."); err != nil {
		fatalf("write registry root: %v", err)
	}
}

func parseGckConfig(path string) (*gckConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var config gckConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func parseReadme(path string) (title string, description string, tags []string, body string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", nil, ""
	}

	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		return "", "", nil, strings.TrimSpace(content)
	}

	rest := content[4:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", "", nil, strings.TrimSpace(content)
	}

	var fm readmeFrontmatter
	if err := yaml.Unmarshal([]byte(rest[:idx]), &fm); err != nil {
		return "", "", nil, strings.TrimSpace(content)
	}

	bodyStart := idx + 4
	if bodyStart < len(rest) {
		body = strings.TrimSpace(rest[bodyStart:])
	}

	body = stripLeadingH1(body)

	return fm.Title, fm.Description, fm.Tags, body
}

func stripLeadingH1(body string) string {
	if !strings.HasPrefix(body, "# ") {
		return body
	}
	if nl := strings.Index(body, "\n"); nl >= 0 {
		return strings.TrimSpace(body[nl+1:])
	}
	return ""
}

func readOptionalFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func collectIntermediateDirs(relDir string, intermediates map[string]bool) {
	parts := strings.Split(relDir, string(filepath.Separator))
	for i := 1; i < len(parts); i++ {
		intermediates[filepath.Join(parts[:i]...)] = true
	}
}

func writePage(path string, frontmatter any, body string) error {
	var buf bytes.Buffer
	buf.WriteString("---\n")

	fmBytes, err := yaml.Marshal(frontmatter)
	if err != nil {
		return fmt.Errorf("marshal frontmatter: %w", err)
	}
	buf.Write(fmBytes)
	buf.WriteString("---\n")

	if body != "" {
		buf.WriteString("\n")
		buf.WriteString(body)
		buf.WriteString("\n")
	}

	return os.WriteFile(path, buf.Bytes(), 0644)
}

func cleanStaticRegistry(staticDir string) error {
	entries, err := os.ReadDir(staticDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", staticDir, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		target := filepath.Join(staticDir, e.Name())
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("remove %s: %w", target, err)
		}
	}
	return nil
}

func copyToStatic(registryDir, staticDir string) error {
	return filepath.WalkDir(registryDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(registryDir, path)
		if err != nil {
			return err
		}

		dest := filepath.Join(staticDir, relPath)
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		fmt.Printf("static: %s\n", relPath)
		return os.WriteFile(dest, data, 0644)
	})
}

func generateContributingDoc(srcPath, outputPath string) {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		fatalf("read %s: %v", srcPath, err)
	}

	body := stripLeadingH1(strings.TrimSpace(string(data)))

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.WriteString("title: \"Contributing\"\n")
	buf.WriteString("weight: 4\n")
	buf.WriteString("type: docs\n")
	buf.WriteString("---\n\n")
	buf.WriteString(body)
	buf.WriteString("\n")

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		fatalf("mkdir for %s: %v", outputPath, err)
	}
	if err := os.WriteFile(outputPath, buf.Bytes(), 0644); err != nil {
		fatalf("write contributing doc: %v", err)
	}

	fmt.Printf("generated contributing doc: %s\n", outputPath)
}

func generateFlagsManifests(registryDir, staticDir string, configs map[string]*gckConfig) error {
	for relDir := range configs {
		flags := resolveFlags(relDir, configs, registryDir)
		if len(flags) == 0 {
			continue
		}

		staticCtxDir := filepath.Join(staticDir, relDir)
		if err := os.MkdirAll(staticCtxDir, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", staticCtxDir, err)
		}

		manifest := struct {
			Flags []flagInfo `yaml:"flags"`
		}{Flags: flags}
		data, err := yaml.Marshal(manifest)
		if err != nil {
			return fmt.Errorf("marshal flags for %s: %w", relDir, err)
		}
		if err := os.WriteFile(filepath.Join(staticCtxDir, "gck.flags.yaml"), data, 0644); err != nil {
			return fmt.Errorf("write flags manifest for %s: %w", relDir, err)
		}

		for _, f := range flags {
			flagFile := "gck--" + f.Name + ".yaml"
			destPath := filepath.Join(staticCtxDir, flagFile)
			if _, err := os.Stat(destPath); err == nil {
				continue
			}
			srcPath := findFlagSource(relDir, f.Name, configs, registryDir)
			if srcPath == "" {
				continue
			}
			srcData, err := os.ReadFile(srcPath)
			if err != nil {
				return fmt.Errorf("read flag source %s: %w", srcPath, err)
			}
			if err := os.WriteFile(destPath, srcData, 0644); err != nil {
				return fmt.Errorf("write inherited flag %s: %w", destPath, err)
			}
			fmt.Printf("static (inherited flag): %s/%s\n", relDir, flagFile)
		}

		fmt.Printf("flags manifest: %s\n", relDir)
	}
	return nil
}

func discoverLocalFlags(dir string) []flagInfo {
	matches, err := filepath.Glob(filepath.Join(dir, "gck--*.yaml"))
	if err != nil || len(matches) == 0 {
		return nil
	}
	sort.Strings(matches)
	var flags []flagInfo
	for _, path := range matches {
		name := strings.TrimPrefix(filepath.Base(path), "gck--")
		name = strings.TrimSuffix(name, ".yaml")
		flags = append(flags, flagInfo{
			Name:        name,
			Description: readFlagDescription(path),
		})
	}
	return flags
}

func readFlagDescription(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var partial struct {
		Description string `yaml:"description"`
	}
	_ = yaml.Unmarshal(data, &partial)
	return partial.Description
}

// resolveFlags walks the from chain for relDir, discovering flags at each
// level and merging them (child overrides parent for the same name).
func resolveFlags(relDir string, configs map[string]*gckConfig, registryDir string) []flagInfo {
	seen := map[string]int{}
	var result []flagInfo
	var walk func(string)
	walk = func(dir string) {
		config, ok := configs[dir]
		if !ok {
			return
		}
		for _, parent := range config.From {
			walk(parent)
		}
		for _, f := range discoverLocalFlags(filepath.Join(registryDir, dir)) {
			if idx, ok := seen[f.Name]; ok {
				result[idx] = f
			} else {
				seen[f.Name] = len(result)
				result = append(result, f)
			}
		}
	}
	walk(relDir)
	return result
}

// resolveVars walks the from chain for relDir, extracting var definitions
// from each gck.yaml and merging them (child overrides parent for same name).
func resolveVars(relDir string, configs map[string]*gckConfig, registryDir string) []varInfo {
	seen := map[string]int{}
	var result []varInfo
	var walk func(string)
	walk = func(dir string) {
		if _, ok := configs[dir]; !ok {
			return
		}
		for _, parent := range configs[dir].From {
			walk(parent)
		}
		path := filepath.Join(registryDir, dir, "gck.yaml")
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		defs, err := gcktmpl.ExtractVarDefs(data)
		if err != nil {
			return
		}
		for _, d := range defs {
			vi := varInfo{Name: d.Name, Default: d.Default, Description: d.Description, Origin: dir}
			if idx, ok := seen[d.Name]; ok {
				result[idx] = vi
			} else {
				seen[d.Name] = len(result)
				result = append(result, vi)
			}
		}
	}
	walk(relDir)
	return result
}

// findFlagSource locates the gck--{flagName}.yaml file by walking the
// from chain of relDir, returning the first match.
func findFlagSource(relDir, flagName string, configs map[string]*gckConfig, registryDir string) string {
	var find func(string) string
	find = func(dir string) string {
		path := filepath.Join(registryDir, dir, "gck--"+flagName+".yaml")
		if _, err := os.Stat(path); err == nil {
			return path
		}
		config, ok := configs[dir]
		if !ok {
			return ""
		}
		for _, parent := range config.From {
			if src := find(parent); src != "" {
				return src
			}
		}
		return ""
	}
	return find(relDir)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
