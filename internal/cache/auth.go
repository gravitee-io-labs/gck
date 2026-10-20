package cache

import (
	"os"

	"github.com/distribution/reference"
	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/credentials"
	"github.com/docker/docker/api/types/registry"
)

const (
	// dockerHubDomain is the domain reference.ParseNormalizedNamed assigns to
	// Docker Hub images.
	dockerHubDomain = "docker.io"
	// dockerHubAuthKey is the key under which the Docker CLI stores Docker Hub
	// credentials (registry.IndexServer in docker/docker).
	dockerHubAuthKey = "https://index.docker.io/v1/"
)

// registryAuthFor returns the X-Registry-Auth value to send when pulling ref:
// the credentials a `docker login` stored for the registry of ref, read from
// the Docker CLI configuration the way the docker CLI does (DOCKER_CONFIG,
// DOCKER_AUTH_CONFIG, credsStore and credHelpers are all honored).
//
// It returns "" when no credential is configured or the configuration cannot
// be read, so the pull falls back to being anonymous rather than failing.
func registryAuthFor(ref string) string {
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return ""
	}
	host := reference.Domain(named)
	if host == dockerHubDomain {
		host = dockerHubAuthKey
	}

	cfg, err := config.Load(os.Getenv(config.EnvOverrideConfigDir))
	if err != nil {
		return ""
	}
	if !cfg.ContainsAuth() {
		cfg.CredentialsStore = credentials.DetectDefaultStore(cfg.CredentialsStore)
	}

	auth, err := cfg.GetAuthConfig(host)
	if err != nil {
		return ""
	}
	if auth.Username == "" && auth.Password == "" && auth.IdentityToken == "" && auth.RegistryToken == "" {
		return ""
	}

	encoded, err := registry.EncodeAuthConfig(registry.AuthConfig(auth))
	if err != nil {
		return ""
	}
	return encoded
}
