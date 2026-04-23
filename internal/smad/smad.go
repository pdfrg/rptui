package smad

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// DetectionResult mirrors the Python script's JSON output.
type DetectionResult struct {
	HasSpeech   bool    `toml:"has_speech" json:"has_speech"`
	SpeechStart float64 `toml:"speech_start" json:"speech_start"`
	SpeechEnd   float64 `toml:"speech_end" json:"speech_end"`
	Confidence  float64 `toml:"confidence" json:"confidence"`
}

type Availability struct {
	Available bool
	Reason    string
}

type cachedResult struct {
	result   DetectionResult
	cachedAt time.Time
}

// DJChecker wraps the Python TVSM detection script with caching.
type DJChecker struct {
	pythonPath   string
	scriptPath   string
	modelPath    string
	cacheDir     string
	mu           sync.Mutex
	availability *Availability
}

func NewDJChecker(pythonPath, scriptPath, modelPath, cacheDir string) *DJChecker {
	return &DJChecker{
		pythonPath: pythonPath,
		scriptPath: scriptPath,
		modelPath:  modelPath,
		cacheDir:   cacheDir,
	}
}

func (d *DJChecker) checkAvailability() *Availability {
	if _, err := exec.LookPath(d.pythonPath); err != nil {
		return &Availability{Available: false, Reason: "python not found in PATH"}
	}
	if _, err := filepath.Abs(d.scriptPath); err != nil || !fileExists(d.scriptPath) {
		return &Availability{Available: false, Reason: "detector script not found"}
	}
	return &Availability{Available: true, Reason: "ok"}
}

func (d *DJChecker) Availability() *Availability {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.availability == nil {
		d.availability = d.checkAvailability()
	}
	return d.availability
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Detect runs the Python TVSM script and returns (speechStart, speechEnd, confidence, hasSpeech).
// It caches results by song file path + mtime + threshold to avoid re-running on every playback.
func (d *DJChecker) Detect(ctx context.Context, songPath string, confidenceThreshold float64, checkSeconds int) (float64, float64, float64, bool, error) {
	avail := d.Availability()
	if !avail.Available {
		return 0, 0, 0, false, fmt.Errorf("DJ detection unavailable: %s", avail.Reason)
	}

	cacheKey, err := d.cacheKey(songPath, confidenceThreshold, checkSeconds)
	if err != nil {
		return 0, 0, 0, false, fmt.Errorf("failed to compute cache key: %w", err)
	}
	if cached, ok := d.loadCache(cacheKey); ok {
		return cached.result.SpeechStart, cached.result.SpeechEnd, cached.result.Confidence, cached.result.HasSpeech, nil
	}

	args := []string{
		d.scriptPath,
		songPath,
		d.modelPath,
		fmt.Sprintf("%.4f", confidenceThreshold),
		fmt.Sprintf("%d", checkSeconds),
	}
	cmd := exec.CommandContext(ctx, d.pythonPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, 0, 0, false, fmt.Errorf("python detection failed: %w, output: %s", err, string(out))
	}

	var result DetectionResult
	if err := json.Unmarshal(out, &result); err != nil {
		return 0, 0, 0, false, fmt.Errorf("failed to parse detection result: %w, raw: %s", err, string(out))
	}

	if err := d.saveCache(cacheKey, &result); err != nil {
		// non-critical: cache write failed, but we can still use the result
	}

	return result.SpeechStart, result.SpeechEnd, result.Confidence, result.HasSpeech, nil
}

func (d *DJChecker) cacheKey(path string, confidenceThreshold float64, checkSeconds int) (string, error) {
	keyInput := fmt.Sprintf("%s|%.4f|%d", path, confidenceThreshold, checkSeconds)
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		hash := sha256.Sum256([]byte(keyInput))
		return fmt.Sprintf("%x", hash), nil
	}
	cmd := exec.Command("sha256sum", path)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("sha256sum not available: %w", err)
	}
	cmd = exec.Command("stat", "--format=%Y", path)
	mtimeOut, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("stat not available: %w", err)
	}
	combined := append(append(out, mtimeOut...), []byte(keyInput)...)
	hash := sha256.Sum256(combined)
	return fmt.Sprintf("%x", hash), nil
}

func (d *DJChecker) saveCache(key string, result *DetectionResult) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	data, err := toml.Marshal(result)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d.cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache dir: %w", err)
	}
	return os.WriteFile(filepath.Join(d.cacheDir, key), data, 0644)
}

func (d *DJChecker) loadCache(key string) (*cachedResult, bool) {
	data, err := os.ReadFile(filepath.Join(d.cacheDir, key))
	if err != nil {
		return nil, false
	}
	var r DetectionResult
	if err := toml.Unmarshal(data, &r); err != nil {
		return nil, false
	}
	return &cachedResult{result: r, cachedAt: time.Now()}, true
}
