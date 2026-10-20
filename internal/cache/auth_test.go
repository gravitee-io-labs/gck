package cache

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/docker/api/types/registry"
)

// useDockerConfigDir points DOCKER_CONFIG at dir for the duration of t and
// clears DOCKER_AUTH_CONFIG, whose credentials the Docker CLI configuration
// would otherwise layer over the file's, so a value in the developer's
// environment cannot change what the tests see.
func useDockerConfigDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("DOCKER_CONFIG", dir)
	t.Setenv("DOCKER_AUTH_CONFIG", "")
}

// writeDockerConfig writes a Docker CLI config.json holding inline auths into
// a fresh directory and points DOCKER_CONFIG at it for the duration of t.
func writeDockerConfig(t *testing.T, auths map[string][2]string) {
	t.Helper()
	entries := make(map[string]map[string]string, len(auths))
	for host, up := range auths {
		entries[host] = map[string]string{
			"auth": base64.StdEncoding.EncodeToString([]byte(up[0] + ":" + up[1])),
		}
	}
	data, err := json.Marshal(map[string]any{"auths": entries})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	useDockerConfigDir(t, dir)
}

func TestRegistryAuthFor(t *testing.T) {
	writeDockerConfig(t, map[string][2]string{
		"registry.example.test":       {"alice", "s3cr3t"},
		"https://index.docker.io/v1/": {"hubuser", "hubpass"},
	})

	tests := []struct {
		name     string
		ref      string
		wantUser string
		wantPass string
	}{
		{"private registry", "registry.example.test/team/app:1.0", "alice", "s3cr3t"},
		{"private registry, short ref", "registry.example.test/app", "alice", "s3cr3t"},
		{"docker hub short ref maps to index key", "postgres:17", "hubuser", "hubpass"},
		{"docker hub explicit domain", "docker.io/library/nginx:1.25", "hubuser", "hubpass"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := registryAuthFor(tc.ref)
			if got == "" {
				t.Fatalf("registryAuthFor(%q) = \"\", want credentials", tc.ref)
			}
			auth, err := registry.DecodeAuthConfig(got)
			if err != nil {
				t.Fatalf("decoding RegistryAuth: %v", err)
			}
			if auth.Username != tc.wantUser || auth.Password != tc.wantPass {
				t.Fatalf("registryAuthFor(%q) = %s:%s, want %s:%s",
					tc.ref, auth.Username, auth.Password, tc.wantUser, tc.wantPass)
			}
		})
	}
}

func TestRegistryAuthForAnonymous(t *testing.T) {
	writeDockerConfig(t, map[string][2]string{
		"registry.example.test": {"alice", "s3cr3t"},
	})

	for _, ref := range []string{
		"ghcr.io/org/repo:v1.2",
		"mongo:7",
		"UPPERCASE/not-a-valid-ref",
	} {
		t.Run(ref, func(t *testing.T) {
			if got := registryAuthFor(ref); got != "" {
				t.Fatalf("registryAuthFor(%q) = %q, want \"\"", ref, got)
			}
		})
	}
}

func TestRegistryAuthForUnreadableConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	useDockerConfigDir(t, dir)

	if got := registryAuthFor("registry.example.test/app:1"); got != "" {
		t.Fatalf("registryAuthFor with malformed config = %q, want \"\"", got)
	}
}
