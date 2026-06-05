package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func boolPtr(b bool) *bool { return &b }

func TestMergeFeatures_OverrideReplacesBase(t *testing.T) {
	base := FeaturesConfig{
		LB: &LBConfig{Enabled: true},
		Gateway:      &GatewayConfig{Enabled: true, Channel: GatewayChannelStandard},
		DNS:          &DNSConfig{Enabled: true, Domain: "ctx.local", Port: 15353},
	}
	override := FeaturesConfig{
		LB: &LBConfig{Enabled: false},
	}
	result := MergeFeatures(base, override)

	if result.LB == nil || result.LB.Enabled {
		t.Fatal("expected LB to be overridden to disabled")
	}
	if result.Gateway == nil || !result.Gateway.Enabled {
		t.Fatal("expected Gateway to be preserved from base")
	}
	if result.DNS == nil || result.DNS.Domain != "ctx.local" {
		t.Fatal("expected DNS to be preserved from base")
	}
}

func TestMergeFeatures_NilOverrideKeepsBase(t *testing.T) {
	base := FeaturesConfig{
		Gateway: &GatewayConfig{Enabled: true, Channel: GatewayChannelExperimental},
	}
	override := FeaturesConfig{}
	result := MergeFeatures(base, override)

	if result.Gateway == nil || result.Gateway.Channel != GatewayChannelExperimental {
		t.Fatal("expected Gateway to be preserved when override is nil")
	}
	if result.LB != nil {
		t.Fatal("expected LB to remain nil")
	}
}

func TestMergeFeatures_BothNil(t *testing.T) {
	result := MergeFeatures(FeaturesConfig{}, FeaturesConfig{})
	if result.LB != nil || result.Gateway != nil || result.DNS != nil {
		t.Fatal("expected all features to remain nil")
	}
}

func TestResolveFeatureDependencies_GatewayAutoEnablesLB(t *testing.T) {
	f := FeaturesConfig{
		Gateway: &GatewayConfig{Enabled: true},
	}
	warnings, err := ResolveFeatureDependencies(&f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if f.LB == nil || !f.LB.Enabled {
		t.Fatal("expected LB to be auto-enabled")
	}
}

func TestResolveFeatureDependencies_GatewayDoesNotDefaultChannel(t *testing.T) {
	f := FeaturesConfig{
		Gateway: &GatewayConfig{Enabled: true},
	}
	if _, err := ResolveFeatureDependencies(&f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Gateway.Channel != "" {
		t.Fatalf("expected channel to remain empty, got %q", f.Gateway.Channel)
	}
}

func TestResolveFeatureDependencies_GatewayPreservesExplicitChannel(t *testing.T) {
	f := FeaturesConfig{
		Gateway: &GatewayConfig{Enabled: true, Channel: GatewayChannelExperimental},
	}
	if _, err := ResolveFeatureDependencies(&f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Gateway.Channel != GatewayChannelExperimental {
		t.Fatalf("expected channel %q, got %q", GatewayChannelExperimental, f.Gateway.Channel)
	}
}

func TestResolveFeatureDependencies_GatewayWithLBDisabledErrors(t *testing.T) {
	f := FeaturesConfig{
		LB:      &LBConfig{Enabled: false},
		Gateway: &GatewayConfig{Enabled: true},
	}
	_, err := ResolveFeatureDependencies(&f)
	if err == nil {
		t.Fatal("expected error when gateway is enabled but lb is explicitly disabled")
	}
}

func TestResolveFeatureDependencies_DNSWithoutGatewayNoWarning(t *testing.T) {
	f := FeaturesConfig{
		DNS:     &DNSConfig{Enabled: true},
	}
	warnings, err := ResolveFeatureDependencies(&f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %d: %v", len(warnings), warnings)
	}
}

func TestResolveFeatureDependencies_DNSAppliesDefaults(t *testing.T) {
	f := FeaturesConfig{
		DNS:     &DNSConfig{Enabled: true},
	}
	if _, err := ResolveFeatureDependencies(&f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.DNS.Domain != DNSDefaultDomain {
		t.Fatalf("expected domain %q, got %q", DNSDefaultDomain, f.DNS.Domain)
	}
	if f.DNS.Port != DNSDefaultPort {
		t.Fatalf("expected port %d, got %d", DNSDefaultPort, f.DNS.Port)
	}
}

func TestResolveFeatureDependencies_DNSPreservesExplicitValues(t *testing.T) {
	f := FeaturesConfig{
		Gateway:  &GatewayConfig{Enabled: true},
		DNS:     &DNSConfig{Enabled: true, Domain: "custom.dev", Port: 9053},
	}
	warnings, err := ResolveFeatureDependencies(&f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if f.DNS.Domain != "custom.dev" {
		t.Fatalf("expected domain %q, got %q", "custom.dev", f.DNS.Domain)
	}
	if f.DNS.Port != 9053 {
		t.Fatalf("expected port %d, got %d", 9053, f.DNS.Port)
	}
}

func TestResolveFeatureDependencies_AllNilNoOp(t *testing.T) {
	f := FeaturesConfig{}
	warnings, err := ResolveFeatureDependencies(&f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
}

func TestMergeWithContext_UserPortsReplaceContext(t *testing.T) {
	k := KindConfig{
		Name: KindDefaultName,
		Nodes: []KindNode{{
			Role: "control-plane",
			ExtraPortMappings: []PortMapping{
				{ContainerPort: 9090, HostPort: 9090},
			},
		}},
	}
	ctx := &KindConfig{
		Nodes: []KindNode{{
			Role: "control-plane",
			ExtraPortMappings: []PortMapping{
				{ContainerPort: 80, HostPort: 80},
				{ContainerPort: 443, HostPort: 443},
			},
		}},
	}
	k.MergeWithContext(ctx)

	if len(k.Nodes[0].ExtraPortMappings) != 3 {
		t.Fatalf("expected 3 port mappings, got %d", len(k.Nodes[0].ExtraPortMappings))
	}
	portMap := make(map[int32]struct{})
	for _, p := range k.Nodes[0].ExtraPortMappings {
		portMap[p.ContainerPort] = struct{}{}
	}
	for _, want := range []int32{80, 443, 9090} {
		if _, ok := portMap[want]; !ok {
			t.Fatalf("expected port %d to be present", want)
		}
	}
}

func TestMergeWithContext_NoUserPortsFallsBackToContext(t *testing.T) {
	k := KindConfig{
		Name:  KindDefaultName,
		Nodes: []KindNode{{Role: "control-plane"}},
	}
	ctx := &KindConfig{
		Nodes: []KindNode{{
			Role: "control-plane",
			ExtraPortMappings: []PortMapping{
				{ContainerPort: 80, HostPort: 80},
				{ContainerPort: 443, HostPort: 443},
			},
		}},
	}
	k.MergeWithContext(ctx)

	if len(k.Nodes[0].ExtraPortMappings) != 2 {
		t.Fatalf("expected 2 port mappings, got %d", len(k.Nodes[0].ExtraPortMappings))
	}
	if k.Nodes[0].ExtraPortMappings[0].ContainerPort != 80 {
		t.Fatalf("expected containerPort 80, got %d", k.Nodes[0].ExtraPortMappings[0].ContainerPort)
	}
	if k.Nodes[0].ExtraPortMappings[1].ContainerPort != 443 {
		t.Fatalf("expected containerPort 443, got %d", k.Nodes[0].ExtraPortMappings[1].ContainerPort)
	}
}

func TestMergeWithDefaults_UserPortsReplaceDefaults(t *testing.T) {
	k := KindConfig{
		Name: KindDefaultName,
		Nodes: []KindNode{{
			Role: "control-plane",
			ExtraPortMappings: []PortMapping{
				{ContainerPort: 9090, HostPort: 9090},
			},
		}},
	}
	defaults := &KindConfig{
		Nodes: []KindNode{{
			Role: "control-plane",
			ExtraPortMappings: []PortMapping{
				{ContainerPort: 80, HostPort: 80},
				{ContainerPort: 443, HostPort: 443},
			},
		}},
	}
	k.MergeWithDefaults(defaults)

	if len(k.Nodes[0].ExtraPortMappings) != 3 {
		t.Fatalf("expected 3 port mappings, got %d", len(k.Nodes[0].ExtraPortMappings))
	}
	ports := k.Nodes[0].ExtraPortMappings
	if ports[0].ContainerPort != 80 {
		t.Fatalf("expected containerPort 80, got %d", ports[0].ContainerPort)
	}
	if ports[1].ContainerPort != 443 {
		t.Fatalf("expected containerPort 443, got %d", ports[1].ContainerPort)
	}
	if ports[2].ContainerPort != 9090 {
		t.Fatalf("expected containerPort 9090, got %d", ports[2].ContainerPort)
	}
}

func TestMergeWithDefaults_NoUserPortsFallsBackToDefaults(t *testing.T) {
	k := KindConfig{
		Name:  KindDefaultName,
		Nodes: []KindNode{{Role: "control-plane"}},
	}
	defaults := &KindConfig{
		Nodes: []KindNode{{
			Role: "control-plane",
			ExtraPortMappings: []PortMapping{
				{ContainerPort: 80, HostPort: 80},
				{ContainerPort: 443, HostPort: 443},
			},
		}},
	}
	k.MergeWithDefaults(defaults)

	if len(k.Nodes[0].ExtraPortMappings) != 2 {
		t.Fatalf("expected 2 port mappings, got %d", len(k.Nodes[0].ExtraPortMappings))
	}
	if k.Nodes[0].ExtraPortMappings[0].ContainerPort != 80 {
		t.Fatalf("expected containerPort 80, got %d", k.Nodes[0].ExtraPortMappings[0].ContainerPort)
	}
	if k.Nodes[0].ExtraPortMappings[1].ContainerPort != 443 {
		t.Fatalf("expected containerPort 443, got %d", k.Nodes[0].ExtraPortMappings[1].ContainerPort)
	}
}

func TestMergeImages_BothEmpty(t *testing.T) {
	result := MergeImages(ImagesConfig{}, ImagesConfig{})
	if result.Preload != nil {
		t.Fatal("expected Preload to be nil")
	}
	if result.Mirrors != nil {
		t.Fatal("expected Mirrors to be nil")
	}
}

func TestMergeImages_BasePreloadPreserved(t *testing.T) {
	base := ImagesConfig{
		Preload: &PreloadConfig{Refs: []string{"img-a", "img-b"}},
	}
	result := MergeImages(base, ImagesConfig{})
	if result.Preload == nil {
		t.Fatal("expected Preload preserved from base")
	}
	if len(result.Preload.Refs) != 2 || result.Preload.Refs[0] != "img-a" || result.Preload.Refs[1] != "img-b" {
		t.Fatalf("expected base refs preserved, got %v", result.Preload.Refs)
	}
}

func TestMergeImages_OverridePreloadUnionsWithBase(t *testing.T) {
	base := ImagesConfig{
		Preload: &PreloadConfig{Refs: []string{"img-a", "img-b"}},
	}
	override := ImagesConfig{
		Preload: &PreloadConfig{Refs: []string{"img-c"}},
	}
	result := MergeImages(base, override)
	if len(result.Preload.Refs) != 3 {
		t.Fatalf("expected 3 refs from union, got %v", result.Preload.Refs)
	}
	expected := map[string]bool{"img-a": true, "img-b": true, "img-c": true}
	for _, r := range result.Preload.Refs {
		if !expected[r] {
			t.Fatalf("unexpected ref %q in union result", r)
		}
	}
}

func TestMergeImages_EmptyRefsOverridePreservesBase(t *testing.T) {
	base := ImagesConfig{
		Preload: &PreloadConfig{Refs: []string{"img-a", "img-b"}},
	}
	override := ImagesConfig{
		Preload: &PreloadConfig{Refs: []string{}},
	}
	result := MergeImages(base, override)
	if result.Preload == nil {
		t.Fatal("expected non-nil Preload")
	}
	if len(result.Preload.Refs) != 2 {
		t.Fatalf("expected base refs preserved when override has empty refs, got %v", result.Preload.Refs)
	}
}

func TestMergeImages_PreloadFromEitherSide(t *testing.T) {
	base := ImagesConfig{Preload: &PreloadConfig{Refs: []string{"img-a"}}}
	result := MergeImages(base, ImagesConfig{})
	if result.Preload == nil {
		t.Fatal("expected Preload preserved from base")
	}

	result = MergeImages(ImagesConfig{}, ImagesConfig{Preload: &PreloadConfig{Refs: []string{"img-b"}}})
	if result.Preload == nil || result.Preload.Refs[0] != "img-b" {
		t.Fatal("expected Preload from override")
	}
}

func TestMergeImages_MirrorsOverrideWins(t *testing.T) {
	base := ImagesConfig{
		Mirrors: &MirrorsConfig{Upstreams: []string{"docker.io"}},
	}
	override := ImagesConfig{
		Mirrors: &MirrorsConfig{Upstreams: []string{"ghcr.io"}},
	}
	result := MergeImages(base, override)
	if len(result.Mirrors.Upstreams) != 1 || result.Mirrors.Upstreams[0] != "ghcr.io" {
		t.Fatalf("expected override mirrors to win, got %v", result.Mirrors.Upstreams)
	}
}

func TestMergeImages_NilMirrorsOverrideKeepsBase(t *testing.T) {
	base := ImagesConfig{
		Mirrors: &MirrorsConfig{Upstreams: []string{"docker.io"}},
	}
	result := MergeImages(base, ImagesConfig{})
	if result.Mirrors == nil || result.Mirrors.Upstreams[0] != "docker.io" {
		t.Fatal("expected base mirrors preserved when override is nil")
	}
}

func TestMergeImages_NilBasePreloadOverrideWins(t *testing.T) {
	override := ImagesConfig{
		Preload: &PreloadConfig{Refs: []string{"img-x"}},
	}
	result := MergeImages(ImagesConfig{}, override)
	if result.Preload == nil || len(result.Preload.Refs) != 1 || result.Preload.Refs[0] != "img-x" {
		t.Fatalf("expected override preload to be used, got %v", result.Preload)
	}
}

func TestMergeWithContext_NilContextIsNoOp(t *testing.T) {
	k := KindConfig{
		Name:  KindDefaultName,
		Nodes: []KindNode{{Role: "control-plane", ExtraPortMappings: []PortMapping{{ContainerPort: 8080, HostPort: 8080}}}},
	}
	k.MergeWithContext(nil)

	if len(k.Nodes[0].ExtraPortMappings) != 1 || k.Nodes[0].ExtraPortMappings[0].ContainerPort != 8080 {
		t.Fatal("expected user ports to be untouched when context is nil")
	}
}

func TestMergeWithContext_NoUserNodesIsNoOp(t *testing.T) {
	k := KindConfig{Name: KindDefaultName}
	ctx := &KindConfig{
		Nodes: []KindNode{{
			Role:              "control-plane",
			ExtraPortMappings: []PortMapping{{ContainerPort: 80, HostPort: 80}},
		}},
	}
	k.MergeWithContext(ctx)

	if len(k.Nodes) != 0 {
		t.Fatal("expected nodes to remain empty")
	}
}

func TestMergeWithContext_ContextNodePortsAppliedWhenUserHasNone(t *testing.T) {
	k := KindConfig{
		Name:  KindDefaultName,
		Nodes: []KindNode{{Role: "control-plane"}},
	}
	ctx := &KindConfig{
		Nodes: []KindNode{{
			Role: "control-plane",
			ExtraPortMappings: []PortMapping{
				{ContainerPort: 80, HostPort: 80},
				{ContainerPort: 443, HostPort: 8443, Protocol: "TCP"},
				{ContainerPort: 8080, HostPort: 8080},
			},
		}},
	}
	k.MergeWithContext(ctx)

	if len(k.Nodes[0].ExtraPortMappings) != 3 {
		t.Fatalf("expected 3 port mappings, got %d", len(k.Nodes[0].ExtraPortMappings))
	}
	portMap := make(map[int32]PortMapping)
	for _, p := range k.Nodes[0].ExtraPortMappings {
		portMap[p.ContainerPort] = p
	}
	if p, ok := portMap[443]; !ok || p.HostPort != 8443 {
		t.Fatal("expected port 443 with hostPort 8443")
	}
	if _, ok := portMap[80]; !ok {
		t.Fatal("expected port 80 to be present")
	}
	if _, ok := portMap[8080]; !ok {
		t.Fatal("expected port 8080 to be present")
	}
}

func TestMergeWithContext_UserPortsReplaceContextNodePorts(t *testing.T) {
	k := KindConfig{
		Name: KindDefaultName,
		Nodes: []KindNode{{
			Role:              "control-plane",
			ExtraPortMappings: []PortMapping{{ContainerPort: 9090, HostPort: 9090}},
		}},
	}
	ctx := &KindConfig{
		Nodes: []KindNode{{
			Role: "control-plane",
			ExtraPortMappings: []PortMapping{
				{ContainerPort: 80, HostPort: 80},
				{ContainerPort: 443, HostPort: 443},
			},
		}},
	}
	k.MergeWithContext(ctx)

	if len(k.Nodes[0].ExtraPortMappings) != 3 {
		t.Fatalf("expected 3 port mappings, got %d", len(k.Nodes[0].ExtraPortMappings))
	}
	portMap := make(map[int32]struct{})
	for _, p := range k.Nodes[0].ExtraPortMappings {
		portMap[p.ContainerPort] = struct{}{}
	}
	for _, want := range []int32{80, 443, 9090} {
		if _, ok := portMap[want]; !ok {
			t.Fatalf("expected port %d to be present", want)
		}
	}
}

func TestMergeWithContext_ContextNameOverridesDefault(t *testing.T) {
	k := KindConfig{
		Name:  KindDefaultName,
		Nodes: []KindNode{{Role: "control-plane"}},
	}
	ctx := &KindConfig{Name: "my-cluster"}
	k.MergeWithContext(ctx)

	if k.Name != "my-cluster" {
		t.Fatalf("expected name %q, got %q", "my-cluster", k.Name)
	}
}

func TestMergeWithContext_CustomNameNotOverriddenByContext(t *testing.T) {
	k := KindConfig{
		Name:  "user-cluster",
		Nodes: []KindNode{{Role: "control-plane"}},
	}
	ctx := &KindConfig{Name: "ctx-cluster"}
	k.MergeWithContext(ctx)

	if k.Name != "user-cluster" {
		t.Fatalf("expected name %q preserved, got %q", "user-cluster", k.Name)
	}
}

func TestMergeWithDefaults_NilDefaultsIsNoOp(t *testing.T) {
	k := KindConfig{
		Name:  KindDefaultName,
		Nodes: []KindNode{{Role: "control-plane", ExtraPortMappings: []PortMapping{{ContainerPort: 8080, HostPort: 8080}}}},
	}
	k.MergeWithDefaults(nil)

	if len(k.Nodes[0].ExtraPortMappings) != 1 || k.Nodes[0].ExtraPortMappings[0].ContainerPort != 8080 {
		t.Fatal("expected user ports to be untouched when defaults is nil")
	}
}

func TestMergeWithDefaults_NoUserNodesCopiesDefaults(t *testing.T) {
	k := KindConfig{Name: KindDefaultName}
	defaults := &KindConfig{
		Nodes: []KindNode{{
			Role:              "control-plane",
			ExtraPortMappings: []PortMapping{{ContainerPort: 80, HostPort: 80}},
		}},
	}
	k.MergeWithDefaults(defaults)

	if len(k.Nodes) != 1 {
		t.Fatalf("expected 1 node copied from defaults, got %d", len(k.Nodes))
	}
	if len(k.Nodes[0].ExtraPortMappings) != 1 || k.Nodes[0].ExtraPortMappings[0].ContainerPort != 80 {
		t.Fatal("expected default port mappings to be copied")
	}
}

func TestMergePortMappings_UnionDedup(t *testing.T) {
	base := []PortMapping{
		{ContainerPort: 80, HostPort: 80},
		{ContainerPort: 443, HostPort: 443},
	}
	override := []PortMapping{
		{ContainerPort: 443, HostPort: 8443},
		{ContainerPort: 9090, HostPort: 9090},
	}
	result := MergePortMappings(base, override)

	if len(result) != 3 {
		t.Fatalf("expected 3 port mappings (union, deduped), got %d", len(result))
	}
	portMap := make(map[int32]int32)
	for _, p := range result {
		portMap[p.ContainerPort] = p.HostPort
	}
	if hp := portMap[80]; hp != 80 {
		t.Fatalf("expected base-only port 80 with hostPort 80, got %d", hp)
	}
	if hp := portMap[443]; hp != 8443 {
		t.Fatalf("expected override to win for port 443 with hostPort 8443, got %d", hp)
	}
	if hp := portMap[9090]; hp != 9090 {
		t.Fatalf("expected override-only port 9090 with hostPort 9090, got %d", hp)
	}
}

func TestMergePortMappings_ProtocolAwareKeying(t *testing.T) {
	base := []PortMapping{
		{ContainerPort: 443, HostPort: 443, Protocol: "TCP"},
	}
	override := []PortMapping{
		{ContainerPort: 443, HostPort: 8443, Protocol: "UDP"},
	}
	result := MergePortMappings(base, override)

	if len(result) != 2 {
		t.Fatalf("expected 2 port mappings (same port, different protocol), got %d", len(result))
	}
	type key struct {
		port     int32
		protocol string
	}
	seen := make(map[key]int32)
	for _, p := range result {
		seen[key{p.ContainerPort, p.Protocol}] = p.HostPort
	}
	if hp, ok := seen[key{443, "TCP"}]; !ok || hp != 443 {
		t.Fatal("expected TCP port 443 from base")
	}
	if hp, ok := seen[key{443, "UDP"}]; !ok || hp != 8443 {
		t.Fatal("expected UDP port 443 from override")
	}
}

func TestMergePortMappings_SameProtocolOverrideWins(t *testing.T) {
	base := []PortMapping{
		{ContainerPort: 443, HostPort: 443, Protocol: "TCP"},
	}
	override := []PortMapping{
		{ContainerPort: 443, HostPort: 8443, Protocol: "TCP"},
	}
	result := MergePortMappings(base, override)

	if len(result) != 1 {
		t.Fatalf("expected 1 port mapping (same key, override wins), got %d", len(result))
	}
	if result[0].HostPort != 8443 {
		t.Fatalf("expected override hostPort 8443, got %d", result[0].HostPort)
	}
}

func TestMergePortMappings_EmptyBaseReturnsOverride(t *testing.T) {
	override := []PortMapping{
		{ContainerPort: 80, HostPort: 80},
		{ContainerPort: 443, HostPort: 443},
	}
	result := MergePortMappings(nil, override)

	if len(result) != 2 {
		t.Fatalf("expected 2 port mappings, got %d", len(result))
	}
}

func TestMergePortMappings_EmptyOverrideReturnsBase(t *testing.T) {
	base := []PortMapping{
		{ContainerPort: 80, HostPort: 80},
	}
	result := MergePortMappings(base, nil)

	if len(result) != 1 {
		t.Fatalf("expected 1 port mapping, got %d", len(result))
	}
	if result[0].ContainerPort != 80 {
		t.Fatalf("expected containerPort 80, got %d", result[0].ContainerPort)
	}
}

func TestMergePortMappings_BothEmpty(t *testing.T) {
	result := MergePortMappings(nil, nil)
	if len(result) != 0 {
		t.Fatalf("expected 0 port mappings, got %d", len(result))
	}
}

func TestMergeWithContext_UnionDedupConflictOverrideWins(t *testing.T) {
	k := KindConfig{
		Name: KindDefaultName,
		Nodes: []KindNode{{
			Role: "control-plane",
			ExtraPortMappings: []PortMapping{
				{ContainerPort: 80, HostPort: 8080},
				{ContainerPort: 9090, HostPort: 9090},
			},
		}},
	}
	ctx := &KindConfig{
		Nodes: []KindNode{{
			Role: "control-plane",
			ExtraPortMappings: []PortMapping{
				{ContainerPort: 80, HostPort: 80},
				{ContainerPort: 443, HostPort: 443},
			},
		}},
	}
	k.MergeWithContext(ctx)

	if len(k.Nodes[0].ExtraPortMappings) != 3 {
		t.Fatalf("expected 3 port mappings, got %d", len(k.Nodes[0].ExtraPortMappings))
	}
	portMap := make(map[int32]int32)
	for _, p := range k.Nodes[0].ExtraPortMappings {
		portMap[p.ContainerPort] = p.HostPort
	}
	if hp := portMap[80]; hp != 8080 {
		t.Fatalf("expected user override for port 80 with hostPort 8080, got %d", hp)
	}
	if hp := portMap[443]; hp != 443 {
		t.Fatalf("expected context-only port 443 with hostPort 443, got %d", hp)
	}
	if hp := portMap[9090]; hp != 9090 {
		t.Fatalf("expected user-only port 9090 with hostPort 9090, got %d", hp)
	}
}

func TestMergeWithDefaults_UnionDedupConflictOverrideWins(t *testing.T) {
	k := KindConfig{
		Name: KindDefaultName,
		Nodes: []KindNode{{
			Role: "control-plane",
			ExtraPortMappings: []PortMapping{
				{ContainerPort: 80, HostPort: 8080},
			},
		}},
	}
	defaults := &KindConfig{
		Nodes: []KindNode{{
			Role: "control-plane",
			ExtraPortMappings: []PortMapping{
				{ContainerPort: 80, HostPort: 80},
				{ContainerPort: 443, HostPort: 443},
			},
		}},
	}
	k.MergeWithDefaults(defaults)

	if len(k.Nodes[0].ExtraPortMappings) != 2 {
		t.Fatalf("expected 2 port mappings, got %d", len(k.Nodes[0].ExtraPortMappings))
	}
	portMap := make(map[int32]int32)
	for _, p := range k.Nodes[0].ExtraPortMappings {
		portMap[p.ContainerPort] = p.HostPort
	}
	if hp := portMap[80]; hp != 8080 {
		t.Fatalf("expected user override for port 80 with hostPort 8080, got %d", hp)
	}
	if hp := portMap[443]; hp != 443 {
		t.Fatalf("expected default-only port 443 with hostPort 443, got %d", hp)
	}
}

func TestIsEnabled_NilDefaultsToTrue(t *testing.T) {
	c := Component{Name: "app"}
	if !c.IsEnabled() {
		t.Fatal("expected nil Enabled to mean enabled")
	}
}

func TestIsEnabled_ExplicitTrue(t *testing.T) {
	v := true
	c := Component{Name: "app", Enabled: &v}
	if !c.IsEnabled() {
		t.Fatal("expected explicit true to be enabled")
	}
}

func TestIsEnabled_ExplicitFalse(t *testing.T) {
	v := false
	c := Component{Name: "app", Enabled: &v}
	if c.IsEnabled() {
		t.Fatal("expected explicit false to be disabled")
	}
}

func TestResolveFeatureDependencies_DisabledGatewayNoAutoLB(t *testing.T) {
	f := FeaturesConfig{
		Gateway: &GatewayConfig{Enabled: false},
	}
	warnings, err := ResolveFeatureDependencies(&f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if f.LB != nil {
		t.Fatal("expected LB to remain nil when gateway is disabled")
	}
}

func TestMergeImages_MergeMode_UnionRefs(t *testing.T) {
	base := ImagesConfig{
		Preload: &PreloadConfig{Refs: []string{"img-a", "img-b"}},
	}
	override := ImagesConfig{
		Preload: &PreloadConfig{Refs: []string{"img-b", "img-c"}},
	}
	result := MergeImages(base, override)

	if len(result.Preload.Refs) != 3 {
		t.Fatalf("expected 3 deduplicated refs, got %v", result.Preload.Refs)
	}
	expected := map[string]bool{"img-a": true, "img-b": true, "img-c": true}
	for _, r := range result.Preload.Refs {
		if !expected[r] {
			t.Fatalf("unexpected ref %q", r)
		}
	}
	if result.Preload.Mode != "" {
		t.Fatalf("expected mode to be empty after merge, got %q", result.Preload.Mode)
	}
}

func TestMergeImages_ReplaceMode_OverrideWins(t *testing.T) {
	base := ImagesConfig{
		Preload: &PreloadConfig{Refs: []string{"img-a", "img-b"}},
	}
	override := ImagesConfig{
		Preload: &PreloadConfig{
			Mode: PreloadModeReplace,
			Refs: []string{"img-x"},
		},
	}
	result := MergeImages(base, override)

	if len(result.Preload.Refs) != 1 || result.Preload.Refs[0] != "img-x" {
		t.Fatalf("expected only override refs [img-x], got %v", result.Preload.Refs)
	}
	if result.Preload.Mode != "" {
		t.Fatalf("expected mode to be cleared after replace merge, got %q", result.Preload.Mode)
	}
}

func TestMergeImages_SkipAccumulates(t *testing.T) {
	layer0 := ImagesConfig{
		Preload: &PreloadConfig{
			Refs: []string{"img-a", "img-b", "img-c"},
			Skip: []string{"img-a"},
		},
	}
	layer1 := ImagesConfig{
		Preload: &PreloadConfig{
			Skip: []string{"img-b"},
		},
	}
	result := MergeImages(layer0, layer1)

	if len(result.Preload.Skip) != 2 {
		t.Fatalf("expected 2 skip entries, got %v", result.Preload.Skip)
	}
	skipSet := make(map[string]bool)
	for _, s := range result.Preload.Skip {
		skipSet[s] = true
	}
	if !skipSet["img-a"] || !skipSet["img-b"] {
		t.Fatalf("expected skip to contain img-a and img-b, got %v", result.Preload.Skip)
	}
}

func TestEffectiveRefs_SubtractsSkip(t *testing.T) {
	p := &PreloadConfig{
		Refs: []string{"img-a", "img-b", "img-c"},
		Skip: []string{"img-b"},
	}
	effective := p.EffectiveRefs()

	if len(effective) != 2 {
		t.Fatalf("expected 2 effective refs, got %v", effective)
	}
	for _, r := range effective {
		if r == "img-b" {
			t.Fatal("expected img-b to be excluded by skip")
		}
	}
}

func TestEffectiveRefs_NoSkip(t *testing.T) {
	p := &PreloadConfig{
		Refs: []string{"img-a", "img-b"},
	}
	effective := p.EffectiveRefs()

	if len(effective) != 2 {
		t.Fatalf("expected 2 effective refs, got %v", effective)
	}
	if effective[0] != "img-a" || effective[1] != "img-b" {
		t.Fatalf("expected refs unchanged, got %v", effective)
	}
}

func TestMergeImages_ReplaceDoesNotPropagate(t *testing.T) {
	base := ImagesConfig{
		Preload: &PreloadConfig{Refs: []string{"img-a", "img-b"}},
	}
	replaceLayer := ImagesConfig{
		Preload: &PreloadConfig{
			Mode: PreloadModeReplace,
			Refs: []string{"img-x"},
		},
	}
	afterReplace := MergeImages(base, replaceLayer)

	if afterReplace.Preload.Mode != "" {
		t.Fatalf("expected mode cleared after replace, got %q", afterReplace.Preload.Mode)
	}

	nextLayer := ImagesConfig{
		Preload: &PreloadConfig{Refs: []string{"img-y"}},
	}
	final := MergeImages(afterReplace, nextLayer)

	if len(final.Preload.Refs) != 2 {
		t.Fatalf("expected 2 refs from union, got %v", final.Preload.Refs)
	}
	expected := map[string]bool{"img-x": true, "img-y": true}
	for _, r := range final.Preload.Refs {
		if !expected[r] {
			t.Fatalf("unexpected ref %q in final result", r)
		}
	}
}

func TestMerge_BuildsOverrideWins(t *testing.T) {
	base := Config{
		Builds: []Build{
			{Name: "gw", Image: "graviteeio/apim-gateway:latest"},
		},
	}
	override := Config{
		Builds: []Build{
			{Name: "ui", Image: "graviteeio/apim-console-ui:latest"},
		},
	}
	Merge(&base, &override)

	if len(base.Builds) != 1 {
		t.Fatalf("expected 1 build (last-writer-wins), got %d", len(base.Builds))
	}
	if base.Builds[0].Name != "ui" {
		t.Fatalf("expected build name %q, got %q", "ui", base.Builds[0].Name)
	}
}

func TestMerge_BuildsBasePreservedWhenOverrideEmpty(t *testing.T) {
	base := Config{
		Builds: []Build{
			{Name: "gw", Image: "graviteeio/apim-gateway:latest"},
		},
	}
	override := Config{}
	Merge(&base, &override)

	if len(base.Builds) != 1 || base.Builds[0].Name != "gw" {
		t.Fatalf("expected base builds preserved, got %v", base.Builds)
	}
}

func TestBuildImageRefs(t *testing.T) {
	builds := []Build{
		{Name: "gw", Image: "graviteeio/apim-gateway:latest"},
		{Name: "ui", Image: "graviteeio/apim-console-ui:latest"},
	}
	refs := BuildImageRefs(builds)
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	if refs[0] != "graviteeio/apim-gateway:latest" || refs[1] != "graviteeio/apim-console-ui:latest" {
		t.Fatalf("unexpected refs: %v", refs)
	}
}

func TestBuildImageRefs_Empty(t *testing.T) {
	refs := BuildImageRefs(nil)
	if refs != nil {
		t.Fatalf("expected nil for empty builds, got %v", refs)
	}
}

func TestBuildImageRefs_SkipsEmptyImage(t *testing.T) {
	builds := []Build{
		{Name: "gw", Image: "graviteeio/apim-gateway:latest"},
		{Name: "empty"},
	}
	refs := BuildImageRefs(builds)
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref (empty image skipped), got %d", len(refs))
	}
	if refs[0] != "graviteeio/apim-gateway:latest" {
		t.Fatalf("unexpected ref: %s", refs[0])
	}
}

func TestLoad_TemplatesVarsBeforeParsing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gck.yaml")
	if err := os.WriteFile(path, []byte(`vars:
  imageTag: "latest"

kind:
  name: test-cluster

components:
  - name: app
    helm:
      chart: repo/app
      values:
        image:
          tag: "{{ .imageTag }}"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Components[0].Helm.Values["image"].(map[string]interface{})["tag"] != "latest" {
		t.Fatalf("expected default imageTag, got %v", cfg.Components[0].Helm.Values)
	}
	if cfg.Vars.Kind != 0 {
		t.Fatal("expected Vars to be cleared after Load")
	}
}

func TestLoad_SetOverridesVars(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gck.yaml")
	if err := os.WriteFile(path, []byte(`vars:
  imageTag: "latest"

components:
  - name: app
    helm:
      chart: repo/app
      values:
        image:
          tag: "{{ .imageTag }}"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path, map[string]string{"imageTag": "4.12.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tag := cfg.Components[0].Helm.Values["image"].(map[string]interface{})["tag"]
	if tag != "4.12.0" {
		t.Fatalf("expected overridden imageTag 4.12.0, got %v", tag)
	}
}

func TestLoad_NoTemplatePassthrough(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gck.yaml")
	if err := os.WriteFile(path, []byte(`kind:
  name: plain-cluster
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Kind.Name != "plain-cluster" {
		t.Fatalf("expected plain-cluster, got %q", cfg.Kind.Name)
	}
}

func TestLoad_EnvFunction(t *testing.T) {
	t.Setenv("GCK_TEST_LOAD_DIR", "/custom/path")
	dir := t.TempDir()
	path := filepath.Join(dir, "gck.yaml")
	if err := os.WriteFile(path, []byte(`builds:
  - name: app
    image: my-app:latest
    dir: '{{ env "GCK_TEST_LOAD_DIR" }}/src'
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Builds[0].Dir != "/custom/path/src" {
		t.Fatalf("expected /custom/path/src, got %q", cfg.Builds[0].Dir)
	}
}

func TestConfigParsesBuildsFromYAML(t *testing.T) {
	input := `
builds:
  - name: gateway
    image: graviteeio/apim-gateway:latest-debian
    dir: $HOME/src/gravitee
    pre:
      - mvn clean install -DskipTests
      - echo done
    context: target
    dockerfile: docker/Dockerfile
  - name: ui
    image: graviteeio/apim-console-ui:latest
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(cfg.Builds) != 2 {
		t.Fatalf("expected 2 builds, got %d", len(cfg.Builds))
	}

	gw := cfg.Builds[0]
	if gw.Name != "gateway" {
		t.Fatalf("expected name %q, got %q", "gateway", gw.Name)
	}
	if gw.Image != "graviteeio/apim-gateway:latest-debian" {
		t.Fatalf("expected image %q, got %q", "graviteeio/apim-gateway:latest-debian", gw.Image)
	}
	if gw.Dir != "$HOME/src/gravitee" {
		t.Fatalf("expected dir %q, got %q", "$HOME/src/gravitee", gw.Dir)
	}
	if len(gw.Pre) != 2 || gw.Pre[0] != "mvn clean install -DskipTests" {
		t.Fatalf("expected 2 pre commands, got %v", gw.Pre)
	}
	if gw.Context != "target" {
		t.Fatalf("expected context %q, got %q", "target", gw.Context)
	}
	if gw.Dockerfile != "docker/Dockerfile" {
		t.Fatalf("expected dockerfile %q, got %q", "docker/Dockerfile", gw.Dockerfile)
	}
	if gw.BuildArgs != nil {
		t.Fatal("expected buildArgs to be nil when not specified")
	}

	ui := cfg.Builds[1]
	if ui.Name != "ui" || ui.Image != "graviteeio/apim-console-ui:latest" {
		t.Fatalf("unexpected second build: %+v", ui)
	}
	if ui.Dir != "" || ui.Context != "" || ui.Dockerfile != "" || len(ui.Pre) != 0 || ui.BuildArgs != nil {
		t.Fatal("expected optional fields to be zero-valued for minimal build entry")
	}
}

func TestConfigParsesBuildArgs(t *testing.T) {
	input := `
builds:
  - name: aes
    image: docker.io/datawire/aes:3.12.7
    dir: $HOME/src/edge-stack
    buildArgs:
      EMISSARY_BASE: docker.io/datawire/aes:3.12.7
      BUILD_VERSION: "1.0.0"
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(cfg.Builds) != 1 {
		t.Fatalf("expected 1 build, got %d", len(cfg.Builds))
	}
	b := cfg.Builds[0]
	if len(b.BuildArgs) != 2 {
		t.Fatalf("expected 2 build args, got %d", len(b.BuildArgs))
	}
	if v := b.BuildArgs["EMISSARY_BASE"]; v != "docker.io/datawire/aes:3.12.7" {
		t.Fatalf("expected EMISSARY_BASE %q, got %q", "docker.io/datawire/aes:3.12.7", v)
	}
	if v := b.BuildArgs["BUILD_VERSION"]; v != "1.0.0" {
		t.Fatalf("expected BUILD_VERSION %q, got %q", "1.0.0", v)
	}
}

func TestConfigParsesBuildArgs_EnvVarSyntaxPreserved(t *testing.T) {
	input := `
builds:
  - name: aes
    image: docker.io/datawire/aes:3.12.7
    buildArgs:
      EMISSARY_BASE: $HOME/images/aes:3.12.7
      BUILD_COMMIT: ${GIT_SHA}
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	b := cfg.Builds[0]
	if v := b.BuildArgs["EMISSARY_BASE"]; v != "$HOME/images/aes:3.12.7" {
		t.Fatalf("expected env var syntax preserved, got %q", v)
	}
	if v := b.BuildArgs["BUILD_COMMIT"]; v != "${GIT_SHA}" {
		t.Fatalf("expected brace env var syntax preserved, got %q", v)
	}
}

func TestConfigParsesBuildArgs_EmptyMap(t *testing.T) {
	input := `
builds:
  - name: aes
    image: docker.io/datawire/aes:3.12.7
    buildArgs: {}
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	b := cfg.Builds[0]
	if b.BuildArgs == nil {
		t.Fatal("expected non-nil empty map for explicit empty buildArgs")
	}
	if len(b.BuildArgs) != 0 {
		t.Fatalf("expected 0 build args, got %d", len(b.BuildArgs))
	}
}

func TestMerge_BuildArgsPreservedInOverride(t *testing.T) {
	base := Config{
		Builds: []Build{
			{Name: "gw", Image: "graviteeio/apim-gateway:latest"},
		},
	}
	override := Config{
		Builds: []Build{
			{
				Name:  "aes",
				Image: "docker.io/datawire/aes:3.12.7",
				BuildArgs: map[string]string{
					"EMISSARY_BASE": "docker.io/datawire/aes:3.12.7",
				},
			},
		},
	}
	Merge(&base, &override)

	if len(base.Builds) != 1 {
		t.Fatalf("expected 1 build, got %d", len(base.Builds))
	}
	if base.Builds[0].Name != "aes" {
		t.Fatalf("expected build name %q, got %q", "aes", base.Builds[0].Name)
	}
	if len(base.Builds[0].BuildArgs) != 1 {
		t.Fatalf("expected 1 build arg, got %d", len(base.Builds[0].BuildArgs))
	}
	if v := base.Builds[0].BuildArgs["EMISSARY_BASE"]; v != "docker.io/datawire/aes:3.12.7" {
		t.Fatalf("expected EMISSARY_BASE preserved, got %q", v)
	}
}

func TestConfigParsesBuildsWithComponents(t *testing.T) {
	input := `
from:
  - gravitee-io/oss/apim
components:
  - name: apim
    type: helm
builds:
  - name: gateway
    image: graviteeio/apim-gateway:latest
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(cfg.From) != 1 || cfg.From[0] != "gravitee-io/oss/apim" {
		t.Fatalf("unexpected from: %v", cfg.From)
	}
	if len(cfg.Components) != 1 || cfg.Components[0].Name != "apim" {
		t.Fatalf("unexpected components: %v", cfg.Components)
	}
	if len(cfg.Builds) != 1 || cfg.Builds[0].Name != "gateway" {
		t.Fatalf("unexpected builds: %v", cfg.Builds)
	}
}

func TestMerge_BuildsMergedWithOtherFields(t *testing.T) {
	base := Config{
		Registry: "gravitee-io/oss/apim",
		From:     []string{"mongodb/standalone"},
		Builds: []Build{
			{Name: "gw", Image: "graviteeio/apim-gateway:latest"},
		},
	}
	override := Config{
		Builds: []Build{
			{Name: "ui", Image: "graviteeio/apim-console-ui:latest"},
			{Name: "api", Image: "graviteeio/apim-rest-api:latest"},
		},
	}
	Merge(&base, &override)

	if base.Registry != "gravitee-io/oss/apim" {
		t.Fatalf("expected registry preserved, got %q", base.Registry)
	}
	if len(base.From) != 1 || base.From[0] != "mongodb/standalone" {
		t.Fatalf("expected from preserved, got %v", base.From)
	}
	if len(base.Builds) != 2 {
		t.Fatalf("expected 2 builds from override, got %d", len(base.Builds))
	}
	if base.Builds[0].Name != "ui" || base.Builds[1].Name != "api" {
		t.Fatalf("unexpected build names: %v", base.Builds)
	}
}
