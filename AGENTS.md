# RadioParadise TUI (Go) - Agent Guidelines

## Project Overview

Port of `rptui` (Python/Terminal TUI for RadioParadise) to Go using bubbletea.
We need to target the new (very recently released) v2 versions of the key
bubbletea framework (bubbletea, lipgloss, bubbles).  Many things changed from v1.
You will need to refer to docs or v2 upgrade guides.

Initially ported to GO using tview framework.  All core UI elements working, 
except image support.  Abandoned tview framework.  Now redoing port with bubbletea.
Python app functionality tested and working.  Much of Go code from tview repo
copied directly to this repo.  Much of other code should be easily adapted.

**Source Repository:** `/home/mds/Projects/rptui/` (Python, read-only reference)
**Source Repository:** `/home/mds/Projects/rptui-go/` (Go, tview framerwork, no image support)  
**Target Repository:** `/home/mds/Projects/rptui-bubbletea/` (Go, active development)

---

## Key documentation sources for reference

https://pkg.go.dev/charm.land/bubbletea/v2
https://pkg.go.dev/charm.land/lipgloss/v2
https://pkg.go.dev/charm.land/bubbles/v2
https://pkg.go.dev/github.com/blacktop/go-termimg

## If documentation is not clear, packages not behaving as expected, or any difficult bugs

Read source files directly at ~/go/pkg/mod/ to understand behavior!

---

## Development Workflow

### Git Commits

- **Commit frequently** when significant features are completed AND manually tested.
  READ THAT AGAIN:  ONLY WHEN MANUALLY TESTED AND CONFIRMED WORKING!!!
- **Commit message format:** `feat: description` or `fix: description` or `refactor: description`


### Python Code Reference

**CRITICAL GUIDELINE:** When implementing any feature or fixing any issue, ALWAYS:

1. **First check the Python and Go tview implementation** in Source Respositories above.
2. **Understand the logic** - how it handles edge cases, timing, state management
3. **Replicate the behavior** in Go unless there's a compelling reason to differ
4. **If proposing a different approach**, document:
   - Python and tview-based approach and why it works
   - Proposed bubbletea approach and why it's better
   - Trade-offs of each approach
   - Recommendation with justification

**Example:** For progress updates, the Python app:
- Updates progress bar/time every 1 second via `set_interval(1.0, self.update_progress)`
- Polls for next block every 5 seconds via `set_interval(5.0, self.poll_wrapper)`
- Detects natural song transitions via MPV playlist position polling
- Updates all UI elements when song changes

The bubbletea implementation should mirror this timing and logic unless a better pattern is identified.

### Debugging with Bash Commands

**PREFER BASH DEBUGGING** over running the app and checking logs manually. Bash commands are faster and more efficient:

```bash
# Check if file exists and view contents
ls -la ~/.cache/rptui/favorites/
cat ~/.cache/rptui/favorites/metadata.json | jq

# Test API endpoints directly
curl -s "https://lrclib.net/api/search?artist_name=Artist&track_name=Track"
curl -s -H "User-Agent: rptui/1.0" "https://en.wikipedia.org/api/rest_v1/page/summary/Artist_Name"

# Create test programs to debug APIs
go run test_lrclib.go
go run test_wikipedia.go
```

**NEVER debug by:**
1. Making code changes with "best guesses"
2. Adding debug logs and waiting for user to test
3. Running the full app repeatedly for simple API tests

**ALWAYS debug by:**
1. Testing APIs directly with curl first
2. Creating small `go run` test programs to verify logic
3. Understanding the exact API response format before parsing
4. Checking Python implementation for reference

**File Creation Rule:** ONLY use write_file or edit tools - NEVER use cat, sed, awk, or similar shell commands to create/edit files.
# Search logs for specific patterns
grep -i "error\|favorite" rptui.log
tail -f rptui.log  # Follow log in real-time

# Test API endpoints
curl -s "https://api.radioparadise.com/api/play?..." | jq

# Check process state
ps aux | grep mpv
ls -la $XDG_RUNTIME_DIR/mpv/

# Quick compilation test
go build ./... && echo "Build OK" || echo "Build FAILED"
```

Use `go run ./cmd/rptui` only when you need to test actual UI behavior. For debugging data/logic issues, use bash commands first.

### Manual Testing Checklist

Before committing, verify:

- [ ] Code compiles without errors: `go build ./...`
- [ ] No linting issues: `go vet ./...`
- [ ] Core functionality works (playback, controls, UI)
- [ ] No obvious regressions from previous version

---

## Architecture

### Directory Structure

```
rptui-bubbletea/
├── cmd/rptui/main.go           # Entry point
├── internal/
│   ├── config/                 # Configuration + theme loading
│   ├── api/                    # External APIs (RP, LRCLib, Wikipedia)
│   ├── mpv/                    # MPV backend with IPC
│   ├── models/                 # Data models (Song, etc.)
│   ├── tui/                    # TUI application + widgets + modals
│   └── cache/                  # Combined favorites + blocklist management
├── go.mod
├── go.sum
└── README.md
```

### Key Design Decisions

| Component | Choice | Rationale |
|-----------|--------|-----------|
| TUI Framework | bubbletea | Rich widgets (Table, Modal, ProgressBar), Compatible with images |
| Album Art | Kitty → Sixel fallback | Native terminal graphics, no external viewer |
| Config Path | `~/.config/rptui/config.toml` | Shared with Python for migration |
| Cache Path | `$XDG_CACHE_HOME/rptui/` | XDG compliance, subdirs: `favorites/`, `blocklist/` |
| MPV Socket | `$XDG_RUNTIME_DIR/mpv/rptui-socket` | Multi-user safe, no stale sockets |
| Logging | `rptui.log` (separate) | Avoid conflicts with Python version |

---

## Code Style

### General

- **Idiomatic Go:** Follow Effective Go guidelines
- **Error handling:** Explicit errors, no silent failures
- **Concurrency:** Use goroutines + channels, protect shared state with mutex
- **Naming:** CamelCase for exported, lowercase for unexported

### Error Handling Pattern

```go
// Good: Explicit error with context
if err != nil {
    return fmt.Errorf("failed to load config: %w", err)
}

// Good: Log and continue (non-critical)
if err != nil {
    log.Printf("Warning: failed to load album art: %v", err)
    // Continue with fallback
}
```

### Concurrency Pattern

```go
// Shared state protection
type App struct {
    mu              sync.Mutex
    currentSongIndex int
    blockSongs      []Song
}

func (a *App) updateSongIndex(idx int) {
    a.mu.Lock()
    defer a.mu.Unlock()
    a.currentSongIndex = idx
}
```

### Constants Over Magic Numbers

```go
// Good: Named constants
const (
    PollIntervalSeconds    = 5
    CleanupIntervalSeconds = 3600
    MaxRetryAttempts       = 3
)

// Bad: Magic numbers
time.Sleep(5 * time.Second)  // Why 5?
```

---

## Configuration

### Config File: `~/.config/rptui/config.toml`

```toml
channel = 0
bitrate = 3
show_album_art = true
album_art_path = "/tmp/cover.jpg"
copy_album_art = false
favorites_dir = "/home/user/.cache/rptui/favorites"
max_favorites = 100
min_favorites = 10
show_skip_warning = true
colors_file = ""
```

### Color Theme Loading

Priority order:
1. User-provided file (if `colors_file` set)
2. Omarchy theme: `~/.config/omarchy/current/theme/colors.toml`
3. Built-in defaults (Catppuccin Mocha)

---

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Space` | Play/Pause |
| `n` | Skip next |
| `r` | Restart song |
| `p` | Previous song |
| `Left` | Seek backward 10 seconds |
| `Right` | Seek forward 10 seconds |
| `o` | Options modal |
| `v` | Toggle bottom view (playlist → lyrics → synced lyrics → artist → off) |
| `f` | Add/remove favorite |
| `b` | Add/remove blocklist |
| `$` | Open RP donate page |
| `q` | Quit |

---

## External Dependencies

```go
require (
	charm.land/bubbletea/v2					// TUI framework
	charm.land/lipgloss/v2					// TUI styling
	charm.land/bubbles/v2					// TUI elements
    github.com/blacktop/go-termimg			// Image support
    github.com/pelletier/go-toml/v2 v2.2.0  // TOML parsing
    github.com/adrg/xdg v0.5.0              // XDG paths
    golang.org/x/net v0.35.0                // HTTP client
)
```

---

## Testing Strategy

### Unit Tests

- API clients (RadioParadise, LRCLib, Wikipedia)
- Config loading/validation
- Favorites/blocklist management
- Song model methods

### Integration Tests

- MPV IPC communication (mock socket)
- TUI widget rendering (visual verification)

### Manual Testing

- Full playback flow
- All keyboard shortcuts
- Modal dialogs
- Album art display (kitty/sixel terminals)
- Theme loading

---

## Debugging

### Logging

- Log file: `rptui.log` in project root
- Level: INFO (configurable to DEBUG)
- Format: `%(asctime)s %(levelname)s %(message)s`

### Common Issues

**MPV not starting:**
- Check socket path: `ls -la $XDG_RUNTIME_DIR/mpv/`
- Verify MPV installed: `which mpv`
- Check IPC support: `mpv --input-ipc-server=/tmp/test --no-video --quit`

**Config not loading:**
- Check path: `cat ~/.config/rptui/config.toml`
- Validate TOML syntax
- Check file permissions

---

## Migration from Python

### Data Migration

- Config: Same path, compatible format (TOML)
- Favorites: Copy `~/.cache/rptui/favorites/` → `$XDG_CACHE_HOME/rptui/favorites/`
- Blocklist: Copy `~/.cache/rptui/blocklist/` → `$XDG_CACHE_HOME/rptui/blocklist/`

---

## Contact/Questions

If uncertain about implementation details:
1. Check Python source: `/home/mds/Projects/rptui/rptui.py`
2. Review this AGENTS.md
3. Ask user for clarification before making assumptions

---

## Known bugs/Roadmap

user generated text file: fixes.txt
OK to edit to keep up to date
