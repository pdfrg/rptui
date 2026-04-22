package smad

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const detectorScript = `#!/usr/bin/env python3
import os
import sys
import json
import numpy as np
import librosa
import torch
import torchvision.transforms as T
import torchaudio


sr = 16000
n_fft = 1024
hop_size = 512
n_features = 128
duration = 20
confidence_threshold = 0.65


class PCENTransform(torch.nn.Module):
    def __init__(self, alpha=0.98, delta=2, root=2, eps=1e-6):
        super().__init__()
        self.alpha = alpha
        self.delta = delta
        self.root = root
        self.eps = eps

    def forward(self, x):
        x = x.squeeze(1)
        smooth = x.clone()
        for t in range(1, x.shape[-1]):
            smooth[..., t] = (1 - self.alpha) * x[..., t] + self.alpha * smooth[
                ..., t - 1
            ]
        x = (
            x / (self.eps + smooth) ** self.alpha + self.delta
        ) ** self.root - self.delta**self.root
        return x.unsqueeze(1)


class CRNN(torch.nn.Module):
    def __init__(self):
        super().__init__()
        self.cnn = torch.nn.Sequential(
            torch.nn.Conv2d(1, 64, kernel_size=(3, 3), padding=1),
            torch.nn.ReLU(),
            torch.nn.MaxPool2d(kernel_size=(2, 2)),
            torch.nn.Conv2d(64, 128, kernel_size=(3, 3), padding=1),
            torch.nn.ReLU(),
            torch.nn.MaxPool2d(kernel_size=(2, 2)),
        )
        self.gru = torch.nn.GRU(128 * 32, 128, bidirectional=True, batch_first=True)
        self.fc = torch.nn.Linear(256, 2)

    def forward(self, x):
        x = x.unsqueeze(1)
        x = self.cnn(x)
        x = x.permute(0, 3, 1, 2).flatten(2)
        x, _ = self.gru(x)
        x = self.fc(x)
        return x.permute(0, 2, 1)


def mono_check(audio):
    if audio.ndim == 1:
        return audio.unsqueeze(0)
    if audio.shape[0] == 2:
        return audio.mean(0, keepdim=True)
    return audio


def detect_speech(audio_path, model_path):
    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    model = CRNN()
    checkpoint = torch.load(model_path, map_location=device, weights_only=True)
    model.load_state_dict(checkpoint)
    model.to(device)
    model.eval()

    pcen_transform = T.Compose(
        [
            torchaudio.transforms.MelSpectrogram(
                sample_rate=sr, n_fft=n_fft, hop_length=hop_size, n_mels=128
            ),
            PCENTransform(),
        ]
    ).to(device)

    y, _ = librosa.load(audio_path, sr=sr, mono=True)
    audio = torch.from_numpy(np.expand_dims(y, 0)).float().to(device)
    audio = mono_check(audio)

    audio_pcen = pcen_transform(audio)
    c_size = int(sr / hop_size * duration)
    n_chunk = int(np.ceil(audio_pcen.shape[-1] / c_size))

    est_label = []
    with torch.inference_mode():
        for i in range(n_chunk):
            chunk = audio_pcen[..., i * c_size : (i + 1) * c_size]
            la = model(chunk).detach().cpu()
            est_label.append(la)

    est_label = torch.cat(est_label, -1)
    est_label = torch.sigmoid(est_label)
    est_label = torch.max_pool1d(est_label, 6, 6)
    frame_time = 1 / ((sr / hop_size) / 6)
    est_label = est_label.detach().cpu().numpy()[0]

    speech_regions = []
    active = False
    start = 0.0

    for i, frame in enumerate(est_label.T):
        speech_prob = float(frame[1])
        t = frame_time * i

        if speech_prob >= confidence_threshold and not active:
            active = True
            start = t
        elif speech_prob < confidence_threshold and active:
            active = False
            speech_regions.append((start, t))

    if active:
        speech_regions.append((start, len(est_label.T) * frame_time))

    if not speech_regions:
        return {
            "has_speech": False,
            "speech_start": 0.0,
            "speech_end": 0.0,
            "confidence": 0.0,
        }

    # Merge overlapping or adjacent regions
    merged = []
    for region in sorted(speech_regions):
        if not merged:
            merged.append(list(region))
        else:
            last = merged[-1]
            if region[0] <= last[1] + 0.5:
                last[1] = max(last[1], region[1])
            else:
                merged.append(list(region))

    # Find the largest speech segment
    largest = max(merged, key=lambda x: x[1] - x[0])
    max_conf = np.max(est_label[1, :])

    return {
        "has_speech": True,
        "speech_start": largest[0],
        "speech_end": largest[1],
        "confidence": float(max_conf),
    }


if __name__ == "__main__":
    if len(sys.argv) < 3:
        print(json.dumps({"error": "Usage: detector.py <audio_path> <model_path>"}))
        sys.exit(1)

    audio_path = sys.argv[1]
    model_path = sys.argv[2]

    try:
        result = detect_speech(audio_path, model_path)
        print(json.dumps(result))
    except Exception as e:
        print(json.dumps({"error": str(e)}))
        sys.exit(1)
`

const (
	modelURL      = "https://github.com/biboamy/TVSM-dataset/raw/refs/heads/master/Models/TVSM-cuesheet/epoch=10-step=4058.ckpt"
	modelFile     = "epoch=10-step=4058.ckpt"
	modelsDirName = "tvsm_models"
)

// Setup creates an isolated Python virtual environment, installs dependencies, and downloads the TVSM model
func Setup(scriptPath, cacheDir string) error {
	venvDir := filepath.Join(cacheDir, "env")

	// Check if virtual environment already exists
	if _, err := os.Stat(venvDir); err == nil {
		fmt.Println("Virtual environment found.")
		// Verify Python is still available
		if _, err := exec.LookPath("python3"); err != nil {
			return fmt.Errorf("python3 not found: %w", err)
		}
		// Verify Python packages are installed
		pythonExec := filepath.Join(venvDir, getBinDir(), "python")
		if runtime.GOOS == "windows" {
			pythonExec += ".exe"
		}
		cmd := exec.Command(pythonExec, "-c", "import torch, torchvision, torchaudio, librosa, numpy; print('Dependencies OK')")
		if err := cmd.Run(); err != nil {
			fmt.Println("Python dependencies missing or incomplete, will reinstall.")
		} else {
			// Verify model files exist
			if modelDir := filepath.Join(cacheDir, modelsDirName); modelExists(modelDir) {
				fmt.Println("TVSM model already downloaded. Setup is complete.")
				return nil
			}
			fmt.Println("Model files missing, will re-download.")
		}
	} else {
		fmt.Println("Creating virtual environment...")
		// Create parent cache dir if needed
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			return fmt.Errorf("failed to create cache dir: %w", err)
		}
	}

	// Create virtual environment
	if _, err := os.Stat(venvDir); os.IsNotExist(err) {
		fmt.Println("Creating virtual environment...")
		pythonPath := "python3"
		cmd := exec.Command(pythonPath, "-m", "venv", venvDir)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			// Fallback for systems without python3 module
			if runtime.GOOS == "windows" {
				cmd = exec.Command("py", "-m", "venv", venvDir)
			} else {
				return fmt.Errorf("failed to create venv: %w (try installing python3-venv)", err)
			}
		}
		fmt.Println("Virtual environment created.")
	}

	// Download TVSM model
	fmt.Println("Downloading TVSM model (~11MB)...")
	if err := downloadAndExtractModels(cacheDir); err != nil {
		return fmt.Errorf("failed to download TVSM model: %w", err)
	}
	fmt.Println("Model downloaded successfully.")

	// Copy detector script to cache dir
	fmt.Println("Setting up detector script...")
	detectorPath := filepath.Join(cacheDir, "smad", "detector.py")
	if err := os.MkdirAll(filepath.Dir(detectorPath), 0755); err != nil {
		return fmt.Errorf("failed to create smad dir: %w", err)
	}
	if err := os.WriteFile(detectorPath, []byte(detectorScript), 0755); err != nil {
		return fmt.Errorf("failed to write detector script: %w", err)
	}
	fmt.Println("Detector script ready.")

	// Activate venv and install dependencies
	pipPath := filepath.Join(venvDir, getBinDir(), "pip")
	if runtime.GOOS == "windows" {
		pipPath += ".exe"
	}

	// Install uv first
	fmt.Println("Installing package manager...")
	cmd := exec.Command(pipPath, "install", "uv")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install uv: %w", err)
	}

	// Install required packages using pip
	fmt.Println("WARNING: Installing required Python packages (~2.5GB download, this may take 10-20 minutes)...")
	fmt.Println("This will download PyTorch and audio processing libraries. Ensure you have sufficient disk space and internet connection.")
	cmd = exec.Command(pipPath, "install", "torch", "torchvision", "torchaudio", "librosa", "numpy")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install required packages: %w", err)
	}
	fmt.Println("Python packages installed.")

	return nil
}

// modelExists checks if TVSM model directory exists and contains expected files
func modelExists(modelDir string) bool {
	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		return false
	}
	// Check for expected model files (models from TVSM dataset)
	expectedFiles := []string{
		"TVSM-cuesheet/Models/" + modelFile,
	}
	for _, f := range expectedFiles {
		if _, err := os.Stat(filepath.Join(modelDir, f)); os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// downloadAndExtractModels downloads the TVSM model file
func downloadAndExtractModels(cacheDir string) error {
	modelsDir := filepath.Join(cacheDir, modelsDirName)
	modelPath := filepath.Join(modelsDir, "TVSM-cuesheet", "Models", modelFile)

	if fileExists(modelPath) {
		return nil
	}

	// Create model directory
	if err := os.MkdirAll(filepath.Dir(modelPath), 0755); err != nil {
		return fmt.Errorf("failed to create model dir: %w", err)
	}

	// Download the model file directly
	resp, err := http.Get(modelURL)
	if err != nil {
		return fmt.Errorf("failed to download model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download model: server returned %d", resp.StatusCode)
	}

	outFile, err := os.Create(modelPath)
	if err != nil {
		return fmt.Errorf("failed to create model file: %w", err)
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, resp.Body); err != nil {
		return fmt.Errorf("failed to write model file: %w", err)
	}

	return nil
}

// createPlaceholderModels creates the expected model directory structure
// with minimal valid files for testing
func createPlaceholderModels(modelsDir string) error {
	subdirs := []string{
		"TVSM-cuesheet/Models",
		"TVSM-pseudo/Models",
	}
	for _, dir := range subdirs {
		fullDir := filepath.Join(modelsDir, dir)
		if err := os.MkdirAll(fullDir, 0755); err != nil {
			return err
		}
	}

	// Create minimal placeholder model files
	// In production, these would be actual downloaded model files
	placeholderContent := []byte("placeholder_model_file")
	models := []string{
		"TVSM-cuesheet/Models/TVSM-cuesheet.ckpt",
		"TVSM-pseudo/Models/TVSM-pseudo.ckpt",
	}
	for _, model := range models {
		path := filepath.Join(modelsDir, model)
		if err := os.WriteFile(path, placeholderContent, 0644); err != nil {
			return err
		}
	}

	// Also create config files used by the detector
	configs := map[string]string{
		"TVSM-cuesheet/hparams.yaml": "model_name: tvsm_cuesheet\nn_features: 128\nn_class: 2",
		"TVSM-pseudo/hparams.yaml":   "model_name: tvsm_pseudo\nn_features: 128\nn_class: 2",
	}
	for path, content := range configs {
		fullPath := filepath.Join(modelsDir, path)
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return err
		}
	}

	return nil
}

func getBinDir() string {
	if runtime.GOOS == "windows" {
		return "Scripts"
	}
	return "bin"
}
