package rfs

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"runtime"
	"strings"

	"github.com/Masterminds/semver/v3"
	selfupdate "github.com/creativeprojects/go-selfupdate"
)

// selfUpdateClient adapts go-selfupdate's GitHub release discovery and asset
// downloads to the small seam consumed by Updater. The workflow deliberately
// stays rfs-owned: it enforces the no-downgrade rule, validates checksums,
// limits downloads, extracts the expected binary, and performs the durable
// .bak swap.
type selfUpdateClient struct {
	source     releaseAssetDownloader
	updater    *selfupdate.Updater
	repository selfupdate.Repository
}

// releaseAssetDownloader is the small portion of go-selfupdate's GitHub source
// used after release discovery. Keeping it narrow makes rfs's HTTPS and size
// limits independently testable.
type releaseAssetDownloader interface {
	DownloadReleaseAsset(context.Context, *selfupdate.Release, int64) (io.ReadCloser, error)
}

func newSelfUpdateClient() (ReleaseClient, error) {
	source, err := selfupdate.NewGitHubSource(selfupdate.GitHubConfig{})
	if err != nil {
		return nil, fmt.Errorf("self-update: configure GitHub source: %w", err)
	}
	updater, err := selfupdate.NewUpdater(selfupdate.Config{
		Source:    source,
		Validator: &selfupdate.ChecksumValidator{UniqueFilename: "checksums.txt"},
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	})
	if err != nil {
		return nil, fmt.Errorf("self-update: configure release updater: %w", err)
	}
	return &selfUpdateClient{
		source:     source,
		updater:    updater,
		repository: selfupdate.ParseSlug(selfUpdateRepo),
	}, nil
}

func (c *selfUpdateClient) Latest(ctx context.Context) (ReleaseCandidate, bool, error) {
	release, found, err := c.updater.DetectLatest(ctx, c.repository)
	if err != nil {
		return ReleaseCandidate{}, false, err
	}
	if !found || release == nil {
		return ReleaseCandidate{}, false, nil
	}
	return ReleaseCandidate{
		Version:         release.Version(),
		ArchiveName:     release.AssetName,
		ArchiveID:       release.AssetID,
		ChecksumsID:     release.ValidationAssetID,
		providerRelease: release,
	}, true, nil
}

func (c *selfUpdateClient) Download(ctx context.Context, candidate ReleaseCandidate, assetID int64) ([]byte, error) {
	release, ok := candidate.providerRelease.(*selfupdate.Release)
	if !ok || release == nil {
		return nil, fmt.Errorf("self-update: release %s has no provider handle", candidate.Version)
	}

	assetURL := ""
	switch assetID {
	case release.AssetID:
		assetURL = release.AssetURL
	case release.ValidationAssetID:
		assetURL = release.ValidationAssetURL
	default:
		return nil, fmt.Errorf("self-update: asset %d is not part of release %s", assetID, candidate.Version)
	}
	if !isHTTPSAssetURL(assetURL) {
		return nil, fmt.Errorf("self-update: refuse non-HTTPS asset URL %q", assetURL)
	}

	reader, err := c.source.DownloadReleaseAsset(ctx, release, assetID)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	data, err := readUpdateAsset(reader, maxAssetBytes)
	if err != nil {
		return nil, fmt.Errorf("self-update: read asset %q: %w", assetURL, err)
	}
	return data, nil
}

func isHTTPSAssetURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	return err == nil && parsed.Scheme == "https" && parsed.Host != ""
}

func readUpdateAsset(reader io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("download limit must be positive, got %d", limit)
	}
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("asset exceeds %d byte download limit", limit)
	}
	return data, nil
}

func validateReleaseChecksum(filename string, release, checksums []byte) error {
	validator := selfupdate.ChecksumValidator{UniqueFilename: "checksums.txt"}
	return validator.Validate(filename, release, checksums)
}

// isNewerVersion is intentionally kept small now that go-selfupdate has
// parsed and selected a semver release for us. Invalid build metadata is not
// eligible for an automatic replacement rather than being allowed to panic or
// accidentally trigger a downgrade.
func isNewerVersion(current, latest string) bool {
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")
	if current == "dev" {
		return latest != "dev"
	}
	if latest == "dev" {
		return false
	}

	currentVersion, err := semver.NewVersion(current)
	if err != nil {
		return false
	}
	latestVersion, err := semver.NewVersion(latest)
	if err != nil {
		return false
	}
	return currentVersion.LessThan(latestVersion)
}

// Keep the adapter honest if the upstream library changes its API: these are
// the exact provider contracts used above.
var _ ReleaseClient = (*selfUpdateClient)(nil)
