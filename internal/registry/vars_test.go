package registry

import (
	"testing"
)

func TestSplitSetOverrides(t *testing.T) {
	raw := map[string]string{
		"imageTag":                    "latest",
		"mysql.standalone.imageTag":   "8.4",
		"gravitee-io.am.helmVer": "4.0",
	}
	s := SplitSetOverrides(raw)
	if len(s.Broadcast) != 1 || s.Broadcast["imageTag"] != "latest" {
		t.Errorf("unexpected broadcast: %v", s.Broadcast)
	}
	if len(s.Scoped) != 2 {
		t.Errorf("expected 2 scoped, got %d: %v", len(s.Scoped), s.Scoped)
	}
	if s.Scoped["mysql.standalone.imageTag"] != "8.4" {
		t.Errorf("unexpected scoped value: %v", s.Scoped)
	}
}

func TestSplitSetOverrides_Empty(t *testing.T) {
	s := SplitSetOverrides(nil)
	if len(s.Broadcast) != 0 || len(s.Scoped) != 0 {
		t.Errorf("expected empty, got broadcast=%d scoped=%d", len(s.Broadcast), len(s.Scoped))
	}
}

func TestResolveScopedOverride_Match(t *testing.T) {
	known := map[string]bool{
		"mysql/standalone":         true,
		"gravitee-io/apim/base": true,
	}

	path, varName := resolveScopedOverride("mysql.standalone.imageTag", known)
	if path != "mysql/standalone" || varName != "imageTag" {
		t.Errorf("expected mysql/standalone + imageTag, got %q + %q", path, varName)
	}

	path, varName = resolveScopedOverride("gravitee-io.apim.base.imageTag", known)
	if path != "gravitee-io/apim/base" || varName != "imageTag" {
		t.Errorf("expected gravitee-io/apim/base + imageTag, got %q + %q", path, varName)
	}
}

func TestResolveScopedOverride_NoMatch(t *testing.T) {
	known := map[string]bool{"mysql/standalone": true}

	path, varName := resolveScopedOverride("unknown.context.imageTag", known)
	if path != "" || varName != "" {
		t.Errorf("expected no match, got %q + %q", path, varName)
	}
}

func TestResolveScopedOverride_LongestPrefixWins(t *testing.T) {
	known := map[string]bool{
		"mysql":            true,
		"mysql/standalone": true,
	}

	path, varName := resolveScopedOverride("mysql.standalone.imageTag", known)
	if path != "mysql/standalone" || varName != "imageTag" {
		t.Errorf("expected longest match mysql/standalone, got %q + %q", path, varName)
	}
}

func TestComputeEffectiveVars(t *testing.T) {
	own := map[string]string{"imageTag": "9", "helmVersion": ""}
	overrides := map[string]string{"imageTag": "8"}
	set := SetOverrides{
		Broadcast: map[string]string{"helmVersion": "4.5"},
		Scoped:    map[string]string{},
	}

	vars := computeEffectiveVars(own, overrides, set)
	if vars["imageTag"] != "8" {
		t.Errorf("expected override imageTag=8, got %q", vars["imageTag"])
	}
	if vars["helmVersion"] != "4.5" {
		t.Errorf("expected broadcast helmVersion=4.5, got %q", vars["helmVersion"])
	}
}

func TestComputeEffectiveVars_PathOverrideBlocksBroadcast(t *testing.T) {
	own := map[string]string{"imageTag": "7", "clusterName": "mongo-standalone"}
	overrides := map[string]string{"imageTag": "7"}
	set := SetOverrides{
		Broadcast: map[string]string{"imageTag": "master-latest"},
		Scoped:    map[string]string{},
	}

	vars := computeEffectiveVars(own, overrides, set)
	if vars["imageTag"] != "7" {
		t.Errorf("path-scoped override should block broadcast; expected imageTag=7, got %q", vars["imageTag"])
	}
	if vars["clusterName"] != "mongo-standalone" {
		t.Errorf("expected clusterName=mongo-standalone, got %q", vars["clusterName"])
	}
}

func TestComputeEffectiveVars_BroadcastOnlyForDeclaredVars(t *testing.T) {
	own := map[string]string{"imageTag": "9"}
	set := SetOverrides{
		Broadcast: map[string]string{"imageTag": "latest", "unknown": "val"},
		Scoped:    map[string]string{},
	}

	vars := computeEffectiveVars(own, nil, set)
	if vars["imageTag"] != "latest" {
		t.Errorf("expected broadcast imageTag=latest, got %q", vars["imageTag"])
	}
	if _, exists := vars["unknown"]; exists {
		t.Errorf("broadcast should not inject undeclared var 'unknown'")
	}
}
