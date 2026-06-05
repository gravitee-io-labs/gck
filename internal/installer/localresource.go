package installer

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gravitee-io-labs/gck/internal/config"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type resolvedEntry struct {
	key   string
	value string
}

func resolveEntries(res config.LocalResource) ([]resolvedEntry, error) {
	if res.FromFile != "" {
		path := res.FromFile
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading file %q: %w", path, err)
		}
		key := filepath.Base(path)
		return []resolvedEntry{{key: key, value: string(data)}}, nil
	}

	entries := make([]resolvedEntry, 0, len(res.Entries))
	for _, e := range res.Entries {
		key := e.Key
		var value string

		switch {
		case e.FromFile != "":
			path := e.FromFile
			if key == "" {
				key = filepath.Base(path)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("reading file %q: %w", path, err)
			}
			value = string(data)
		case e.FromEnv != "":
			if key == "" {
				key = e.FromEnv
			}
			v, ok := os.LookupEnv(e.FromEnv)
			if !ok {
				return nil, fmt.Errorf("environment variable %q is not set", e.FromEnv)
			}
			value = v
		default:
			return nil, fmt.Errorf("entry with key %q has neither fromFile nor fromEnv", key)
		}

		entries = append(entries, resolvedEntry{key: key, value: value})
	}
	return entries, nil
}

func shouldIgnore(res config.LocalResource) bool {
	return res.OnMissing == "ignore"
}

func buildResource(res config.LocalResource, kind string, dataField string, encodeBase64 bool) (*unstructured.Unstructured, string, error) {
	entries, err := resolveEntries(res)
	if err != nil {
		if shouldIgnore(res) {
			warning := fmt.Sprintf("%s %q skipped: %v", kind, res.Name, err)
			return nil, warning, nil
		}
		return nil, "", fmt.Errorf("%s %q: %w", kind, res.Name, err)
	}

	data := make(map[string]interface{}, len(entries))
	for _, e := range entries {
		if encodeBase64 {
			data[e.key] = base64.StdEncoding.EncodeToString([]byte(e.value))
		} else {
			data[e.key] = e.value
		}
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       kind,
			"metadata": map[string]interface{}{
				"name": res.Name,
			},
			dataField: data,
		},
	}
	return obj, "", nil
}

// BuildSecrets materializes LocalResource specs into Kubernetes Secret objects.
// It returns the built objects, any warnings (for skipped resources), and an error
// if a required source is missing.
func BuildSecrets(secrets []config.LocalResource) ([]*unstructured.Unstructured, []string, error) {
	return buildResources(secrets, "Secret", "data", true)
}

// BuildConfigMaps materializes LocalResource specs into Kubernetes ConfigMap objects.
// It returns the built objects, any warnings (for skipped resources), and an error
// if a required source is missing.
func BuildConfigMaps(configMaps []config.LocalResource) ([]*unstructured.Unstructured, []string, error) {
	return buildResources(configMaps, "ConfigMap", "data", false)
}

func buildResources(resources []config.LocalResource, kind string, dataField string, encodeBase64 bool) ([]*unstructured.Unstructured, []string, error) {
	var objects []*unstructured.Unstructured
	var warnings []string

	for _, res := range resources {
		obj, warning, err := buildResource(res, kind, dataField, encodeBase64)
		if err != nil {
			return nil, warnings, err
		}
		if warning != "" {
			warnings = append(warnings, warning)
			continue
		}
		objects = append(objects, obj)
	}
	return objects, warnings, nil
}
