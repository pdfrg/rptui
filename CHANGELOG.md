# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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