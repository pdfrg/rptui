#!/usr/bin/env python3
# NOTE: The detect_speech logic in this file must be kept in sync with:
# - internal/smad/detector.py (canonical source)
# - internal/smad/setup.go (detectorScript constant)
# Any changes to detection logic MUST be reflected in all three.
"""Test DJ speech detection on audio files.

Usage:
  test-dj Song.m4a
  test-dj Song1.m4a Song2.flac
  test-dj ~/.cache/rptui/favorites/
  test-dj --confidence 0.5 --check-seconds 60 --min-duration 3.0 Song.m4a
  test-dj -h
"""

import argparse
import os
import sys
import time

import detector

AUDIO_EXTENSIONS = {".m4a", ".flac", ".mp3", ".ogg", ".wav"}

DEFAULT_MODEL = os.path.join(
    os.path.expanduser("~"),
    ".cache",
    "rptui",
    "tvsm_models",
    "TVSM-cuesheet",
    "Models",
    "model.pt",
)


def fmt_time(seconds):
    minutes = int(seconds) // 60
    secs = seconds - minutes * 60
    return f"{minutes}:{secs:04.1f}"


def collect_files(paths):
    files = []
    for p in paths:
        p = os.path.expanduser(p)
        if os.path.isfile(p):
            files.append(p)
        elif os.path.isdir(p):
            for root, _, filenames in os.walk(p):
                for f in sorted(filenames):
                    if os.path.splitext(f)[1].lower() in AUDIO_EXTENSIONS:
                        files.append(os.path.join(root, f))
        else:
            print(f"Warning: skipping non-existent path: {p}", file=sys.stderr)
    return files


def process_file(filepath, model_path, confidence, check_seconds, min_duration):
    basename = os.path.basename(filepath)
    duration = detector.get_audio_duration(filepath)
    if duration is None:
        print(f"--- {basename} ---")
        print(f"Error: could not read audio file")
        return False

    print(f"--- {basename} ({fmt_time(duration)}) ---")
    print(
        f"Params: confidence={confidence} check_seconds={check_seconds} min_duration={min_duration}"
    )

    start = time.time()
    result = detector.detect_speech(
        filepath, model_path, confidence, check_seconds, min_duration
    )
    elapsed = time.time() - start

    if result["has_speech"]:
        seg_len = result["speech_end"] - result["speech_start"]
        near_end = result["song_duration"] - result["speech_end"]
        print(
            f"Speech detected: {fmt_time(result['speech_start'])} - "
            f"{fmt_time(result['speech_end'])} ({seg_len:.1f}s, "
            f"confidence {result['confidence']:.2f}, "
            f"song_duration {result['song_duration']:.1f}s, "
            f"speech ends {near_end:.1f}s from end)"
        )
    else:
        print("No speech detected")

    print(f"Processed in {elapsed:.1f}s")
    return result["has_speech"]


def main():
    parser = argparse.ArgumentParser(
        description="Test DJ speech detection on audio files",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""examples:
  test-dj Song.m4a
  test-dj Song1.m4a Song2.flac
  test-dj ~/.cache/rptui/favorites/
  test-dj --confidence 0.5 --check-seconds 60 Song.m4a
""",
    )
    parser.add_argument(
        "paths",
        nargs="+",
        metavar="path",
        help="audio files and/or directories to scan",
    )
    parser.add_argument(
    "--confidence",
    type=float,
    default=0.88,
    help="speech confidence threshold (default: 0.88)",
    )
    parser.add_argument(
        "--check-seconds",
        type=int,
        default=80,
        help="seconds from end of song to check (default: 80)",
    )
    parser.add_argument(
        "--min-duration",
        type=float,
        default=15.0,
        help="minimum speech segment duration in seconds (default: 15.0)",
    )
    parser.add_argument(
        "--model",
        default=DEFAULT_MODEL,
        help=f"path to TVSM model.pt (default: {DEFAULT_MODEL})",
    )
    args = parser.parse_args()

    files = collect_files(args.paths)
    if not files:
        print("Error: no audio files found", file=sys.stderr)
        sys.exit(1)

    if not os.path.isfile(args.model):
        print(f"Error: model not found at {args.model}", file=sys.stderr)
        print("Run 'rptui --setup-dj-skip' first", file=sys.stderr)
        sys.exit(1)

    speech_count = 0
    for f in files:
        print()
        if process_file(
            f, args.model, args.confidence, args.check_seconds, args.min_duration
        ):
            speech_count += 1

    print()
    print("=" * 50)
    print(
        f"{len(files)} file(s) processed: {speech_count} speech, {len(files) - speech_count} clean"
    )


if __name__ == "__main__":
    main()
