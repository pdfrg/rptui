package smad

import (
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

//go:embed detector.py
var detectorScript string

const (
	ckptModelURL  = "https://github.com/biboamy/TVSM-dataset/raw/refs/heads/master/Models/TVSM-cuesheet/epoch=10-step=4058.ckpt"
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
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to create venv with py: %w", err)
			}
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
