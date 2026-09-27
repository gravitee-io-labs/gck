package installer

import (
	"errors"
	"fmt"

	"github.com/gravitee-io-labs/gck/internal/config"
)

// PreflightLocalResources checks, without building anything, that every
// secret and config map of the enabled components can be read from its local
// files and environment variables. Resources marked onMissing: ignore are
// skipped. It reports all failures at once, so a missing input is caught
// before any cluster work starts rather than when its component installs.
func PreflightLocalResources(components []config.Component) error {
	var errs []error
	for _, c := range components {
		if !c.IsEnabled() || c.K8s == nil {
			continue
		}
		check := func(kind string, resources []config.LocalResource) {
			for _, res := range resources {
				if shouldIgnore(res) {
					continue
				}
				if _, err := resolveEntries(res); err != nil {
					errs = append(errs, fmt.Errorf("component %q: %s %q: %w", c.Name, kind, res.Name, err))
				}
			}
		}
		check("secret", c.K8s.Secrets)
		check("config map", c.K8s.ConfigMaps)
	}
	return errors.Join(errs...)
}
