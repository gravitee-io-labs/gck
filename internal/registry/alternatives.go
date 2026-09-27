package registry

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/gravitee-io-labs/gck/internal/config"
	"github.com/gravitee-io-labs/gck/internal/logger"
	gcktmpl "github.com/gravitee-io-labs/gck/internal/template"
	"gopkg.in/yaml.v3"
)

// AlternativePrefix is the flag-name prefix reserved for alternatives: flags
// that switch between mutually exclusive implementations of the same concern
// (e.g. --use-mongodb vs --use-jdbc-postgres).
const AlternativePrefix = "use-"

// AlternativeFlagName turns a member name as written in a use: block
// ("jdbc-mysql") into its flag name ("use-jdbc-mysql"). Names that already
// carry the prefix are returned unchanged.
func AlternativeFlagName(member string) string {
	if strings.HasPrefix(member, AlternativePrefix) {
		return member
	}
	return AlternativePrefix + member
}

type useKey struct{}

// SelectionKey is the key of an alternative group in
// ResolvedContext.Selected: the declaring context's path and the group name.
func SelectionKey(contextPath, group string) string {
	return contextPath + ":" + group
}

// SplitSelectionKey returns the context path and group of a SelectionKey.
func SplitSelectionKey(key string) (contextPath, group string) {
	i := strings.LastIndexByte(key, ':')
	if i < 0 {
		return "", key
	}
	return key[:i], key[i+1:]
}

// useSelection carries alternative selections down the composition while
// contexts are resolved: explicit ones come from the command line,
// configured ones from the user's gck.yaml use: (or a cluster's saved
// selection), pinned ones from a composing context's use: block. saved ones
// are a cluster's selection per group, and count as configured for their
// group only. flags holds the plain flags the user turned on, whose
// disables entries switch alternative groups off.
type useSelection struct {
	explicit   map[string]bool   // flag name -> selected on the command line
	configured map[string]bool   // flag name -> selected by configuration
	saved      map[string]string // SelectionKey -> flag name
	pinned     map[string]string // flag name -> path of the pinning context
	flags      map[string]bool   // plain flag name -> active
}

func selectionFrom(ctx context.Context) useSelection {
	sel, _ := ctx.Value(useKey{}).(useSelection)
	return sel
}

// WithUse returns a context carrying the alternatives selected by the user.
// Names may be given with or without the "use-" prefix. Resolvers apply them
// to every alternative group in the composition that has a member of that
// name.
func WithUse(ctx context.Context, names []string) context.Context {
	if len(names) == 0 {
		return ctx
	}
	sel := selectionFrom(ctx)
	next := useSelection{explicit: make(map[string]bool, len(sel.explicit)+len(names)), configured: sel.configured, saved: sel.saved, pinned: sel.pinned, flags: sel.flags}
	for k := range sel.explicit {
		next.explicit[k] = true
	}
	for _, n := range names {
		next.explicit[AlternativeFlagName(n)] = true
	}
	return context.WithValue(ctx, useKey{}, next)
}

// WithConfiguredUse returns a context carrying the alternatives selected by
// configuration: the user's gck.yaml use:, or the selection a cluster was
// created with. A member chosen on the command line (WithUse) overrides them
// in its group, as the command line overrides the configuration elsewhere.
func WithConfiguredUse(ctx context.Context, names []string) context.Context {
	if len(names) == 0 {
		return ctx
	}
	sel := selectionFrom(ctx)
	next := sel
	next.configured = make(map[string]bool, len(sel.configured)+len(names))
	for k := range sel.configured {
		next.configured[k] = true
	}
	for _, n := range names {
		next.configured[AlternativeFlagName(n)] = true
	}
	return context.WithValue(ctx, useKey{}, next)
}

// WithSavedUse returns a context carrying the members a cluster was created
// with, keyed by SelectionKey. Each counts as configured for its own group
// only, so two composed contexts whose groups share a name and members get
// back what each had.
func WithSavedUse(ctx context.Context, saved map[string]string) context.Context {
	if len(saved) == 0 {
		return ctx
	}
	next := selectionFrom(ctx)
	next.saved = saved
	return context.WithValue(ctx, useKey{}, next)
}

// WithFlags returns a context carrying the plain flags the user turned on.
// Resolvers need them before applying alternatives: a flag that disables an
// alternative group (e.g. --disable-analytics) keeps its members out of the
// composition altogether.
func WithFlags(ctx context.Context, names []string) context.Context {
	if len(names) == 0 {
		return ctx
	}
	sel := selectionFrom(ctx)
	next := useSelection{explicit: sel.explicit, configured: sel.configured, saved: sel.saved, pinned: sel.pinned, flags: make(map[string]bool, len(sel.flags)+len(names))}
	for k := range sel.flags {
		next.flags[k] = true
	}
	for _, n := range names {
		next.flags[n] = true
	}
	return context.WithValue(ctx, useKey{}, next)
}

// withPins returns a context in which the alternatives listed in a composing
// context's use: block are pinned for every context it composes.
func withPins(ctx context.Context, contextPath string, names []string) context.Context {
	if len(names) == 0 {
		return ctx
	}
	sel := selectionFrom(ctx)
	next := useSelection{explicit: sel.explicit, configured: sel.configured, saved: sel.saved, pinned: make(map[string]string, len(sel.pinned)+len(names)), flags: sel.flags}
	for k, v := range sel.pinned {
		next.pinned[k] = v
	}
	for _, n := range names {
		name := AlternativeFlagName(n)
		// The outermost pin wins: it was set by the context closest to the
		// user, which composes everything below it.
		if _, ok := next.pinned[name]; !ok {
			next.pinned[name] = contextPath
		}
	}
	return context.WithValue(ctx, useKey{}, next)
}

// alternativeLayer is a selected alternative being folded into the context
// that declares it.
type alternativeLayer struct {
	flag config.ContextFlag
	raw  []byte
	tree *gcktmpl.VarsTree
	from []string // read before templating: it decides what gets resolved
	cfg  config.Config
	// composeOnly marks an active plain flag that declares from: only its
	// parents are composed here, its body is applied later by ApplyFlags.
	composeOnly bool
}

// selectAlternatives picks one member per alternative group declared in
// flags (the flags of a single context directory) and loads its file.
// Precedence per group: a pin from a composing context, then the command
// line's selection, then the configured one, then the group default.
// Members of pinned groups are marked Pinned in flags.
//
// A group is skipped altogether while a plain flag of the same context that
// disables it is active -- turned on by the user, or implied by another
// selected member (use-dbless implies disable-analytics). A skipped group's
// own implies do not count. Layers are returned in group-name order.
func selectAlternatives(ctx context.Context, contextPath string, flags []config.ContextFlag) ([]*alternativeLayer, error) {
	groups := make(map[string][]int)
	var names []string
	for i, f := range flags {
		if !f.IsAlternative() {
			continue
		}
		if _, ok := groups[f.Group]; !ok {
			names = append(names, f.Group)
		}
		groups[f.Group] = append(groups[f.Group], i)
	}
	sort.Strings(names)

	sel := selectionFrom(ctx)
	chosen := make(map[string]int, len(names))
	explicitlyChosen := make(map[string]bool)
	for _, group := range names {
		var def, pinned, explicit, configured []int
		for _, i := range groups[group] {
			f := flags[i]
			if f.Default {
				def = append(def, i)
			}
			if _, ok := sel.pinned[f.Name]; ok {
				pinned = append(pinned, i)
			}
			if sel.explicit[f.Name] {
				explicit = append(explicit, i)
			}
			if sel.configured[f.Name] || sel.saved[SelectionKey(contextPath, group)] == f.Name {
				configured = append(configured, i)
			}
		}
		if len(configured) > 1 && len(explicit) == 0 {
			return nil, fmt.Errorf("use: %s are mutually exclusive (group %s)", flagList(flags, configured), group)
		}
		if len(explicit) == 0 {
			explicit = configured
		}

		switch {
		case len(pinned) > 1:
			return nil, fmt.Errorf("context %s: %s are mutually exclusive (group %s)", contextPath, flagList(flags, pinned), group)
		case len(explicit) > 1:
			return nil, fmt.Errorf("%s are mutually exclusive (group %s)", flagList(flags, explicit), group)
		case len(pinned) == 1:
			chosen[group] = pinned[0]
			if len(explicit) == 1 && explicit[0] != pinned[0] {
				pinner := sel.pinned[flags[pinned[0]].Name]
				return nil, fmt.Errorf("--%s cannot be used: %s pins group %s to --%s", flags[explicit[0]].Name, pinner, group, flags[pinned[0]].Name)
			}
			for _, i := range groups[group] {
				flags[i].Pinned = true
			}
		case len(explicit) == 1:
			chosen[group] = explicit[0]
			explicitlyChosen[group] = true
		case len(def) == 1:
			chosen[group] = def[0]
		default:
			return nil, fmt.Errorf("context %s: alternative group %s must have exactly one default member", contextPath, group)
		}
	}

	disabled := disabledGroups(flags, chosen, sel.flags)

	var layers []*alternativeLayer
	for _, group := range names {
		if by, off := disabled[group]; off {
			if explicitlyChosen[group] {
				logger.Warn("--%s has no effect: group %s is disabled by %s", flags[chosen[group]].Name, group, by)
			}
			continue
		}
		layer, err := loadAlternativeLayer(flags[chosen[group]])
		if err != nil {
			return nil, err
		}
		layers = append(layers, layer)
	}

	// Active plain flags that compose contexts bring them in here, after the
	// alternatives; their patches are applied with the other flags later.
	active := make(map[string]bool, len(sel.flags))
	for name := range sel.flags {
		active[name] = true
	}
	for _, group := range names {
		if _, off := disabled[group]; !off {
			for _, implied := range flags[chosen[group]].Implies {
				active[implied] = true
			}
		}
	}
	for _, f := range flags {
		if f.IsAlternative() || !active[f.Name] {
			continue
		}
		layer, err := loadAlternativeLayer(f)
		if err != nil {
			return nil, err
		}
		if len(layer.from) == 0 {
			continue
		}
		layer.composeOnly = true
		layers = append(layers, layer)
	}
	return layers, nil
}

// disabledGroups returns the alternative groups switched off by the active
// plain flags of a context, mapped to the flag responsible. Active flags are
// the user's plus those implied by the chosen members of groups that are not
// themselves switched off, so it iterates until nothing changes.
func disabledGroups(flags []config.ContextFlag, chosen map[string]int, userFlags map[string]bool) map[string]string {
	byName := make(map[string]config.ContextFlag, len(flags))
	for _, f := range flags {
		byName[f.Name] = f
	}
	disabled := make(map[string]string)
	for {
		active := make(map[string]bool, len(userFlags))
		for name := range userFlags {
			active[name] = true
		}
		for group, i := range chosen {
			if _, off := disabled[group]; off {
				continue
			}
			for _, implied := range flags[i].Implies {
				active[implied] = true
			}
		}
		changed := false
		for name := range active {
			for _, group := range byName[name].Disables {
				if _, off := disabled[group]; !off {
					disabled[group] = "--" + name
					changed = true
				}
			}
		}
		if !changed {
			return disabled
		}
	}
}

func flagList(flags []config.ContextFlag, idx []int) string {
	parts := make([]string, len(idx))
	for i, j := range idx {
		parts[i] = "--" + flags[j].Name
	}
	return strings.Join(parts, " and ")
}

func loadAlternativeLayer(flag config.ContextFlag) (*alternativeLayer, error) {
	path := filepath.Join(flag.Dir, flagFilePrefix+flag.Name+".yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading alternative %s: %w", path, err)
	}
	tree, err := gcktmpl.ExtractVarsTree(raw)
	if err != nil {
		return nil, fmt.Errorf("extracting vars from alternative %s: %w", path, err)
	}
	var head struct {
		From []string `yaml:"from"`
	}
	if err := yaml.Unmarshal(raw, &head); err != nil {
		return nil, fmt.Errorf("parsing alternative %s: %w", path, err)
	}
	return &alternativeLayer{flag: flag, raw: raw, tree: tree, from: head.From}, nil
}

// addAlternativeVars folds the var declarations and path-scoped overrides of
// the selected alternatives into those of the declaring context. An
// alternative's own defaults win over the context's for the same name. A
// compose-only flag contributes its path-scoped overrides (they target the
// parents it composes); its own defaults stay with its patch.
func addAlternativeVars(layers []*alternativeLayer, ownDefaults map[string]string, overrides []gcktmpl.VarOverride) []gcktmpl.VarOverride {
	for _, l := range layers {
		if !l.composeOnly {
			for _, d := range l.tree.Defs {
				ownDefaults[d.Name] = d.Default
			}
		}
		overrides = append(overrides, l.tree.Overrides...)
	}
	return overrides
}

// renderAlternatives renders each selected alternative once the declaring
// context is resolved, with the composition's effective vars -- like a plain
// flag file, an alternative may use the vars its context inherits (e.g.
// {{ .imagePrefix }} from an abstract base).
func renderAlternatives(layers []*alternativeLayer, vars map[string]string) error {
	for _, l := range layers {
		if l.composeOnly {
			continue
		}
		rendered, err := gcktmpl.RenderWithVars(l.raw, vars)
		if err != nil {
			return fmt.Errorf("templating alternative --%s: %w", l.flag.Name, err)
		}
		if err := yaml.Unmarshal(rendered, &l.cfg); err != nil {
			return fmt.Errorf("parsing alternative --%s: %w", l.flag.Name, err)
		}
	}
	return nil
}

// alternativeFrom returns the from entries the selected alternatives add in
// front of the declaring context's own: the resulting composition is the one
// a dedicated variant context would have declared.
func alternativeFrom(layers []*alternativeLayer, own []string) []string {
	var from []string
	for _, l := range layers {
		from = append(from, l.from...)
	}
	return append(from, own...)
}

// applyAlternatives merges the selected alternatives' bodies on top of the
// resolved declaring context and records the selection.
func applyAlternatives(resolved *config.ResolvedContext, layers []*alternativeLayer) {
	for _, l := range layers {
		if l.composeOnly {
			continue
		}
		MergeComponents(resolved, l.cfg.Components, l.flag.Dir)
		resolved.Repos = MergeRepos(resolved.Repos, l.cfg.Helm.Repos)
		resolved.Features = config.MergeFeatures(resolved.Features, l.cfg.Features)
		resolved.Kind = mergeKind(resolved.Kind, l.cfg.Kind)
		resolved.Images = config.MergeImages(resolved.Images, l.cfg.Images)
		if resolved.Selected == nil {
			resolved.Selected = make(map[string]string)
		}
		resolved.Selected[SelectionKey(l.flag.Context, l.flag.Group)] = l.flag.Name
		resolved.Implied = appendUnique(resolved.Implied, l.flag.Implies...)
	}
}

// appendUnique appends the names not already in list.
func appendUnique(list []string, names ...string) []string {
	for _, n := range names {
		if !slices.Contains(list, n) {
			list = append(list, n)
		}
	}
	return list
}

// withImplied returns a context in which the flags implied by the selected
// members of layers count as turned on for every context they compose, as
// the user's flags do: an implied flag declared by a parent then disables
// its groups and composes its from: there.
func withImplied(ctx context.Context, layers []*alternativeLayer) context.Context {
	var names []string
	for _, l := range layers {
		if !l.composeOnly {
			names = appendUnique(names, l.flag.Implies...)
		}
	}
	return WithFlags(ctx, names)
}

// checkImplied verifies that every flag the selected alternatives imply is a
// plain flag of the composition.
func checkImplied(contextPath string, resolved *config.ResolvedContext) error {
	for _, name := range resolved.Implied {
		found := false
		for _, f := range resolved.Flags {
			if !f.IsAlternative() && f.Name == name {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("context %s: an alternative implies --%s, which is not a flag of the composition", contextPath, name)
		}
	}
	return nil
}

// checkPins verifies that every member named in a context's use: block
// matches an alternative of the contexts it composes, and none of its own:
// those are selected before use: is read, so pinning one would do nothing.
func checkPins(contextPath string, use []string, own []config.ContextFlag, resolved *config.ResolvedContext) error {
	for _, n := range use {
		name := AlternativeFlagName(n)
		for _, f := range own {
			if f.IsAlternative() && f.Name == name {
				return fmt.Errorf("context %s: use: %s names an alternative the context declares itself; use: only pins composed contexts, so make it the default of group %s instead", contextPath, n, f.Group)
			}
		}
		found := false
		for _, f := range resolved.Flags {
			if f.IsAlternative() && f.Name == name {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("context %s: use: %s matches no alternative of the composed contexts", contextPath, n)
		}
	}
	return nil
}

// checkInheritedAlternatives rejects a context that redeclares an
// alternative or an alternative group it inherits. Alternatives are applied
// where they are declared, so an override in a child would never take
// effect.
func checkInheritedAlternatives(contextPath string, inherited, own []config.ContextFlag) error {
	names := make(map[string]bool)
	groups := make(map[string]bool)
	for _, f := range inherited {
		if f.IsAlternative() {
			names[f.Name] = true
			groups[f.Group] = true
		}
	}
	for _, f := range own {
		if names[f.Name] {
			return fmt.Errorf("context %s: --%s redeclares an inherited alternative", contextPath, f.Name)
		}
		if f.IsAlternative() && groups[f.Group] {
			return fmt.Errorf("context %s: --%s redeclares inherited alternative group %s", contextPath, f.Name, f.Group)
		}
	}
	return nil
}

// EffectiveFlags returns the flags in force on a resolved context: the
// selected alternatives (defaults included), in group order, then the flags
// they imply, then the active plain flags in the order given.
func EffectiveFlags(resolved *config.ResolvedContext, active []string) []string {
	var out []string
	if resolved != nil {
		groups := make([]string, 0, len(resolved.Selected))
		for g := range resolved.Selected {
			groups = append(groups, g)
		}
		sort.Strings(groups)
		for _, g := range groups {
			out = appendUnique(out, resolved.Selected[g])
		}
		out = appendUnique(out, resolved.Implied...)
	}
	seen := make(map[string]bool, len(out))
	for _, f := range out {
		seen[f] = true
	}
	for _, f := range active {
		if seen[f] || strings.HasPrefix(f, AlternativePrefix) {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}
