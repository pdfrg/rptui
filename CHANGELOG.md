# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v1.3.0] - 2026-05-08

### Added

- **SSH audio forwarding**: `--audio-forward` flag redirects playback to your local machine over SSH, with automatic PulseAudio output detection
- **tmux Kitty image support**: Full Kitty image protocol passthrough inside tmux sessions using combined DCS escape sequences, outer terminal detection, and client cell size queries
- **DJ speech pre-screening**: Upcoming songs are pre-screened for DJ speech before playback starts, eliminating gaps at block boundaries
- **Skip DJ Speech toggle**: New option in the options modal to enable/disable DJ speech skipping (visible for users with setup complete)

### Fixed

- **Visualizer stuck on loading**: Added retry logic, default sink monitor fallback, and ring buffer fixes to prevent the visualizer from hanging on startup
- **tmux startup hang**: Fixed hang caused by `/dev/tty` termios restore inside tmux
- **tmux cell ratio detection**: Corrected cell size detection inside tmux using `CSI 16t` queries
- **tmux multi-pane image passthrough**: Fixed image rendering with `allow-passthrough=all`, quit delay, and gallery timing
- **tmux gallery image pixel area**: Capped pixel area in tmux to avoid DCS splitting issues
- **Rio + SSH Kitty detection**: Fixed Kitty protocol detection when running Rio over SSH
- **WezTerm + SSH font dimensions**: Swapped font width/height dimensions for correct cell ratio over SSH
- **SSH audio forwarding**: Now properly forces PulseAudio output instead of auto-detecting
- **DJ speech detection**: Fixed boundary window, gap tracking, and merge bugs in two-phase detection
- **CI workflow**: Fixed golangci-lint compatibility, indentation, and setup-go caching
- **Code formatting**: Applied `gofmt` across all files

### Changed

- **DJ speech detection**: Two-phase boundary scanning now runs on all songs, not just block-position candidates; removed `dj_check_seconds` config option (auto-detected now); `dj_min_speech_duration` default changed from 15.0 to 10.0
- **Config file**: New `[audio]` section with `ssh_audio_server` setting for SSH audio forwarding
- **Terminal color queries**: Updated `termenv` API usage for compatibility (`.Writer()` instead of `.TTY()`)
- **Theme style building**: Refactored to build base styles first, then conditionally apply backgrounds

### Documentation

- Added offline mode section to DOCUMENTATION.md
- Added HELP.md for MPV and NerdFont installation
- Added note about updating/fixing config file by changing stations in-app
- Added contributing guide (CONTRIBUTING.md)
- Updated CI badge to branch=main
- Updated issue templates

### Note for Existing Users

Your existing config file is fully backward-compatible. Removed fields (like `dj_check_seconds`) are silently ignored, and new fields use sensible defaults. To regenerate your config with the latest format and default values, simply change stations from within the app to force an overwrite.

## [v1.2.0] - 2025-04-27

### Added

- **DJ speech detection and skipping**: Automatic detection of DJ talk at song ends using the TVSM neural network model, with configurable confidence threshold, check zone, and minimum speech duration; auto-skip or manual review modes
- **Lidarr integration**: Artist/album monitoring status display with direct link to Lidarr artist pages using MusicBrainz IDs
- **CI/CD**: GitHub Actions workflows for testing and GoReleaser-based releases (draft mode)
- **Overflow pruning**: Automatic playlist pruning when queue exceeds 16 songs
- **Song duration in DJ cache**: Detection cache entries now include song duration for easier manual inspection
- **Block-position pre-filter**: Improved DJ detection with block-position pre-filtering and threshold tuning

### Fixed

- **DJ skip bypass**: Prevented DJ skip from bypassing the favorite queue on the last song
- **Viewport scroll**: Reset viewport scroll when comments are fetched for a new song while not in comments view
- **Favorite star prefix**: Skip favorite star prefix when authenticated to prevent truncation of user rating
- **Duplicate favorites**: Use SongID/Artist-Album-Title for identity instead of EventID to prevent duplicate favorites
- **DJ detection boundary enforcement**: Fixed boundary enforcement, duplicate handler, and resilience in speech detection
- **DJ detection model architecture**: Rewrote DJ speech detection with correct model architecture (F2M, PCENTransform, CRNN)
- **Gap bridging**: Added gap bridging to DJ speech detector for fragmented speech regions
- **Lidarr artist URL**: Use MusicBrainz ID instead of numeric ID, filter bootlegs from discography
- **Cross-platform browser opening**: Replaced hardcoded `xdg-open` with cross-platform `OpenBrowser()` (Linux, macOS, Windows)

### Changed

- **DJ detection refactor**: Removed beginning-zone scan, extended end-zone to 80s, added min_speech_duration config option, end-of-file validation
- **Detector script embedding**: Replaced duplicated Python script constant with `//go:embed` for single source of truth
- **CI Go version**: Updated from 1.22 to 1.26 to match go.mod

## [v1.1.0] - 2025-04-21

### Added

- **Multi-platform support**: Windows and macOS support (previously Linux-only)
- **Audio visualizer**: Full visualizer infrastructure with ViewVisualizer widget, 9 spectrum modes (Bars, Braille, BrailleBars, ClassicPeak, Wave, Stars, Rain, Segmented, Binary), fullscreen toggle, and song info overlay
- **macOS audio visualizer**: SoX + BlackHole support for real audio capture
- **Scrobbling**: Last.fm and ListenBrainz support with `--lastfm-auth` OAuth flow
- **Artist image gallery**: Modal with Discogs/TheAudioDB priority, tea.Raw rendering
- **Image protocol support**: Sixel, iTerm2, and Halfblocks protocols beyond Kitty
- **Terminal compatibility**: `--transparent_background` and `--disable_theme` config options for legacy terminals
- **Terminal color detection**: `--test-terminal-colors` debug command, smart detection for disable_theme and transparent_background modes
- **Force protocol**: `force_protocol` config option to override image protocol auto-detection
- **PulseAudio visualizer fallback**: Audio tap for PulseAudio when PipeWire unavailable

### Fixed

- **PulseAudio fixes**: Numerous fixes for parecord/pw-record audio capture (sample buffering, stdout/stderr routing, format parsing, chunk handling)
- **Image rendering**: Sixel positioning, halfblocks dimensions, album art sizing and slicing issues
- **Terminal colors**: Progress bar colors with disable_theme, NoColor handling, empty color fallback
- **Theme styling**: Transparent background support, legacy terminal compatibility
- **Cursor positioning**: Correct per-line cursor positioning for image protocols

### Documentation

- Updated installation instructions for multiplatform support
- Updated --help with new config options
- Added force_protocol to documentation
- Added MATE screenshots to gallery