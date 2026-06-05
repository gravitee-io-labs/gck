package installer

import (
	"testing"

	"github.com/gravitee-io-labs/gck/internal/config"
)

func TestSplitYAMLDocuments_SingleDocument(t *testing.T) {
	input := []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test")
	docs := splitYAMLDocuments(input)
	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}
}

func TestSplitYAMLDocuments_MultipleDocuments(t *testing.T) {
	input := []byte("apiVersion: v1\nkind: ConfigMap\n---\napiVersion: v1\nkind: Secret")
	docs := splitYAMLDocuments(input)
	if len(docs) != 2 {
		t.Fatalf("expected 2 documents, got %d", len(docs))
	}
}

func TestSplitYAMLDocuments_LeadingSeparator(t *testing.T) {
	input := []byte("---\napiVersion: v1\nkind: ConfigMap")
	docs := splitYAMLDocuments(input)
	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}
}

func TestSplitYAMLDocuments_EmptyInput(t *testing.T) {
	docs := splitYAMLDocuments([]byte(""))
	if len(docs) != 0 {
		t.Fatalf("expected 0 documents, got %d", len(docs))
	}
}

func TestSplitYAMLDocuments_OnlySeparators(t *testing.T) {
	input := []byte("---\n---\n---")
	docs := splitYAMLDocuments(input)
	if len(docs) != 0 {
		t.Fatalf("expected 0 documents, got %d", len(docs))
	}
}

func TestSplitYAMLDocuments_TrailingNewline(t *testing.T) {
	input := []byte("apiVersion: v1\nkind: ConfigMap\n---\napiVersion: v1\nkind: Secret\n")
	docs := splitYAMLDocuments(input)
	if len(docs) != 2 {
		t.Fatalf("expected 2 documents, got %d", len(docs))
	}
}

func TestHasK8sWork_ManifestFiles(t *testing.T) {
	spec := &config.K8sSpec{ManifestFiles: []string{"file.yaml"}}
	if !hasK8sWork(spec) {
		t.Fatal("expected hasK8sWork=true with manifest files")
	}
}

func TestHasK8sWork_InlineManifests(t *testing.T) {
	spec := &config.K8sSpec{
		Manifests: []map[string]interface{}{{"apiVersion": "v1"}},
	}
	if !hasK8sWork(spec) {
		t.Fatal("expected hasK8sWork=true with inline manifests")
	}
}

func TestHasK8sWork_Secrets(t *testing.T) {
	spec := &config.K8sSpec{
		Secrets: []config.LocalResource{{Name: "s"}},
	}
	if !hasK8sWork(spec) {
		t.Fatal("expected hasK8sWork=true with secrets")
	}
}

func TestHasK8sWork_ConfigMaps(t *testing.T) {
	spec := &config.K8sSpec{
		ConfigMaps: []config.LocalResource{{Name: "cm"}},
	}
	if !hasK8sWork(spec) {
		t.Fatal("expected hasK8sWork=true with configMaps")
	}
}

func TestHasK8sWork_Empty(t *testing.T) {
	spec := &config.K8sSpec{}
	if hasK8sWork(spec) {
		t.Fatal("expected hasK8sWork=false with empty spec")
	}
}
