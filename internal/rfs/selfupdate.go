package rfs

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Asset is a single downloadable file attached to a GitHub release: the
// rfs_<version>_<os>_<arch>.tar.gz archive or the checksums.txt file the
// self-update tick verifies against.
type Asset struct {
	Name string
	URL  string
}

// Release is the latest GitHub release as the self-update tick sees it.
type Release struct {
	// Version is the release tag with any leading "v" stripped.
	Version string
	Assets  []Asset
}

// isNewerVersion reports whether latest is a newer release than current.
// Versions are UTC build timestamps (semver-shaped YYYY.MD.HMS, per the release
// workflow), compared field-by-field as numbers left to right — so a later
// timestamp is always newer. A leading "v" is tolerated but stripped (unused
// sentinel is older than any real release for callers that ask the comparison;
// cmd/rfs deliberately skips constructing an updater for dev builds.
func isNewerVersion(current, latest string) bool {
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")
	if current == "dev" {
		return latest != "dev"
	}
	if latest == "dev" {
		return false
	}
	return compareVersions(current, latest) < 0
}

// compareVersions returns -1, 0, or +1 as a is less than, equal to, or
// greater than b, comparing dot-separated numeric fields left to right. A
// shorter version padded with zeros (1.2 == 1.2.0). Non-numeric trailing
// segments (pre-release suffixes) are compared as strings and rank below a
// numeric segment of the same position.
func compareVersions(a, b string) int {
	af := strings.Split(a, ".")
	bf := strings.Split(b, ".")
	n := len(af)
	if len(bf) > n {
		n = len(bf)
	}
	for i := 0; i < n; i++ {
		as := ""
		bs := ""
		if i < len(af) {
			as = af[i]
		}
		if i < len(bf) {
			bs = bf[i]
		}
		if ai, aerr := atoiSafe(as); aerr == nil {
			if bi, berr := atoiSafe(bs); berr == nil {
				switch {
				case ai < bi:
					return -1
				case ai > bi:
					return 1
				}
				continue
			}
			// a is numeric, b is not: numeric ranks higher.
			return 1
		}
		if _, berr := atoiSafe(bs); berr == nil {
			return -1 // b is numeric, a is not: a ranks lower.
		}
		if as != bs {
			if as < bs {
				return -1
			}
			return 1
		}
	}
	return 0
}

func atoiSafe(s string) (int, error) {
	if s == "" {
		return 0, errEmptyInt
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errEmptyInt
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

var errEmptyInt = errors.New("not a non-negative integer")

// selectAsset finds the release archive matching goos/goarch by its name
// suffix (_<goos>_<goarch>.tar.gz), the naming convention .goreleaser.yaml
// produces. It returns ok=false if no archive matches the running platform.
func selectAsset(assets []Asset, goos, goarch string) (Asset, bool) {
	suffix := "_" + goos + "_" + goarch + ".tar.gz"
	for _, a := range assets {
		if strings.HasSuffix(a.Name, suffix) {
			return a, true
		}
	}
	return Asset{}, false
}

// findAssetByName locates an asset by exact name, used to find checksums.txt.
func findAssetByName(assets []Asset, name string) (Asset, bool) {
	for _, a := range assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

// verifyChecksum confirms archive matches the entry for filename in
// checksumsTxt (sha256sum format: "<hex>  <filename>"). It hashes the archive
// and compares in constant time. It returns an error if filename is absent or
// the hash differs — the security gate that stops a tampered or replaced
// archive from being swapped in.
func verifyChecksum(archive []byte, checksumsTxt, filename string) error {
	want, ok := checksumForFile(checksumsTxt, filename)
	if !ok {
		return fmt.Errorf("self-update: no checksum entry for %s", filename)
	}
	sum := sha256.Sum256(archive)
	got := hex.EncodeToString(sum[:])
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		return fmt.Errorf("self-update: checksum mismatch for %s", filename)
	}
	return nil
}

// checksumForFile parses sha256sum-format lines ("<hex>  <name>") and returns
// the hex hash for name. The leading char of the name field may be a binary/text
// indicator (' ' or '*'); it is stripped.
func checksumForFile(checksumsTxt, filename string) (string, bool) {
	for _, line := range strings.Split(checksumsTxt, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		hash := fields[0]
		name := fields[len(fields)-1]
		if name == filename {
			return hash, true
		}
	}
	return "", false
}

// extractBinaryFromArchive reads a verified goreleaser tar.gz archive and
// returns the bytes of the single "rfs" binary inside it. The archive is
// already checksum-verified before this runs, so the binary is authentic.
func extractBinaryFromArchive(archive []byte) ([]byte, error) {
	gr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("self-update: read archive gzip: %w", err)
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("self-update: archive has no rfs entry")
		}
		if err != nil {
			return nil, fmt.Errorf("self-update: read archive: %w", err)
		}
		if hdr.Name == "rfs" {
			b, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("self-update: read rfs entry: %w", err)
			}
			return b, nil
		}
	}
}

// ReleaseSource fetches the latest release metadata. The real implementation
// hits the GitHub releases API; tests fake it.
type ReleaseSource interface {
	Latest(ctx context.Context) (Release, error)
}

// AssetDownloader fetches a release asset's bytes by URL. The real
// implementation uses HTTP over HTTPS-only; tests fake it.
type AssetDownloader interface {
	Download(ctx context.Context, url string) ([]byte, error)
}

// BinarySwapper atomically replaces the running binary with newBinary. The
// real implementation renames the old binary aside (.bak) and the new into
// place; tests fake it to observe what would have been swapped in.
type BinarySwapper interface {
	Swap(newBinary []byte) error
}

// Updater runs one self-update check: fetch the latest release, and if it is
// newer than Current, download the matching archive and checksums.txt,
// verify the archive, extract the rfs binary, and swap it in. Check returns a
// result describing the observed latest release and whether a binary was
// swapped.
type Updater struct {
	Current      string
	GOOS, GOARCH string
	Source       ReleaseSource
	Downloader   AssetDownloader
	Swapper      BinarySwapper
}

// UpdateCheckResult describes the outcome of one self-update check.
type UpdateCheckResult struct {
	Current string
	Latest  string
	Applied bool
}

func (u Updater) Check(ctx context.Context) (UpdateCheckResult, error) {
	result := UpdateCheckResult{Current: u.Current}
	rel, err := u.Source.Latest(ctx)
	if err != nil {
		return result, fmt.Errorf("self-update: fetch latest release: %w", err)
	}
	result.Latest = rel.Version
	if !isNewerVersion(u.Current, rel.Version) {
		return result, nil
	}
	chkAsset, ok := findAssetByName(rel.Assets, "checksums.txt")
	if !ok {
		return result, fmt.Errorf("self-update: release %s has no checksums.txt", rel.Version)
	}
	archAsset, ok := selectAsset(rel.Assets, u.GOOS, u.GOARCH)
	if !ok {
		return result, fmt.Errorf("self-update: release %s has no archive for %s/%s", rel.Version, u.GOOS, u.GOARCH)
	}
	checksums, err := u.Downloader.Download(ctx, chkAsset.URL)
	if err != nil {
		return result, fmt.Errorf("self-update: download checksums: %w", err)
	}
	archive, err := u.Downloader.Download(ctx, archAsset.URL)
	if err != nil {
		return result, fmt.Errorf("self-update: download archive: %w", err)
	}
	if err := verifyChecksum(archive, string(checksums), archAsset.Name); err != nil {
		return result, err // a tampered archive must never reach the swapper
	}
	binary, err := extractBinaryFromArchive(archive)
	if err != nil {
		return result, err
	}
	if err := u.Swapper.Swap(binary); err != nil {
		return result, fmt.Errorf("self-update: swap binary: %w", err)
	}
	result.Applied = true
	return result, nil
}

// GitHubReleaseSource fetches the latest release from the GitHub API and maps
// it to a Release with the leading "v" stripped from the tag. repo is
// "owner/name".
type GitHubReleaseSource struct {
	repo   string
	client *http.Client
}

func NewGitHubReleaseSource(repo string) GitHubReleaseSource {
	return GitHubReleaseSource{repo: repo, client: &http.Client{Timeout: 30 * time.Second}}
}

func (g GitHubReleaseSource) Latest(ctx context.Context) (Release, error) {
	url := "https://api.github.com/repos/" + g.repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "rfs-self-update")
	resp, err := g.client.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("github releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return Release{}, nil // no releases yet — nothing to update to
	}
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("github releases: unexpected status %s", resp.Status)
	}
	var body struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Release{}, fmt.Errorf("github releases: decode: %w", err)
	}
	rel := Release{Version: strings.TrimPrefix(body.TagName, "v")}
	for _, a := range body.Assets {
		rel.Assets = append(rel.Assets, Asset{Name: a.Name, URL: a.BrowserDownloadURL})
	}
	return rel, nil
}

// HTTPSDownloader fetches asset bytes, refusing any non-https URL so a
// checksum-verified file cannot be downgraded to plain HTTP in transit.
type HTTPSDownloader struct {
	client *http.Client
}

func NewHTTPSDownloader() HTTPSDownloader {
	return HTTPSDownloader{client: &http.Client{Timeout: 5 * time.Minute}}
}

func (d HTTPSDownloader) Download(ctx context.Context, url string) ([]byte, error) {
	if !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("self-update: refusing non-https asset URL: %s", url)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "rfs-self-update")
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: unexpected status %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxAssetBytes))
}

// FileSwapper replaces the running binary on disk atomically: the old binary
// is renamed aside to .bak (manual rollback per ADR 0004) and the new bytes
// are written to a temp file in the same directory then renamed into place.
type FileSwapper struct {
	// path is the running binary's path (os.Executable()). Overridden in tests.
	path string
}

func NewFileSwapper() (FileSwapper, error) {
	p, err := os.Executable()
	if err != nil {
		return FileSwapper{}, fmt.Errorf("self-update: locate running binary: %w", err)
	}
	return FileSwapper{path: p}, nil
}

// Path returns the binary install path captured before any swap. Use this path
// for re-exec after Swap: on Linux, a fresh os.Executable() after the rename can
// point at the .bak backup (the old binary), not the newly installed file.
func (s FileSwapper) Path() string { return s.path }

func (s FileSwapper) Swap(newBinary []byte) error {
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".rfs-new-")
	if err != nil {
		return fmt.Errorf("create temp binary: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once renamed away
	if _, err := tmp.Write(newBinary); err != nil {
		tmp.Close()
		return fmt.Errorf("write new binary: %w", err)
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod new binary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp binary: %w", err)
	}
	bak := s.path + ".bak"
	os.Remove(bak) // best-effort: clear any previous backup
	if err := os.Rename(s.path, bak); err != nil {
		return fmt.Errorf("set aside old binary: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		// Try to restore the old binary so the box isn't left without one.
		_ = os.Rename(bak, s.path)
		return fmt.Errorf("install new binary: %w", err)
	}
	return nil
}

const maxAssetBytes = 256 << 20 // 256 MiB: a generous cap on a release archive or checksums file.
