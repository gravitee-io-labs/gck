package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseNetrc_SingleMachine(t *testing.T) {
	data := `machine example.com login user password secret`
	entries := parseNetrc(data)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].machine != "example.com" || entries[0].login != "user" || entries[0].password != "secret" {
		t.Fatalf("unexpected entry: %+v", entries[0])
	}
}

func TestParseNetrc_MultiLine(t *testing.T) {
	data := "machine example.com\n  login user\n  password secret\n"
	entries := parseNetrc(data)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.machine != "example.com" || e.login != "user" || e.password != "secret" {
		t.Fatalf("unexpected entry: %+v", e)
	}
}

func TestParseNetrc_MultipleMachines(t *testing.T) {
	data := `machine a.com login u1 password p1
machine b.com login u2 password p2
machine c.com login u3 password p3`
	entries := parseNetrc(data)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	for i, want := range []struct{ m, l, p string }{
		{"a.com", "u1", "p1"},
		{"b.com", "u2", "p2"},
		{"c.com", "u3", "p3"},
	} {
		if entries[i].machine != want.m || entries[i].login != want.l || entries[i].password != want.p {
			t.Fatalf("entry %d: expected %+v, got %+v", i, want, entries[i])
		}
	}
}

func TestParseNetrc_StopsAtDefault(t *testing.T) {
	data := `machine a.com login u1 password p1
default login fallback password fallbackpw
machine b.com login u2 password p2`
	entries := parseNetrc(data)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (stop at default), got %d", len(entries))
	}
}

func TestParseNetrc_EmptyFile(t *testing.T) {
	entries := parseNetrc("")
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestParseNetrc_MacdefSkipped(t *testing.T) {
	data := `machine a.com login u1 password p1
macdef init
cd /pub
bin

machine b.com login u2 password p2`
	entries := parseNetrc(data)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestLookupNetrc_WithEnvOverride(t *testing.T) {
	dir := t.TempDir()
	netrcFile := filepath.Join(dir, "custom-netrc")
	if err := os.WriteFile(netrcFile, []byte("machine registry.example.com login admin password s3cret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NETRC", netrcFile)

	login, password, ok := lookupNetrc("registry.example.com")
	if !ok {
		t.Fatal("expected credentials to be found")
	}
	if login != "admin" || password != "s3cret" {
		t.Fatalf("expected admin/s3cret, got %s/%s", login, password)
	}
}

func TestLookupNetrc_NoMatch(t *testing.T) {
	dir := t.TempDir()
	netrcFile := filepath.Join(dir, "netrc")
	if err := os.WriteFile(netrcFile, []byte("machine other.com login u password p\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NETRC", netrcFile)

	_, _, ok := lookupNetrc("registry.example.com")
	if ok {
		t.Fatal("expected no match")
	}
}

func TestLookupNetrc_MissingFile(t *testing.T) {
	t.Setenv("NETRC", filepath.Join(t.TempDir(), "does-not-exist"))

	_, _, ok := lookupNetrc("anything.com")
	if ok {
		t.Fatal("expected no match when file is missing")
	}
}

func TestNewAuthenticatedClient_WithCredentials(t *testing.T) {
	dir := t.TempDir()
	netrcFile := filepath.Join(dir, "netrc")
	if err := os.WriteFile(netrcFile, []byte("machine pages.example.com login deploy password ghp_token123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NETRC", netrcFile)

	client := newAuthenticatedClient("https://pages.example.com/gck")
	if client == nil {
		t.Fatal("expected authenticated client")
	}
}

func TestNewAuthenticatedClient_NoCredentials(t *testing.T) {
	t.Setenv("NETRC", filepath.Join(t.TempDir(), "does-not-exist"))

	client := newAuthenticatedClient("https://pages.example.com/gck")
	if client != nil {
		t.Fatal("expected nil client when no credentials match")
	}
}

func TestNewAuthenticatedClient_InvalidURL(t *testing.T) {
	client := newAuthenticatedClient("://bad-url")
	if client != nil {
		t.Fatal("expected nil client for invalid URL")
	}
}
