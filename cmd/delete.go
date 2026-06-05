package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gravitee-io-labs/gck/internal/cache"
	"github.com/gravitee-io-labs/gck/internal/cloudprovider"
	"github.com/gravitee-io-labs/gck/internal/config"
	"github.com/gravitee-io-labs/gck/internal/dns"
	"github.com/gravitee-io-labs/gck/internal/kind"
	"github.com/gravitee-io-labs/gck/internal/logger"
	"github.com/gravitee-io-labs/gck/internal/state"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
	kindcluster "sigs.k8s.io/kind/pkg/cluster"
)

var downCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Delete a cluster and clean up associated resources",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDown,
}

func init() {
	rootCmd.AddCommand(downCmd)
}

// deleteTarget holds the resolved information needed to tear down a cluster.
type deleteTarget struct {
	Name   string
	Images config.ImagesConfig
	Notes  string
}

func runDown(_ *cobra.Command, args []string) error {
	if err := requireDocker(); err != nil {
		return err
	}

	start := time.Now()

	stateDir := filepath.Join(gckHome, "clusters")

	var clusterName string
	if len(args) > 0 {
		clusterName = args[0]
	}

	target, err := resolveDeleteTarget(stateDir, clusterName)
	if err != nil {
		return err
	}

	logDir := filepath.Join(gckHome, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("creating log directory %s: %w", logDir, err)
	}
	logPath := filepath.Join(logDir, "delete.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}
	defer logFile.Close()
	klog.LogToStderr(false)
	klog.SetOutput(logFile)
	logger.SetLogFile(logPath)

	dnsDir := filepath.Join(gckHome, "dns")
	if err := dns.RemoveRecordFile(dnsDir, target.Name); err != nil {
		logger.Warn("failed to remove DNS record file: %v", err)
	} else {
		logger.Success("Removed DNS records for cluster %q", target.Name)
	}

	if err := logger.WithSpinner("Cleaning up load balancer containers", func() error {
		return cloudprovider.CleanupLBs(target.Name)
	}); err != nil {
		return err
	}

	if err := logger.WithSpinner(
		fmt.Sprintf("Deleting cluster %q", target.Name),
		func() error {
			return kind.Delete(target.Name)
		},
	); err != nil {
		return err
	}

	if target.Images.Mirrors != nil {
		ctx := context.Background()
		if err := logger.WithSpinner("Stopping image mirror proxies", func() error {
			return cache.StopProxies(ctx, target.Images.Mirrors)
		}); err != nil {
			return err
		}
	}

	if target.Images.Preload != nil && len(target.Images.Preload.Refs) > 0 {
		ctx := context.Background()
		if err := logger.WithSpinner("Stopping preload registry", func() error {
			return cache.StopPreloadRegistry(ctx)
		}); err != nil {
			return err
		}
	}

	stopCPKIfNoKindClusters()
	stopDNSIfNoRecords()

	if err := state.Remove(stateDir, target.Name); err != nil {
		logger.Warn("failed to remove state file: %v", err)
	}

	fmt.Println()
	color.Blue("  Total: %s", time.Since(start).Round(time.Millisecond))

	if target.Notes != "" {
		printNotes(target.Notes, cfg, nil)
	}

	return nil
}

// resolveDeleteTarget determines which cluster to delete and returns the
// teardown information. Resolution order:
//  1. Explicit name argument: load state file, fall back to best-effort.
//  2. State files in ~/.gck/clusters/: auto-select if one, prompt if many.
//  3. Config fallback: use cfg.Kind.Name from gck.yaml.
func resolveDeleteTarget(stateDir, name string) (*deleteTarget, error) {
	if name != "" {
		return resolveByName(stateDir, name)
	}

	names, err := state.List(stateDir)
	if err != nil {
		return nil, fmt.Errorf("listing cluster states: %w", err)
	}

	switch len(names) {
	case 0:
		return resolveFromConfig()
	case 1:
		return loadStateTarget(stateDir, names[0])
	default:
		return promptClusterSelection(stateDir, names)
	}
}

// resolveByName loads the state file for the given name. If no state file
// exists, it falls back to a best-effort target that can still delete the
// Kind cluster, LB containers, and DNS records.
func resolveByName(stateDir, name string) (*deleteTarget, error) {
	cs, err := state.Load(stateDir, name)
	if err == nil {
		return targetFromState(cs), nil
	}

	exists, kerr := kind.Exists(name)
	if kerr != nil {
		return nil, fmt.Errorf("checking cluster %q: %w", name, kerr)
	}
	if !exists {
		return nil, fmt.Errorf("cluster %q not found (no state file and no running Kind cluster)", name)
	}

	logger.Warn("No state file for cluster %q; performing best-effort cleanup (mirrors and preload will not be stopped)", name)
	return &deleteTarget{Name: name}, nil
}

func loadStateTarget(stateDir, name string) (*deleteTarget, error) {
	cs, err := state.Load(stateDir, name)
	if err != nil {
		return nil, err
	}
	return targetFromState(cs), nil
}

func targetFromState(cs *state.ClusterState) *deleteTarget {
	return &deleteTarget{
		Name:   cs.Name,
		Images: cs.Images,
		Notes:  cs.Notes.Delete,
	}
}

// resolveFromConfig falls back to reading the cluster name from gck.yaml
// config. This handles clusters created before the state file feature.
func resolveFromConfig() (*deleteTarget, error) {
	if cfg.Kind.Name == "" {
		return nil, fmt.Errorf("no cluster state files found and no cluster name in config; pass the cluster name as argument")
	}
	logger.Warn("No state file found; falling back to config (cluster %q)", cfg.Kind.Name)
	return &deleteTarget{
		Name:   cfg.Kind.Name,
		Images: cfg.Images,
	}, nil
}

func promptClusterSelection(stateDir string, names []string) (*deleteTarget, error) {
	fmt.Println("Multiple clusters found. Which one do you want to delete?")
	fmt.Println()
	for i, name := range names {
		cs, err := state.Load(stateDir, name)
		if err != nil {
			fmt.Printf("  %d) %s\n", i+1, name)
			continue
		}
		fmt.Printf("  %d) %s  (created %s)\n", i+1, name, cs.CreatedAt.Format("2006-01-02 15:04"))
	}
	fmt.Println()
	fmt.Printf("Enter number [1-%d]: ", len(names))

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return nil, fmt.Errorf("no input received")
	}
	choice, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
	if err != nil || choice < 1 || choice > len(names) {
		return nil, fmt.Errorf("invalid selection %q; expected a number between 1 and %d", scanner.Text(), len(names))
	}

	return loadStateTarget(stateDir, names[choice-1])
}

func cpkProcessRunning() bool {
	out, err := exec.Command("pgrep", "-f", "gck.*cpk serve").Output()
	return err == nil && len(strings.TrimSpace(string(out))) > 0
}

func stopDNSIfNoRecords() {
	dnsDir := filepath.Join(gckHome, "dns")
	entries, err := os.ReadDir(dnsDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			return
		}
	}

	pidPath := filepath.Join(gckHome, "pids", "dns.pid")
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	if proc.Signal(syscall.Signal(0)) == nil {
		_ = proc.Signal(syscall.SIGTERM)
		logger.Success("Stopped DNS server (pid %d)", pid)
	}
	_ = os.Remove(pidPath)
}

func stopCPKIfNoKindClusters() {
	provider := kindcluster.NewProvider()
	clusters, err := provider.List()
	if err != nil || len(clusters) > 0 {
		return
	}

	pidPath := filepath.Join(gckHome, "pids", "cpk.pid")

	if cloudprovider.NeedsTunnels() {
		if cpkProcessRunning() {
			cmd := exec.Command("sudo", "-p",
				"\n  gck needs administrator privileges to stop the cloud provider controller.\n  Password: ",
				"pkill", "-f", "gck.*cpk serve")
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			err := cmd.Run()
			fmt.Println()
			if err == nil {
				logger.Success("Stopped cloud provider controller")
			}
		}
	} else {
		data, err := os.ReadFile(pidPath)
		if err != nil {
			return
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			return
		}
		proc, err := os.FindProcess(pid)
		if err != nil {
			return
		}
		if proc.Signal(syscall.Signal(0)) == nil {
			_ = proc.Signal(syscall.SIGTERM)
			logger.Success("Stopped cloud provider controller (pid %d)", pid)
		}
	}

	_ = os.Remove(pidPath)
}
