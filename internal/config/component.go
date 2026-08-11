package config

import "github.com/gravitee-io-labs/gck/internal/notes"

type GatewayChannel string

const (
	GatewayChannelStandard     GatewayChannel = "standard"
	GatewayChannelExperimental GatewayChannel = "experimental"
)

type Repo struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

type HelmSpec struct {
	Chart      string                 `yaml:"chart"`
	Version    string                 `yaml:"version,omitempty"`
	ValueFiles []string               `yaml:"valueFiles,omitempty"`
	Values     map[string]interface{} `yaml:"values,omitempty"`
}

type ResourceEntry struct {
	Key      string `yaml:"key,omitempty"`
	FromFile string `yaml:"fromFile,omitempty"`
	FromEnv  string `yaml:"fromEnv,omitempty"`
}

type LocalResource struct {
	Name      string          `yaml:"name"`
	OnMissing string          `yaml:"onMissing,omitempty"` // "fail" (default) or "ignore"
	FromFile  string          `yaml:"fromFile,omitempty"`  // shorthand for single-file
	Entries   []ResourceEntry `yaml:"entries,omitempty"`
}

type K8sSpec struct {
	ManifestFiles []string                 `yaml:"manifestFiles,omitempty"`
	Manifests     []map[string]interface{} `yaml:"manifests,omitempty"`
	Secrets       []LocalResource          `yaml:"secrets,omitempty"`
	ConfigMaps    []LocalResource          `yaml:"configMaps,omitempty"`
}

type Conditions struct {
	Ready bool `yaml:"ready,omitempty"`
}

type Selector struct {
	MatchLabels map[string]string `yaml:"matchLabels,omitempty"`
}

// Requirement declares a dependency on another component.
type Requirement struct {
	Component  string     `yaml:"component"`
	Conditions Conditions `yaml:"conditions,omitempty"`
	Selector   *Selector  `yaml:"selector,omitempty"`
	Timeout    string     `yaml:"timeout,omitempty"`
}

type Component struct {
	Name       string        `yaml:"name"`
	Enabled    *bool         `yaml:"enabled,omitempty"`
	Type       string        `yaml:"type,omitempty"`
	Namespace  string        `yaml:"namespace,omitempty"`
	Conditions Conditions    `yaml:"conditions,omitempty"`
	Selector   *Selector     `yaml:"selector,omitempty"`
	Timeout    string        `yaml:"timeout,omitempty"`
	Requires   []Requirement `yaml:"requires,omitempty"`
	Helm       *HelmSpec     `yaml:"helm,omitempty"`
	K8s        *K8sSpec      `yaml:"k8s,omitempty"`
}

// IsEnabled returns true when the component should be deployed.
// A nil Enabled pointer means the component is enabled (default).
func (c *Component) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// EffectiveType returns Type, defaulting to "helm".
func (c *Component) EffectiveType() string {
	if c.Type == "" {
		return "helm"
	}
	return c.Type
}

// ResolvedNotes collects the notes contributed by every context in a
// composition. Unlike most fields, notes are not last-wins: each layer keeps
// its entry so `gck create` can merge them into a single set of instructions.
type ResolvedNotes struct {
	Create []notes.Layer
	Delete []notes.Layer
}

// ContextFlag represents an optional toggle defined by a gck--{name}.yaml
// patch file in a context directory.
type ContextFlag struct {
	Name        string
	Description string
	Dir         string // directory containing the gck--{name}.yaml file
}

// ResolvedContext is a fully resolved context with all referenced files in Dir.
type ResolvedContext struct {
	Repos         []Repo
	Components    []Component
	Dir           string
	Kind          KindConfig
	Features      FeaturesConfig
	Images        ImagesConfig
	Notes         ResolvedNotes
	Abstract      bool
	Flags         []ContextFlag
	EffectiveVars map[string]string
}
