//go:build linux

package dns

import "testing"

// The port separator is the whole bug this file guards. resolved.conf(5) gives
// "#" to Server Name Indication and ":" to the port, so a "#" here resolves
// every name against 127.0.0.1:53 while looking correct at a glance.
func TestResolvedConfigSeparatesPortWithColon(t *testing.T) {
	got := resolvedConfig("gck.local", 15353)

	if want := "DNS=127.0.0.1:15353"; !contains(got, want) {
		t.Errorf("config does not carry %q:\n%s", want, got)
	}
	if contains(got, "127.0.0.1#15353") {
		t.Errorf("port separated with '#', which resolved reads as SNI:\n%s", got)
	}
	// Route-only, or the entry competes with the system's own resolvers for
	// every name rather than just this domain.
	if want := "Domains=~gck.local"; !contains(got, want) {
		t.Errorf("config does not carry %q:\n%s", want, got)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
