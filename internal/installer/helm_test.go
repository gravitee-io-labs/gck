package installer

import (
	"testing"

	"github.com/gravitee-io-labs/gck/internal/config"
	"helm.sh/helm/v3/pkg/release"
)

func compWithoutHelm() config.Component {
	return config.Component{Name: "no-helm"}
}

func compWithHelm() config.Component {
	return config.Component{
		Name: "with-helm",
		Helm: &config.HelmSpec{Chart: "test/chart"},
	}
}

func TestIsReleaseUninstalled_Empty(t *testing.T) {
	if isReleaseUninstalled(nil) {
		t.Fatal("expected false for nil slice")
	}
	if isReleaseUninstalled([]*release.Release{}) {
		t.Fatal("expected false for empty slice")
	}
}

func TestIsReleaseUninstalled_Deployed(t *testing.T) {
	versions := []*release.Release{
		{Info: &release.Info{Status: release.StatusDeployed}},
	}
	if isReleaseUninstalled(versions) {
		t.Fatal("expected false for deployed release")
	}
}

func TestIsReleaseUninstalled_Uninstalled(t *testing.T) {
	versions := []*release.Release{
		{Info: &release.Info{Status: release.StatusUninstalled}},
	}
	if !isReleaseUninstalled(versions) {
		t.Fatal("expected true for uninstalled release")
	}
}

func TestIsReleaseUninstalled_MultipleVersions(t *testing.T) {
	versions := []*release.Release{
		{Info: &release.Info{Status: release.StatusDeployed}},
		{Info: &release.Info{Status: release.StatusUninstalled}},
	}
	if !isReleaseUninstalled(versions) {
		t.Fatal("expected true when last version is uninstalled")
	}
}

func TestIsReleaseUninstalled_LastIsDeployed(t *testing.T) {
	versions := []*release.Release{
		{Info: &release.Info{Status: release.StatusUninstalled}},
		{Info: &release.Info{Status: release.StatusDeployed}},
	}
	if isReleaseUninstalled(versions) {
		t.Fatal("expected false when last version is deployed")
	}
}

func TestHelmInstaller_InstallRejectsNilHelmSpec(t *testing.T) {
	h := &HelmInstaller{}
	err := h.Install(t.Context(), compWithoutHelm(), ".", InstallOpts{})
	if err == nil {
		t.Fatal("expected error for component without helm spec")
	}
}

func TestHelmInstaller_InstallRejectsEmptyHome(t *testing.T) {
	h := &HelmInstaller{}
	err := h.Install(t.Context(), compWithHelm(), ".", InstallOpts{})
	if err == nil {
		t.Fatal("expected error when home is not set")
	}
}

func TestHelmInstaller_InstallDryRunRejectsNilHelmSpec(t *testing.T) {
	h := &HelmInstaller{}
	err := h.Install(t.Context(), compWithoutHelm(), ".", InstallOpts{DryRun: true})
	if err == nil {
		t.Fatal("expected error for component without helm spec even in dry-run")
	}
}
