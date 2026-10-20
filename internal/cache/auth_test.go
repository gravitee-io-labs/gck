package cache

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
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
			if got.header == "" || got.lookupErr != nil {
				t.Fatalf("registryAuthFor(%q) = %+v, want credentials", tc.ref, got)
			}
			auth, err := registry.DecodeAuthConfig(got.header)
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
			if got := registryAuthFor(ref); got != (registryAuth{}) {
				t.Fatalf("registryAuthFor(%q) = %+v, want no credentials and no error", ref, got)
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

	got := registryAuthFor("registry.example.test/app:1")
	if got.header != "" || got.lookupErr == nil {
		t.Fatalf("registryAuthFor with malformed config = %+v, want no credentials and the read error", got)
	}
}

func TestRegistryAuthsLooksUpEachHostOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake credential helper is a shell script")
	}
	bin := t.TempDir()
	calls := filepath.Join(t.TempDir(), "calls")
	helper := "#!/bin/sh\necho get >> \"$GCK_TEST_HELPER_CALLS\"\ncat >/dev/null\n" +
		`echo '{"ServerURL":"registry.example.test","Username":"alice","Secret":"s3cr3t"}'` + "\n"
	if err := os.WriteFile(filepath.Join(bin, "docker-credential-gcktest"), []byte(helper), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GCK_TEST_HELPER_CALLS", calls)

	dir := t.TempDir()
	cfg := `{"credHelpers":{"registry.example.test":"gcktest"}}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	useDockerConfigDir(t, dir)

	auths := registryAuths([]string{
		"registry.example.test/a:1",
		"registry.example.test/b:1",
		"registry.example.test/c:1",
		"mongo:7",
	})

	data, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), "get"); n != 1 {
		t.Fatalf("credential helper ran %d times for one host, want 1", n)
	}
	auth, err := registry.DecodeAuthConfig(auths["registry.example.test"].header)
	if err != nil || auth.Username != "alice" || auth.Password != "s3cr3t" {
		t.Fatalf("registry.example.test auth = %+v (%v), want alice:s3cr3t", auth, err)
	}
	if got := auths[dockerHubAuthKey]; got != (registryAuth{}) {
		t.Fatalf("Docker Hub auth = %+v, want no credentials and no error", got)
	}
}

// fakePullDaemon serves the Docker Engine image pull endpoint. A pull that
// sends X-Registry-Auth is refused as unauthorized when refuseCredentials is
// set; an anonymous pull succeeds only when allowAnonymous is set.
type fakePullDaemon struct {
	refuseCredentials bool
	allowAnonymous    bool
	headers           []string
}

func (d *fakePullDaemon) client(t *testing.T) *client.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("X-Registry-Auth")
		d.headers = append(d.headers, header)
		refused := (header != "" && d.refuseCredentials) || (header == "" && !d.allowAnonymous)
		w.Header().Set("Content-Type", "application/json")
		if refused {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"unauthorized: authentication required"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"Status: Downloaded newer image"}` + "\n"))
	}))
	t.Cleanup(srv.Close)
	cli, err := client.NewClientWithOpts(client.WithHost("tcp://"+srv.Listener.Addr().String()), client.WithVersion("1.47"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	return cli
}

func TestPullImage(t *testing.T) {
	tests := []struct {
		name        string
		daemon      fakePullDaemon
		auth        registryAuth
		wantErr     string
		wantHeaders []string
	}{
		{
			name:        "accepted credentials pull once",
			daemon:      fakePullDaemon{allowAnonymous: true},
			auth:        registryAuth{header: "creds"},
			wantHeaders: []string{"creds"},
		},
		{
			name:        "refused credentials retry anonymously",
			daemon:      fakePullDaemon{refuseCredentials: true, allowAnonymous: true},
			auth:        registryAuth{header: "creds"},
			wantHeaders: []string{"creds", ""},
		},
		{
			name:        "refused credentials on a private image",
			daemon:      fakePullDaemon{refuseCredentials: true},
			auth:        registryAuth{header: "creds"},
			wantErr:     "refused the credentials stored by docker login",
			wantHeaders: []string{"creds", ""},
		},
		{
			name:        "anonymous pull is not retried",
			daemon:      fakePullDaemon{},
			wantErr:     "unauthorized",
			wantHeaders: []string{""},
		},
		{
			name:        "lookup error is reported",
			daemon:      fakePullDaemon{},
			auth:        registryAuth{lookupErr: errors.New("credential helper crashed")},
			wantErr:     "pulled anonymously: credential helper crashed",
			wantHeaders: []string{""},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := tc.daemon
			err := pullImage(context.Background(), d.client(t), "registry.example.test/app:1", tc.auth)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("pullImage: %v, want success", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("pullImage: %v, want an error containing %q", err, tc.wantErr)
			}
			if strings.Join(d.headers, ",") != strings.Join(tc.wantHeaders, ",") {
				t.Fatalf("pulls sent X-Registry-Auth %q, want %q", d.headers, tc.wantHeaders)
			}
		})
	}
}
