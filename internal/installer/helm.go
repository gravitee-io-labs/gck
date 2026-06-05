package installer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gravitee-io-labs/gck/internal/config"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
	"helm.sh/helm/v3/pkg/storage/driver"
)

// HelmInstaller installs Helm charts (upgrade --install).
type HelmInstaller struct {
	home string // gck home directory (absolute), set by AddRepos
}

// AddRepos adds the given Helm repositories and downloads their indexes.
// Must be called with an absolute home path before Install for chart resolution to use these repos.
func (h *HelmInstaller) AddRepos(repos []config.Repo, home string) error {
	if home == "" {
		return fmt.Errorf("home directory must be set")
	}
	h.home = home

	helmRepoConfig := filepath.Join(home, "helm", "repositories.yaml")
	// Helm repo package writes index to HELM_CACHE_HOME/repository; downloader reads from RepositoryCache.
	// So we use the same base: helm/repository under home.
	helmRepoCache := filepath.Join(home, "helm", "repository")

	if err := os.MkdirAll(filepath.Dir(helmRepoConfig), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(helmRepoCache, 0o755); err != nil {
		return err
	}
	// So that repo.DownloadIndexFile writes to helmRepoCache (helmpath.CachePath("repository") = HELM_CACHE_HOME/repository)
	os.Setenv("HELM_CACHE_HOME", filepath.Join(home, "helm"))

	var f *repo.File
	if _, err := os.Stat(helmRepoConfig); err == nil {
		f, err = repo.LoadFile(helmRepoConfig)
		if err != nil {
			return fmt.Errorf("loading repositories file: %w", err)
		}
	}
	if f == nil {
		f = repo.NewFile()
	}

	settings := cli.New()
	settings.RepositoryConfig = helmRepoConfig
	settings.RepositoryCache = helmRepoCache
	os.Setenv("HELM_REPOSITORY_CONFIG", helmRepoConfig)
	os.Setenv("HELM_CONFIG_HOME", filepath.Join(home, "helm"))

	getters := getter.All(settings)
	for _, r := range repos {
		entry := &repo.Entry{Name: r.Name, URL: r.URL}
		chartRepo, err := repo.NewChartRepository(entry, getters)
		if err != nil {
			return fmt.Errorf("adding repo %q: %w", r.Name, err)
		}
		// Always download index so it lives in our cache (required for LocateChart)
		if _, err := chartRepo.DownloadIndexFile(); err != nil {
			return fmt.Errorf("downloading index for %q: %w", r.Name, err)
		}
		if !f.Has(r.Name) {
			f.Update(entry)
		}
	}

	return f.WriteFile(helmRepoConfig, 0o644)
}

// isReleaseUninstalled returns true if the last release in history is uninstalled.
func isReleaseUninstalled(versions []*release.Release) bool {
	return len(versions) > 0 && versions[len(versions)-1].Info.Status == release.StatusUninstalled
}

// Install runs helm upgrade --install for the component: install if release does not exist, else upgrade.
func (h *HelmInstaller) Install(ctx context.Context, comp config.Component, dir string, opts InstallOpts) error {
	if comp.Helm == nil {
		return fmt.Errorf("component %q has no helm spec", comp.Name)
	}
	if h.home == "" {
		return fmt.Errorf("home directory must be set (call AddRepos first)")
	}

	helmRepoConfig := filepath.Join(h.home, "helm", "repositories.yaml")
	helmRepoCache := filepath.Join(h.home, "helm", "repository")

	os.Setenv("HELM_CACHE_HOME", filepath.Join(h.home, "helm"))
	os.Setenv("HELM_REPOSITORY_CONFIG", helmRepoConfig)
	os.Setenv("HELM_CONFIG_HOME", filepath.Join(h.home, "helm"))

	settings := cli.New()
	settings.RepositoryConfig = helmRepoConfig
	settings.RepositoryCache = helmRepoCache

	namespace := comp.Namespace
	if namespace == "" {
		namespace = "default"
	}
	settings.SetNamespace(namespace)

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), namespace, os.Getenv("HELM_DRIVER"), func(_ string, _ ...interface{}) {}); err != nil {
		return fmt.Errorf("initializing helm: %w", err)
	}

	registryClient, err := registry.NewClient(
		registry.ClientOptEnableCache(true),
		registry.ClientOptCredentialsFile(settings.RegistryConfig),
	)
	if err != nil {
		return fmt.Errorf("creating registry client: %w", err)
	}
	actionConfig.RegistryClient = registryClient

	valueOpts := &values.Options{
		ValueFiles: make([]string, 0, len(comp.Helm.ValueFiles)),
	}
	for _, v := range comp.Helm.ValueFiles {
		valueOpts.ValueFiles = append(valueOpts.ValueFiles, filepath.Join(dir, v))
	}

	// Inline values are serialized to a temp file appended last so they
	// take highest precedence in the Helm merge order.
	if len(comp.Helm.Values) > 0 {
		data, err := yaml.Marshal(comp.Helm.Values)
		if err != nil {
			return fmt.Errorf("serializing inline values: %w", err)
		}
		tmp, err := os.CreateTemp("", "gck-values-*.yaml")
		if err != nil {
			return fmt.Errorf("creating temp values file: %w", err)
		}
		defer os.Remove(tmp.Name())
		if _, err := tmp.Write(data); err != nil {
			tmp.Close()
			return fmt.Errorf("writing temp values file: %w", err)
		}
		tmp.Close()
		valueOpts.ValueFiles = append(valueOpts.ValueFiles, tmp.Name())
	}

	vals, err := valueOpts.MergeValues(getter.All(settings))
	if err != nil {
		return fmt.Errorf("merging values: %w", err)
	}

	// Check if release exists (same logic as helm upgrade --install)
	histClient := action.NewHistory(actionConfig)
	histClient.Max = 1
	versions, histErr := histClient.Run(comp.Name)
	useInstall := histErr == driver.ErrReleaseNotFound || isReleaseUninstalled(versions)
	if histErr != nil && !useInstall {
		return fmt.Errorf("checking release history: %w", histErr)
	}

	if useInstall {
		instClient := action.NewInstall(actionConfig)
		instClient.SetRegistryClient(registryClient)
		instClient.ReleaseName = comp.Name
		instClient.Namespace = namespace
		instClient.CreateNamespace = true
		instClient.DryRun = opts.DryRun
		if opts.DryRun {
			instClient.DryRunOption = "server"
		}
		instClient.ChartPathOptions = action.ChartPathOptions{}
		if comp.Helm.Version != "" {
			instClient.ChartPathOptions.Version = comp.Helm.Version
		}
		chartPath, err := instClient.ChartPathOptions.LocateChart(comp.Helm.Chart, settings)
		if err != nil {
			return fmt.Errorf("locating chart %q: %w", comp.Helm.Chart, err)
		}
		ch, err := loader.Load(chartPath)
		if err != nil {
			return fmt.Errorf("loading chart: %w", err)
		}
		rel, err := instClient.RunWithContext(ctx, ch, vals)
		if err != nil {
			return fmt.Errorf("running install: %w", err)
		}
		if opts.DryRun && opts.DiffWriter != nil && rel != nil {
			if err := RenderDiff(comp.Name, "", rel.Manifest, opts.DiffWriter); err != nil {
				return fmt.Errorf("rendering diff: %w", err)
			}
		}
		return nil
	}

	upgradeClient := action.NewUpgrade(actionConfig)
	upgradeClient.SetRegistryClient(registryClient)
	upgradeClient.Namespace = namespace
	upgradeClient.DryRun = opts.DryRun
	if opts.DryRun {
		upgradeClient.DryRunOption = "server"
	}
	if comp.Helm.Version != "" {
		upgradeClient.Version = comp.Helm.Version
		upgradeClient.ChartPathOptions.Version = comp.Helm.Version
	}
	chartPath, err := upgradeClient.ChartPathOptions.LocateChart(comp.Helm.Chart, settings)
	if err != nil {
		return fmt.Errorf("locating chart %q: %w", comp.Helm.Chart, err)
	}
	ch, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("loading chart: %w", err)
	}
	oldManifest := versions[0].Manifest
	rel, err := upgradeClient.RunWithContext(ctx, comp.Name, ch, vals)
	if err != nil {
		return fmt.Errorf("running upgrade: %w", err)
	}
	if opts.DryRun && opts.DiffWriter != nil && rel != nil {
		if err := RenderDiff(comp.Name, oldManifest, rel.Manifest, opts.DiffWriter); err != nil {
			return fmt.Errorf("rendering diff: %w", err)
		}
	}
	return nil
}

// Uninstall runs helm uninstall for the component.
func (h *HelmInstaller) Uninstall(_ context.Context, comp config.Component) error {
	if h.home == "" {
		return fmt.Errorf("home directory must be set (call AddRepos first)")
	}

	helmRepoCache := filepath.Join(h.home, "helm", "repository")
	os.Setenv("HELM_CACHE_HOME", filepath.Join(h.home, "helm"))
	settings := cli.New()
	settings.RepositoryConfig = filepath.Join(h.home, "helm", "repositories.yaml")
	settings.RepositoryCache = helmRepoCache

	namespace := comp.Namespace
	if namespace == "" {
		namespace = "default"
	}
	settings.SetNamespace(namespace)

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), namespace, os.Getenv("HELM_DRIVER"), func(string, ...interface{}) {}); err != nil {
		return fmt.Errorf("initializing helm: %w", err)
	}

	client := action.NewUninstall(actionConfig)
	_, err := client.Run(comp.Name)
	return err
}
