#!/usr/bin/env python3
"""
DJ Segment Detection via TVSM (TV Speech/Music)
Detects speech segments at start/end of audio files for DJ skip logic.
Usage: python3 detector.py <audio_file_path>
Output: JSON {"has_speech": bool, "speech_start": float, "speech_end": float, "confidence": float}
"""

import json
import sys
import os
import numpy as np
import onnxruntime as ort
import librosa

# --- Offline TVSM model loading ---
# TVSM model files are expected to be bundled locally alongside this script
MODEL_DIR = os.path.join(os.path.dirname(__file__), "tvsm_models")
MODEL_PATH = os.path.join(MODEL_DIR, "tvsm_model.onnx")


def load_model():
    """Load the ONNX TVSM model."""
    if not os.path.exists(MODEL_PATH):
        raise FileNotFoundError(f"TVSM model not found at {MODEL_PATH}")
    return ort.InferenceSession(MODEL_PATH)


def extract_features(audio_path, sr=16000, n_mels=80, n_fft=1024, hop_length=512):
    """Extract log-mel spectrogram features from audio file."""
    y, _ = librosa.load(audio_path, sr=sr)

    # Compute mel spectrogram
    mel_spec = librosa.feature.melspectrogram(
        y=y, sr=sr, n_fft=n_fft, hop_length=hop_length, n_mels=n_mels
    )

    # Convert to log scale
    log_mel_spec = librosa.power_to_db(mel_spec, ref=np.max)

    # Normalize
    log_mel_spec = (log_mel_spec - log_mel_spec.min()) / (
        log_mel_spec.max() - log_mel_spec.min() + 1e-8
    )

    # Pad or truncate to fixed size (e.g., 100 time steps)
    target_length = 100
    if log_mel_spec.shape[1] < target_length:
        pad_length = target_length - log_mel_spec.shape[1]
        log_mel_spec = np.pad(log_mel_spec, ((0, 0), (0, pad_length)), mode="constant")
    else:
        log_mel_spec = log_mel_spec[:, :target_length]

    # Add batch and channel dimensions
    features = log_mel_spec[np.newaxis, np.newaxis, :, :].astype(np.float32)
    return features


def detect_speech_tvsm(audio_path, session=None):
    """
    Run TVSM inference on audio file to detect speech segments.
    Returns speech start time, speech end time, confidence, and whether speech is present.
    """
    if session is None:
        session = load_model()

    # Extract features
    features = extract_features(audio_path)

    # Run inference
    input_name = session.get_inputs()[0].name
    output_name = session.get_outputs()[0].name
    result = session.run([output_name], {input_name: features})[0]

    # result shape: (1, num_frames) with probabilities
    probs = result[0]

    # Threshold for speech detection
    threshold = 0.5
    speech_frames = probs > threshold

    if not np.any(speech_frames):
        return 0.0, 0.0, 0.1, False

    # Find speech segments
    # Convert frame indices to time (assuming 10ms per frame for 100ms hop)
    hop_length = 512  # same as in extract_features
    sr = 16000
    frame_duration = hop_length / sr  # ~0.032s for 16kHz with 512 hop

    # Convert speech frames to time positions
    speech_times = np.where(speech_frames)[0] * frame_duration

    if len(speech_times) == 0:
        return 0.0, 0.0, 0.1, False

    # Compute confidence as mean probability in speech regions
    speech_probs = probs[speech_frames]
    confidence = float(np.mean(speech_probs))

    # Get start and end times
    speech_start = float(speech_times[0])
    speech_end = float(speech_times[-1] + frame_duration)

    return speech_start, speech_end, confidence, True


# --- Main ---
if __name__ == "__main__":
    if len(sys.argv) < 2:
        print(json.dumps({"error": "missing audio file path"}), file=sys.stderr)
        sys.exit(1)

    audio_path = sys.argv[1]
    if not os.path.isfile(audio_path):
        print(json.dumps({"error": "file not found"}), file=sys.stderr)
        sys.exit(1)

    try:
        result = detect_speech_tvsm(audio_path)
        output = {
            "has_speech": result[3],
            "speech_start": result[0],
            "speech_end": result[1],
            "confidence": result[2],
        }
        print(json.dumps(output))
    except Exception as e:
        print(json.dumps({"error": str(e)}), file=sys.stderr)
        sys.exit(1)
