package registry

import (
	"path/filepath"
	"testing"
)

func TestNewResolver_FileURL(t *testing.T) {
	r := NewResolver("file:///some/path", "/gck-home", nil)
	fs, ok := r.(*FSResolver)
	if !ok {
		t.Fatal("expected FSResolver for file:// URL")
	}
	if fs.Root != "/some/path" {
		t.Fatalf("expected root %q, got %q", "/some/path", fs.Root)
	}
	if fs.GckHome != "/gck-home" {
		t.Fatalf("expected GckHome %q, got %q", "/gck-home", fs.GckHome)
	}
}

func TestNewResolver_HTTPURL(t *testing.T) {
	r := NewResolver("https://example.com/registry", "/gck-home", nil)
	h, ok := r.(*HTTPResolver)
	if !ok {
		t.Fatal("expected HTTPResolver for HTTP URL")
	}
	if h.BaseURL != "https://example.com/registry" {
		t.Fatalf("expected base URL %q, got %q", "https://example.com/registry", h.BaseURL)
	}
	expectedCache := filepath.Join("/gck-home", "cache")
	if h.CacheRoot != expectedCache {
		t.Fatalf("expected cache root %q, got %q", expectedCache, h.CacheRoot)
	}
	if h.GckHome != "/gck-home" {
		t.Fatalf("expected GckHome %q, got %q", "/gck-home", h.GckHome)
	}
}
