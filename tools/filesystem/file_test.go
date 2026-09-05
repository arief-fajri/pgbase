package filesystem_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/arief-fajri/pgbase/tests"
	"github.com/arief-fajri/pgbase/tools/filesystem"
	"github.com/arief-fajri/pgbase/tools/security"
)

func TestFileAsMap(t *testing.T) {
	file, err := filesystem.NewFileFromBytes([]byte("test"), "test123.txt")
	if err != nil {
		t.Fatal(err)
	}

	result := file.AsMap()

	if len(result) != 3 {
		t.Fatalf("Expected map with %d keys, got\n%v", 3, result)
	}

	if result["size"] != int64(4) {
		t.Fatalf("Expected size %d, got %#v", 4, result["size"])
	}

	if str, ok := result["name"].(string); !ok || !strings.HasPrefix(str, "test123") {
		t.Fatalf("Expected name to have prefix %q, got %#v", "test123", result["name"])
	}

	if result["originalName"] != "test123.txt" {
		t.Fatalf("Expected originalName %q, got %#v", "test123.txt", result["originalName"])
	}
}

func TestNewFileFromPath(t *testing.T) {
	testDir := createTestDir(t)
	defer os.RemoveAll(testDir)

	// missing file
	_, err := filesystem.NewFileFromPath("missing")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	// existing file
	originalName := "image_!@ special"
	normalizedNamePattern := regexp.QuoteMeta("image_special_") + `\w{10}` + regexp.QuoteMeta(".png")
	f, err := filesystem.NewFileFromPath(filepath.Join(testDir, originalName))
	if err != nil {
		t.Fatalf("Expected nil error, got %v", err)
	}
	if f.OriginalName != originalName {
		t.Fatalf("Expected OriginalName %q, got %q", originalName, f.OriginalName)
	}
	if match, err := regexp.Match(normalizedNamePattern, []byte(f.Name)); !match {
		t.Fatalf("Expected Name to match %v, got %q (%v)", normalizedNamePattern, f.Name, err)
	}
	if f.Size != 73 {
		t.Fatalf("Expected Size %v, got %v", 73, f.Size)
	}
	if _, ok := f.Reader.(*filesystem.PathReader); !ok {
		t.Fatalf("Expected Reader to be PathReader, got %v", f.Reader)
	}
}

func TestNewFileFromBytes(t *testing.T) {
	// nil bytes
	if _, err := filesystem.NewFileFromBytes(nil, "photo.jpg"); err == nil {
		t.Fatal("Expected error, got nil")
	}

	// zero bytes
	if _, err := filesystem.NewFileFromBytes([]byte{}, "photo.jpg"); err == nil {
		t.Fatal("Expected error, got nil")
	}

	originalName := "image_!@ special"
	normalizedNamePattern := regexp.QuoteMeta("image_special_") + `\w{10}` + regexp.QuoteMeta(".txt")
	f, err := filesystem.NewFileFromBytes([]byte("text\n"), originalName)
	if err != nil {
		t.Fatal(err)
	}
	if f.Size != 5 {
		t.Fatalf("Expected Size %v, got %v", 5, f.Size)
	}
	if f.OriginalName != originalName {
		t.Fatalf("Expected OriginalName %q, got %q", originalName, f.OriginalName)
	}
	if match, err := regexp.Match(normalizedNamePattern, []byte(f.Name)); !match {
		t.Fatalf("Expected Name to match %v, got %q (%v)", normalizedNamePattern, f.Name, err)
	}
}

func TestNewFileFromMultipart(t *testing.T) {
	formData, mp, err := tests.MockMultipartData(nil, "test")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("", "/", formData)
	req.Header.Set("Content-Type", mp.FormDataContentType())
	req.ParseMultipartForm(32 << 20)

	_, mh, err := req.FormFile("test")
	if err != nil {
		t.Fatal(err)
	}

	f, err := filesystem.NewFileFromMultipart(mh)
	if err != nil {
		t.Fatal(err)
	}

	originalNamePattern := regexp.QuoteMeta("tmpfile-") + `\w+` + regexp.QuoteMeta(".txt")
	if match, err := regexp.Match(originalNamePattern, []byte(f.OriginalName)); !match {
		t.Fatalf("Expected OriginalName to match %v, got %q (%v)", originalNamePattern, f.OriginalName, err)
	}

	normalizedNamePattern := regexp.QuoteMeta("tmpfile_") + `\w+\_\w{10}` + regexp.QuoteMeta(".txt")
	if match, err := regexp.Match(normalizedNamePattern, []byte(f.Name)); !match {
		t.Fatalf("Expected Name to match %v, got %q (%v)", normalizedNamePattern, f.Name, err)
	}

	if f.Size != 4 {
		t.Fatalf("Expected Size %v, got %v", 4, f.Size)
	}

	if _, ok := f.Reader.(*filesystem.MultipartReader); !ok {
		t.Fatalf("Expected Reader to be MultipartReader, got %v", f.Reader)
	}
}

func TestNewFileFromURLTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/error" {
			w.WriteHeader(http.StatusInternalServerError)
		}

		fmt.Fprintf(w, "test")
	}))
	defer srv.Close()

	// cancelled context
	{
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		f, err := filesystem.NewFileFromURL(ctx, srv.URL+"/cancel")
		if err == nil {
			t.Fatal("[ctx_cancel] Expected error, got nil")
		}
		if f != nil {
			t.Fatalf("[ctx_cancel] Expected file to be nil, got %v", f)
		}
	}

	// error response
	{
		f, err := filesystem.NewFileFromURL(context.Background(), srv.URL+"/error")
		if err == nil {
			t.Fatal("[error_status] Expected error, got nil")
		}
		if f != nil {
			t.Fatalf("[error_status] Expected file to be nil, got %v", f)
		}
	}

	// valid response via the unsafe variant (loopback is blocked by the default NewFileFromURL)
	{
		originalName := "image_!@ special"
		normalizedNamePattern := regexp.QuoteMeta("image_special_") + `\w{10}` + regexp.QuoteMeta(".txt")

		f, err := filesystem.NewUnsafeFileFromURL(context.Background(), srv.URL+"/"+originalName)
		if err != nil {
			t.Fatalf("[valid] Unexpected error %v", err)
		}
		if f == nil {
			t.Fatal("[valid] Expected non-nil file")
		}

		// check the created file fields
		if f.OriginalName != originalName {
			t.Fatalf("Expected OriginalName %q, got %q", originalName, f.OriginalName)
		}
		if match, err := regexp.Match(normalizedNamePattern, []byte(f.Name)); !match {
			t.Fatalf("Expected Name to match %v, got %q (%v)", normalizedNamePattern, f.Name, err)
		}
		if f.Size != 4 {
			t.Fatalf("Expected Size %v, got %v", 4, f.Size)
		}
		if _, ok := f.Reader.(*filesystem.BytesReader); !ok {
			t.Fatalf("Expected Reader to be BytesReader, got %v", f.Reader)
		}
	}
}

func TestNewFileFromURLRejectsPrivateAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "private")
	}))
	defer srv.Close()

	// both the loopback server URL and explicit private/loopback IPs should be rejected
	scenarios := []string{
		srv.URL + "/x",         // httptest binds to 127.0.0.1 (loopback)
		"http://127.0.0.1/x",   // loopback
		"http://10.0.0.1/x",    // private
		"http://169.254.1.1/x", // link-local
		"http://::1/x",         // IPv6 loopback
	}

	for _, url := range scenarios {
		if _, err := filesystem.NewFileFromURL(context.Background(), url); err == nil {
			t.Fatalf("Expected error for %q, got nil", url)
		}
	}
}

func TestValidateDialAddress(t *testing.T) {
	// allowed addresses (public, non-RFC1918)
	for _, addr := range []string{"8.8.8.8:443", "1.2.3.4:80", "123.45.67.89:443"} {
		if err := security.ValidateDialAddress(addr); err != nil {
			t.Fatalf("Expected no error for %q, got %v", addr, err)
		}
	}

	// disallowed addresses
	for _, addr := range []string{"127.0.0.1:80", "10.0.0.1:80", "192.168.1.1:80", "169.254.1.1:80", "[::1]:80"} {
		if err := security.ValidateDialAddress(addr); err == nil {
			t.Fatalf("Expected error for %q, got nil", addr)
		}
	}
}

func TestFileNameNormalizations(t *testing.T) {
	scenarios := []struct {
		name    string
		pattern string
	}{
		{"", `^\w{10}_\w{10}\.txt$`},
		{".png", `^\w{10}_\w{10}\.png$`},
		{".tar.gz", `^\w{10}_\w{10}\.tar\.gz$`},
		{"a.tar.gz", `^a\w{10}_\w{10}\.tar\.gz$`},
		{"....abc", `^\w{10}_\w{10}\.abc$`},
		{"a.b.c.?.?.?.2", `^a_b_c_\w{10}\.2$`},
		{"a.b.c.d.tar.gz", `^a_b_c_d_\w{10}\.tar\.gz$`},
		{"abcd", `^abcd_\w{10}\.txt$`},
		{".abcd.123.", `^abcd_\w{10}\.123$`},
		{"a  b! c d  . 456", `^a_b_c_d_\w{10}\.456$`},                                              // normalize spaces
		{strings.Repeat("a", 101) + "." + strings.Repeat("b", 21), `^a{100}_\w{10}\.b{20}$`},       // name and extension length cut
		{"abc" + strings.Repeat("d", 290) + "." + strings.Repeat("b", 9), `^d{100}_\w{10}\.b{9}$`}, // initial total length cut
	}

	for i, s := range scenarios {
		t.Run(strconv.Itoa(i)+"_"+s.name, func(t *testing.T) {
			f, err := filesystem.NewFileFromBytes([]byte("abc"), s.name)
			if err != nil {
				t.Fatal(err)
			}
			match, err := regexp.Match(s.pattern, []byte(f.Name))
			if !match {
				t.Fatalf("Expected Name to match %v, got %q (%v)", s.pattern, f.Name, err)
			}
		})
	}
}
