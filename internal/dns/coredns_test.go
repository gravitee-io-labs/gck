package dns

import (
	"strings"
	"testing"
)

func TestBuildCorefileBlock_ExactRecords(t *testing.T) {
	records := map[string]string{
		"am.gck.local":      "10.96.0.5",
		"console.gck.local": "10.96.0.10",
	}

	block := buildCorefileBlock("gck.local", records)

	if !strings.Contains(block, markerBegin) {
		t.Error("missing BEGIN marker")
	}
	if !strings.Contains(block, markerEnd) {
		t.Error("missing END marker")
	}
	if !strings.Contains(block, "gck.local:53 {") {
		t.Error("missing server block header")
	}
	if !strings.Contains(block, "10.96.0.5 am.gck.local") {
		t.Error("missing am.gck.local hosts entry")
	}
	if !strings.Contains(block, "10.96.0.10 console.gck.local") {
		t.Error("missing console.gck.local hosts entry")
	}
	if !strings.Contains(block, "hosts {") {
		t.Error("missing hosts block")
	}
	if !strings.Contains(block, "fallthrough") {
		t.Error("missing fallthrough directive")
	}
	if !strings.Contains(block, "cache 5") {
		t.Error("missing cache directive")
	}
	if strings.Contains(block, "template") {
		t.Error("unexpected template block for exact-only records")
	}
}

func TestBuildCorefileBlock_WildcardRecords(t *testing.T) {
	records := map[string]string{
		"*.kafka.gck.local": "10.96.0.7",
	}

	block := buildCorefileBlock("gck.local", records)

	if !strings.Contains(block, "template IN A {") {
		t.Error("missing template block")
	}
	if !strings.Contains(block, `match "^[^.]+\.kafka\.gck\.local\.$"`) {
		t.Errorf("incorrect or missing match regex, got:\n%s", block)
	}
	if !strings.Contains(block, "10.96.0.7") {
		t.Error("missing wildcard IP in answer")
	}
	if strings.Contains(block, "hosts {") {
		t.Error("unexpected hosts block for wildcard-only records")
	}
}

func TestBuildCorefileBlock_Mixed(t *testing.T) {
	records := map[string]string{
		"*.kafka.gck.local": "10.96.0.7",
		"am.gck.local":      "10.96.0.5",
	}

	block := buildCorefileBlock("gck.local", records)

	if !strings.Contains(block, "template IN A {") {
		t.Error("missing template block")
	}
	if !strings.Contains(block, "hosts {") {
		t.Error("missing hosts block")
	}
	templateIdx := strings.Index(block, "template")
	hostsIdx := strings.Index(block, "hosts {")
	if templateIdx > hostsIdx {
		t.Error("template block should appear before hosts block")
	}
}

func TestBuildCorefileBlock_Empty(t *testing.T) {
	block := buildCorefileBlock("gck.local", map[string]string{})

	if !strings.Contains(block, markerBegin) {
		t.Error("missing BEGIN marker even for empty records")
	}
	if strings.Contains(block, "hosts {") {
		t.Error("should not have hosts block for empty records")
	}
	if strings.Contains(block, "template") {
		t.Error("should not have template block for empty records")
	}
}

func TestBuildCorefileBlock_CustomDomain(t *testing.T) {
	records := map[string]string{
		"app.custom.dev": "10.96.0.1",
	}

	block := buildCorefileBlock("custom.dev", records)

	if !strings.Contains(block, "custom.dev:53 {") {
		t.Error("should use custom domain in server block")
	}
}

func TestStripMarkerBlock_Present(t *testing.T) {
	corefile := `.:53 {
    kubernetes cluster.local
    forward . /etc/resolv.conf
    cache 30
    reload
}
# BEGIN gck.local
gck.local:53 {
    hosts {
        10.96.0.5 am.gck.local
        fallthrough
    }
    cache 5
}
# END gck.local
`

	result := stripMarkerBlock(corefile)

	if strings.Contains(result, "gck.local") {
		t.Error("gck.local block should be removed")
	}
	if !strings.Contains(result, "kubernetes cluster.local") {
		t.Error("original content should be preserved")
	}
	if !strings.Contains(result, "reload") {
		t.Error("original content should be preserved")
	}
}

func TestStripMarkerBlock_NotPresent(t *testing.T) {
	corefile := `.:53 {
    kubernetes cluster.local
    forward . /etc/resolv.conf
}
`

	result := stripMarkerBlock(corefile)

	if result != corefile {
		t.Error("should return original content when no markers present")
	}
}

func TestStripMarkerBlock_PartialMarkers(t *testing.T) {
	corefile := `.:53 {
    kubernetes cluster.local
}
# BEGIN gck.local
gck.local:53 { }
`
	result := stripMarkerBlock(corefile)

	if result != corefile {
		t.Error("should return original content when END marker is missing")
	}
}

func TestStripMarkerBlock_ReplaceIdempotent(t *testing.T) {
	original := `.:53 {
    kubernetes cluster.local
    forward . /etc/resolv.conf
    cache 30
    reload
}
`
	block1 := buildCorefileBlock("gck.local", map[string]string{
		"am.gck.local": "10.96.0.5",
	})
	patched := original + block1

	block2 := buildCorefileBlock("gck.local", map[string]string{
		"am.gck.local":      "10.96.0.5",
		"console.gck.local": "10.96.0.10",
	})
	cleaned := stripMarkerBlock(patched)
	repatched := cleaned + block2

	if strings.Count(repatched, markerBegin) != 1 {
		t.Error("should have exactly one BEGIN marker after re-patch")
	}
	if strings.Count(repatched, markerEnd) != 1 {
		t.Error("should have exactly one END marker after re-patch")
	}
	if !strings.Contains(repatched, "console.gck.local") {
		t.Error("should contain new records after re-patch")
	}
	if !strings.Contains(repatched, "kubernetes cluster.local") {
		t.Error("original content should be preserved after re-patch")
	}
}
