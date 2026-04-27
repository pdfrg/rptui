package smad

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/pdfrg/rptui/internal/loginit"
)

var dlog = loginit.InitLogger("[SMAD-DL] ")

var bitratePattern = regexp.MustCompile(`/x/\d+/(\d+)/g/`)

const detectionBitrate = "1"

// DetectionURL converts a RadioParadise gapless URL to its 64k AAC equivalent
// for speech detection downloads. The 64k file is ~2.2MB vs ~11MB for 320k or
// ~27MB for FLAC, while still being perfectly adequate for speech detection.
//
// Examples:
//
//	.../x/1534/3/g/1534-1.m4a  →  .../x/1534/1/g/1534-1.m4a
//	.../x/1534/4/g/1534-1.flac →  .../x/1534/1/g/1534-1.m4a
func DetectionURL(gaplessURL string) (string, error) {
	if !strings.HasPrefix(gaplessURL, "http://") && !strings.HasPrefix(gaplessURL, "https://") {
		return "", fmt.Errorf("not an HTTP URL: %s", gaplessURL)
	}

	m := bitratePattern.FindStringSubmatchIndex(gaplessURL)
	if m == nil {
		return "", fmt.Errorf("cannot parse bitrate from URL: %s", gaplessURL)
	}

	replaced := gaplessURL[:m[2]] + detectionBitrate + gaplessURL[m[3]:]

	lastDot := strings.LastIndex(replaced, ".")
	if lastDot == -1 {
		return "", fmt.Errorf("no file extension in URL: %s", replaced)
	}
	replaced = replaced[:lastDot+1] + "m4a"

	return replaced, nil
}

// DownloadAudioFile downloads a URL to a temporary file in dir and returns the
// file path. The caller is responsible for deleting the file when done.
func DownloadAudioFile(ctx context.Context, url, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "rptui/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp(dir, "dj-*.m4a")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	_, err = io.Copy(tmpFile, resp.Body)
	tmpFile.Close()
	if err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("download write failed: %w", err)
	}

	dlog.Printf("Downloaded %s → %s (%d bytes)", url, tmpPath, resp.ContentLength)

	return tmpPath, nil
}

// CleanupStaleTempFiles removes temp files older than maxAge from dir.
func CleanupStaleTempFiles(dir string, maxAge time.Duration) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			dlog.Printf("Failed to read temp dir %s: %v", dir, err)
		}
		return
	}

	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if strings.HasPrefix(entry.Name(), "dj-") && info.ModTime().Before(cutoff) {
			p := filepath.Join(dir, entry.Name())
			os.Remove(p)
			dlog.Printf("Cleaned stale temp file: %s", p)
		}
	}
}

// TmpDir returns the path for DJ detection temporary audio files.
func TmpDir(cacheDir string) string {
	return filepath.Join(cacheDir, "smad", "tmp")
}
