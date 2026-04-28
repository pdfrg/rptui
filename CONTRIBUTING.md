# Contributing to rptui

Thanks for your interest! This project is a Go + Bubble Tea TUI for Radio Paradise.

## Bug Reports and other Issues

Please include:

- **Version**: run `rptui -v` (includes OS/arch, e.g. `rptui v1.2.0 (go1.26.0, linux/amd64)`)
- **Terminal emulator**: e.g. Kitty 0.35, Ghostty 1.1, gnome-terminal 3.52
- **OS**: especially important for Windows/macOS issues
- **Steps to reproduce**
- **Log excerpt**: check `$XDG_STATE_HOME/rptui/rptui.log` (default: `~/.local/state/rptui/rptui.log`)
- **Use appropriate template**: please fill out to the best of your ability

## Pull Requests

- Target the `main` branch
- One feature or fix per PR
- Run these before pushing:

```bash
go build ./...
go vet ./...
gofmt -s -l .  # should print nothing
go test ./...
```

## Code Style

- Follow [Effective Go](https://go.dev/doc/effective_go)
- Named constants over magic numbers
- Explicit error handling — no silent failures
- Protect shared state with mutexes

## Development Setup

```bash
go build -o rptui ./cmd/rptui
```

- **mpv** is required for audio playback
- Python 3 + venv is needed only for DJ skip feature testing (`--setup-dj-skip`)
