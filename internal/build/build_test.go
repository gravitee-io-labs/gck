package build

import (
	"testing"

	"github.com/gravitee-io-labs/gck/internal/config"
	v1 "k8s.io/api/core/v1"
	"gopkg.in/yaml.v3"
)

func TestPodSpecReferencesImage_MatchInContainer(t *testing.T) {
	spec := v1.PodSpec{
		Containers: []v1.Container{
			{Name: "gateway", Image: "graviteeio/apim-gateway:latest"},
		},
	}
	if !podSpecReferencesImage(spec, "graviteeio/apim-gateway:latest") {
		t.Fatal("expected match for container image")
	}
}

func TestPodSpecReferencesImage_MatchInInitContainer(t *testing.T) {
	spec := v1.PodSpec{
		InitContainers: []v1.Container{
			{Name: "init", Image: "busybox:latest"},
		},
		Containers: []v1.Container{
			{Name: "app", Image: "myapp:v1"},
		},
	}
	if !podSpecReferencesImage(spec, "busybox:latest") {
		t.Fatal("expected match for init container image")
	}
}

func TestPodSpecReferencesImage_NoMatch(t *testing.T) {
	spec := v1.PodSpec{
		Containers: []v1.Container{
			{Name: "app", Image: "myapp:v1"},
		},
	}
	if podSpecReferencesImage(spec, "other:v2") {
		t.Fatal("expected no match for unrelated image")
	}
}

func TestPodSpecReferencesImage_EmptySpec(t *testing.T) {
	if podSpecReferencesImage(v1.PodSpec{}, "any:image") {
		t.Fatal("expected no match for empty pod spec")
	}
}

func TestPodSpecReferencesImage_MultipleContainersOneMatches(t *testing.T) {
	spec := v1.PodSpec{
		Containers: []v1.Container{
			{Name: "sidecar", Image: "envoy:v1"},
			{Name: "app", Image: "graviteeio/apim-gateway:latest"},
			{Name: "logger", Image: "fluentd:v1"},
		},
	}
	if !podSpecReferencesImage(spec, "graviteeio/apim-gateway:latest") {
		t.Fatal("expected match when one of multiple containers uses the image")
	}
}

func TestPodSpecReferencesImage_ExactMatchRequired(t *testing.T) {
	spec := v1.PodSpec{
		Containers: []v1.Container{
			{Name: "app", Image: "graviteeio/apim-gateway:latest-debian"},
		},
	}
	if podSpecReferencesImage(spec, "graviteeio/apim-gateway:latest") {
		t.Fatal("expected no match when tag differs")
	}
	if podSpecReferencesImage(spec, "graviteeio/apim-gateway") {
		t.Fatal("expected no match when tag is missing from search")
	}
}

func TestPodSpecReferencesImage_BothContainersAndInitContainers(t *testing.T) {
	spec := v1.PodSpec{
		InitContainers: []v1.Container{
			{Name: "init-db", Image: "flyway:v1"},
		},
		Containers: []v1.Container{
			{Name: "app", Image: "myapp:v1"},
		},
	}
	if !podSpecReferencesImage(spec, "flyway:v1") {
		t.Fatal("expected match in init containers")
	}
	if !podSpecReferencesImage(spec, "myapp:v1") {
		t.Fatal("expected match in regular containers")
	}
	if podSpecReferencesImage(spec, "unknown:v1") {
		t.Fatal("expected no match for image not in any container")
	}
}


func TestBuildArgPointers_Nil(t *testing.T) {
	result := buildArgPointers(nil)
	if result != nil {
		t.Fatalf("expected nil for nil input, got %v", result)
	}
}

func TestBuildArgPointers_Empty(t *testing.T) {
	result := buildArgPointers(map[string]string{})
	if result != nil {
		t.Fatalf("expected nil for empty input, got %v", result)
	}
}

func TestBuildArgPointers_ValuesPassedThrough(t *testing.T) {
	args := map[string]string{
		"LITERAL":  "plain",
		"WITH_VAR": "$GCK_TEST_VAR",
	}
	result := buildArgPointers(args)

	if len(result) != 2 {
		t.Fatalf("expected 2 build args, got %d", len(result))
	}
	if result["LITERAL"] == nil || *result["LITERAL"] != "plain" {
		t.Fatalf("expected LITERAL %q, got %v", "plain", result["LITERAL"])
	}
	if result["WITH_VAR"] == nil || *result["WITH_VAR"] != "$GCK_TEST_VAR" {
		t.Fatalf("expected WITH_VAR %q (no expansion), got %v", "$GCK_TEST_VAR", result["WITH_VAR"])
	}
}

func TestBuildArgPointers_AllKeysPreserved(t *testing.T) {
	args := map[string]string{
		"A": "1",
		"B": "2",
		"C": "3",
	}
	result := buildArgPointers(args)
	if len(result) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(result))
	}
	for k, v := range args {
		if _, ok := result[k]; !ok {
			t.Fatalf("expected key %q to be present in result", k)
		}
		if result[k] == nil {
			t.Fatalf("expected non-nil pointer for key %q", k)
		}
		if *result[k] != v {
			t.Fatalf("expected key %q value %q, got %q", k, v, *result[k])
		}
	}
}

func TestBuildArgPointers_PointersAreDistinct(t *testing.T) {
	args := map[string]string{
		"A": "same",
		"B": "same",
	}
	result := buildArgPointers(args)
	if result["A"] == result["B"] {
		t.Fatal("expected distinct pointers for each key even with identical values")
	}
}

func TestBuildArgPointers_IntegrationWithConfig(t *testing.T) {
	input := `
builds:
  - name: aes
    image: docker.io/datawire/aes:3.12.7
    dir: /home/dev/src/edge-stack
    buildArgs:
      EMISSARY_BASE: docker.io/datawire/aes:3.12.7
      BUILD_VERSION: "1.0.0"
      SRC_DIR: /home/dev/src
`
	var cfg config.Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	b := cfg.Builds[0]

	result := buildArgPointers(b.BuildArgs)
	if len(result) != 3 {
		t.Fatalf("expected 3 build args, got %d", len(result))
	}
	if v := *result["EMISSARY_BASE"]; v != "docker.io/datawire/aes:3.12.7" {
		t.Fatalf("expected literal value preserved, got %q", v)
	}
	if v := *result["BUILD_VERSION"]; v != "1.0.0" {
		t.Fatalf("expected BUILD_VERSION %q, got %q", "1.0.0", v)
	}
	if v := *result["SRC_DIR"]; v != "/home/dev/src" {
		t.Fatalf("expected SRC_DIR %q, got %q", "/home/dev/src", v)
	}
}

