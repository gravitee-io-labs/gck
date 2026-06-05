package config

import (
	"fmt"
	"os"
	"path/filepath"

	gcktmpl "github.com/gravitee-io-labs/gck/internal/template"
	"gopkg.in/yaml.v3"
)

type HelmConfig struct {
	Repos []Repo `yaml:"repos,omitempty"`
}

type Config struct {
	Vars        yaml.Node `yaml:"vars,omitempty"`
	Description string            `yaml:"description,omitempty"`
	Registry    string            `yaml:"registry"`
	From        []string          `yaml:"from,omitempty"`
	Abstract    bool              `yaml:"abstract,omitempty"`
	Kind        KindConfig        `yaml:"kind"`
	Features    FeaturesConfig    `yaml:"features,omitempty"`
	Images      ImagesConfig      `yaml:"images,omitempty"`
	Helm        HelmConfig        `yaml:"helm,omitempty"`
	Components  []Component       `yaml:"components,omitempty"`
	Builds      []Build           `yaml:"builds,omitempty"`

	// Dir is set by Load to resolve relative paths in component value files.
	Dir string `yaml:"-"`
}

// ApplyDefaults merges embedded default values into cfg. Only fields that are
// still at their zero value are populated. The from field is intentionally
// not defaulted so that omitting it yields a plain Kind cluster.
func ApplyDefaults(cfg *Config, defaultData []byte) {
	if len(defaultData) == 0 {
		return
	}
	var defaults Config
	if err := yaml.Unmarshal(defaultData, &defaults); err != nil {
		return
	}
	if cfg.Registry == "" {
		cfg.Registry = defaults.Registry
	}
	cfg.Kind.MergeWithDefaults(&defaults.Kind)
}

// Merge applies non-zero fields from override onto base. For each top-level
// Config field, the override value wins when set.
func Merge(base, override *Config) {
	if override.Registry != "" {
		base.Registry = override.Registry
	}
	if len(override.From) > 0 {
		base.From = override.From
	}
	if override.Dir != "" {
		base.Dir = override.Dir
	}
	if len(override.Helm.Repos) > 0 {
		base.Helm.Repos = override.Helm.Repos
	}
	if len(override.Components) > 0 {
		base.Components = override.Components
	}
	if len(override.Builds) > 0 {
		base.Builds = override.Builds
	}
	base.Images = MergeImages(base.Images, override.Images)
	base.Features = MergeFeatures(base.Features, override.Features)
	override.Kind.MergeWithDefaults(&base.Kind)
	base.Kind = override.Kind
}

func Load(path string, setOverrides map[string]string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	rendered, err := gcktmpl.Render(data, setOverrides)
	if err != nil {
		return nil, fmt.Errorf("templating config file %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(rendered, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}

	cfg.Vars = yaml.Node{}
	cfg.Kind.ApplyDefaults()
	cfg.Dir = filepath.Dir(path)
	return &cfg, nil
}
