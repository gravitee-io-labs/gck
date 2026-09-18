package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/cloud-provider-kind/pkg/config"
)

// The controller used to be declared started on the strength of fork/exec
// returning, so a process that exited immediately produced the same green line
// as a healthy one. Liveness has to be checked against the real thing.
func TestProcessAlive(t *testing.T) {
	live := exec.Command("sleep", "30")
	if err := live.Start(); err != nil {
		t.Fatalf("starting: %v", err)
	}
	defer func() { _ = live.Process.Kill() }()

	if !processAlive(live.Process.Pid) {
		t.Fatal("a running process must report alive")
	}

	dead := exec.Command("true")
	if err := dead.Run(); err != nil {
		t.Fatalf("running: %v", err)
	}
	if processAlive(dead.Process.Pid) {
		t.Fatal("an exited process must not report alive")
	}
}

func TestConfirmCPKAlive_ReportsTheLogWhenTheControllerDies(t *testing.T) {
	home := t.TempDir()
	old := gckHome
	gckHome = home
	t.Cleanup(func() { gckHome = old })

	if err := os.MkdirAll(filepath.Join(home, "logs"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(cpkLogPath(), []byte("line one\nunknown flag: --enable-gateway\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	dead := exec.Command("true")
	if err := dead.Run(); err != nil {
		t.Fatalf("running: %v", err)
	}

	err := confirmCPKAlive(dead.Process.Pid)
	if err == nil {
		t.Fatal("a controller that exited immediately must be an error")
	}
	// The point of the change: the reason travels with the failure, instead of
	// being discarded to /dev/null.
	if !strings.Contains(err.Error(), "unknown flag: --enable-gateway") {
		t.Fatalf("error should carry the controller's own output, got: %v", err)
	}
}

func TestCPKStatus_DistinguishesNeverStartedFromDied(t *testing.T) {
	home := t.TempDir()
	old := gckHome
	gckHome = home
	t.Cleanup(func() { gckHome = old })

	if got := cpkStatus(); !strings.Contains(got, "never started") {
		t.Fatalf("with no PID file, expected 'never started', got %q", got)
	}

	if err := os.MkdirAll(filepath.Join(home, "pids"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dead := exec.Command("true")
	if err := dead.Run(); err != nil {
		t.Fatalf("running: %v", err)
	}
	pidFile := filepath.Join(home, "pids", "cpk.pid")
	if err := os.WriteFile(pidFile, fmt.Appendf(nil, "%d\n", dead.Process.Pid), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// "alive but idle" and "died" call for different fixes, so the message
	// must not collapse them into "not working".
	if got := cpkStatus(); !strings.Contains(got, "GONE") {
		t.Fatalf("with a dead pid, expected 'GONE', got %q", got)
	}
}

func TestLastLogLines_TailsAndSurvivesAMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cpk.log")
	if err := os.WriteFile(path, []byte("a\nb\nc\nd\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if got := lastLogLines(path, 2); got != "c\nd" {
		t.Fatalf("expected the last two lines, got %q", got)
	}
	if got := lastLogLines(filepath.Join(dir, "absent.log"), 5); got != "" {
		t.Fatalf("a missing log must not break the error path, got %q", got)
	}
}

// The Ingress→HTTPRoute translation is the reason a gamma create once died on
// apim.example.com: CPK registered itself as the default IngressClass, adopted
// the two placeholder Ingresses the APIM chart enables by default, and turned
// them into HTTPRoutes that gck then collected as DNS records. Contexts declare
// their own HTTPRoutes, so CPK must never claim an Ingress that did not ask
// for it — with or without the gateway feature.
func TestApplyCPKConfig_NeverTheDefaultIngressClass(t *testing.T) {
	for _, gateway := range []bool{true, false} {
		config.DefaultConfig.IngressDefault = true

		applyCPKConfig(gateway)

		if config.DefaultConfig.IngressDefault {
			t.Fatalf("enableGateway=%v: CPK must not register as the default IngressClass", gateway)
		}
	}
}

func TestApplyCPKConfig_GatewayChannelFollowsTheFeature(t *testing.T) {
	applyCPKConfig(true)
	if config.DefaultConfig.GatewayReleaseChannel != config.Standard {
		t.Fatalf("gateway enabled must select the standard channel, got %q",
			config.DefaultConfig.GatewayReleaseChannel)
	}

	applyCPKConfig(false)
	if config.DefaultConfig.GatewayReleaseChannel != config.Disabled {
		t.Fatalf("gateway disabled must disable the channel, got %q",
			config.DefaultConfig.GatewayReleaseChannel)
	}
}
