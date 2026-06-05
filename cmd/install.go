package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/gravitee-io-labs/gck/internal/config"
	"github.com/gravitee-io-labs/gck/internal/installer"
	"github.com/gravitee-io-labs/gck/internal/logger"
	"github.com/gravitee-io-labs/gck/internal/registry"
)

const defaultReadyTimeout = 5 * time.Minute

// installComponents validates, topo-sorts, initialises Helm repos and installs
// the components from the resolved context. When filter is non-nil only
// components for which filter returns true are installed; dependency readiness
// checks still consider the full component set.
func installComponents(
	ctx context.Context,
	resolved *config.ResolvedContext,
	filter func(config.Component) bool,
	opts installer.InstallOpts,
) error {
	resolved.Components = filterDisabledComponents(resolved.Components)

	if err := registry.Validate(resolved.Components); err != nil {
		return fmt.Errorf("validating components: %w", err)
	}
	sorted, err := registry.TopoSort(resolved.Components)
	if err != nil {
		return fmt.Errorf("resolving dependencies: %w", err)
	}

	helmInst, _ := installer.ForType("helm")
	if hi, ok := helmInst.(*installer.HelmInstaller); ok {
		if err := logger.WithSpinner("Initializing Helm", func() error {
			return hi.AddRepos(resolved.Repos, gckHome)
		}); err != nil {
			return err
		}
	}

	compByName := make(map[string]config.Component)
	for _, c := range resolved.Components {
		compByName[c.Name] = c
	}

	for _, comp := range sorted {
		for _, req := range comp.Requires {
			if req.Conditions.Ready && !opts.DryRun {
				dep := compByName[req.Component]
				depNamespace := dep.Namespace
				if depNamespace == "" {
					depNamespace = "default"
				}
				timeout := defaultReadyTimeout
				if req.Timeout != "" {
					if d, err := time.ParseDuration(req.Timeout); err == nil && d > 0 {
						timeout = d
					}
				}
				var matchLabels map[string]string
				if req.Selector != nil && len(req.Selector.MatchLabels) > 0 {
					matchLabels = req.Selector.MatchLabels
				}
				if err := logger.WithSpinner(fmt.Sprintf("Waiting for %q to be ready", req.Component), func() error {
					return installer.WaitForReady(ctx, req.Component, depNamespace, timeout, matchLabels)
				}); err != nil {
					return fmt.Errorf("requirement %q not ready: %w", req.Component, err)
				}
			}
		}

		if filter != nil && !filter(comp) {
			continue
		}

		inst, err := installer.ForType(comp.EffectiveType())
		if err != nil {
			return fmt.Errorf("component %q: %w", comp.Name, err)
		}
		comp := comp

		compOpts := opts
		var diffBuf *bytes.Buffer
		if opts.DryRun {
			diffBuf = &bytes.Buffer{}
			compOpts.DiffWriter = diffBuf
		}

		spinnerMsg := fmt.Sprintf("Installing %q", comp.Name)
		if opts.DryRun {
			spinnerMsg = fmt.Sprintf("Dry-run %q", comp.Name)
		}

		if err := logger.WithSpinner(spinnerMsg, func() error {
			return inst.Install(ctx, comp, resolved.Dir, compOpts)
		}); err != nil {
			return err
		}

		if diffBuf != nil && diffBuf.Len() > 0 {
			fmt.Fprintln(os.Stderr)
			fmt.Fprint(os.Stderr, diffBuf.String())
		}
		if comp.Conditions.Ready && !opts.DryRun {
			ns := comp.Namespace
			if ns == "" {
				ns = "default"
			}
			timeout := defaultReadyTimeout
			if comp.Timeout != "" {
				if d, err := time.ParseDuration(comp.Timeout); err == nil && d > 0 {
					timeout = d
				}
			}
			var matchLabels map[string]string
			if comp.Selector != nil && len(comp.Selector.MatchLabels) > 0 {
				matchLabels = comp.Selector.MatchLabels
			}
			if err := logger.WithSpinner(
				fmt.Sprintf("Waiting for %q to be ready", comp.Name),
				func() error {
					return installer.WaitForReady(ctx, comp.Name, ns, timeout, matchLabels)
				},
			); err != nil {
				return fmt.Errorf("component %q not ready: %w", comp.Name, err)
			}
		}
	}

	return nil
}

// filterDisabledComponents removes components with enabled: false and prunes
// any requires entries that reference disabled components so readiness waits
// don't block on something that won't be deployed.
func filterDisabledComponents(components []config.Component) []config.Component {
	disabled := make(map[string]bool)
	for i := range components {
		if !components[i].IsEnabled() {
			disabled[components[i].Name] = true
		}
	}
	if len(disabled) == 0 {
		return components
	}

	var result []config.Component
	for _, c := range components {
		if disabled[c.Name] {
			continue
		}
		if len(c.Requires) > 0 {
			var kept []config.Requirement
			for _, r := range c.Requires {
				if !disabled[r.Component] {
					kept = append(kept, r)
				}
			}
			c.Requires = kept
		}
		result = append(result, c)
	}
	return result
}
