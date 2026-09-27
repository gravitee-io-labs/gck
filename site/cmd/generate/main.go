package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/gravitee-io-labs/gck/internal/notes"
	gcktmpl "github.com/gravitee-io-labs/gck/internal/template"
	"gopkg.in/yaml.v3"
)

type gckConfig struct {
	Abstract   bool           `yaml:"abstract"`
	From       []string       `yaml:"from"`
	Use        []string       `yaml:"use"`
	Kind       gckKind        `yaml:"kind"`
	Components []gckComponent `yaml:"components"`
}

type gckKind struct {
	Name string `yaml:"name"`
}

type gckComponent struct {
	Name    string `yaml:"name"`
	Enabled *bool  `yaml:"enabled"`
}

type readmeFrontmatter struct {
	Title       string   `yaml:"title"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags"`
}

type flagInfo struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	// Source is the registry path of the context that declares the flag.
	// The CLI resolves a flag through that context, so it does not apply an
	// inherited alternative a second time.
	Source    string   `yaml:"source,omitempty"`
	Group     string   `yaml:"group,omitempty"`
	Default   bool     `yaml:"default,omitempty"`
	Requires  []string `yaml:"requires,omitempty"`
	Conflicts []string `yaml:"conflicts,omitempty"`
	Implies   []string `yaml:"implies,omitempty"`

	from     []string
	disables []string
	pinned   bool
}

// altGroup is an alternative group offered on a context page: the mutually
// exclusive --use-* flags that pick one implementation.
type altGroup struct {
	Name    string     `yaml:"name"`
	Options []flagInfo `yaml:"options"`
}

// componentInfo is a component that only some alternatives bring in.
type componentInfo struct {
	Name   string   `yaml:"name"`
	Use    []string `yaml:"use,omitempty"`
	Unless []string `yaml:"unless,omitempty"`
}

type varInfo struct {
	Name        string `yaml:"name"`
	Default     string `yaml:"default"`
	Description string `yaml:"description,omitempty"`
	Origin      string `yaml:"origin,omitempty"`
	// Use lists the alternatives under which this variable exists with this
	// default; one of them must be selected. Unless lists the ones under
	// which it does not. Both empty means always.
	Use    []string `yaml:"use,omitempty"`
	Unless []string `yaml:"unless,omitempty"`
}

type endpointInfo struct {
	Name   string `yaml:"name"`
	URL    string `yaml:"url"`
	Note   string `yaml:"note,omitempty"`
	Origin string `yaml:"origin,omitempty"`
	// Requires lists the context flags that must all be passed for this
	// endpoint to exist. Empty means a plain "gck create" exposes it.
	Requires []string `yaml:"requires,omitempty"`
	// Use lists the alternatives under which this endpoint exists; one of
	// them must be selected (a default member counts). Unless lists the ones
	// that hide it. Both empty means always.
	Use    []string `yaml:"use,omitempty"`
	Unless []string `yaml:"unless,omitempty"`
}

type componentPage struct {
	Title       string   `yaml:"title"`
	Layout      string   `yaml:"layout"`
	Path        string   `yaml:"path"`
	Context     bool     `yaml:"context"`
	Description string   `yaml:"description,omitempty"`
	Tags        []string `yaml:"tags,omitempty"`
	From        []string `yaml:"from,omitempty"`
	Components  []string `yaml:"components,omitempty"`
	// ComponentTable lists every component with the alternatives it depends
	// on. It is only set when at least one component depends on one;
	// Components always holds the plain names.
	ComponentTable []componentInfo `yaml:"component_table,omitempty"`
	// Flags lists every context flag: alternatives first, grouped, then the
	// plain flags.
	Flags     []flagInfo     `yaml:"flags,omitempty"`
	Vars      []varInfo      `yaml:"vars,omitempty"`
	Endpoints []endpointInfo `yaml:"endpoints,omitempty"`
	Icon      string         `yaml:"icon,omitempty"`
	Type      string         `yaml:"type"`
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
		components, componentTable := resolveComponents(relDir, configs, registryDir)
		page := componentPage{
			Title:          title,
			Layout:         "detail",
			Path:           relDir,
			Context:        true,
			Description:    description,
			Tags:           tags,
			From:           config.From,
			Components:     components,
			ComponentTable: componentTable,
			Flags:          pageFlags(relDir, configs, registryDir),
			Vars:           resolveVars(relDir, configs, registryDir),
			Endpoints:      resolveEndpoints(relDir, configs, registryDir),
			Icon:           resolveIcon(registryDir, relDir),
			Type:           "registry",
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

// selection maps an alternative group to the selected member's flag name.
// Groups it does not mention take their default member.
type selection map[string]string

// chainLevel is one context visited while walking a composition, with the
// alternatives it declares that are selected.
type chainLevel struct {
	dir          string
	flags        []flagInfo // every flag the context declares
	alternatives []flagInfo // the selected member of each of its groups
}

// walkChain lists the contexts composed by relDir in merge order, the way
// the CLI resolver does: a context's selected alternatives' from entries come
// ahead of its own, then the context itself. A composing context's use: block
// pins the alternatives of everything below it.
func walkChain(relDir string, configs map[string]*gckConfig, registryDir string, sel selection) []chainLevel {
	return walkChainWith(relDir, configs, registryDir, sel, nil)
}

// walkChainWith is walkChain with plain flags turned on: an active flag that
// declares from composes those contexts after the alternatives' (as the CLI
// resolver does), and one that disables a group keeps its members out.
func walkChainWith(relDir string, configs map[string]*gckConfig, registryDir string, sel selection, active map[string]bool) []chainLevel {
	visited := map[string]bool{}
	var levels []chainLevel
	var walk func(dir string, pins map[string]bool)
	walk = func(dir string, pins map[string]bool) {
		config, ok := configs[dir]
		if !ok || visited[dir] {
			return
		}
		visited[dir] = true

		flags := discoverLocalFlags(registryDir, dir)
		selected := selectAlternatives(flags, pins, sel, active)
		pinnedGroups := map[string]bool{}
		for _, a := range selected {
			if a.pinned {
				pinnedGroups[a.Group] = true
			}
		}
		for i := range flags {
			flags[i].pinned = pinnedGroups[flags[i].Group]
		}

		childPins := pins
		if len(config.Use) > 0 {
			childPins = make(map[string]bool, len(pins)+len(config.Use))
			for k := range pins {
				childPins[k] = true
			}
			for _, u := range config.Use {
				childPins[alternativeName(u)] = true
			}
		}
		for _, alt := range selected {
			for _, parent := range alt.from {
				walk(parent, childPins)
			}
		}
		for _, f := range flags {
			if f.Group == "" && active[f.Name] {
				for _, parent := range f.from {
					walk(parent, childPins)
				}
			}
		}
		for _, parent := range config.From {
			walk(parent, childPins)
		}
		levels = append(levels, chainLevel{dir: dir, flags: flags, alternatives: selected})
	}
	walk(relDir, nil)
	return levels
}

func alternativeName(member string) string {
	if strings.HasPrefix(member, "use-") {
		return member
	}
	return "use-" + member
}

// selectAlternatives picks one member per alternative group among the flags
// of one context: a pinned member, else the one in sel, else the default.
// Members of pinned groups are marked pinned. A group switched off by a flag
// the other chosen members imply (use-embedded implies disable-metrics) gets
// no member, as in the CLI resolver. Groups come in name order.
func selectAlternatives(flags []flagInfo, pins map[string]bool, sel selection, active map[string]bool) []flagInfo {
	byGroup := map[string][]flagInfo{}
	var groups []string
	for _, f := range flags {
		if f.Group == "" {
			continue
		}
		if _, ok := byGroup[f.Group]; !ok {
			groups = append(groups, f.Group)
		}
		byGroup[f.Group] = append(byGroup[f.Group], f)
	}
	sort.Strings(groups)

	var selected []flagInfo
	for _, g := range groups {
		var pinned, chosen, def *flagInfo
		for i := range byGroup[g] {
			f := &byGroup[g][i]
			if pins[f.Name] {
				pinned = f
			}
			if sel[g] == f.Name {
				chosen = f
			}
			if f.Default {
				def = f
			}
		}
		pick := def
		switch {
		case pinned != nil:
			pick = pinned
			pick.pinned = true
		case chosen != nil:
			pick = chosen
		}
		if pick != nil {
			selected = append(selected, *pick)
		}
	}

	byName := map[string]flagInfo{}
	for _, f := range flags {
		byName[f.Name] = f
	}
	disabled := map[string]bool{}
	for name := range active {
		for _, g := range byName[name].disables {
			disabled[g] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, a := range selected {
			if disabled[a.Group] {
				continue
			}
			for _, implied := range a.Implies {
				for _, g := range byName[implied].disables {
					if !disabled[g] {
						disabled[g] = true
						changed = true
					}
				}
			}
		}
	}
	kept := selected[:0]
	for _, a := range selected {
		if !disabled[a.Group] {
			kept = append(kept, a)
		}
	}
	return kept
}

// altViews holds the compositions of a context worth rendering: the defaults
// first, then each non-default member of each selectable group swapped in on
// its own.
type altViews struct {
	sels     []selection
	levels   [][]chainLevel
	defaults map[string]string // group -> default member
}

func newAltViews(relDir string, configs map[string]*gckConfig, registryDir string) *altViews {
	v := &altViews{sels: []selection{{}}, defaults: map[string]string{}}
	for _, g := range resolveAlternatives(relDir, configs, registryDir) {
		for _, o := range g.Options {
			if o.Default {
				v.defaults[g.Name] = o.Name
			} else {
				v.sels = append(v.sels, selection{g.Name: o.Name})
			}
		}
	}
	for _, sel := range v.sels {
		v.levels = append(v.levels, walkChain(relDir, configs, registryDir, sel))
	}
	return v
}

// flagsIn returns the alternatives selected in view i and the flags they
// imply, for notes guards.
func (v *altViews) flagsIn(i int) []string {
	var out []string
	for _, l := range v.levels[i] {
		for _, a := range l.alternatives {
			out = append(out, a.Name)
			out = append(out, a.Implies...)
		}
	}
	return out
}

// gate returns the alternatives a row depends on, given the row keys each
// view produced. A group gates the row when its members disagree about it.
// When the row exists under fewer members than not, those members go to use
// ("with --use-x"); otherwise the members that hide it go to unless ("not
// with --use-x"). Both empty means always there.
func (v *altViews) gate(keysByView []map[string]bool, key string) (use, unless []string) {
	for g, def := range v.defaults {
		present := map[string]bool{def: keysByView[0][key]}
		for i, sel := range v.sels {
			if m, ok := sel[g]; ok {
				present[m] = keysByView[i][key]
			}
		}
		var with, without []string
		for m, ok := range present {
			if ok {
				with = append(with, m)
			} else {
				without = append(without, m)
			}
		}
		switch {
		case len(without) == 0:
		case len(with) <= len(without):
			use = append(use, with...)
		default:
			unless = append(unless, without...)
		}
	}
	sort.Strings(use)
	sort.Strings(unless)
	return use, unless
}

// resolveComponents lists the component names of relDir across every
// alternative view, and, when some of them depend on an alternative, a table
// of every component with the alternatives it needs or is dropped by.
func resolveComponents(relDir string, configs map[string]*gckConfig, registryDir string) ([]string, []componentInfo) {
	collect := func(levels []chainLevel) []string {
		seen := map[string]bool{}
		var names []string
		add := func(comps []gckComponent) {
			for _, c := range comps {
				if c.Enabled != nil && !*c.Enabled {
					if seen[c.Name] {
						seen[c.Name] = false
						names = slices.DeleteFunc(names, func(n string) bool { return n == c.Name })
					}
					continue
				}
				if !seen[c.Name] {
					seen[c.Name] = true
					names = append(names, c.Name)
				}
			}
		}
		for _, l := range levels {
			add(configs[l.dir].Components)
			for _, a := range l.alternatives {
				if cfg, err := parseGckConfig(filepath.Join(registryDir, a.Source, "gck--"+a.Name+".yaml")); err == nil {
					add(cfg.Components)
				}
			}
		}
		return names
	}

	views := newAltViews(relDir, configs, registryDir)
	var order []string
	seen := map[string]bool{}
	keysByView := make([]map[string]bool, len(views.sels))
	for i := range views.sels {
		keysByView[i] = map[string]bool{}
		for _, name := range collect(views.levels[i]) {
			keysByView[i][name] = true
			if !seen[name] {
				seen[name] = true
				order = append(order, name)
			}
		}
	}

	var names []string
	var table []componentInfo
	gated := false
	for _, name := range order {
		use, unless := views.gate(keysByView, name)
		gated = gated || len(use)+len(unless) > 0
		names = append(names, name)
		table = append(table, componentInfo{Name: name, Use: use, Unless: unless})
	}
	if !gated {
		table = nil
	}
	return names, table
}

// resolveEndpoints walks the from chain for relDir, collecting each level's
// notes.create, and folds them with the same merge the CLI uses so a row
// declared on an abstract base surfaces on every concrete context that
// composes it -- attributed, via Origin, to the context that declared it.
//
// The composition is walked once per alternative view (see altViews): rows
// that only some members of a group bring in carry those members in Use and
// follow the unconditional ones. Rows that only exist behind a context flag
// come last, each carrying the flags that reveal it -- see requiredFlags.
func resolveEndpoints(relDir string, configs map[string]*gckConfig, registryDir string) []endpointInfo {
	layersOf := func(levels []chainLevel) []notes.Layer {
		var layers []notes.Layer
		for _, l := range levels {
			data, err := os.ReadFile(filepath.Join(registryDir, l.dir, "notes.create"))
			if err != nil {
				continue
			}
			layers = append(layers, notes.Layer{Source: l.dir, Raw: string(data)})
		}
		return layers
	}

	toInfo := func(ep notes.Endpoint, requires []string) endpointInfo {
		return endpointInfo{
			Name:     ep.Name,
			URL:      ep.URL,
			Note:     ep.Note,
			Origin:   ep.Origin,
			Requires: requires,
		}
	}

	views := newAltViews(relDir, configs, registryDir)
	var order []notes.Endpoint
	shown := map[string]bool{}
	keysByView := make([]map[string]bool, len(views.sels))
	for i := range views.sels {
		keysByView[i] = map[string]bool{}
		for _, ep := range visibleWith(relDir, layersOf(views.levels[i]), views.flagsIn(i)) {
			keysByView[i][ep.Name] = true
			if !shown[ep.Name] {
				shown[ep.Name] = true
				order = append(order, ep)
			}
		}
	}
	// Rows every alternative exposes come first, then the ones tied to an
	// alternative, each group in the order the composition declares them.
	var result, gated []endpointInfo
	for _, ep := range order {
		info := toInfo(ep, nil)
		if info.Use, info.Unless = views.gate(keysByView, ep.Name); len(info.Use)+len(info.Unless) > 0 {
			gated = append(gated, info)
		} else {
			result = append(result, info)
		}
	}
	result = append(result, gated...)

	// Anything the full flag set reveals but the default view does not is
	// gated; work out which flags each of those rows actually needs.
	var flagNames []string
	for _, f := range plainFlags(resolveFlags(relDir, configs, registryDir)) {
		flagNames = append(flagNames, f.Name)
	}
	if len(flagNames) == 0 {
		return result
	}
	// Flags that compose contexts change the layers, not just the guards, so
	// the layers are rebuilt for each flag set.
	layersFor := func(flags []string) []notes.Layer {
		active := make(map[string]bool, len(flags))
		for _, f := range flags {
			active[f] = true
		}
		return layersOf(walkChainWith(relDir, configs, registryDir, selection{}, active))
	}
	selected := views.flagsIn(0)
	for _, ep := range visibleWith(relDir, layersFor(flagNames), append(append([]string{}, selected...), flagNames...)) {
		if shown[ep.Name] {
			continue
		}
		shown[ep.Name] = true
		result = append(result, toInfo(ep, requiredFlags(relDir, layersFor, selected, flagNames, ep.Name)))
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
// its expression, so an `and` of two flags reports both. The selected
// alternatives stay active throughout.
func requiredFlags(relDir string, layersFor func([]string) []notes.Layer, selected, allFlags []string, name string) []string {
	var required []string
	for _, candidate := range allFlags {
		var others []string
		for _, f := range allFlags {
			if f != candidate {
				others = append(others, f)
			}
		}
		without := append(append([]string{}, selected...), others...)
		if !containsEndpoint(visibleWith(relDir, layersFor(others), without), name) {
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
		// The manifest also lists inherited flags so tools can show them;
		// each entry's source tells the CLI which context declares it.
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
			srcPath := filepath.Join(registryDir, f.Source, flagFile)
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

// discoverLocalFlags reads the flags a context declares in its own
// directory, alternatives included.
func discoverLocalFlags(registryDir, relDir string) []flagInfo {
	matches, err := filepath.Glob(filepath.Join(registryDir, relDir, "gck--*.yaml"))
	if err != nil || len(matches) == 0 {
		return nil
	}
	sort.Strings(matches)
	var flags []flagInfo
	for _, path := range matches {
		name := strings.TrimPrefix(filepath.Base(path), "gck--")
		name = strings.TrimSuffix(name, ".yaml")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var meta struct {
			Description string   `yaml:"description"`
			Group       string   `yaml:"group"`
			Default     bool     `yaml:"default"`
			Requires    []string `yaml:"requires"`
			Conflicts   []string `yaml:"conflicts"`
			Implies     []string `yaml:"implies"`
			Disables    []string `yaml:"disables"`
			From        []string `yaml:"from"`
		}
		_ = yaml.Unmarshal(data, &meta)
		flags = append(flags, flagInfo{
			Name:        name,
			Description: meta.Description,
			Source:      relDir,
			Group:       meta.Group,
			Default:     meta.Default,
			Requires:    meta.Requires,
			Conflicts:   meta.Conflicts,
			Implies:     meta.Implies,
			from:        meta.From,
			disables:    meta.Disables,
		})
	}
	return flags
}

// resolveFlags walks the default composition of relDir, discovering flags at
// each level and merging them (child overrides parent for the same name).
// Alternatives are included, with the members of pinned groups marked.
func resolveFlags(relDir string, configs map[string]*gckConfig, registryDir string) []flagInfo {
	seen := map[string]int{}
	var result []flagInfo
	for _, l := range walkChain(relDir, configs, registryDir, nil) {
		for _, f := range l.flags {
			if idx, ok := seen[f.Name]; ok {
				result[idx] = f
			} else {
				seen[f.Name] = len(result)
				result = append(result, f)
			}
		}
	}
	return result
}

// pageFlags lists the flags a page offers: the selectable alternatives,
// grouped and in group-name order, then the plain flags. Pinned groups are
// left out.
func pageFlags(relDir string, configs map[string]*gckConfig, registryDir string) []flagInfo {
	var out []flagInfo
	for _, g := range resolveAlternatives(relDir, configs, registryDir) {
		out = append(out, g.Options...)
	}
	return append(out, plainFlags(resolveFlags(relDir, configs, registryDir))...)
}

// plainFlags drops the alternatives from flags.
func plainFlags(flags []flagInfo) []flagInfo {
	var out []flagInfo
	for _, f := range flags {
		if f.Group == "" {
			out = append(out, f)
		}
	}
	return out
}

// resolveAlternatives groups the alternatives a user can select on relDir,
// in group-name order. Groups pinned by a composing context are left out.
func resolveAlternatives(relDir string, configs map[string]*gckConfig, registryDir string) []altGroup {
	byGroup := map[string]int{}
	var groups []altGroup
	for _, f := range resolveFlags(relDir, configs, registryDir) {
		if f.Group == "" || f.pinned {
			continue
		}
		idx, ok := byGroup[f.Group]
		if !ok {
			idx = len(groups)
			byGroup[f.Group] = idx
			groups = append(groups, altGroup{Name: f.Group})
		}
		groups[idx].Options = append(groups[idx].Options, f)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
	return groups
}

// resolveVars walks the composition of relDir, extracting var definitions
// from each gck.yaml and from the alternatives selected at each level, and
// merging them (child overrides parent for same name; an alternative
// overrides the context that declares it). Variables that only some
// alternatives declare, or declare with their own default, carry those
// alternatives in Use.
func resolveVars(relDir string, configs map[string]*gckConfig, registryDir string) []varInfo {
	collect := func(levels []chainLevel) []varInfo {
		seen := map[string]int{}
		var result []varInfo
		add := func(path, origin string) {
			data, err := os.ReadFile(path)
			if err != nil {
				return
			}
			defs, err := gcktmpl.ExtractVarDefs(data)
			if err != nil {
				return
			}
			for _, d := range defs {
				vi := varInfo{Name: d.Name, Default: d.Default, Description: d.Description, Origin: origin}
				if idx, ok := seen[d.Name]; ok {
					result[idx] = vi
				} else {
					seen[d.Name] = len(result)
					result = append(result, vi)
				}
			}
		}
		for _, l := range levels {
			add(filepath.Join(registryDir, l.dir, "gck.yaml"), l.dir)
			for _, a := range l.alternatives {
				add(filepath.Join(registryDir, a.Source, "gck--"+a.Name+".yaml"), a.Source)
			}
		}
		return result
	}

	key := func(v varInfo) string { return v.Name + "\x00" + v.Default + "\x00" + v.Origin }

	views := newAltViews(relDir, configs, registryDir)
	var order []varInfo
	seen := map[string]bool{}
	keysByView := make([]map[string]bool, len(views.sels))
	for i := range views.sels {
		keysByView[i] = map[string]bool{}
		for _, v := range collect(views.levels[i]) {
			keysByView[i][key(v)] = true
			if !seen[key(v)] {
				seen[key(v)] = true
				order = append(order, v)
			}
		}
	}
	for i := range order {
		order[i].Use, order[i].Unless = views.gate(keysByView, key(order[i]))
	}
	return order
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
