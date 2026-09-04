package archive_test

import (
	"archive/zip"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/arief-fajri/pgbase/tools/archive"
)

func TestExtractFailure(t *testing.T) {
	testDir := createTestDir(t)
	defer os.RemoveAll(testDir)

	missingZipPath := filepath.Join(os.TempDir(), "pb_missing_test.zip")
	extractedPath := filepath.Join(os.TempDir(), "pb_zip_extract")
	defer os.RemoveAll(extractedPath)

	if err := archive.Extract(missingZipPath, extractedPath); err == nil {
		t.Fatal("Expected Extract to fail due to missing zipPath")
	}

	if _, err := os.Stat(extractedPath); err == nil {
		t.Fatalf("Expected %q to not be created", extractedPath)
	}
}

func TestExtractSuccess(t *testing.T) {
	testDir := createTestDir(t)
	defer os.RemoveAll(testDir)

	zipPath := filepath.Join(os.TempDir(), "pb_test.zip")
	defer os.RemoveAll(zipPath)

	extractedPath := filepath.Join(os.TempDir(), "pb_zip_extract")
	defer os.RemoveAll(extractedPath)

	// zip testDir content (with exclude)
	if err := archive.Create(testDir, zipPath, "a/b/c", "test2", "sub2"); err != nil {
		t.Fatalf("Failed to create archive: %v", err)
	}

	if err := archive.Extract(zipPath, extractedPath); err != nil {
		t.Fatalf("Failed to extract %q in %q", zipPath, extractedPath)
	}

	availableFiles := []string{}

	walkErr := filepath.WalkDir(extractedPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		availableFiles = append(availableFiles, path)

		return nil
	})
	if walkErr != nil {
		t.Fatalf("Failed to read the extracted dir: %v", walkErr)
	}

	// (note: symbolic links and other regular files should be missing)
	expectedFiles := []string{
		filepath.Join(extractedPath, "test"),
		filepath.Join(extractedPath, "a/test"),
		filepath.Join(extractedPath, "a/b/sub1"),
	}

	if len(availableFiles) != len(expectedFiles) {
		t.Fatalf("Expected \n%v, \ngot \n%v", expectedFiles, availableFiles)
	}

ExpectedLoop:
	for _, expected := range expectedFiles {
		for _, available := range availableFiles {
			if available == expected {
				continue ExpectedLoop
			}
		}

		t.Fatalf("Missing file %q in \n%v", expected, availableFiles)
	}
}

// writeZipBytes builds a zip archive at path from the given named entries.
func writeZipBytes(t *testing.T, path string, files map[string][]byte) {
	t.Helper()

	zf, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create zip file: %v", err)
	}
	defer zf.Close()

	zw := zip.NewWriter(zf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("Failed to create zip entry %q: %v", name, err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatalf("Failed to write zip entry %q: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Failed to close zip writer: %v", err)
	}
}

func TestExtractWithLimitEnforced(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "limit.zip")
	writeZipBytes(t, zipPath, map[string][]byte{
		"a.txt": make([]byte, 4096),
		"b.txt": make([]byte, 4096),
	})

	// limit below the total size must be enforced
	extractPath := filepath.Join(t.TempDir(), "out")
	if err := archive.ExtractWithLimit(zipPath, extractPath, 4096); err == nil {
		t.Fatal("Expected ExtractWithLimit to fail when total output exceeds the limit")
	}

	// a limit above the total size must succeed
	extractPath2 := filepath.Join(t.TempDir(), "out2")
	if err := archive.ExtractWithLimit(zipPath, extractPath2, 8192+1); err != nil {
		t.Fatalf("Expected ExtractWithLimit to succeed within the limit, got: %v", err)
	}

	// limit below a single entry size -> entry rejected
	bigPath := filepath.Join(t.TempDir(), "big")
	if err := archive.ExtractWithLimit(zipPath, bigPath, 1024); err == nil {
		t.Fatal("Expected ExtractWithLimit to reject an entry exceeding the remaining limit")
	}
}

func TestConfiguredExtractTotalLimit(t *testing.T) {
	t.Setenv(archive.ExtractTotalLimitEnv, "")

	if got := archive.ConfiguredExtractTotalLimit(); got != 8<<30 {
		t.Fatalf("expected default 8 GiB limit, got %d", got)
	}

	t.Setenv(archive.ExtractTotalLimitEnv, "1048576")
	if got := archive.ConfiguredExtractTotalLimit(); got != 1048576 {
		t.Fatalf("expected 1048576, got %d", got)
	}

	// invalid/zero values fall back to the default
	t.Setenv(archive.ExtractTotalLimitEnv, "not-a-number")
	if got := archive.ConfiguredExtractTotalLimit(); got != 8<<30 {
		t.Fatalf("expected default for invalid value, got %d", got)
	}
}
