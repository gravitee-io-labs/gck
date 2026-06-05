//go:build darwin

package dns

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const resolverDir = "/etc/resolver"

// SetupResolver creates a per-domain resolver file in /etc/resolver/ that
// routes DNS queries for the given domain to 127.0.0.1 on the specified port.
// This survives reboots and only affects the target domain -- no other DNS
// resolution is impacted.
func SetupResolver(domain string, port int) error {
	content := fmt.Sprintf("nameserver 127.0.0.1\nport %d\n", port)
	path := filepath.Join(resolverDir, domain)

	script := fmt.Sprintf("mkdir -p %s && printf '%%s' '%s' > %s", resolverDir, content, path)
	cmd := exec.Command("sudo", "-p",
		"\n  gck needs administrator privileges to configure DNS routing.\n  Password: ",
		"sh", "-c", script,
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("creating resolver file %s: %w", path, err)
	}
	return nil
}

// TeardownResolver removes the per-domain resolver file.
func TeardownResolver(domain string) error {
	path := filepath.Join(resolverDir, domain)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	cmd := exec.Command("sudo", "-p",
		"\n  gck needs administrator privileges to remove DNS routing.\n  Password: ",
		"rm", path,
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("removing resolver file %s: %w", path, err)
	}
	return nil
}

// ResolverConfigured returns true if the resolver file for the domain exists
// and contains the expected port.
func ResolverConfigured(domain string, port int) bool {
	data, err := os.ReadFile(filepath.Join(resolverDir, domain))
	if err != nil {
		return false
	}
	expected := fmt.Sprintf("port %d", port)
	return strings.Contains(string(data), expected)
}
