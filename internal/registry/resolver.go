package registry

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/gravitee-io-labs/gck/internal/config"
)

// Resolver resolves a context path against a registry into a ResolvedContext.
type Resolver interface {
	Resolve(ctx context.Context, contextPath string) (*config.ResolvedContext, error)
}

// NewResolver builds the appropriate Resolver from the registry URL.
// A "file://" prefix selects the filesystem resolver; anything else
// is treated as an HTTP registry with cache rooted under gckHome.
// setOverrides are the --set key=value pairs forwarded to the template
// engine when rendering context gck.yaml files.
func NewResolver(registry string, gckHome string, setOverrides map[string]string) Resolver {
	if strings.HasPrefix(registry, "file://") {
		return &FSResolver{
			Root:         strings.TrimPrefix(registry, "file://"),
			GckHome:      gckHome,
			SetOverrides: setOverrides,
		}
	}
	return &HTTPResolver{
		BaseURL:      registry,
		CacheRoot:    filepath.Join(gckHome, "cache"),
		GckHome:      gckHome,
		HTTPClient:   newAuthenticatedClient(registry),
		SetOverrides: setOverrides,
	}
}
