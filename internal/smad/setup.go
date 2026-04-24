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
# NOTE: The detect_speech logic in this file must be kept in sync with:
# - internal/smad/detector.py (canonical source)
# - internal/smad/test_detector.py (imports this module)
# Any changes to detection logic MUST be reflected in all three.
import os
import sys
import json
import warnings
import numpy as np
import librosa
import torch
import torch.nn as nn
import torchaudio

warnings.filterwarnings("ignore", message="PySoundFile failed")
warnings.filterwarnings("ignore", message=".*audioread.*", category=FutureWarning)
warnings.filterwarnings("ignore", message=".*get_duration.*", category=FutureWarning)


# Audio processing constants
sr = 16000
n_fft = 1024
hop_size = 512
n_features = 128
duration = 20

# Minimum speech duration (seconds) to count as a DJ segment after gap bridging
# DJ talk on Radio Paradise is typically 10s+ (William's slow, soothing style)
# Shorter speech bursts are typically sung "spoken vocals", not DJ talk
min_speech_duration = 10.0

# Maximum gap (seconds) between speech regions to bridge into one segment
# DJ speech often has brief musical interludes/station IDs that split detections
max_speech_gap = 2.5


class F2M(nn.Module):
	def __init__(
		self, n_mels=128, sr=16000, f_max=None, f_min=0.0, n_fft=1024, onesided=True
	):
		super().__init__()
		self.n_mels = n_mels
		self.sr = sr
		self.f_max = f_max if f_max is not None else sr // 2
		self.f_min = f_min
		self.n_fft = n_fft
		if onesided:
			self.n_fft = self.n_fft // 2 + 1
		self._init_buffers()

	def _init_buffers(self):
		m_min = 0.0 if self.f_min == 0 else 2595 * np.log10(1.0 + (self.f_min / 700))
		m_max = 2595 * np.log10(1.0 + (self.f_max / 700))
		m_pts = torch.linspace(m_min, m_max, self.n_mels + 2)
		f_pts = 700 * (10 ** (m_pts / 2595) - 1)
		bins = torch.floor(((self.n_fft - 1) * 2) * f_pts / self.sr).long()
		fb = torch.zeros(self.n_fft, self.n_mels)
		for m in range(1, self.n_mels + 1):
			f_m_minus = bins[m - 1].item()
			f_m = bins[m].item()
			f_m_plus = bins[m + 1].item()
			if f_m_minus != f_m:
				fb[f_m_minus:f_m, m - 1] = (
					torch.arange(f_m_minus, f_m) - f_m_minus
				).float() / (f_m - f_m_minus)
			if f_m != f_m_plus:
				fb[f_m:f_m_plus, m - 1] = torch.div(
					(float(f_m_plus) - torch.arange(f_m, f_m_plus)), (f_m_plus - f_m)
				)
		self.register_buffer("fb", fb)

	def forward(self, spec_f):
		spec_m = torch.matmul(spec_f, self.fb)
		return spec_m


def pcen(x, eps=1e-6, s=0.025, alpha=0.98, delta=2, r=0.5, training=False):
	frames = x.split(1, -2)
	m_frames = []
	last_state = None
	for frame in frames:
		if last_state is None:
			last_state = frame
			m_frames.append(frame)
			continue
		if training:
			m_frame = ((1 - s) * last_state) + (s * frame)
		else:
			m_frame = (1 - s) * last_state + s * frame
		last_state = m_frame
		m_frames.append(m_frame)
	M = torch.cat(m_frames, 1)
	pcen_ = (x / (M + eps).pow(alpha) + delta).pow(r) - delta**r
	return pcen_


class PCENTransform(nn.Module):
	def __init__(self):
		super().__init__()
		self.f2m = F2M(n_fft=1024, n_mels=128)

	def forward(self, x, is_mel=True):
		if not is_mel:
			x = torch.stft(x, n_fft=1024, hop_length=512).norm(dim=-1, p=2)
		x = self.f2m(x.permute(0, 2, 1))
		x = pcen(x, eps=1e-6, s=0.025, alpha=0.98, delta=2, r=0.5, training=False)
		return x


class CRNN(nn.Module):
	def __init__(self):
		super().__init__()
		self.c1 = nn.Sequential(
			nn.Conv2d(1, 64, 3, 1, 1),
			nn.ReLU(),
			nn.MaxPool2d((2, 1)),
			nn.BatchNorm2d(64),
		)
		self.c2 = nn.Sequential(
			nn.Conv2d(64, 64, 11, 1, 5),
			nn.ReLU(),
			nn.MaxPool2d((2, 1)),
			nn.BatchNorm2d(64),
		)
		self.c3 = nn.Sequential(
			nn.Conv2d(64, 16, 11, 1, 5),
			nn.ReLU(),
			nn.MaxPool2d((2, 1)),
			nn.BatchNorm2d(16),
		)
		self.lstm1 = nn.Sequential(
			nn.GRU(
				input_size=256,
				hidden_size=80,
				num_layers=1,
				bidirectional=True,
				batch_first=True,
			),
		)
		self.b1 = nn.BatchNorm1d(160)
		self.lstm2 = nn.Sequential(
			nn.GRU(
				input_size=160,
				hidden_size=40,
				num_layers=1,
				bidirectional=True,
				batch_first=True,
			)
		)
		self.b2 = nn.BatchNorm1d(80)
		self.last = nn.Linear(80, 2)

	def forward(self, x):
		x = x.unsqueeze(1)
		x = self.c1(x)
		x = self.c2(x)
		x = self.c3(x)
		x = self.b1(
			self.lstm1(x.reshape(x.shape[0], -1, x.shape[-1]).permute(0, 2, 1))[
				0
			].permute(0, 2, 1)
		)
		x = self.b2(self.lstm2(x.permute(0, 2, 1))[0].permute(0, 2, 1))
		x = self.last(x.permute(0, 2, 1))
		return x.permute(0, 2, 1)


def mono_check(audio):
	if audio.ndim == 1:
		return audio.unsqueeze(0)
	if audio.shape[0] == 2:
		return audio.mean(0, keepdim=True)
	return audio


def get_audio_duration(audio_path):
	try:
		full_duration = librosa.get_duration(path=audio_path)
		return float(full_duration)
	except Exception:
		return None


def bridge_regions(regions, max_speech_gap):
	bridged = []
	for region in sorted(regions, key=lambda x: x[0]):
		if not bridged:
			bridged.append(list(region))
		else:
			last = bridged[-1]
			if region[0] <= last[1] + max_speech_gap:
				new_weight = last[3] + region[3]
				last[2] = (last[2] * last[3] + region[2] * region[3]) / new_weight
				last[1] = max(last[1], region[1])
				last[3] = new_weight
			else:
				bridged.append(list(region))
	return bridged


def detect_speech(
	audio_path, model_path, confidence_threshold, check_seconds, min_speech_duration=5.0
):
	device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
	model = CRNN()
	checkpoint = torch.load(model_path, map_location=device, weights_only=True)
	model.load_state_dict(checkpoint)
	model.to(device)
	model.eval()

	mel_transform = torchaudio.transforms.MelSpectrogram(
		sample_rate=sr, n_fft=n_fft, hop_length=hop_size, n_mels=128
	).to(device)
	pcen_transform = PCENTransform().to(device)

	y, _ = librosa.load(audio_path, sr=sr, mono=True)
	audio_duration = len(y) / sr
	audio = torch.from_numpy(np.expand_dims(y, 0)).float().to(device)
	audio = mono_check(audio)

	audio_mel = mel_transform(audio)
	audio_pcen = pcen_transform(audio_mel)
	c_size = int(sr / hop_size * duration)

	# Determine which chunks overlap with boundary regions
	n_chunk = int(np.ceil(audio_pcen.shape[-1] / c_size))
	if check_seconds > 0 and audio_duration > check_seconds * 2:
		chunks_to_process = []
		for i in range(n_chunk):
			chunk_start_sec = i * duration
			chunk_end_sec = (i + 1) * duration
			in_boundary = (
				chunk_start_sec < check_seconds
				or chunk_end_sec > audio_duration - check_seconds
			)
			if in_boundary:
				chunks_to_process.append(i)
	else:
		chunks_to_process = list(range(n_chunk))

	if not chunks_to_process:
		return {
			"has_speech": False,
			"speech_start": 0.0,
			"speech_end": 0.0,
			"confidence": 0.0,
		}

	# Process each boundary chunk independently and collect speech frames
	frame_time = 1 / ((sr / hop_size) / 6)
	all_speech_frames = []

	with torch.inference_mode():
		for i in chunks_to_process:
			chunk = audio_pcen[..., i * c_size : (i + 1) * c_size]
			if chunk.shape[-1] == 0:
				continue
			la = model(chunk)
			la = torch.sigmoid(la)
			la = torch.max_pool1d(la, 6, 6)
			chunk_labels = la.detach().cpu().numpy()[0]

			chunk_start_sec = i * duration
			chunk_offset_frames = int(chunk_start_sec / frame_time)

			for j, frame in enumerate(chunk_labels.T):
				speech_prob = float(frame[1])
				t = frame_time * (chunk_offset_frames + j)
				all_speech_frames.append((t, speech_prob))


	# Scan speech frames for contiguous regions within boundary zones
	boundary_start_limit = check_seconds
	boundary_end_limit = audio_duration - check_seconds

	raw_regions = []
	active = False
	start = 0.0
	region_probs = []

	for t, speech_prob in all_speech_frames:
		in_boundary = (
			check_seconds <= 0 or t < boundary_start_limit or t > boundary_end_limit
		)

		if speech_prob >= confidence_threshold and not active and in_boundary:
			active = True
			start = t
			region_probs = [speech_prob]
		elif speech_prob >= confidence_threshold and active and in_boundary:
			region_probs.append(speech_prob)
		elif active and (speech_prob < confidence_threshold or not in_boundary):
			active = False
			avg_conf = float(np.mean(region_probs))
			raw_regions.append((start, t, avg_conf, len(region_probs)))
			region_probs = []

	if active:
		avg_conf = float(np.mean(region_probs))
		t_end = min(all_speech_frames[-1][0] + frame_time, audio_duration)
		raw_regions.append((start, t_end, avg_conf, len(region_probs)))

	if not raw_regions:
		return {
			"has_speech": False,
			"speech_start": 0.0,
			"speech_end": 0.0,
			"confidence": 0.0,
		}

	# Separate raw regions into beginning and end zones, then bridge within each
	# zone independently. This prevents false bridges between start-of-song and
	# end-of-song detections that span the entire track.
	beginning_regions = [
		r for r in raw_regions if check_seconds <= 0 or r[0] < boundary_start_limit
	]
	end_regions = [
		r for r in raw_regions if check_seconds <= 0 or r[1] > boundary_end_limit
	]

	beginning_bridged = bridge_regions(beginning_regions, max_speech_gap)
	end_bridged = bridge_regions(end_regions, max_speech_gap)

	all_bridged = beginning_bridged + end_bridged

	# Filter bridged regions by minimum duration
	speech_regions = [
		(r[0], r[1], r[2]) for r in all_bridged if r[1] - r[0] >= min_speech_duration
	]

	if not speech_regions:
		return {
			"has_speech": False,
			"speech_start": 0.0,
			"speech_end": 0.0,
			"confidence": 0.0,
		}

	largest = max(speech_regions, key=lambda x: x[1] - x[0])

	return {
		"has_speech": True,
		"speech_start": largest[0],
		"speech_end": largest[1],
		"confidence": largest[2],
	}


if __name__ == "__main__":
	if len(sys.argv) < 5:
		print(
			json.dumps(
				{
					"error": "Usage: detector.py <audio_path> <model_path> <confidence_threshold> <check_seconds>"
				}
			)
		)
		sys.exit(1)

	audio_path = sys.argv[1]
	model_path = sys.argv[2]
	try:
		confidence_threshold = float(sys.argv[3])
	except ValueError:
		print(json.dumps({"error": "Invalid confidence_threshold value"}))
		sys.exit(1)
	try:
		check_seconds = int(sys.argv[4])
	except ValueError:
		print(json.dumps({"error": "Invalid check_seconds value"}))
		sys.exit(1)

	try:
		result = detect_speech(
			audio_path, model_path, confidence_threshold, check_seconds
		)
		print(json.dumps(result))
	except Exception as e:
		print(json.dumps({"error": str(e)}))
		sys.exit(1)
`

const (
	ckptModelURL = "https://github.com/biboamy/TVSM-dataset/raw/refs/heads/master/Models/TVSM-cuesheet/epoch=10-step=4058.ckpt"
	ckptModelFile = "epoch=10-step=4058.ckpt"
	ptModelFile   = "model.pt"
	modelsDirName = "tvsm_models"
)

// Setup creates an isolated Python virtual environment, installs dependencies, downloads the TVSM model,
// and converts it to a plain state_dict .pt file for efficient runtime loading.
func Setup(scriptPath, cacheDir string) error {
	venvDir := filepath.Join(cacheDir, "env")

	if _, err := os.Stat(venvDir); err == nil {
		fmt.Println("Virtual environment found.")
		if _, err := exec.LookPath("python3"); err != nil {
			return fmt.Errorf("python3 not found: %w", err)
		}
		pythonExec := filepath.Join(venvDir, getBinDir(), "python")
		if runtime.GOOS == "windows" {
			pythonExec += ".exe"
		}
		cmd := exec.Command(pythonExec, "-c", "import torch, torchvision, torchaudio, librosa, numpy; print('Dependencies OK')")
		if err := cmd.Run(); err != nil {
			fmt.Println("Python dependencies missing or incomplete, will reinstall.")
		} else {
			if modelDir := filepath.Join(cacheDir, modelsDirName); modelExists(modelDir) {
				fmt.Println("TVSM model already downloaded. Setup is complete.")
				return nil
			}
			fmt.Println("Model files missing, will re-download.")
		}
	} else {
		fmt.Println("Creating virtual environment...")
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			return fmt.Errorf("failed to create cache dir: %w", err)
		}
	}

	if _, err := os.Stat(venvDir); os.IsNotExist(err) {
		fmt.Println("Creating virtual environment...")
		pythonPath := "python3"
		cmd := exec.Command(pythonPath, "-m", "venv", venvDir)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			if runtime.GOOS == "windows" {
				cmd = exec.Command("py", "-m", "venv", venvDir)
			} else {
				return fmt.Errorf("failed to create venv: %w (try installing python3-venv)", err)
			}
		}
		fmt.Println("Virtual environment created.")
	}

	pipPath := filepath.Join(venvDir, getBinDir(), "pip")
	if runtime.GOOS == "windows" {
		pipPath += ".exe"
	}

	fmt.Println("Installing package manager (uv)...")
	cmd := exec.Command(pipPath, "install", "uv")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install uv: %w", err)
	}

	uvPath := filepath.Join(venvDir, getBinDir(), "uv")
	if runtime.GOOS == "windows" {
		uvPath += ".exe"
	}

	fmt.Println("WARNING: Installing required Python packages (~2.5GB download, this may take 10-20 minutes)...")
	fmt.Println("This will download PyTorch and audio processing libraries. Ensure you have sufficient disk space and internet connection.")
	cmd = exec.Command(uvPath, "pip", "install", "torch", "torchvision", "torchaudio", "librosa", "numpy", "pytorch_lightning", "loguru")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install required packages: %w", err)
	}
	fmt.Println("Python packages installed.")

	// Download TVSM checkpoint
	fmt.Println("Downloading TVSM model (~11MB)...")
	if err := downloadModel(cacheDir); err != nil {
		return fmt.Errorf("failed to download TVSM model: %w", err)
	}
	fmt.Println("Model downloaded successfully.")

	// Convert .ckpt to .pt (plain state_dict for efficient loading)
	fmt.Println("Converting model checkpoint to runtime format...")
	if err := convertCheckpoint(cacheDir, venvDir); err != nil {
		return fmt.Errorf("failed to convert model checkpoint: %w", err)
	}
	fmt.Println("Model converted successfully.")

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

	return nil
}

// modelExists checks if the converted .pt model file exists
func modelExists(modelDir string) bool {
	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		return false
	}
	expectedFiles := []string{
		"TVSM-cuesheet/Models/" + ptModelFile,
	}
	for _, f := range expectedFiles {
		if _, err := os.Stat(filepath.Join(modelDir, f)); os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// downloadModel downloads the raw .ckpt model file from GitHub
func downloadModel(cacheDir string) error {
	modelsDir := filepath.Join(cacheDir, modelsDirName)
	ckptPath := filepath.Join(modelsDir, "TVSM-cuesheet", "Models", ckptModelFile)

	if fileExists(ckptPath) {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(ckptPath), 0755); err != nil {
		return fmt.Errorf("failed to create model dir: %w", err)
	}

	resp, err := http.Get(ckptModelURL)
	if err != nil {
		return fmt.Errorf("failed to download model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download model: server returned %d", resp.StatusCode)
	}

	outFile, err := os.Create(ckptPath)
	if err != nil {
		return fmt.Errorf("failed to create model file: %w", err)
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, resp.Body); err != nil {
		return fmt.Errorf("failed to write model file: %w", err)
	}

	return nil
}

// convertCheckpoint converts the PyTorch Lightning .ckpt file to a plain state_dict .pt file.
// This requires pytorch_lightning (and its dependencies like loguru) to be installed,
// since the .ckpt contains pickled Lightning objects.
// The conversion strips the 'model.' prefix from state_dict keys and saves a clean tensor dict.
// After conversion, the original .ckpt is deleted since it's no longer needed.
func convertCheckpoint(cacheDir, venvDir string) error {
	modelsDir := filepath.Join(cacheDir, modelsDirName)
	ptPath := filepath.Join(modelsDir, "TVSM-cuesheet", "Models", ptModelFile)
	ckptPath := filepath.Join(modelsDir, "TVSM-cuesheet", "Models", ckptModelFile)

	if fileExists(ptPath) {
		return nil
	}

	if !fileExists(ckptPath) {
		return fmt.Errorf("checkpoint file not found at %s", ckptPath)
	}

	pythonExec := filepath.Join(venvDir, getBinDir(), "python")
	if runtime.GOOS == "windows" {
		pythonExec += ".exe"
	}

	convertScript := fmt.Sprintf(`
import torch
import sys
ckpt_path = %q
pt_path = %q
try:
    ckpt = torch.load(ckpt_path, map_location='cpu', weights_only=False)
    sd = {k.replace('model.', '', 1): v for k, v in ckpt['state_dict'].items() if k.startswith('model.')}
    torch.save(sd, pt_path)
    print(f'Converted checkpoint: {len(sd)} model parameters saved to model.pt')
except Exception as e:
    print(f'ERROR: {e}', file=sys.stderr)
    sys.exit(1)
`, ckptPath, ptPath)

	cmd := exec.Command(pythonExec, "-c", convertScript)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("checkpoint conversion failed: %w", err)
	}

	if !fileExists(ptPath) {
		return fmt.Errorf("model.pt was not created after conversion")
	}

	_ = os.Remove(ckptPath)

	return nil
}

// ModelPath returns the full path to the converted .pt model file
func ModelPath(cacheDir string) string {
	return filepath.Join(cacheDir, modelsDirName, "TVSM-cuesheet", "Models", ptModelFile)
}

// DetectorPath returns the full path to the deployed detector script
func DetectorPath(cacheDir string) string {
	return filepath.Join(cacheDir, "smad", "detector.py")
}

// PythonPath returns the full path to the venv Python executable
func PythonPath(cacheDir string) string {
	pythonPath := filepath.Join(cacheDir, "env", getBinDir(), "python")
	if runtime.GOOS == "windows" {
		pythonPath += ".exe"
	}
	return pythonPath
}

// CacheDir returns the full path to the SMAD cache directory
func CacheDir(cacheDir string) string {
	return filepath.Join(cacheDir, "smad", "cache")
}

// IsSetupComplete checks if the DJ skip feature is fully set up
func IsSetupComplete(cacheDir string) bool {
	pythonPath := PythonPath(cacheDir)
	if _, err := exec.LookPath(pythonPath); err != nil {
		if !fileExists(pythonPath) {
			return false
		}
	}

	if !fileExists(DetectorPath(cacheDir)) {
		return false
	}

	if !fileExists(ModelPath(cacheDir)) {
		return false
	}

	cmd := exec.Command(pythonPath, "-c", "import torch, torchvision, torchaudio, librosa, numpy; print('ok')")
	if err := cmd.Run(); err != nil {
		return false
	}

	return true
}

func getBinDir() string {
	if runtime.GOOS == "windows" {
		return "Scripts"
	}
	return "bin"
}

// fileExists is defined in smad.go
