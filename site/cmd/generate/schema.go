package main

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type jsonSchema struct {
	Title       string                `yaml:"title"`
	Description string                `yaml:"description"`
	Type        string                `yaml:"type"`
	Properties  map[string]jsonSchema `yaml:"properties"`
	Defs        map[string]jsonSchema `yaml:"$defs"`
	Ref         string                `yaml:"$ref"`
	Items       *jsonSchema           `yaml:"items"`
	Required    []string              `yaml:"required"`
	Default     any                   `yaml:"default"`
	Enum        []any                 `yaml:"enum"`
	Format      string                `yaml:"format"`
	AdditionalProperties any          `yaml:"additionalProperties"`
}

func generateSchemaDoc(schemaPath, outputPath string) {
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		fatalf("read schema: %v", err)
	}

	var schema jsonSchema
	if err := yaml.Unmarshal(data, &schema); err != nil {
		fatalf("parse schema: %v", err)
	}

	var buf bytes.Buffer

	buf.WriteString("---\n")
	buf.WriteString("title: \"Configuration\"\n")
	buf.WriteString("weight: 1\n")
	buf.WriteString("type: docs\n")
	buf.WriteString("---\n\n")

	buf.WriteString("This page is generated from the ")
	buf.WriteString("[gck.yaml JSON Schema](https://github.com/gravitee-io-labs/gck/blob/main/schema/gck.schema.yaml). ")
	buf.WriteString("It documents every field you can use in your `gck.yaml` configuration file.\n\n")

	buf.WriteString("## Overview\n\n")
	buf.WriteString("A `gck.yaml` file is a YAML document with the following top-level fields:\n\n")

	buf.WriteString("| Field | Type | Description |\n")
	buf.WriteString("|-------|------|-------------|\n")

	keys := sortedKeys(schema.Properties)
	for _, name := range keys {
		prop := schema.Properties[name]
		prop = resolveRef(prop, schema.Defs)
		typeName := displayType(prop, schema.Defs)
		desc := oneLine(prop.Description)
		buf.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", name, typeName, desc))
	}

	buf.WriteString("\n---\n\n")

	for _, name := range keys {
		prop := schema.Properties[name]
		resolved := resolveRef(prop, schema.Defs)
		writePropertySection(&buf, name, resolved, schema.Defs, 2)
	}

	if err := os.MkdirAll("site/content/docs/reference", 0755); err != nil {
		fatalf("mkdir reference: %v", err)
	}
	if err := os.WriteFile(outputPath, buf.Bytes(), 0644); err != nil {
		fatalf("write schema doc: %v", err)
	}

	fmt.Printf("generated schema doc: %s\n", outputPath)
}

func writePropertySection(buf *bytes.Buffer, name string, prop jsonSchema, defs map[string]jsonSchema, level int) {
	heading := strings.Repeat("#", level)
	buf.WriteString(fmt.Sprintf("%s `%s`\n\n", heading, name))

	if prop.Description != "" {
		buf.WriteString(prop.Description + "\n\n")
	}

	writeFieldMeta(buf, prop)

	if prop.Type == "object" && len(prop.Properties) > 0 {
		writePropertiesTable(buf, prop, defs)

		for _, childName := range sortedKeys(prop.Properties) {
			child := prop.Properties[childName]
			resolved := resolveRef(child, defs)
			if resolved.Type == "object" && len(resolved.Properties) > 0 {
				writePropertySection(buf, name+"."+childName, resolved, defs, min(level+1, 4))
			} else if resolved.Type == "array" && resolved.Items != nil {
				itemResolved := resolveRef(*resolved.Items, defs)
				if itemResolved.Type == "object" && len(itemResolved.Properties) > 0 {
					writePropertySection(buf, name+"."+childName+"[*]", itemResolved, defs, min(level+1, 4))
				}
			}
		}
	}

	if prop.Type == "array" && prop.Items != nil {
		itemResolved := resolveRef(*prop.Items, defs)
		if itemResolved.Type == "object" && len(itemResolved.Properties) > 0 {
			buf.WriteString("Each entry is an object with the following fields:\n\n")
			writePropertiesTable(buf, itemResolved, defs)

			for _, childName := range sortedKeys(itemResolved.Properties) {
				child := itemResolved.Properties[childName]
				resolved := resolveRef(child, defs)
				if resolved.Type == "object" && len(resolved.Properties) > 0 {
					writePropertySection(buf, name+"[*]."+childName, resolved, defs, min(level+1, 4))
				} else if resolved.Type == "array" && resolved.Items != nil {
					nestedItem := resolveRef(*resolved.Items, defs)
					if nestedItem.Type == "object" && len(nestedItem.Properties) > 0 {
						writePropertySection(buf, name+"[*]."+childName+"[*]", nestedItem, defs, min(level+1, 4))
					}
				}
			}
		}
	}
}

func writeFieldMeta(buf *bytes.Buffer, prop jsonSchema) {
	var meta []string
	if prop.Type != "" {
		meta = append(meta, fmt.Sprintf("**Type:** `%s`", prop.Type))
	}
	if prop.Default != nil {
		meta = append(meta, fmt.Sprintf("**Default:** `%v`", prop.Default))
	}
	if len(prop.Enum) > 0 {
		vals := make([]string, len(prop.Enum))
		for i, v := range prop.Enum {
			vals[i] = fmt.Sprintf("`%v`", v)
		}
		meta = append(meta, fmt.Sprintf("**Allowed values:** %s", strings.Join(vals, ", ")))
	}
	if len(meta) > 0 {
		buf.WriteString(strings.Join(meta, " | ") + "\n\n")
	}
}

func writePropertiesTable(buf *bytes.Buffer, prop jsonSchema, defs map[string]jsonSchema) {
	requiredSet := map[string]bool{}
	for _, r := range prop.Required {
		requiredSet[r] = true
	}

	buf.WriteString("| Field | Type | Required | Description |\n")
	buf.WriteString("|-------|------|----------|-------------|\n")

	for _, childName := range sortedKeys(prop.Properties) {
		child := prop.Properties[childName]
		resolved := resolveRef(child, defs)
		typeName := displayType(resolved, defs)
		required := "No"
		if requiredSet[childName] {
			required = "Yes"
		}
		desc := oneLine(resolved.Description)
		if resolved.Default != nil {
			desc += fmt.Sprintf(" Default: `%v`.", resolved.Default)
		}
		if len(resolved.Enum) > 0 {
			vals := make([]string, len(resolved.Enum))
			for i, v := range resolved.Enum {
				vals[i] = fmt.Sprintf("`%v`", v)
			}
			desc += fmt.Sprintf(" Values: %s.", strings.Join(vals, ", "))
		}
		buf.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s |\n", childName, typeName, required, desc))
	}

	buf.WriteString("\n")
}

func resolveRef(prop jsonSchema, defs map[string]jsonSchema) jsonSchema {
	if prop.Ref == "" {
		return prop
	}
	refName := strings.TrimPrefix(prop.Ref, "#/$defs/")
	if def, ok := defs[refName]; ok {
		return def
	}
	return prop
}

func displayType(prop jsonSchema, defs map[string]jsonSchema) string {
	if prop.Type == "array" && prop.Items != nil {
		inner := resolveRef(*prop.Items, defs)
		if inner.Type == "object" {
			return "object[]"
		}
		return inner.Type + "[]"
	}
	if prop.Type == "object" && prop.AdditionalProperties != nil {
		return "map"
	}
	if prop.Type == "" && prop.Ref != "" {
		resolved := resolveRef(prop, defs)
		return displayType(resolved, defs)
	}
	if prop.Type == "" {
		return "any"
	}
	return prop.Type
}

func oneLine(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return s
}

func sortedKeys(m map[string]jsonSchema) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
