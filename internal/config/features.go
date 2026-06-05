package config

import (
	"fmt"
)

const (
	DNSDefaultDomain = "gck.local"
	DNSDefaultPort   = 15353
)

type LBConfig struct {
	Enabled bool `yaml:"enabled"`
}

type GatewayConfig struct {
	Enabled bool           `yaml:"enabled"`
	Channel GatewayChannel `yaml:"channel,omitempty"`
}

type DNSRecord struct {
	Hostname  string `yaml:"hostname"`
	Service   string `yaml:"service"`
	Namespace string `yaml:"namespace"`
}

type DNSConfig struct {
	Enabled bool        `yaml:"enabled"`
	Domain  string      `yaml:"domain,omitempty"`
	Port    int         `yaml:"port,omitempty"`
	Records []DNSRecord `yaml:"records,omitempty"`
}

// FeaturesConfig groups optional networking features. Pointer semantics: a
// non-nil sub-config means the user explicitly set the feature; nil means
// "use the context default."
type FeaturesConfig struct {
	LB      *LBConfig      `yaml:"lb,omitempty"`
	Gateway *GatewayConfig `yaml:"gateway,omitempty"`
	DNS     *DNSConfig     `yaml:"dns,omitempty"`
}

// MergeFeatures merges context defaults (base) with user overrides. For each
// feature key, a non-nil override replaces the base; otherwise the base stands.
func MergeFeatures(base, override FeaturesConfig) FeaturesConfig {
	result := base
	if override.LB != nil {
		result.LB = override.LB
	}
	if override.Gateway != nil {
		result.Gateway = override.Gateway
	}
	if override.DNS != nil {
		result.DNS = override.DNS
	}
	return result
}

// ResolveFeatureDependencies validates inter-feature constraints and
// auto-enables implied features. It mutates f in place (e.g. enabling
// lb when gateway requires it, filling defaults for DNS).
// It returns warnings for non-fatal issues and an error for conflicts.
func ResolveFeatureDependencies(f *FeaturesConfig) (warnings []string, err error) {
	gwEnabled := f.Gateway != nil && f.Gateway.Enabled
	if gwEnabled {
		if f.LB == nil {
			f.LB = &LBConfig{Enabled: true}
		} else if !f.LB.Enabled {
			return nil, fmt.Errorf("gateway requires lb, but lb is explicitly disabled")
		}
	}

	dnsEnabled := f.DNS != nil && f.DNS.Enabled
	if dnsEnabled {
		if f.DNS.Domain == "" {
			f.DNS.Domain = DNSDefaultDomain
		}
		if f.DNS.Port == 0 {
			f.DNS.Port = DNSDefaultPort
		}
	}

	return warnings, nil
}
