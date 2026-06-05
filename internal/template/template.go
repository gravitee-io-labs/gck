package gcktmpl

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// Render templates raw gck.yaml bytes by extracting the vars block,
// merging with --set overrides, and executing the document as a Go
// text/template. The returned bytes are ready for YAML unmarshaling.
func Render(raw []byte, setOverrides map[string]string) ([]byte, error) {
	vars, err := extractVars(raw)
	if err != nil {
		return nil, fmt.Errorf("extracting vars: %w", err)
	}

	for k, v := range setOverrides {
		vars[k] = v
	}

	return RenderWithVars(raw, vars)
}

// RenderWithVars templates raw gck.yaml bytes using a pre-computed
// variable map. Unlike Render, it does not extract vars from the document
// or apply --set overrides — the caller is responsible for providing the
// fully-merged effective vars.
func RenderWithVars(raw []byte, vars map[string]string) ([]byte, error) {
	tmpl, err := template.New("gck").
		Option("missingkey=error").
		Funcs(funcMap()).
		Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return nil, fmt.Errorf("executing template: %w", err)
	}
	return buf.Bytes(), nil
}

// VarDef describes a template variable with its default value and
// optional documentation.
type VarDef struct {
	Name        string
	Default     string
	Description string
}

// VarOverride describes a variable override targeting a parent context's
// variable via path-scoped nesting in the vars block.
type VarOverride struct {
	ContextPath string // e.g. "mysql/standalone"
	Name        string // e.g. "imageTag"
	Default     string // e.g. "8"
}

// VarsTree holds the result of parsing a vars block: own variable
// declarations and path-scoped overrides for parent contexts.
type VarsTree struct {
	Defs      []VarDef
	Overrides []VarOverride
}

// ExtractVarsTree scans raw gck.yaml bytes for a top-level vars block and
// separates own var declarations from path-scoped overrides.
//
// Disambiguation: a mapping node with a "default" key is a var declaration
// (or override leaf). A mapping node without "default" is a path segment
// leading deeper toward override leaves. Scalar values are simple var
// declarations.
func ExtractVarsTree(raw []byte) (*VarsTree, error) {
	varsYAML, ok := isolateVarsBlock(raw)
	if !ok {
		return &VarsTree{}, nil
	}

	var wrapper struct {
		Vars yaml.Node `yaml:"vars"`
	}
	if err := yaml.Unmarshal([]byte(varsYAML), &wrapper); err != nil {
		return nil, fmt.Errorf("parsing vars block: %w", err)
	}

	node := &wrapper.Vars
	if node.Kind == 0 || node.Kind != yaml.MappingNode {
		return &VarsTree{}, nil
	}

	tree := &VarsTree{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		val := node.Content[i+1]

		switch val.Kind {
		case yaml.ScalarNode:
			tree.Defs = append(tree.Defs, VarDef{
				Name:    key.Value,
				Default: val.Value,
			})
		case yaml.MappingNode:
			if hasDefaultKey(val) {
				var ext struct {
					Default     string `yaml:"default"`
					Description string `yaml:"description"`
				}
				if err := val.Decode(&ext); err != nil {
					return nil, fmt.Errorf("parsing var %q: %w", key.Value, err)
				}
				tree.Defs = append(tree.Defs, VarDef{
					Name:        key.Value,
					Default:     ext.Default,
					Description: ext.Description,
				})
			} else {
				overrides, err := collectOverrides(val, key.Value)
				if err != nil {
					return nil, err
				}
				tree.Overrides = append(tree.Overrides, overrides...)
			}
		}
	}
	return tree, nil
}

// hasDefaultKey returns true if the mapping node has a direct child key named "default".
func hasDefaultKey(node *yaml.Node) bool {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == "default" {
			return true
		}
	}
	return false
}

// collectOverrides walks a mapping node that represents path segments,
// collecting VarOverride entries. pathPrefix is the slash-joined path
// built from parent keys (e.g. "mysql/standalone").
func collectOverrides(node *yaml.Node, pathPrefix string) ([]VarOverride, error) {
	var overrides []VarOverride
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		val := node.Content[i+1]

		switch val.Kind {
		case yaml.ScalarNode:
			overrides = append(overrides, VarOverride{
				ContextPath: pathPrefix,
				Name:        key.Value,
				Default:     val.Value,
			})
		case yaml.MappingNode:
			if hasDefaultKey(val) {
				var ext struct {
					Default string `yaml:"default"`
				}
				if err := val.Decode(&ext); err != nil {
					return nil, fmt.Errorf("parsing override %s/%s: %w", pathPrefix, key.Value, err)
				}
				overrides = append(overrides, VarOverride{
					ContextPath: pathPrefix,
					Name:        key.Value,
					Default:     ext.Default,
				})
			} else {
				nested, err := collectOverrides(val, pathPrefix+"/"+key.Value)
				if err != nil {
					return nil, err
				}
				overrides = append(overrides, nested...)
			}
		}
	}
	return overrides, nil
}

// ExtractVarDefs scans raw gck.yaml bytes for a top-level vars block and
// returns structured metadata for each variable. Supports both simple
// (key: "value") and extended (key: {default: "value", description: "..."})
// forms. Path-scoped override entries are excluded.
func ExtractVarDefs(raw []byte) ([]VarDef, error) {
	tree, err := ExtractVarsTree(raw)
	if err != nil {
		return nil, err
	}
	if tree == nil || len(tree.Defs) == 0 {
		return nil, nil
	}
	return tree.Defs, nil
}

// extractVars scans raw YAML bytes for a top-level vars block and returns
// a flat string map suitable for template execution. Supports both simple
// (key: "value") and extended (key: {default: "value", description: "..."})
// forms — the extended form is normalized to just key → default.
func extractVars(raw []byte) (map[string]string, error) {
	defs, err := ExtractVarDefs(raw)
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(defs))
	for _, d := range defs {
		m[d.Name] = d.Default
	}
	return m, nil
}

// isolateVarsBlock finds the top-level vars: block boundaries in raw YAML
// that may contain template expressions elsewhere. Returns the slice of
// text containing only the vars block (valid YAML), and whether one was found.
func isolateVarsBlock(raw []byte) (string, bool) {
	lines := strings.Split(string(raw), "\n")

	start := -1
	end := len(lines)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if start == -1 {
			if trimmed == "vars:" {
				start = i
				continue
			}
			if strings.HasPrefix(trimmed, "vars:") {
				start = i
				end = i + 1
				break
			}
			continue
		}
		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' && !strings.HasPrefix(trimmed, "#") {
			end = i
			break
		}
	}

	if start == -1 {
		return "", false
	}
	return strings.Join(lines[start:end], "\n"), true
}

// funcMap returns the template.FuncMap shared by all gck template
// rendering. It provides:
//
//   - env: returns the value of an environment variable.
//   - default: returns a fallback when the pipeline value is empty.
//   - required: returns the pipeline value or an error with the given message.
func funcMap() template.FuncMap {
	return template.FuncMap{
		"env": func(key string) string {
			return os.Getenv(key)
		},
		"default": func(fallback, val string) string {
			if val == "" {
				return fallback
			}
			return val
		},
		"required": func(msg, val string) (string, error) {
			if val == "" {
				return "", fmt.Errorf("%s", msg)
			}
			return val, nil
		},
	}
}
