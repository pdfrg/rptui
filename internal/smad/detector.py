#!/usr/bin/env python3
# NOTE: This file is the canonical source for the DJ speech detection logic.
# It is embedded into the Go binary via //go:embed in setup.go.
# internal/smad/test_detector.py imports this module for testing.
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
duration = 10

# Maximum gap (seconds) between speech regions to bridge into one segment
# DJ speech often has brief musical interludes/station IDs that split detections
max_speech_gap = 2.5

# Phase 1 boundary window: check only the first and last N seconds of each
# song for speech. If any frame within this window exceeds the confidence
# threshold, Phase 2 expands from that boundary until a gap > max_speech_gap
# seconds with no speech is found. Must match Go-side maxSpeechDistanceFromBoundary.
boundary_window = 1.5


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


def process_chunk(model, pcen_data, chunk_index, c_size, confidence_threshold):
    """Run inference on one 10s chunk, return list of (time, speech_prob) frames."""
    chunk = pcen_data[..., chunk_index * c_size : (chunk_index + 1) * c_size]
    if chunk.shape[-1] == 0:
        return []

    frame_time = 1 / ((sr / hop_size) / 6)
    chunk_start_sec = chunk_index * duration
    chunk_offset_frames = int(chunk_start_sec / frame_time)

    la = model(chunk)
    la = torch.sigmoid(la)
    la = torch.max_pool1d(la, 6, 6)
    chunk_labels = la.detach().cpu().numpy()[0]

    frames = []
    for j, frame in enumerate(chunk_labels.T):
        speech_prob = float(frame[1])
        t = frame_time * (chunk_offset_frames + j)
        frames.append((t, speech_prob))
    return frames


def has_speech_in_window(frames, confidence_threshold, window_start, window_end):
    """Check if any frame within the time window exceeds the confidence threshold."""
    return any(
        prob >= confidence_threshold and window_start <= t <= window_end
        for t, prob in frames
    )


def expand_forward(model, pcen_data, start_chunk, n_chunks, c_size, confidence_threshold):
    """Scan forward from start_chunk until a gap > max_speech_gap with no speech frames.

    Track last_speech_time: the timestamp of the most recent frame above threshold.
    If the current chunk's end exceeds last_speech_time + max_speech_gap, stop.
    """
    all_frames = []
    last_speech_time = None

    for ci in range(start_chunk, n_chunks):
        chunk_frames = process_chunk(model, pcen_data, ci, c_size, confidence_threshold)
        all_frames.extend(chunk_frames)

        for t, prob in chunk_frames:
            if prob >= confidence_threshold:
                last_speech_time = t

        if last_speech_time is not None:
            chunk_end_sec = (ci + 1) * duration
            if chunk_end_sec - last_speech_time >= max_speech_gap:
                break

    return all_frames


def expand_backward(model, pcen_data, end_chunk, c_size, confidence_threshold):
    """Scan backward from end_chunk until a gap > max_speech_gap with no speech frames.

    Moving leftward from the song end, track earliest_speech_time: the smallest
    timestamp among frames above threshold found so far (closest to song start).
    If the current chunk begins more than max_speech_gap before earliest_speech_time, stop.
    """
    all_frames = []
    earliest_speech_time = None

    for ci in range(end_chunk, -1, -1):
        chunk_frames = process_chunk(model, pcen_data, ci, c_size, confidence_threshold)
        all_frames = chunk_frames + all_frames

        for t, prob in chunk_frames:
            if prob >= confidence_threshold:
                if earliest_speech_time is None or t < earliest_speech_time:
                    earliest_speech_time = t

        if earliest_speech_time is not None:
            chunk_start_sec = ci * duration
            if chunk_start_sec <= earliest_speech_time - max_speech_gap:
                break

    return all_frames


def extract_regions(all_frames, confidence_threshold, frame_time, audio_duration):
    """Extract contiguous speech regions from a list of (time, prob) frames."""
    raw_regions = []
    active = False
    start = 0.0
    region_probs = []

    for t, speech_prob in all_frames:
        if speech_prob >= confidence_threshold and not active:
            active = True
            start = t
            region_probs = [speech_prob]
        elif speech_prob >= confidence_threshold and active:
            region_probs.append(speech_prob)
        elif active and speech_prob < confidence_threshold:
            active = False
            avg_conf = float(np.mean(region_probs))
            raw_regions.append((start, t, avg_conf, len(region_probs)))
            region_probs = []

    if active and region_probs:
        avg_conf = float(np.mean(region_probs))
        t_end = min(all_frames[-1][0] + frame_time, audio_duration)
        raw_regions.append((start, t_end, avg_conf, len(region_probs)))

    return raw_regions


def speech_empty():
    """Return the standard 'no speech found' result."""
    return {
        "has_speech": False,
        "speech_start": 0.0,
        "speech_end": 0.0,
        "confidence": 0.0,
    }


def detect_speech(audio_path, model_path, confidence_threshold, min_speech_duration=10.0):
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
    n_chunks = int(np.ceil(audio_pcen.shape[-1] / c_size))

    if n_chunks == 0:
        result = speech_empty()
        result["song_duration"] = round(audio_duration, 3)
        return result

    frame_time = 1 / ((sr / hop_size) / 6)
    with torch.inference_mode():
        # Phase 1: check first and last boundary chunks for speech
        last_chunk = n_chunks - 1
        start_frames = process_chunk(model, audio_pcen, 0, c_size, confidence_threshold)
        start_has_speech = has_speech_in_window(start_frames, confidence_threshold, 0, boundary_window)

        end_frames = process_chunk(model, audio_pcen, last_chunk, c_size, confidence_threshold)
        end_has_speech = has_speech_in_window(
            end_frames, confidence_threshold, audio_duration - boundary_window, audio_duration
        )

        if not start_has_speech and not end_has_speech:
            result = speech_empty()
            result["song_duration"] = round(audio_duration, 3)
            return result

        # Phase 2: expand from detected boundaries
        all_frames = []

        if start_has_speech:
            forward_frames = expand_forward(
                model, audio_pcen, 0, n_chunks, c_size, confidence_threshold
            )
            all_frames.extend(forward_frames)

        if end_has_speech:
            backward_frames = expand_backward(
                model, audio_pcen, last_chunk, c_size, confidence_threshold
            )
            all_frames.extend(backward_frames)

    # Deduplicate by timestamp (same frame may appear in both forward/backward lists)
    seen = set()
    all_frames = [
        f for f in sorted(all_frames, key=lambda x: x[0])
        if not (round(f[0], 4) in seen or seen.add(round(f[0], 4)))
    ]

    raw_regions = extract_regions(all_frames, confidence_threshold, frame_time, audio_duration)

    if not raw_regions:
        result = speech_empty()
        result["song_duration"] = round(audio_duration, 3)
        return result

    bridged = bridge_regions(raw_regions, max_speech_gap)

    speech_regions = [
        (r[0], r[1], r[2]) for r in bridged if r[1] - r[0] >= min_speech_duration
    ]

    if not speech_regions:
        result = speech_empty()
        result["song_duration"] = round(audio_duration, 3)
        return result

    largest = max(speech_regions, key=lambda x: x[1] - x[0])

    return {
        "has_speech": True,
        "speech_start": largest[0],
        "speech_end": largest[1],
        "confidence": largest[2],
        "song_duration": round(audio_duration, 3),
    }


if __name__ == "__main__":
    if len(sys.argv) < 5:
        print(
            json.dumps(
                {"error": "Usage: detector.py <audio_path> <model_path> <confidence_threshold> <min_speech_duration>"}
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
        min_speech_duration = float(sys.argv[4])
    except ValueError:
        print(json.dumps({"error": "Invalid min_speech_duration value"}))
        sys.exit(1)

    try:
        result = detect_speech(
            audio_path, model_path, confidence_threshold, min_speech_duration
        )
        print(json.dumps(result))
    except Exception as e:
        print(json.dumps({"error": str(e)}))
        sys.exit(1)