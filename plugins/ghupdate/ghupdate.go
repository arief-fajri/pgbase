// Package ghupdate implements a new command to selfupdate the current
// PGBase executable with the latest GitHub release.
//
// Example usage:
//
//	ghupdate.MustRegister(app, app.RootCmd, ghupdate.Config{})
package ghupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tools/archive"
	"github.com/arief-fajri/pgbase/tools/osutils"
	"github.com/spf13/cobra"
)

// HttpClient is a base HTTP client interface (usually used for test purposes).
type HttpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Config defines the config options of the ghupdate plugin.
//
// NB! This plugin is considered experimental and its config options may change in the future.
type Config struct {
	// Owner specifies the account owner of the repository (default to "arief-fajri").
	Owner string

	// Repo specifies the name of the repository (default to "pgbase").
	Repo string

	// ArchiveExecutable specifies the name of the executable file in the release archive
	// (default to "pgbase"; an additional ".exe" check is also performed as a fallback).
	ArchiveExecutable string

	// BaseURL is the base URL of the GitHub API (or similar compatible)
	// used to fetch the latest releases information.
	//
	// Defaults to "https://api.github.com".
	BaseURL string

	// Optional context to use when fetching and downloading the latest release.
	Context context.Context

	// The HTTP client to use when fetching and downloading the latest release.
	// Defaults to `http.DefaultClient`.
	HttpClient HttpClient
}

// MustRegister registers the ghupdate plugin to the provided app instance
// and panic if it fails.
func MustRegister(app core.App, rootCmd *cobra.Command, config Config) {
	if err := Register(app, rootCmd, config); err != nil {
		panic(err)
	}
}

// Register registers the ghupdate plugin to the provided app instance.
func Register(app core.App, rootCmd *cobra.Command, config Config) error {
	p := &plugin{
		app:            app,
		currentVersion: rootCmd.Version,
		config:         config,
	}

	if p.config.Owner == "" {
		p.config.Owner = "arief-fajri"
	}

	if p.config.Repo == "" {
		p.config.Repo = "pgbase"
	}

	if p.config.ArchiveExecutable == "" {
		p.config.ArchiveExecutable = "pgbase"
	}

	if p.config.BaseURL == "" {
		p.config.BaseURL = "https://api.github.com"
	} else {
		p.config.BaseURL = strings.TrimRight(p.config.BaseURL, "/")
	}

	// PGB-L09: refuse to fetch release metadata/artifacts over plaintext (MITM
	// could tamper with the release JSON or the downloaded binary). Loopback
	// http (e.g. a local API mirror) is allowed.
	if err := validateBaseURL(p.config.BaseURL); err != nil {
		return err
	}

	if p.config.HttpClient == nil {
		p.config.HttpClient = http.DefaultClient
	}

	if p.config.Context == nil {
		p.config.Context = context.Background()
	}

	rootCmd.AddCommand(p.updateCmd())

	return nil
}

type plugin struct {
	app            core.App
	config         Config
	currentVersion string
}

func (p *plugin) updateCmd() *cobra.Command {
	var withBackup bool

	command := &cobra.Command{
		Use:          "update",
		Short:        "Automatically updates the current app executable with the latest available version",
		SilenceUsage: true,
		RunE: func(command *cobra.Command, args []string) error {
			var needConfirm bool
			if isMaybeRunningInDocker() {
				needConfirm = true
				color.Yellow("NB! It seems that you are in a Docker container.")
				color.Yellow("The update command may not work as expected in this context because usually the version of the app is managed by the container image itself.")
			} else if isMaybeRunningInNixOS() {
				needConfirm = true
				color.Yellow("NB! It seems that you are in a NixOS.")
				color.Yellow("Due to the non-standard filesystem implementation of the environment, the update command may not work as expected.")
			}

			if needConfirm {
				confirm := osutils.YesNoPrompt("Do you want to proceed with the update?", false)
				if !confirm {
					fmt.Println("The command has been cancelled.")
					return nil
				}
			}

			return p.update(withBackup)
		},
	}

	command.PersistentFlags().BoolVar(
		&withBackup,
		"backup",
		true,
		"Creates a pb_data backup at the end of the update process",
	)

	return command
}

func (p *plugin) update(withBackup bool) error {
	color.Yellow("Fetching release information...")

	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", p.config.BaseURL, p.config.Owner, p.config.Repo)

	latest, err := fetchLatestRelease(p.config.Context, p.config.HttpClient, url)
	if err != nil {
		return err
	}

	if compareVersions(strings.TrimPrefix(p.currentVersion, "v"), strings.TrimPrefix(latest.Tag, "v")) <= 0 {
		color.Green("You already have the latest version %s.", p.currentVersion)
		return nil
	}

	suffix := archiveSuffix(runtime.GOOS, runtime.GOARCH)
	if suffix == "" {
		return errors.New("unsupported platform")
	}

	asset, err := latest.findAssetBySuffix(suffix)
	if err != nil {
		return err
	}

	releaseDir := filepath.Join(p.app.DataDir(), core.LocalTempDirName)
	defer os.RemoveAll(releaseDir)

	color.Yellow("Downloading %s...", asset.Name)

	// download the release asset
	assetZip := filepath.Join(releaseDir, asset.Name)
	if err := downloadFile(p.config.Context, p.config.HttpClient, asset.DownloadUrl, assetZip); err != nil {
		return err
	}

	// PGB-L09: verify the downloaded asset against the release checksums.txt
	// (goreleaser) BEFORE extracting/replacing the running executable, so a
	// tampered or wrong artifact can never overwrite the binary.
	if err := verifyAssetChecksum(p.config.Context, p.config.HttpClient, latest, asset, assetZip); err != nil {
		return err
	}

	color.Yellow("Extracting %s...", asset.Name)

	extractDir := filepath.Join(releaseDir, "extracted_"+asset.Name)
	defer os.RemoveAll(extractDir)

	if err := archive.Extract(assetZip, extractDir); err != nil {
		return err
	}

	color.Yellow("Replacing the executable...")

	oldExec, err := os.Executable()
	if err != nil {
		return err
	}
	renamedOldExec := oldExec + ".old"
	defer os.Remove(renamedOldExec)

	newExec := filepath.Join(extractDir, p.config.ArchiveExecutable)
	if _, err := os.Stat(newExec); err != nil {
		// try again with an .exe extension
		newExec = newExec + ".exe"
		if _, fallbackErr := os.Stat(newExec); fallbackErr != nil {
			return fmt.Errorf("the executable in the extracted path is missing or it is inaccessible: %v, %v", err, fallbackErr)
		}
	}

	// rename the current executable
	if err := os.Rename(oldExec, renamedOldExec); err != nil {
		return fmt.Errorf("failed to rename the current executable: %w", err)
	}

	tryToRevertExecChanges := func() {
		if revertErr := os.Rename(renamedOldExec, oldExec); revertErr != nil {
			p.app.Logger().Debug(
				"Failed to revert executable",
				slog.String("old", renamedOldExec),
				slog.String("new", oldExec),
				slog.String("error", revertErr.Error()),
			)
		}
	}

	// replace with the extracted binary
	if err := os.Rename(newExec, oldExec); err != nil {
		tryToRevertExecChanges()
		return fmt.Errorf("failed replacing the executable: %w", err)
	}

	if withBackup {
		color.Yellow("Creating pb_data backup...")

		backupName := fmt.Sprintf("@update_%s.zip", latest.Tag)
		if err := p.app.CreateBackup(p.config.Context, backupName); err != nil {
			tryToRevertExecChanges()
			return err
		}
	}

	color.HiBlack("---")
	color.Green("Update completed successfully! You can start the executable as usual.")

	// print the release notes
	if latest.Body != "" {
		fmt.Print("\n")
		color.Cyan("Here is a list with some of the %s changes:", latest.Tag)
		// remove the update command note to avoid "stuttering"
		// (@todo consider moving to a config option)
		releaseNotes := strings.TrimSpace(strings.Replace(latest.Body, "> _To update the prebuilt executable you can run `./"+p.config.ArchiveExecutable+" update`._", "", 1))
		color.Cyan(releaseNotes)
		fmt.Print("\n")
	}

	return nil
}

func fetchLatestRelease(
	ctx context.Context,
	client HttpClient,
	url string,
) (*release, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	rawBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	// http.Client doesn't treat non 2xx responses as error
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf(
			"(%d) failed to fetch latest releases:\n%s",
			res.StatusCode,
			string(rawBody),
		)
	}

	result := &release{}
	if err := json.Unmarshal(rawBody, result); err != nil {
		return nil, err
	}

	return result, nil
}

func downloadFile(
	ctx context.Context,
	client HttpClient,
	url string,
	destPath string,
) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	// http.Client doesn't treat non 2xx responses as error
	if res.StatusCode >= 400 {
		return fmt.Errorf("(%d) failed to send download file request", res.StatusCode)
	}

	// ensure that the dest parent dir(s) exist
	if err := os.MkdirAll(filepath.Dir(destPath), os.ModePerm); err != nil {
		return err
	}

	dest, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer dest.Close()

	if _, err := io.Copy(dest, res.Body); err != nil {
		return err
	}

	return nil
}

// checksumsAssetName is the goreleaser checksums file name. GoReleaser publishes
// a "checksums.txt" asset whose lines are "<hex sha256>  <archive filename>".
const checksumsAssetName = "checksums.txt"

// validateBaseURL refuses non-HTTPS base URLs (loopback http is allowed for
// local mirrors / tests) - PGB-L09.
func validateBaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid ghupdate BaseURL %q: %w", raw, err)
	}

	if u.Scheme == "https" {
		return nil
	}

	if u.Scheme == "http" {
		host := u.Hostname()
		if host == "localhost" || host == "127.0.0.1" || host == "::1" {
			return nil
		}
	}

	return fmt.Errorf("ghupdate BaseURL must be https:// (got %q)", raw)
}

// verifyAssetChecksum downloads the release "checksums.txt" (goreleaser) and
// verifies that the sha256 of assetPath matches the digest listed for
// asset.Name. It returns an error when the checksums asset is missing, the
// filename is not listed, or the digest does not match.
func verifyAssetChecksum(
	ctx context.Context,
	client HttpClient,
	rel *release,
	asset *releaseAsset,
	assetPath string,
) error {
	checksumsAsset := rel.findAssetByName(checksumsAssetName)
	if checksumsAsset == nil {
		return fmt.Errorf(
			"missing %s asset in the release; refusing to update without a checksum",
			checksumsAssetName,
		)
	}

	checksumsPath := assetPath + ".checksums.txt"
	if err := downloadFile(ctx, client, checksumsAsset.DownloadUrl, checksumsPath); err != nil {
		return fmt.Errorf("failed to download %s: %w", checksumsAssetName, err)
	}
	defer os.Remove(checksumsPath)

	raw, err := os.ReadFile(checksumsPath)
	if err != nil {
		return err
	}

	expected, ok := findChecksum(raw, asset.Name)
	if !ok {
		return fmt.Errorf(
			"checksums.txt does not list %q; refusing to update",
			asset.Name,
		)
	}

	actual, err := sha256HexFile(assetPath)
	if err != nil {
		return fmt.Errorf("failed to hash downloaded asset: %w", err)
	}

	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf(
			"checksum mismatch for %q (expected %s, got %s); refusing to replace the executable",
			asset.Name,
			expected,
			actual,
		)
	}

	return nil
}

// findChecksum parses a goreleaser checksums file ("<hex digest>  <filename>")
// and returns the digest for the given filename (compared by the trailing
// whitespace-separated token, ignoring any leading "./").
func findChecksum(raw []byte, filename string) (string, bool) {
	filename = strings.TrimPrefix(filename, "./")

	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		if strings.TrimPrefix(fields[1], "./") != filename {
			continue
		}

		return strings.ToLower(fields[0]), true
	}

	return "", false
}

// sha256HexFile returns the lowercase hex sha256 digest of the file at path.
func sha256HexFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func archiveSuffix(goos, goarch string) string {
	switch goos {
	case "linux":
		switch goarch {
		case "amd64":
			return "_linux_amd64.zip"
		case "arm64":
			return "_linux_arm64.zip"
		case "arm":
			return "_linux_armv7.zip"
		}
	case "darwin":
		switch goarch {
		case "amd64":
			return "_darwin_amd64.zip"
		case "arm64":
			return "_darwin_arm64.zip"
		}
	case "windows":
		switch goarch {
		case "amd64":
			return "_windows_amd64.zip"
		case "arm64":
			return "_windows_arm64.zip"
		}
	}

	return ""
}

func compareVersions(a, b string) int {
	aSplit := strings.Split(a, ".")
	aTotal := len(aSplit)

	bSplit := strings.Split(b, ".")
	bTotal := len(bSplit)

	limit := aTotal
	if bTotal > aTotal {
		limit = bTotal
	}

	for i := 0; i < limit; i++ {
		var x, y int

		if i < aTotal {
			x, _ = strconv.Atoi(aSplit[i])
		}

		if i < bTotal {
			y, _ = strconv.Atoi(bSplit[i])
		}

		if x < y {
			return 1 // b is newer
		}

		if x > y {
			return -1 // a is newer
		}
	}

	return 0 // equal
}

// note: not completely reliable as it may not work on all platforms
// but should at least provide a warning for the most common use cases
func isMaybeRunningInDocker() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

// note: untested
func isMaybeRunningInNixOS() bool {
	_, err := os.Stat("/etc/NIXOS")
	return err == nil
}
