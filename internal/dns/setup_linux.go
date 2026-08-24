//go:build linux

package dns

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// dropIn is where the routing configuration lives. A drop-in rather than an
// edit of resolved.conf so teardown is a file removal and an existing
// hand-rolled configuration is never rewritten.
const dropIn = "/etc/systemd/resolved.conf.d/gck.conf"

// resolvedConfig renders the drop-in contents.
//
// The port is separated with ":" and NOT with "#": per resolved.conf(5) an
// address takes "a port number separated with ':' ... and a Server Name
// Indication (SNI) separated with '#'", so "127.0.0.1#15353" silently asks for
// SNI "15353" on port 53 and every lookup goes to the wrong server.
//
// The "~" prefix makes gck.local a route-only domain: queries for it prefer
// this server, while every other name keeps resolving through whatever the
// links already provide. Without it, a global DNS= entry would compete with
// the system's own resolvers.
func resolvedConfig(domain string, port int) string {
	return fmt.Sprintf(`# Managed by gck. Removed by "gck teardown dns".
[Resolve]
DNS=127.0.0.1:%d
Domains=~%s
`, port, domain)
}

// SetupResolver routes queries for the given domain to the local DNS server.
//
// This configures systemd-resolved GLOBALLY rather than per-link. Per-link is
// the obvious approach and cannot work: the server listens on loopback, and
// `resolvectl dns lo ...` fails with "Link lo is loopback device" because
// systemd-resolved refuses per-link configuration on the loopback device.
// Binding it to a real link instead would make resolved send the query out
// that link's scope, where 127.0.0.1 is not the local machine.
func SetupResolver(domain string, port int) error {
	script := fmt.Sprintf(
		"mkdir -p %s && cat > %s <<'GCKEOF'\n%sGCKEOF\nsystemctl restart systemd-resolved",
		"/etc/systemd/resolved.conf.d", dropIn, resolvedConfig(domain, port),
	)
	cmd := exec.Command("sudo", "-p",
		"\n  gck needs administrator privileges to configure DNS routing.\n  Password: ",
		"sh", "-c", script,
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("configuring systemd-resolved: %w", err)
	}
	return nil
}

// TeardownResolver removes the drop-in and reloads systemd-resolved.
func TeardownResolver(_ string) error {
	script := fmt.Sprintf("rm -f %s && systemctl restart systemd-resolved", dropIn)
	cmd := exec.Command("sudo", "-p",
		"\n  gck needs administrator privileges to remove DNS routing.\n  Password: ",
		"sh", "-c", script,
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("reverting systemd-resolved config: %w", err)
	}
	return nil
}

// ResolverConfigured reports whether resolved is currently routing the domain
// to the local server.
//
// It asks resolved what it is doing rather than reading the drop-in back: the
// file can be present while the daemon has not picked it up yet, and a stale
// "configured" answer there would send `gck create` on to fail much later.
func ResolverConfigured(domain string, port int) bool {
	out, err := exec.Command("resolvectl", "status").Output()
	if err != nil {
		return false
	}
	status := string(out)
	return strings.Contains(status, fmt.Sprintf("127.0.0.1:%d", port)) &&
		strings.Contains(status, "~"+domain)
}
