//go:build darwin

package privilege

import (
	"fmt"
	"os"
	"os/exec"
)

// Elevate runs the given shell command with administrator privileges using
// sudo. If the user has Touch ID configured for sudo (pam_tid.so), the
// system will prompt via Touch ID; otherwise it falls back to a password
// prompt in the terminal.
func Elevate(command string) error {
	cmd := exec.Command("sudo", "-p",
		"\n  gck needs administrator privileges to configure load balancer network routing.\n  Password: ",
		"sh", "-c", command,
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("privilege escalation failed: %w", err)
	}
	return nil
}
