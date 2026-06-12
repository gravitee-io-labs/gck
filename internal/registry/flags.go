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
// per valid file. Each file is parsed to extract its description field. Files
// whose names don't match the naming convention are returned as errors.
func DiscoverFlags(dir string) ([]config.ContextFlag, error) {
	matches, err := filepath.Glob(filepath.Join(dir, flagFilePrefix+"*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("globbing flag files in %s: %w", dir, err)
	}
	sort.Strings(matches)

	var flags []config.ContextFlag
	for _, path := range matches {
		name, err := FlagNameFromFile(filepath.Base(path))
		if err != nil {
			return nil, err
		}

		desc, err := readFlagDescription(path)
		if err != nil {
			return nil, fmt.Errorf("reading flag file %s: %w", path, err)
		}

		flags = append(flags, config.ContextFlag{
			Name:        name,
			Description: desc,
			Dir:         dir,
		})
	}
	return flags, nil
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
		byName[f.Name] = i
	}
	for _, f := range child {
		if idx, ok := byName[f.Name]; ok {
			result[idx] = f
		} else {
			byName[f.Name] = len(result)
			result = append(result, f)
		}
	}
	return result
}

// ApplyFlags loads each active flag's patch file and merges it into the
// resolved context using the same merge semantics as context composition.
// Active flags are validated against the available flags on the context.
// The resolved context's EffectiveVars are used as the base variable set
// when rendering flag patches, so flags can reference vars declared by
// the parent context (e.g. {{ .imageTag }}). setOverrides are merged on
// top.
func ApplyFlags(resolved *config.ResolvedContext, activeFlags []string, setOverrides map[string]string) error {
	if len(activeFlags) == 0 {
		return nil
	}

	available := make(map[string]config.ContextFlag, len(resolved.Flags))
	for _, f := range resolved.Flags {
		available[f.Name] = f
	}

	for _, name := range activeFlags {
		flag, ok := available[name]
		if !ok {
			var known []string
			for _, f := range resolved.Flags {
				known = append(known, "--"+f.Name)
			}
			return fmt.Errorf("unknown context flag --%s (available: %s)", name, strings.Join(known, ", "))
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

// readFlagDescription parses a flag YAML file and returns the description
// field. The description is optional; an empty string is returned when absent.
func readFlagDescription(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var partial struct {
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal(data, &partial); err != nil {
		return "", err
	}
	return partial.Description, nil
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
