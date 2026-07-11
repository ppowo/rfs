package rfs

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	selfupdate "github.com/creativeprojects/go-selfupdate"
)

type fakeReleaseAssetDownloader struct {
	body  io.ReadCloser
	err   error
	calls int
}

func (f *fakeReleaseAssetDownloader) DownloadReleaseAsset(context.Context, *selfupdate.Release, int64) (io.ReadCloser, error) {
	f.calls++
	return f.body, f.err
}

func TestSelfUpdateClientRefusesNonHTTPSAssetBeforeDownload(t *testing.T) {
	downloader := &fakeReleaseAssetDownloader{}
	client := selfUpdateClient{source: downloader}
	candidate := ReleaseCandidate{
		Version:   "0.2.0",
		ArchiveID: 1,
		providerRelease: &selfupdate.Release{
			AssetID:  1,
			AssetURL: "http://example.test/rfs_0.2.0_linux_amd64.tar.gz",
		},
	}

	_, err := client.Download(t.Context(), candidate, candidate.ArchiveID)
	if err == nil {
		t.Fatal("non-HTTPS asset URL was accepted")
	}
	if downloader.calls != 0 {
		t.Fatalf("downloader calls = %d, want 0 for a non-HTTPS URL", downloader.calls)
	}
}

func TestSelfUpdateClientDownloadsHTTPSAsset(t *testing.T) {
	downloader := &fakeReleaseAssetDownloader{body: io.NopCloser(strings.NewReader("archive"))}
	client := selfUpdateClient{source: downloader}
	candidate := ReleaseCandidate{
		Version:   "0.2.0",
		ArchiveID: 1,
		providerRelease: &selfupdate.Release{
			AssetID:  1,
			AssetURL: "https://example.test/rfs_0.2.0_linux_amd64.tar.gz",
		},
	}

	got, err := client.Download(t.Context(), candidate, candidate.ArchiveID)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if !bytes.Equal(got, []byte("archive")) {
		t.Fatalf("downloaded bytes = %q, want %q", got, "archive")
	}
	if downloader.calls != 1 {
		t.Fatalf("downloader calls = %d, want 1", downloader.calls)
	}
}

func TestReadUpdateAssetRejectsDataOverLimit(t *testing.T) {
	_, err := readUpdateAsset(strings.NewReader("four"), 3)
	if err == nil {
		t.Fatal("asset larger than its configured limit was accepted")
	}
}
