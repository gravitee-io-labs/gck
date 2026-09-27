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

	// Group is set on alternatives (use-* flags): mutually exclusive
	// implementations of the same concern, exactly one of which is always
	// applied. Empty for plain flags.
	Group string
	// Default marks the alternative applied when no member of its group is
	// selected.
	Default bool
	// Pinned is set on every member of a group whose selection was fixed by
	// a composing context's use: block. Pinned groups are not offered to
	// users.
	Pinned bool
	// Requires lists the flags or alternatives that must be active for this
	// flag to be applied.
	Requires []string
	// Conflicts lists the flags or alternatives that must not be active
	// alongside this flag.
	Conflicts []string
	// Implies, on an alternative, lists the plain flags turned on whenever it
	// is selected (e.g. use-embedded implies disable-ui).
	Implies []string
	// Disables, on a plain flag, lists the alternative groups of which no
	// member is applied while the flag is active (e.g. disable-metrics
	// disables the metrics group).
	Disables []string
}

// IsAlternative reports whether the flag is a member of an alternative group.
func (f ContextFlag) IsAlternative() bool {
	return f.Group != ""
}

// ResolvedContext is a fully resolved context with all referenced files in Dir.
type ResolvedContext struct {
	Repos      []Repo
	Components []Component
	Dir        string
	Kind       KindConfig
	Features   FeaturesConfig
	Images     ImagesConfig
	Notes      ResolvedNotes
	Abstract   bool
	Flags      []ContextFlag
	// Selected maps each alternative group in the composition to the name of
	// the applied member (e.g. "database" -> "use-postgres").
	Selected map[string]string
	// Implied lists the plain flags turned on by the selected alternatives.
	Implied       []string
	EffectiveVars map[string]string
}
