package schema_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gravitee-io-labs/gck/internal/config"
	internalschema "github.com/gravitee-io-labs/gck/internal/schema"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	yamlBytes, err := os.ReadFile("gck.schema.yaml")
	if err != nil {
		t.Fatalf("reading schema: %v", err)
	}
	sch, err := internalschema.Compile(yamlBytes)
	if err != nil {
		t.Fatalf("compiling schema: %v", err)
	}
	return sch
}

// TestRegistryValidation walks registry/**/gck.yaml and validates each file
// against the JSON Schema. This proves the schema accepts every real config.
func TestRegistryValidation(t *testing.T) {
	sch := compileSchema(t)

	registryDir := filepath.Join("..", "registry")
	var files []string
	err := filepath.Walk(registryDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && info.Name() == "gck.yaml" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking registry: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no gck.yaml files found in registry/")
	}

	for _, file := range files {
		rel, _ := filepath.Rel(registryDir, file)
		t.Run(rel, func(t *testing.T) {
			if err := internalschema.ValidateFile(sch, file); err != nil {
				t.Errorf("schema validation failed:\n%v", err)
			}
		})
	}
}

// TestRegistryFlagValidation walks registry/**/gck--*.yaml and validates each
// flag file against the JSON Schema and checks for a valid name and description.
func TestRegistryFlagValidation(t *testing.T) {
	sch := compileSchema(t)

	registryDir := filepath.Join("..", "registry")
	var files []string
	err := filepath.Walk(registryDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasPrefix(info.Name(), "gck--") && strings.HasSuffix(info.Name(), ".yaml") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking registry: %v", err)
	}
	if len(files) == 0 {
		t.Skip("no flag files found in registry/")
	}

	for _, file := range files {
		rel, _ := filepath.Rel(registryDir, file)
		t.Run(rel, func(t *testing.T) {
			if err := internalschema.ValidateFile(sch, file); err != nil {
				t.Errorf("schema validation failed:\n%v", err)
			}
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("reading flag file: %v", err)
			}
			var partial struct {
				Description string `yaml:"description"`
			}
			if err := yaml.Unmarshal(data, &partial); err != nil {
				t.Fatalf("parsing flag file: %v", err)
			}
			if strings.TrimSpace(partial.Description) == "" {
				t.Error("flag file must have a non-empty 'description' field")
			}
		})
	}
}

// TestStructCoverage reflects on config.Config and all nested types, collecting
// every yaml tag, and asserts a corresponding property exists in the schema.
// This catches drift when a Go field is added but not mirrored in the schema.
func TestStructCoverage(t *testing.T) {
	yamlBytes, err := os.ReadFile("gck.schema.yaml")
	if err != nil {
		t.Fatalf("reading schema: %v", err)
	}
	var schemaMap map[string]any
	if err := yaml.Unmarshal(yamlBytes, &schemaMap); err != nil {
		t.Fatalf("parsing schema: %v", err)
	}

	schemaProps := extractSchemaProperties(schemaMap)

	goFields := make(map[string][]string)
	visited := make(map[reflect.Type]bool)
	collectYAMLFields(reflect.TypeOf(config.Config{}), goFields, visited)

	for typeName, fields := range goFields {
		props, ok := schemaProps[typeName]
		if !ok {
			t.Errorf("Go type %q has no corresponding schema definition", typeName)
			continue
		}
		for _, field := range fields {
			if !props[field] {
				t.Errorf("Go type %s: yaml field %q missing from schema", typeName, field)
			}
		}
	}
}

// extractSchemaProperties builds a map of type name → property name set.
// "Config" maps to root properties; each $defs/<Name> maps to <Name>.
func extractSchemaProperties(schema map[string]any) map[string]map[string]bool {
	result := make(map[string]map[string]bool)

	if props, ok := schema["properties"].(map[string]any); ok {
		set := make(map[string]bool, len(props))
		for k := range props {
			set[k] = true
		}
		result["Config"] = set
	}

	defs, _ := schema["$defs"].(map[string]any)
	for name, def := range defs {
		defMap, ok := def.(map[string]any)
		if !ok {
			continue
		}
		props, ok := defMap["properties"].(map[string]any)
		if !ok {
			continue
		}
		set := make(map[string]bool, len(props))
		for k := range props {
			set[k] = true
		}
		result[name] = set
	}

	return result
}

// collectYAMLFields recursively walks a struct type, recording each yaml tag
// name grouped by Go type name. Fields tagged yaml:"-" are skipped.
func collectYAMLFields(t reflect.Type, result map[string][]string, visited map[reflect.Type]bool) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || visited[t] {
		return
	}
	visited[t] = true

	var fields []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("yaml")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.SplitN(tag, ",", 2)[0]
		if name == "" || name == "-" {
			continue
		}
		fields = append(fields, name)

		ft := f.Type
		for ft.Kind() == reflect.Ptr || ft.Kind() == reflect.Slice {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct {
			collectYAMLFields(ft, result, visited)
		}
	}
	if len(fields) > 0 {
		result[t.Name()] = fields
	}
}
