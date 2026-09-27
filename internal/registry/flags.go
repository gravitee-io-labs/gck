package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gravitee-io-labs/gck/internal/config"
	gcktmpl "github.com/gravitee-io-labs/gck/internal/template"
	"gopkg.in/yaml.v3"
)

// flagFilePrefix is the filename prefix for context flag patch files.
const flagFilePrefix = "gck--"

// flagNamePattern validates that a flag name is lowercase kebab-case.
var flagNamePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// DiscoverFlags scans dir for gck--*.yaml files and returns one ContextFlag
// per valid file. Each file is parsed for its description and, for
// alternatives, its group, default marker and requirements. Files whose names
// don't match the naming convention, and alternative groups that don't have
// exactly one default, are returned as errors.
func DiscoverFlags(dir string) ([]config.ContextFlag, error) {
	matches, err := filepath.Glob(filepath.Join(dir, flagFilePrefix+"*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("globbing flag files in %s: %w", dir, err)
	}
	sort.Strings(matches)

	var flags []config.ContextFlag
	for _, path := range matches {
		flag, err := ReadFlag(path)
		if err != nil {
			return nil, err
		}
		flags = append(flags, flag)
	}
	if err := validateAlternativeGroups(flags); err != nil {
		return nil, fmt.Errorf("flags in %s: %w", dir, err)
	}
	return flags, nil
}

// flagMeta holds the fields of a flag file that describe the flag itself
// rather than the patch it applies.
type flagMeta struct {
	Description string   `yaml:"description"`
	Group       string   `yaml:"group"`
	Default     bool     `yaml:"default"`
	Requires    []string `yaml:"requires"`
	Conflicts   []string `yaml:"conflicts"`
	Implies     []string `yaml:"implies"`
	Disables    []string `yaml:"disables"`
	From        []string `yaml:"from"`
}

// ReadFlag builds a ContextFlag from a gck--{name}.yaml file, checking the
// naming convention and the fields that separate alternatives from plain
// flags.
func ReadFlag(path string) (config.ContextFlag, error) {
	name, err := FlagNameFromFile(filepath.Base(path))
	if err != nil {
		return config.ContextFlag{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return config.ContextFlag{}, fmt.Errorf("reading flag file %s: %w", path, err)
	}
	var meta flagMeta
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return config.ContextFlag{}, fmt.Errorf("reading flag file %s: %w", path, err)
	}
	flag := config.ContextFlag{
		Name:        name,
		Description: meta.Description,
		Dir:         filepath.Dir(path),
		Group:       meta.Group,
		Default:     meta.Default,
		Requires:    meta.Requires,
		Conflicts:   meta.Conflicts,
		Implies:     meta.Implies,
		Disables:    meta.Disables,
	}
	if err := validateFlagMeta(name, meta); err != nil {
		return config.ContextFlag{}, fmt.Errorf("flag file %s: %w", path, err)
	}
	return flag, nil
}

// validateFlagMeta enforces the split between plain flags and alternatives:
// only use-* flags belong to a group, and only they may be a default or imply
// flags; only plain flags may require, conflict with or disable something.
// Both kinds may compose other contexts with from.
func validateFlagMeta(name string, meta flagMeta) error {
	isAlt := strings.HasPrefix(name, AlternativePrefix)
	switch {
	case isAlt && meta.Group == "":
		return fmt.Errorf("--%s: %s* flags are alternatives and must declare a group", name, AlternativePrefix)
	case !isAlt && meta.Group != "":
		return fmt.Errorf("--%s: only %s* flags may declare a group", name, AlternativePrefix)
	case !isAlt && meta.Default:
		return fmt.Errorf("--%s: only alternatives (%s*) may be a default", name, AlternativePrefix)
	case !isAlt && len(meta.Implies) > 0:
		return fmt.Errorf("--%s: only alternatives (%s*) may declare implies", name, AlternativePrefix)
	case isAlt && len(meta.Requires) > 0:
		return fmt.Errorf("--%s: alternatives may not declare requires", name)
	case isAlt && len(meta.Conflicts) > 0:
		return fmt.Errorf("--%s: alternatives may not declare conflicts", name)
	case isAlt && len(meta.Disables) > 0:
		return fmt.Errorf("--%s: alternatives may not declare disables", name)
	}
	for _, implied := range meta.Implies {
		if strings.HasPrefix(implied, AlternativePrefix) {
			return fmt.Errorf("--%s: implies names plain flags, not alternatives (%s)", name, implied)
		}
	}
	return nil
}

// validateAlternativeGroups checks that every alternative group declared by
// one context directory has exactly one default member.
func validateAlternativeGroups(flags []config.ContextFlag) error {
	defaults := make(map[string]int)
	var groups []string
	for _, f := range flags {
		if !f.IsAlternative() {
			continue
		}
		if _, ok := defaults[f.Group]; !ok {
			groups = append(groups, f.Group)
			defaults[f.Group] = 0
		}
		if f.Default {
			defaults[f.Group]++
		}
	}
	for _, g := range groups {
		if defaults[g] != 1 {
			return fmt.Errorf("alternative group %s must have exactly one default member, found %d", g, defaults[g])
		}
	}
	return nil
}

// MergeFlags merges child flags on top of base flags. Child flags with the
// same name override the corresponding base flag; new child flags are
// appended. The result preserves base ordering for existing flags.
func MergeFlags(base, child []config.ContextFlag) []config.ContextFlag {
	if len(child) == 0 {
		return base
	}
	if len(base) == 0 {
		return child
	}

	byName := make(map[string]int, len(base))
	result := make([]config.ContextFlag, len(base))
	copy(result, base)
	for i, f := range result {
		byName[flagMergeKey(f)] = i
	}
	for _, f := range child {
		if idx, ok := byName[flagMergeKey(f)]; ok {
			result[idx] = f
		} else {
			byName[flagMergeKey(f)] = len(result)
			result = append(result, f)
		}
	}
	return result
}

// flagMergeKey identifies a flag when flag lists are merged. A context
// cannot redeclare an alternative it inherits, so two alternatives of the
// same name come from two composed contexts (--use-mongodb of two
// products), and both are kept.
func flagMergeKey(f config.ContextFlag) string {
	if f.IsAlternative() {
		return SelectionKey(f.Context, f.Name)
	}
	return f.Name
}

// ApplyFlags loads each active flag's patch file and merges it into the
// resolved context using the same merge semantics as context composition.
// Active flags are validated against the available flags on the context.
// Alternatives are skipped: they were applied during resolution. A flag's
// requires entries must name active flags or selected alternatives, and its
// conflicts entries must name none. The flags implied by the selected
// alternatives are applied first, as if they had been passed.
// The resolved context's EffectiveVars are used as the base variable set
// when rendering flag patches, so flags can reference vars declared by
// the parent context (e.g. {{ .imageTag }}). setOverrides are merged on
// top.
func ApplyFlags(resolved *config.ResolvedContext, activeFlags []string, setOverrides map[string]string) error {
	activeFlags = appendUnique(append([]string{}, resolved.Implied...), activeFlags...)
	if len(activeFlags) == 0 {
		return nil
	}

	available := make(map[string]config.ContextFlag, len(resolved.Flags))
	for _, f := range resolved.Flags {
		available[f.Name] = f
	}

	inForce := make(map[string]bool, len(activeFlags)+len(resolved.Selected))
	for _, name := range activeFlags {
		inForce[name] = true
	}
	for _, name := range resolved.Selected {
		inForce[name] = true
	}

	for _, name := range activeFlags {
		flag, ok := available[name]
		if !ok {
			var known []string
			for _, f := range resolved.Flags {
				known = appendUnique(known, "--"+f.Name)
			}
			return fmt.Errorf("unknown context flag --%s (available: %s)", name, strings.Join(known, ", "))
		}
		// Alternatives are applied while the context is resolved.
		if flag.IsAlternative() {
			continue
		}
		for _, req := range flag.Requires {
			if !inForce[req] {
				return fmt.Errorf("--%s requires --%s", name, req)
			}
		}
		for _, c := range flag.Conflicts {
			if inForce[c] {
				return fmt.Errorf("--%s cannot be used with --%s", name, c)
			}
		}

		patch, err := loadFlagConfig(flag, resolved.EffectiveVars, setOverrides)
		if err != nil {
			return fmt.Errorf("loading flag --%s: %w", name, err)
		}

		MergeComponents(resolved, patch.Components, flag.Dir)
		resolved.Repos = MergeRepos(resolved.Repos, patch.Helm.Repos)
		resolved.Features = config.MergeFeatures(resolved.Features, patch.Features)
		resolved.Kind = mergeKind(resolved.Kind, patch.Kind)
		resolved.Images = config.MergeImages(resolved.Images, patch.Images)
	}
	return nil
}

// FlagNameFromFile extracts and validates the flag name from a filename like
// "gck--disable-portal.yaml". Returns an error if the name doesn't match the
// kebab-case convention.
func FlagNameFromFile(basename string) (string, error) {
	name := strings.TrimPrefix(basename, flagFilePrefix)
	name = strings.TrimSuffix(name, ".yaml")
	if !flagNamePattern.MatchString(name) {
		return "", fmt.Errorf("invalid flag file name %q: flag name %q must match %s", basename, name, flagNamePattern.String())
	}
	return name, nil
}

// ValidateFlagDescription checks that raw YAML flag file content contains a
// non-empty description field. Returns an error when the description is
// missing or blank.
func ValidateFlagDescription(data []byte) error {
	var partial struct {
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal(data, &partial); err != nil {
		return fmt.Errorf("parsing flag file: %w", err)
	}
	if strings.TrimSpace(partial.Description) == "" {
		return fmt.Errorf("flag file must have a non-empty 'description' field")
	}
	return nil
}

// loadFlagConfig loads and parses a flag's gck--{name}.yaml file into a
// Config struct ready for merging. It renders the flag patch using the
// parent context's effective vars as the base, with the flag's own var
// defs and --set overrides merged on top.
func loadFlagConfig(flag config.ContextFlag, contextVars map[string]string, setOverrides map[string]string) (*config.Config, error) {
	path := filepath.Join(flag.Dir, flagFilePrefix+flag.Name+".yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading flag file %s: %w", path, err)
	}

	flagVars, err := gcktmpl.ExtractVarDefs(data)
	if err != nil {
		return nil, fmt.Errorf("extracting vars from flag %s: %w", path, err)
	}

	vars := make(map[string]string, len(contextVars)+len(flagVars)+len(setOverrides))
	for k, v := range contextVars {
		vars[k] = v
	}
	for _, d := range flagVars {
		vars[d.Name] = d.Default
	}
	for k, v := range setOverrides {
		vars[k] = v
	}

	rendered, err := gcktmpl.RenderWithVars(data, vars)
	if err != nil {
		return nil, fmt.Errorf("templating flag file %s: %w", path, err)
	}

	var cfg config.Config
	if err := yaml.Unmarshal(rendered, &cfg); err != nil {
		return nil, fmt.Errorf("parsing flag file %s: %w", path, err)
	}

	cfg.Vars = yaml.Node{}
	cfg.Kind.ApplyDefaults()
	cfg.Dir = filepath.Dir(path)
	return &cfg, nil
}
