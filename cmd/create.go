package cmd

import (
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
	"github.com/gravitee-io-labs/gck/internal/config"
	"github.com/gravitee-io-labs/gck/internal/installer"
	"github.com/gravitee-io-labs/gck/internal/cloudprovider"
	"github.com/gravitee-io-labs/gck/internal/dns"
	"github.com/gravitee-io-labs/gck/internal/kind"
	"github.com/gravitee-io-labs/gck/internal/logger"
	"github.com/gravitee-io-labs/gck/internal/notes"
	"github.com/gravitee-io-labs/gck/internal/registry"
	"github.com/gravitee-io-labs/gck/internal/state"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
)

var createSkipPreload bool

var upCmd = &cobra.Command{
	Use:   "create",
	Short: "Create the cluster and install the context",
	Long: `Create the cluster and install the context defined in gck.yaml.

Contexts may define optional flags that customize the deployment.
Flags are passed directly on the command line:

  gck create --disable-es --disable-portal

Run "gck info" to see available flags for your context.`,
	FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
	RunE:               runUp,
}

func init() {
	upCmd.Flags().BoolVar(&createSkipPreload, "skip-preload", false, "skip image preloading even when images.preload is configured")
	rootCmd.AddCommand(upCmd)
}

func runUp(cmd *cobra.Command, _ []string) error {
	if err := requireDocker(); err != nil {
		return err
	}

	resolved, err := resolveContextConfig()
	if err != nil {
		return err
	}

	activeFlags, err := applyContextFlags(cmd, resolved)
	if err != nil {
		return err
	}

	if resolved != nil {
		cfg.Images = config.MergeImages(cfg.Images, resolved.Images)
	}
	cfg.Kind.MergeWithContext(&resolved.Kind)

	return createCluster(resolved, activeFlags)
}

// createCluster runs the full cluster creation flow: preload, mirrors,
// Kind cluster, component install, DNS, and state saving. It is called
// by both `gck create` and `gck build --create`.
func createCluster(resolved *config.ResolvedContext, activeFlags []string) error {
	start := time.Now()

	if err := os.MkdirAll(gckHome, 0o755); err != nil {
		return fmt.Errorf("failed to create home directory %s: %w", gckHome, err)
	}

	featWarnings, err := config.ResolveFeatureDependencies(&cfg.Features)
	if err != nil {
		return fmt.Errorf("validating feature dependencies: %w", err)
	}
	fmt.Println()
	for _, w := range featWarnings {
		logger.Warn("%s", w)
	}

	exists, err := kind.Exists(cfg.Kind.Name)
	if err != nil {
		return fmt.Errorf("checking cluster existence: %w", err)
	}
	if exists {
		return fmt.Errorf("cluster %q already exists — delete it first with: gck delete %s", cfg.Kind.Name, cfg.Kind.Name)
	}

	logDir := filepath.Join(gckHome, "logs")
	if len(cfg.From) > 0 {
		logDir = filepath.Join(logDir, strings.Join(cfg.From, "_"))
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("creating log directory %s: %w", logDir, err)
	}
	logFile, err := os.OpenFile(filepath.Join(logDir, "install.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}
	defer logFile.Close()
	klog.LogToStderr(false)
	klog.SetOutput(logFile)

	ctx := context.Background()

	var preloadRefs []string
	if !createSkipPreload {
		preloadRefs = getPreloadRefs(cfg)
	}

	if len(preloadRefs) > 0 {
		if err := logger.WithSpinner("Pulling images for preload", func() error {
			return cache.PullImages(ctx, preloadRefs)
		}); err != nil {
			return err
		}
		if err := logger.WithSpinner("Starting preload registry", func() error {
			return cache.EnsurePreloadRegistry(ctx, gckHome)
		}); err != nil {
			return err
		}
		if err := logger.WithSpinner("Pushing images to preload registry", func() error {
			return cache.PushImages(ctx, preloadRefs)
		}); err != nil {
			return err
		}
	}

	if cfg.Images.Mirrors != nil {
		if err := logger.WithSpinner("Starting image mirror proxies", func() error {
			return cache.EnsureProxies(ctx, cfg.Images.Mirrors, gckHome)
		}); err != nil {
			return err
		}
	}

	var preloadUpstreams []string
	if len(preloadRefs) > 0 {
		preloadUpstreams = cache.PreloadUpstreams(preloadRefs)
	}
	if len(cfg.Builds) > 0 {
		preloadUpstreams = append(preloadUpstreams, cache.PreloadUpstreams(config.BuildImageRefs(cfg.Builds))...)
	}

	if cfg.Images.Mirrors != nil || len(preloadUpstreams) > 0 {
		hostsCfg, err := cache.PrepareContainerdHosts(cfg.Images.Mirrors, preloadUpstreams, gckHome)
		if err != nil {
			return fmt.Errorf("preparing containerd hosts config: %w", err)
		}
		cfg.Kind.ContainerdConfigPatches = append(cfg.Kind.ContainerdConfigPatches, hostsCfg.Patch)
		for i := range cfg.Kind.Nodes {
			cfg.Kind.Nodes[i].ExtraMounts = append(cfg.Kind.Nodes[i].ExtraMounts, hostsCfg.Mounts...)
		}
	}

	kindConfig, err := cfg.Kind.RawYAML()
	if err != nil {
		return fmt.Errorf("serializing kind config: %w", err)
	}
	if err := logger.WithSpinner(
		fmt.Sprintf("Creating cluster %q", cfg.Kind.Name),
		func() error { return kind.Create(cfg.Kind.Name, kindConfig) },
	); err != nil {
		return err
	}
	saveClusterState(cfg, nil, nil)

	if cfg.Images.Mirrors != nil {
		if err := logger.WithSpinner("Connecting image mirrors to Kind network", func() error {
			return cache.ConnectToKindNetwork(ctx, cfg.Kind.Name, cfg.Images.Mirrors)
		}); err != nil {
			return err
		}
	}

	if len(preloadRefs) > 0 {
		if err := logger.WithSpinner("Connecting preload registry to Kind network", func() error {
			return cache.ConnectPreloadToKindNetwork(ctx)
		}); err != nil {
			return err
		}
	}

	if cfg.Registry == "" || len(cfg.From) == 0 {
		return nil
	}

	gatewayEnabled := cfg.Features.Gateway != nil && cfg.Features.Gateway.Enabled
	lbEnabled := cfg.Features.LB != nil && cfg.Features.LB.Enabled

	if lbEnabled {
		if err := ensureCPKController(cfg, gatewayEnabled); err != nil {
			return err
		}
	}

	registry.MergeComponents(resolved, cfg.Components, cfg.Dir)
	resolved.Repos = registry.MergeRepos(resolved.Repos, cfg.Helm.Repos)

	if gatewayEnabled {
		injectGatewayComponents(resolved)
	}

	if err := installComponents(ctx, resolved, nil, installer.InstallOpts{}); err != nil {
		return err
	}

	if cfg.Features.DNS != nil && cfg.Features.DNS.Enabled {
		if err := setupDNSRecords(ctx, cfg); err != nil {
			return err
		}
	}

	saveClusterState(cfg, resolved, activeFlags)

	fmt.Println()
	color.Blue("  Total: %s", time.Since(start).Round(time.Millisecond))

	if resolved != nil {
		printNotes(resolved.Notes.Create, cfg, activeFlags)
	}

	return nil
}

func saveClusterState(cfg *config.Config, resolved *config.ResolvedContext, activeFlags []string) {
	cs := &state.ClusterState{
		Name:      cfg.Kind.Name,
		CreatedAt: time.Now(),
		Registry:  cfg.Registry,
		From:      cfg.From,
		Flags:     activeFlags,
		Set:       setOverrides,
		Features:  cfg.Features,
		Images:    cfg.Images,
	}
	if resolved != nil {
		cs.Notes.Delete = resolved.Notes.Delete
	}
	stateDir := filepath.Join(gckHome, "clusters")
	if err := state.Save(stateDir, cs); err != nil {
		logger.Warn("failed to save cluster state: %v", err)
	}
}

func getPreloadRefs(cfg *config.Config) []string {
	refs := cfg.Images.Preload.EffectiveRefs()
	if len(refs) == 0 || len(cfg.Builds) == 0 {
		return refs
	}
	buildSkip := config.BuildImageRefs(cfg.Builds)
	if len(buildSkip) == 0 {
		return refs
	}
	skip := make(map[string]bool, len(buildSkip))
	for _, s := range buildSkip {
		skip[s] = true
	}
	filtered := make([]string, 0, len(refs))
	for _, r := range refs {
		if !skip[r] {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// injectGatewayComponents prepends the shared gck-gateway Gateway resource to
// the resolved component list so the topo-sort places it before any
// user-defined HTTPRoutes. CPK's gateway controller (cloud-provider-kind)
// handles provisioning the Envoy data-plane container automatically.
func injectGatewayComponents(resolved *config.ResolvedContext) {
	gckGW := config.Component{
		Name: "gck-gateway",
		Type: "k8s",
		K8s: &config.K8sSpec{
			Manifests: []map[string]interface{}{
				{
					"apiVersion": "gateway.networking.k8s.io/v1",
					"kind":       "Gateway",
					"metadata":   map[string]interface{}{"name": "gck-gateway"},
					"spec": map[string]interface{}{
						"gatewayClassName": "cloud-provider-kind",
						"listeners": []map[string]interface{}{
							{
								"name": "http", "protocol": "HTTP", "port": 80,
								"allowedRoutes": map[string]interface{}{
									"namespaces": map[string]interface{}{"from": "All"},
								},
							},
						},
					},
				},
			},
		},
	}
	resolved.Components = append([]config.Component{gckGW}, resolved.Components...)
}

// The CPK controller is restarted fresh on each gck create, so it discovers the
// cluster immediately. 90s gives time for the CCM to start, informers to
// sync, and the gateway controller to reconcile and set status.addresses.
const gatewayPollTimeout = 90 * time.Second

func setupDNSRecords(ctx context.Context, cfg *config.Config) error {
	dnsDir := filepath.Join(gckHome, "dns")
	var dnsRecords []config.DNSRecord
	if cfg.Features.DNS.Records != nil {
		dnsRecords = cfg.Features.DNS.Records
	}

	introspectGateway := cfg.Features.Gateway != nil && cfg.Features.Gateway.Enabled
	if err := logger.WithSpinner("Collecting DNS records from cluster", func() error {
		return dns.IntrospectCluster(ctx, cfg.Kind.Name, dnsDir, gatewayPollTimeout, introspectGateway, dnsRecords)
	}); err != nil {
		return err
	}

	if err := ensureDNSServer(cfg); err != nil {
		logger.Warn("failed to start DNS server: %v", err)
	}

	gwEnabled := cfg.Features.Gateway != nil && cfg.Features.Gateway.Enabled
	if err := logger.WithSpinner("Syncing DNS records to cluster CoreDNS", func() error {
		return dns.SyncCoreDNS(ctx, cfg.Kind.Name, cfg.Features.DNS.Domain, dnsRecords, gwEnabled)
	}); err != nil {
		logger.Warn("failed to sync in-cluster DNS: %v", err)
	}

	if !dns.ResolverConfigured(cfg.Features.DNS.Domain, cfg.Features.DNS.Port) {
		fmt.Println()
		color.Yellow("  DNS server is running but OS-level routing is not configured.")
		color.Yellow("  Run \"gck setup dns\" once to enable automatic DNS resolution.")
	}

	return nil
}

func ensureCPKController(_ *config.Config, gatewayEnabled bool) error {
	pidDir := filepath.Join(gckHome, "pids")
	pidPath := filepath.Join(pidDir, "cpk.pid")

	// Kill any stale controller so the new one immediately discovers the
	// current cluster on its first poll iteration (no 30-second wait).
	if cloudprovider.NeedsTunnels() {
		// On macOS, CPK runs as root (via sudo). A non-root process cannot
		// signal a root process, so killProcess (which uses os.Signal) is
		// ineffective. Use sudo pkill to kill ALL root-owned CPK processes
		// accumulated from prior gck create invocations.
		if cpkProcessRunning() {
			cmd := exec.Command("sudo", "-p",
				"\n  gck needs administrator privileges for network routing.\n  Password: ",
				"pkill", "-f", "gck.*cpk serve")
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			_ = cmd.Run()
		}
	} else {
		killProcess(pidPath)
	}

	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		return fmt.Errorf("creating pid directory: %w", err)
	}

	gckBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding gck executable: %w", err)
	}

	var cmdArgs []string
	if cfgFile != "" {
		cmdArgs = append(cmdArgs, "--config", cfgFile)
	}
	cmdArgs = append(cmdArgs, "cpk", "serve")
	if gatewayEnabled {
		cmdArgs = append(cmdArgs, "--enable-gateway")
	}

	// On macOS, CPK needs root for loopback aliases and tunnels.
	if cloudprovider.NeedsTunnels() {
		fullCmd := gckBin
		for _, a := range cmdArgs {
			fullCmd += " " + a
		}
		cmd := exec.Command("sudo", "-p",
			"\n  gck needs administrator privileges for network routing.\n  Password: ",
			"sh", "-c", fullCmd+" &")
		cmd.Stdin = os.Stdin
		cmd.Stdout = nil
		cmd.Stderr = nil

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("starting cloud provider controller: %w", err)
		}

		// sudo spawns the background process; find its PID.
		time.Sleep(500 * time.Millisecond)
		pidCmd := exec.Command("pgrep", "-f", "gck.*cpk serve")
		out, err := pidCmd.Output()
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			if len(lines) > 0 {
				if err := os.WriteFile(pidPath, []byte(lines[0]+"\n"), 0o644); err != nil {
					return fmt.Errorf("writing CPK PID file: %w", err)
				}
				logger.Success("Cloud provider controller started (pid %s)", lines[0])
			}
		}
		return nil
	}

	cmd := exec.Command(gckBin, cmdArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting cloud provider controller: %w", err)
	}

	pid := cmd.Process.Pid
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)+"\n"), 0o644); err != nil {
		return fmt.Errorf("writing CPK PID file: %w", err)
	}

	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("releasing CPK process: %w", err)
	}

	logger.Success("Cloud provider controller started (pid %d)", pid)
	return nil
}

func killProcess(pidPath string) {
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
	_ = proc.Signal(syscall.SIGKILL)
	_, _ = proc.Wait()
	_ = os.Remove(pidPath)
}

func dnsServerParams(cfg *config.Config) (domain string, port int, dir string) {
	domain = config.DNSDefaultDomain
	port = config.DNSDefaultPort
	dir = filepath.Join(gckHome, "dns")
	if cfg.Features.DNS != nil {
		if cfg.Features.DNS.Domain != "" {
			domain = cfg.Features.DNS.Domain
		}
		if cfg.Features.DNS.Port != 0 {
			port = cfg.Features.DNS.Port
		}
	}
	return
}

// startDNSServer launches a new DNS server process and writes its PID file.
func startDNSServer(domain string, port int, dir string) error {
	pidDir := filepath.Join(gckHome, "pids")
	pidPath := filepath.Join(pidDir, "dns.pid")

	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		return fmt.Errorf("creating pid directory: %w", err)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)

	gckBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding gck executable: %w", err)
	}

	cmd := exec.Command(gckBin, "dns", "serve",
		"--dir", dir,
		"--domain", domain,
		"--addr", addr,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting DNS server: %w", err)
	}

	pid := cmd.Process.Pid
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)+"\n"), 0o644); err != nil {
		return fmt.Errorf("writing DNS PID file: %w", err)
	}

	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("releasing DNS server process: %w", err)
	}

	logger.Success("DNS server started (pid %d, %s)", pid, addr)
	return nil
}

func ensureDNSServer(cfg *config.Config) error {
	pidPath := filepath.Join(gckHome, "pids", "dns.pid")
	killProcess(pidPath)
	domain, port, dir := dnsServerParams(cfg)
	return startDNSServer(domain, port, dir)
}

// ensureDNSServerRunning starts the DNS server only if it is not already alive.
// Used by "gck refresh dns" to restart a server that exited early (e.g. because
// no record files existed at initial startup).
func ensureDNSServerRunning(cfg *config.Config) error {
	pidPath := filepath.Join(gckHome, "pids", "dns.pid")
	if isProcessAlive(pidPath) {
		return nil
	}
	domain, port, dir := dnsServerParams(cfg)
	return startDNSServer(domain, port, dir)
}

func printNotes(templateContent string, cfg *config.Config, activeFlags []string) {
	if templateContent == "" {
		return
	}
	rendered, err := notes.RenderWithFlags(templateContent, cfg, activeFlags)
	if err != nil {
		logger.Warn("failed to render notes: %v", err)
		return
	}
	fmt.Println()
	fmt.Println(rendered)
}

func isProcessAlive(pidPath string) bool {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
