//go:build linux

package dns

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// SetupResolver configures systemd-resolved to route queries for the given
// domain to the local DNS server on the loopback interface. Uses the
// resolvectl IP#port syntax (systemd 244+) to specify a non-standard port.
// This is a runtime configuration that does not survive reboots.
func SetupResolver(domain string, port int) error {
	dnsServer := fmt.Sprintf("127.0.0.1#%d", port)
	routingDomain := "~" + domain

	cmds := fmt.Sprintf("resolvectl dns lo %s && resolvectl domain lo %s", dnsServer, routingDomain)
	cmd := exec.Command("sudo", "-p",
		"\n  gck needs administrator privileges to configure DNS routing.\n  Password: ",
		"sh", "-c", cmds,
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("configuring systemd-resolved: %w", err)
	}
	return nil
}

// TeardownResolver reverts the loopback interface DNS configuration in
// systemd-resolved, removing the domain routing.
func TeardownResolver(_ string) error {
	cmd := exec.Command("sudo", "-p",
		"\n  gck needs administrator privileges to remove DNS routing.\n  Password: ",
		"resolvectl", "revert", "lo",
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("reverting systemd-resolved config: %w", err)
	}
	return nil
}

// ResolverConfigured checks whether the loopback interface has DNS routing
// configured in systemd-resolved for the given domain and port.
func ResolverConfigured(domain string, port int) bool {
	domOut, err := exec.Command("resolvectl", "domain", "lo").Output()
	if err != nil {
		return false
	}
	if !strings.Contains(string(domOut), domain) {
		return false
	}
	dnsOut, err := exec.Command("resolvectl", "dns", "lo").Output()
	if err != nil {
		return false
	}
	expected := fmt.Sprintf("127.0.0.1#%d", port)
	return strings.Contains(string(dnsOut), expected)
}
