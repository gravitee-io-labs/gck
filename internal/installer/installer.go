package installer

import (
	"context"
	"fmt"
	"io"

	"github.com/gravitee-io-labs/gck/internal/config"
)

// InstallOpts carries optional flags for an install operation.
type InstallOpts struct {
	DryRun     bool
	DiffWriter io.Writer
}

// Installer deploys and removes components.
type Installer interface {
	Install(ctx context.Context, comp config.Component, dir string, opts InstallOpts) error
	Uninstall(ctx context.Context, comp config.Component) error
}

var installers = map[string]Installer{
	"helm": &HelmInstaller{},
	"k8s":  &ManifestInstaller{},
}

func ForType(componentType string) (Installer, error) {
	inst, ok := installers[componentType]
	if !ok {
		return nil, fmt.Errorf("unknown component type: %q", componentType)
	}
	return inst, nil
}
