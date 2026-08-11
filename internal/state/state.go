package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gravitee-io-labs/gck/internal/config"
	"github.com/gravitee-io-labs/gck/internal/notes"
	"gopkg.in/yaml.v3"
)

// ClusterState holds the information captured at create time so a cluster can be
// torn down (delete) or upgraded (patch) without re-resolving the original config
// or registry. Set records the --set overrides so patch can reuse them.
type ClusterState struct {
	Name      string                `yaml:"name"`
	CreatedAt time.Time             `yaml:"createdAt"`
	Registry  string                `yaml:"registry,omitempty"`
	From      []string              `yaml:"from,omitempty"`
	Flags     []string              `yaml:"flags,omitempty"`
	Set       map[string]string     `yaml:"set,omitempty"`
	Features  config.FeaturesConfig `yaml:"features,omitempty"`
	Images    config.ImagesConfig   `yaml:"images,omitempty"`
	Notes     DeleteNotes           `yaml:"notes,omitempty"`
}

// DeleteNotes carries the notes.delete layers captured at create time so
// `gck delete` can print them without re-resolving the registry.
type DeleteNotes struct {
	Delete []notes.Layer `yaml:"delete,omitempty"`
}

// Save writes the cluster state to <dir>/<name>.yaml, creating the directory
// if it does not exist.
func Save(dir string, s *ClusterState) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating state directory %s: %w", dir, err)
	}
	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshalling cluster state: %w", err)
	}
	path := filepath.Join(dir, s.Name+".yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing state file %s: %w", path, err)
	}
	return nil
}

// Load reads a cluster state file from <dir>/<name>.yaml.
func Load(dir, name string) (*ClusterState, error) {
	path := filepath.Join(dir, name+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading state file %s: %w", path, err)
	}
	var s ClusterState
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing state file %s: %w", path, err)
	}
	return &s, nil
}

// List returns the names of all clusters that have a state file in dir.
func List(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading state directory %s: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	return names, nil
}

// Remove deletes the state file for the given cluster name.
func Remove(dir, name string) error {
	path := filepath.Join(dir, name+".yaml")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing state file %s: %w", path, err)
	}
	return nil
}
