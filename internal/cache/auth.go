package cache

import (
	"context"
	"fmt"
	"os"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/distribution/reference"
	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/config/credentials"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
)

const (
	// dockerHubDomain is the domain reference.ParseNormalizedNamed assigns to
	// Docker Hub images.
	dockerHubDomain = "docker.io"
	// dockerHubAuthKey is the key under which the Docker CLI stores Docker Hub
	// credentials (registry.IndexServer in docker/docker).
	dockerHubAuthKey = "https://index.docker.io/v1/"
)

// registryAuth is what a pull from one registry sends: the X-Registry-Auth
// header, or "" when no credential is stored. lookupErr says why no
// credential could be read; the pull then goes out anonymously.
type registryAuth struct {
	header    string
	lookupErr error
}

// authHost returns the key under which the Docker CLI stores the credentials
// for the registry of ref, or "" when ref is not a valid reference.
func authHost(ref string) string {
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return ""
	}
	host := reference.Domain(named)
	if host == dockerHubDomain {
		return dockerHubAuthKey
	}
	return host
}

// registryAuths returns the credentials a `docker login` stored for each
// registry of refs, keyed by authHost. The Docker CLI configuration is read
// once, the way the docker CLI does (DOCKER_CONFIG, DOCKER_AUTH_CONFIG,
// credsStore and credHelpers are all honored), and each host is looked up
// once, one after another: a credential helper that prompts asks once per
// registry, never once per image or several times at once.
func registryAuths(refs []string) map[string]registryAuth {
	auths := make(map[string]registryAuth)
	cfg, loadErr := config.Load(os.Getenv(config.EnvOverrideConfigDir))
	if loadErr == nil && !cfg.ContainsAuth() {
		cfg.CredentialsStore = credentials.DetectDefaultStore(cfg.CredentialsStore)
	}
	for _, ref := range refs {
		host := authHost(ref)
		if _, seen := auths[host]; seen || host == "" {
			continue
		}
		if loadErr != nil {
			auths[host] = registryAuth{lookupErr: fmt.Errorf("reading the Docker CLI configuration: %w", loadErr)}
			continue
		}
		auths[host] = lookupAuth(cfg, host)
	}
	return auths
}

// registryAuthFor returns the credentials stored for the registry of ref.
func registryAuthFor(ref string) registryAuth {
	return registryAuths([]string{ref})[authHost(ref)]
}

func lookupAuth(cfg *configfile.ConfigFile, host string) registryAuth {
	auth, err := cfg.GetAuthConfig(host)
	if err != nil {
		return registryAuth{lookupErr: fmt.Errorf("reading the credentials for %s: %w", host, err)}
	}
	if auth.Username == "" && auth.Password == "" && auth.IdentityToken == "" && auth.RegistryToken == "" {
		return registryAuth{}
	}
	header, err := registry.EncodeAuthConfig(registry.AuthConfig(auth))
	if err != nil {
		return registryAuth{lookupErr: fmt.Errorf("encoding the credentials for %s: %w", host, err)}
	}
	return registryAuth{header: header}
}

// pullImage pulls ref with auth and waits for the pull to finish. A pull that
// sent credentials and is refused as unauthorized is retried once
// anonymously, so a stale login still pulls a public image, as it did before
// gck sent credentials. When the pull fails, the error says whether stored
// credentials were refused or why none could be read.
func pullImage(ctx context.Context, cli *client.Client, ref string, auth registryAuth) error {
	err := pullOnce(ctx, cli, ref, auth.header)
	switch {
	case err == nil:
		return nil
	case auth.header != "" && cerrdefs.IsUnauthorized(err):
		if pullOnce(ctx, cli, ref, "") == nil {
			return nil
		}
		return fmt.Errorf("%w (the registry refused the credentials stored by docker login, and the anonymous retry failed too)", err)
	case auth.lookupErr != nil:
		return fmt.Errorf("%w (pulled anonymously: %v)", err, auth.lookupErr)
	default:
		return err
	}
}

func pullOnce(ctx context.Context, cli *client.Client, ref, header string) error {
	rc, err := cli.ImagePull(ctx, ref, image.PullOptions{RegistryAuth: header})
	if err != nil {
		return err
	}
	return drainPullStream(rc)
}
