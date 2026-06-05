package schema

import (
	"bytes"
	"fmt"
	"os"

	"github.com/santhosh-tekuri/jsonschema/v6"
	sigsyaml "sigs.k8s.io/yaml"
)

// Compile compiles a JSON Schema from raw YAML bytes and returns the
// ready-to-use validator.
func Compile(yamlBytes []byte) (*jsonschema.Schema, error) {
	jsonBytes, err := sigsyaml.YAMLToJSON(yamlBytes)
	if err != nil {
		return nil, fmt.Errorf("converting schema to JSON: %w", err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("unmarshaling schema: %w", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("gck.schema.json", doc); err != nil {
		return nil, fmt.Errorf("adding schema resource: %w", err)
	}
	sch, err := c.Compile("gck.schema.json")
	if err != nil {
		return nil, fmt.Errorf("compiling schema: %w", err)
	}
	return sch, nil
}

// ValidateFile reads the YAML file at path and validates it against sch.
func ValidateFile(sch *jsonschema.Schema, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}
	return ValidateBytes(sch, data)
}

// ValidateBytes validates raw YAML content against sch.
func ValidateBytes(sch *jsonschema.Schema, yamlBytes []byte) error {
	jsonBytes, err := sigsyaml.YAMLToJSON(yamlBytes)
	if err != nil {
		return fmt.Errorf("converting to JSON: %w", err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(jsonBytes))
	if err != nil {
		return fmt.Errorf("unmarshaling instance: %w", err)
	}
	return sch.Validate(inst)
}
