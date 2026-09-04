package archive

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ExtractTotalLimitEnv overrides the total decompressed-size limit applied by
// [Extract] (the default is [defaultExtractTotalLimit]). A value <= 0 disables
// the limit (not recommended).
const ExtractTotalLimitEnv = "PB_BACKUP_MAX_EXTRACT_BYTES"

// defaultExtractTotalLimit bounds the total decompressed output of a single
// archive extract to 8 GiB. It guards against zip bombs (a tiny compressed
// upload decompressing to an unbounded amount of disk), while staying far above
// any legitimate backup size.
const defaultExtractTotalLimit = int64(8 << 30)

// configuredExtractTotalLimit returns the total decompressed-size limit for
// extraction, from the PB_BACKUP_MAX_EXTRACT_BYTES env var (falling back to
// [defaultExtractTotalLimit] for unset/invalid/<=0 values).
func configuredExtractTotalLimit() int64 {
	raw := strings.TrimSpace(os.Getenv(ExtractTotalLimitEnv))
	if raw == "" {
		return defaultExtractTotalLimit
	}

	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v <= 0 {
		return defaultExtractTotalLimit
	}

	return v
}

// Extract extracts the zip archive at "src" to "dest".
//
// Note that only dirs and regular files will be extracted.
// Symbolic links, named pipes, sockets, or any other irregular files
// are skipped because they come with too many edge cases and ambiguities.
//
// The total decompressed output is bounded by the configured limit
// (PB_BACKUP_MAX_EXTRACT_BYTES, default 8 GiB) to protect against zip bombs.
func Extract(src, dest string) error {
	return ExtractWithLimit(src, dest, configuredExtractTotalLimit())
}

// ConfiguredExtractTotalLimit exposes the effective total decompressed-size
// limit used by [Extract] (from PB_BACKUP_MAX_EXTRACT_BYTES, or the 8 GiB
// default when unset/invalid).
func ConfiguredExtractTotalLimit() int64 {
	return configuredExtractTotalLimit()
}

// ExtractWithLimit is [Extract] with an explicit total decompressed-size limit
// in bytes. A totalLimit of 0 or less disables the limit.
func ExtractWithLimit(src, dest string, totalLimit int64) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()

	// normalize dest path to check later for Zip Slip
	dest = filepath.Clean(dest) + string(os.PathSeparator)

	var totalExtracted int64

	for _, f := range zr.File {
		err := extractFile(f, dest, totalLimit, &totalExtracted)
		if err != nil {
			return err
		}
	}

	return nil
}

// extractFile extracts the provided zipFile into "basePath/zipFileName" path,
// creating all the necessary path directories.
func extractFile(zipFile *zip.File, basePath string, totalLimit int64, totalExtracted *int64) error {
	path := filepath.Join(basePath, zipFile.Name)

	// check for Zip Slip
	if !strings.HasPrefix(path, basePath) {
		return fmt.Errorf("invalid file path: %s", path)
	}

	r, err := zipFile.Open()
	if err != nil {
		return err
	}
	defer r.Close()

	// allow only dirs or regular files
	if zipFile.FileInfo().IsDir() {
		if err := os.MkdirAll(path, os.ModePerm); err != nil {
			return err
		}
	} else if zipFile.FileInfo().Mode().IsRegular() {
		var remaining int64
		if totalLimit > 0 {
			remaining = totalLimit - *totalExtracted
			if remaining <= 0 {
				return fmt.Errorf(
					"archive decompressed size exceeds the %d-byte limit",
					totalLimit,
				)
			}

			// the declared size is a cheap upfront guard; the LimitReader below
			// is the authoritative enforcement (declared sizes can lie)
			if zipFile.UncompressedSize64 > uint64(remaining) {
				return fmt.Errorf(
					"archive entry %q exceeds the %d-byte decompression limit",
					zipFile.Name,
					totalLimit,
				)
			}
		}

		// ensure that the file path directories are created
		if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
			return err
		}

		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, zipFile.Mode())
		if err != nil {
			return err
		}
		defer f.Close()

		var written int64
		if totalLimit > 0 {
			written, err = io.Copy(f, io.LimitReader(r, remaining))
			*totalExtracted += written
			if err != nil {
				return err
			}

			if written >= remaining {
				// verify the entry is genuinely exhausted (a malicious archive
				// may declare a small size while the stream carries more data)
				var probe [1]byte
				if n, readErr := r.Read(probe[:]); readErr == nil && n > 0 {
					return fmt.Errorf(
						"archive decompressed size exceeds the %d-byte limit",
						totalLimit,
					)
				}
			}
		} else {
			written, err = io.Copy(f, r)
			*totalExtracted += written
			if err != nil {
				return err
			}
		}
	}

	return nil
}