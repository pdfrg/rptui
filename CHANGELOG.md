# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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