package installer

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gravitee-io-labs/gck/internal/config"
)

func TestBuildSecrets_SingleFileShorthand(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "license.key")
	if err := os.WriteFile(filePath, []byte("my-license-data"), 0644); err != nil {
		t.Fatal(err)
	}

	secrets := []config.LocalResource{
		{Name: "gravitee-license", FromFile: filePath},
	}

	objects, warnings, err := BuildSecrets(secrets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(objects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(objects))
	}

	obj := objects[0]
	if obj.GetKind() != "Secret" {
		t.Fatalf("expected kind Secret, got %q", obj.GetKind())
	}
	if obj.GetName() != "gravitee-license" {
		t.Fatalf("expected name %q, got %q", "gravitee-license", obj.GetName())
	}

	data, ok := obj.Object["data"].(map[string]interface{})
	if !ok {
		t.Fatal("expected data field")
	}
	expected := base64.StdEncoding.EncodeToString([]byte("my-license-data"))
	if data["license.key"] != expected {
		t.Fatalf("expected base64-encoded value, got %v", data)
	}
}

func TestBuildConfigMaps_SingleFileShorthand(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "logback.xml")
	if err := os.WriteFile(filePath, []byte("<configuration/>"), 0644); err != nil {
		t.Fatal(err)
	}

	cms := []config.LocalResource{
		{Name: "custom-logging", FromFile: filePath},
	}

	objects, warnings, err := BuildConfigMaps(cms)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(objects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(objects))
	}

	obj := objects[0]
	if obj.GetKind() != "ConfigMap" {
		t.Fatalf("expected kind ConfigMap, got %q", obj.GetKind())
	}

	data, ok := obj.Object["data"].(map[string]interface{})
	if !ok {
		t.Fatal("expected data field")
	}
	if data["logback.xml"] != "<configuration/>" {
		t.Fatalf("expected logback.xml content, got %v", data)
	}
}

func TestBuildSecrets_MultiEntryFromFileAndEnv(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token.txt")
	if err := os.WriteFile(tokenPath, []byte("s3cret-token"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TEST_API_KEY", "api-key-value")

	secrets := []config.LocalResource{
		{
			Name: "my-credentials",
			Entries: []config.ResourceEntry{
				{Key: "token", FromFile: tokenPath},
				{Key: "API_KEY", FromEnv: "TEST_API_KEY"},
			},
		},
	}

	objects, warnings, err := BuildSecrets(secrets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(objects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(objects))
	}

	secretData := objects[0].Object["data"].(map[string]interface{})
	if secretData["token"] != base64.StdEncoding.EncodeToString([]byte("s3cret-token")) {
		t.Fatalf("expected base64-encoded token value, got %q", secretData["token"])
	}
	if secretData["API_KEY"] != base64.StdEncoding.EncodeToString([]byte("api-key-value")) {
		t.Fatalf("expected base64-encoded API_KEY value, got %q", secretData["API_KEY"])
	}
}

func TestBuildConfigMaps_MultiEntryFromFileAndEnv(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "app.conf")
	if err := os.WriteFile(configPath, []byte("key=value"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TEST_LOG_LEVEL", "DEBUG")

	cms := []config.LocalResource{
		{
			Name: "app-config",
			Entries: []config.ResourceEntry{
				{Key: "app.conf", FromFile: configPath},
				{Key: "LOG_LEVEL", FromEnv: "TEST_LOG_LEVEL"},
			},
		},
	}

	objects, warnings, err := BuildConfigMaps(cms)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	data := objects[0].Object["data"].(map[string]interface{})
	if data["app.conf"] != "key=value" {
		t.Fatalf("expected app.conf content, got %q", data["app.conf"])
	}
	if data["LOG_LEVEL"] != "DEBUG" {
		t.Fatalf("expected LOG_LEVEL=DEBUG, got %q", data["LOG_LEVEL"])
	}
}

func TestBuildSecrets_OnMissingFailReturnsError_MissingFile(t *testing.T) {
	secrets := []config.LocalResource{
		{Name: "missing-secret", FromFile: "/nonexistent/path/secret.txt"},
	}

	_, _, err := BuildSecrets(secrets)
	if err == nil {
		t.Fatal("expected error for missing file with default onMissing")
	}
	if !strings.Contains(err.Error(), "reading file") {
		t.Fatalf("expected file read error, got: %v", err)
	}
}

func TestBuildSecrets_OnMissingFailReturnsError_MissingEnv(t *testing.T) {
	os.Unsetenv("DEFINITELY_NOT_SET_12345")

	secrets := []config.LocalResource{
		{
			Name: "env-secret",
			Entries: []config.ResourceEntry{
				{Key: "val", FromEnv: "DEFINITELY_NOT_SET_12345"},
			},
		},
	}

	_, _, err := BuildSecrets(secrets)
	if err == nil {
		t.Fatal("expected error for missing env var with default onMissing")
	}
	if !strings.Contains(err.Error(), "environment variable") {
		t.Fatalf("expected env var error, got: %v", err)
	}
}

func TestBuildSecrets_OnMissingFailExplicit(t *testing.T) {
	secrets := []config.LocalResource{
		{
			Name:      "explicit-fail",
			OnMissing: "fail",
			FromFile:  "/nonexistent/file.txt",
		},
	}

	_, _, err := BuildSecrets(secrets)
	if err == nil {
		t.Fatal("expected error for missing file with onMissing=fail")
	}
}

func TestBuildSecrets_OnMissingIgnore_MissingFile(t *testing.T) {
	secrets := []config.LocalResource{
		{
			Name:      "optional-license",
			OnMissing: "ignore",
			FromFile:  "/nonexistent/path/license.key",
		},
	}

	objects, warnings, err := BuildSecrets(secrets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(objects) != 0 {
		t.Fatalf("expected 0 objects (skipped), got %d", len(objects))
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	if !strings.Contains(warnings[0], "optional-license") {
		t.Fatalf("expected warning to mention resource name, got %q", warnings[0])
	}
	if !strings.Contains(warnings[0], "skipped") {
		t.Fatalf("expected warning to mention skipping, got %q", warnings[0])
	}
}

func TestBuildSecrets_OnMissingIgnore_MissingEnv(t *testing.T) {
	os.Unsetenv("DEFINITELY_NOT_SET_67890")

	secrets := []config.LocalResource{
		{
			Name:      "optional-creds",
			OnMissing: "ignore",
			Entries: []config.ResourceEntry{
				{Key: "val", FromEnv: "DEFINITELY_NOT_SET_67890"},
			},
		},
	}

	objects, warnings, err := BuildSecrets(secrets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(objects) != 0 {
		t.Fatalf("expected 0 objects (skipped), got %d", len(objects))
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
}

func TestBuildConfigMaps_OnMissingIgnore(t *testing.T) {
	cms := []config.LocalResource{
		{
			Name:      "optional-config",
			OnMissing: "ignore",
			FromFile:  "/nonexistent/config.xml",
		},
	}

	objects, warnings, err := BuildConfigMaps(cms)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(objects) != 0 {
		t.Fatalf("expected 0 objects (skipped), got %d", len(objects))
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	if !strings.Contains(warnings[0], "ConfigMap") {
		t.Fatalf("expected warning to mention ConfigMap, got %q", warnings[0])
	}
}

func TestBuildSecrets_KeyDefaultsToBasename(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "server.pem")
	if err := os.WriteFile(certPath, []byte("cert-data"), 0644); err != nil {
		t.Fatal(err)
	}

	secrets := []config.LocalResource{
		{
			Name: "tls-certs",
			Entries: []config.ResourceEntry{
				{FromFile: certPath},
			},
		},
	}

	objects, _, err := BuildSecrets(secrets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	secretData := objects[0].Object["data"].(map[string]interface{})
	if _, ok := secretData["server.pem"]; !ok {
		t.Fatalf("expected key to default to basename %q, got keys: %v", "server.pem", secretData)
	}
}

func TestBuildSecrets_KeyDefaultsToEnvVarName(t *testing.T) {
	t.Setenv("MY_SECRET_TOKEN", "token-value")

	secrets := []config.LocalResource{
		{
			Name: "env-defaults",
			Entries: []config.ResourceEntry{
				{FromEnv: "MY_SECRET_TOKEN"},
			},
		},
	}

	objects, _, err := BuildSecrets(secrets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	secretData := objects[0].Object["data"].(map[string]interface{})
	expected := base64.StdEncoding.EncodeToString([]byte("token-value"))
	if val, ok := secretData["MY_SECRET_TOKEN"]; !ok || val != expected {
		t.Fatalf("expected key to default to env var name with base64 value, got: %v", secretData)
	}
}

func TestBuildSecrets_ExplicitKeyOverridesDefault(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "original-name.txt")
	if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SOME_ENV_VAR", "env-data")

	secrets := []config.LocalResource{
		{
			Name: "custom-keys",
			Entries: []config.ResourceEntry{
				{Key: "custom-file-key", FromFile: filePath},
				{Key: "custom-env-key", FromEnv: "SOME_ENV_VAR"},
			},
		},
	}

	objects, _, err := BuildSecrets(secrets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	secretData := objects[0].Object["data"].(map[string]interface{})
	if secretData["custom-file-key"] != base64.StdEncoding.EncodeToString([]byte("data")) {
		t.Fatalf("expected base64-encoded custom-file-key, got: %v", secretData)
	}
	if secretData["custom-env-key"] != base64.StdEncoding.EncodeToString([]byte("env-data")) {
		t.Fatalf("expected base64-encoded custom-env-key, got: %v", secretData)
	}
}

func TestBuildSecrets_MultipleResources(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.txt")
	f2 := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(f1, []byte("aaa"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("bbb"), 0644); err != nil {
		t.Fatal(err)
	}

	secrets := []config.LocalResource{
		{Name: "secret-a", FromFile: f1},
		{Name: "secret-b", FromFile: f2},
	}

	objects, warnings, err := BuildSecrets(secrets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(objects) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(objects))
	}
	if objects[0].GetName() != "secret-a" {
		t.Fatalf("expected first secret name %q, got %q", "secret-a", objects[0].GetName())
	}
	if objects[1].GetName() != "secret-b" {
		t.Fatalf("expected second secret name %q, got %q", "secret-b", objects[1].GetName())
	}
}

func TestBuildSecrets_MixOfValidAndIgnoredResources(t *testing.T) {
	dir := t.TempDir()
	validFile := filepath.Join(dir, "valid.txt")
	if err := os.WriteFile(validFile, []byte("valid-data"), 0644); err != nil {
		t.Fatal(err)
	}

	secrets := []config.LocalResource{
		{Name: "valid-secret", FromFile: validFile},
		{Name: "missing-secret", OnMissing: "ignore", FromFile: "/nonexistent/file.txt"},
	}

	objects, warnings, err := BuildSecrets(secrets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(objects) != 1 {
		t.Fatalf("expected 1 object (second skipped), got %d", len(objects))
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning for skipped resource, got %d", len(warnings))
	}
	if objects[0].GetName() != "valid-secret" {
		t.Fatalf("expected valid-secret, got %q", objects[0].GetName())
	}
}

func TestBuildSecrets_EmptySlice(t *testing.T) {
	objects, warnings, err := BuildSecrets(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(objects) != 0 {
		t.Fatalf("expected 0 objects, got %d", len(objects))
	}
	if len(warnings) != 0 {
		t.Fatalf("expected 0 warnings, got %d", len(warnings))
	}
}

func TestBuildSecrets_EntryWithNeitherFileNorEnv(t *testing.T) {
	secrets := []config.LocalResource{
		{
			Name: "bad-entry",
			Entries: []config.ResourceEntry{
				{Key: "orphan"},
			},
		},
	}

	_, _, err := BuildSecrets(secrets)
	if err == nil {
		t.Fatal("expected error for entry with neither fromFile nor fromEnv")
	}
	if !strings.Contains(err.Error(), "neither fromFile nor fromEnv") {
		t.Fatalf("expected descriptive error, got: %v", err)
	}
}

func TestBuildSecrets_ApiVersionAndKind(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	objects, _, err := BuildSecrets([]config.LocalResource{
		{Name: "test-secret", FromFile: filePath},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	obj := objects[0]
	apiVersion, _, _ := strings.Cut(obj.GetAPIVersion(), "/")
	if apiVersion != "v1" {
		t.Fatalf("expected apiVersion v1, got %q", obj.GetAPIVersion())
	}
	if obj.GetKind() != "Secret" {
		t.Fatalf("expected kind Secret, got %q", obj.GetKind())
	}
}

func TestBuildConfigMaps_ApiVersionAndKind(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	objects, _, err := BuildConfigMaps([]config.LocalResource{
		{Name: "test-cm", FromFile: filePath},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	obj := objects[0]
	if obj.GetAPIVersion() != "v1" {
		t.Fatalf("expected apiVersion v1, got %q", obj.GetAPIVersion())
	}
	if obj.GetKind() != "ConfigMap" {
		t.Fatalf("expected kind ConfigMap, got %q", obj.GetKind())
	}
}

func TestBuildSecrets_UsesDataFieldWithBase64(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(filePath, []byte("secret-value"), 0644); err != nil {
		t.Fatal(err)
	}

	objects, _, err := BuildSecrets([]config.LocalResource{
		{Name: "s", FromFile: filePath},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := objects[0].Object["data"]; !ok {
		t.Fatal("expected Secrets to use data field")
	}
	if _, ok := objects[0].Object["stringData"]; ok {
		t.Fatal("expected Secrets to NOT use stringData field")
	}
	data := objects[0].Object["data"].(map[string]interface{})
	expected := base64.StdEncoding.EncodeToString([]byte("secret-value"))
	if data["secret.txt"] != expected {
		t.Fatalf("expected base64-encoded value %q, got %q", expected, data["secret.txt"])
	}
}

func TestBuildConfigMaps_UsesDataField(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "config.txt")
	if err := os.WriteFile(filePath, []byte("config-value"), 0644); err != nil {
		t.Fatal(err)
	}

	objects, _, err := BuildConfigMaps([]config.LocalResource{
		{Name: "c", FromFile: filePath},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := objects[0].Object["data"]; !ok {
		t.Fatal("expected ConfigMaps to use data field")
	}
	if _, ok := objects[0].Object["stringData"]; ok {
		t.Fatal("expected ConfigMaps to NOT use stringData field")
	}
}

func TestBuildSecrets_OnMissingFailStopsOnFirstError(t *testing.T) {
	dir := t.TempDir()
	validFile := filepath.Join(dir, "exists.txt")
	if err := os.WriteFile(validFile, []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}

	secrets := []config.LocalResource{
		{Name: "missing-first", FromFile: "/nonexistent/a.txt"},
		{Name: "valid-second", FromFile: validFile},
	}

	objects, _, err := BuildSecrets(secrets)
	if err == nil {
		t.Fatal("expected error from first resource")
	}
	if objects != nil {
		t.Fatalf("expected nil objects on error, got %d", len(objects))
	}
}
