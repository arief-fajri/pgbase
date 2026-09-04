package ghupdate

import (
	"strings"
	"testing"
)

func TestReleaseFindAssetBySuffix(t *testing.T) {
	r := release{
		Assets: []*releaseAsset{
			{Name: "test1.zip", Id: 1},
			{Name: "test2.zip", Id: 2},
			{Name: "test22.zip", Id: 22},
			{Name: "test3.zip", Id: 3},
		},
	}

	asset, err := r.findAssetBySuffix("2.zip")
	if err != nil {
		t.Fatalf("Expected nil, got err: %v", err)
	}

	if asset.Id != 2 {
		t.Fatalf("Expected asset with id %d, got %v", 2, asset)
	}
}

func TestReleaseFindAssetByName(t *testing.T) {
	r := release{
		Assets: []*releaseAsset{
			{Name: "test1.zip"},
			{Name: "checksums.txt"},
		},
	}

	if got := r.findAssetByName("checksums.txt"); got == nil || got.Name != "checksums.txt" {
		t.Fatalf("expected to find checksums.txt, got %v", got)
	}
	if got := r.findAssetByName("missing.txt"); got != nil {
		t.Fatalf("expected nil for missing asset, got %v", got)
	}
}

func TestFindChecksum(t *testing.T) {
	scenarios := []struct {
		name     string
		raw      string
		filename string
		expected string
		ok       bool
	}{
		{"empty", "", "x.zip", "", false},
		{"no match", "abc   other.zip", "x.zip", "", false},
		{"exact match", "abc   x.zip", "x.zip", "abc", true},
		{"uppercase digest normalized", "ABCDEF   x.zip", "x.zip", "abcdef", true},
		{"leading ./", "abc ./x.zip", "x.zip", "abc", true},
		{"multiple lines", "111   a.zip\n222   b.zip\n", "b.zip", "222", true},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			digest, ok := findChecksum([]byte(s.raw), s.filename)
			if ok != s.ok || digest != s.expected {
				t.Fatalf("expected ok=%v digest=%q, got ok=%v digest=%q", s.ok, s.expected, ok, digest)
			}
		})
	}
}

func TestValidateBaseURL(t *testing.T) {
	scenarios := []struct {
		name string
		url  string
		ok   bool
	}{
		{"https api", "https://api.github.com", true},
		{"https github", "https://github.com/owner/repo", true},
		{"loopback host http", "http://localhost:9090", true},
		{"loopback ip http", "http://127.0.0.1:9090", true},
		{"plaintext remote", "http://api.github.com", false},
		{"ftp scheme", "ftp://api.github.com", false},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			err := validateBaseURL(s.url)
			if (err == nil) != s.ok {
				t.Fatalf("validateBaseURL(%q) ok=%v, err=%v", s.url, s.ok, err)
			}
			if err != nil && !strings.HasPrefix(err.Error(), "ghupdate BaseURL") {
				t.Fatalf("expected a descriptive error, got %v", err)
			}
		})
	}
}
