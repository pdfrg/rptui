# rptui - RadioParadise TUI (Go) - Agent Guidelines

## Project Overview

Bubbletea TUI app for Radio Paradise internet radio.
We need to target the new (very recently released) v2 versions of the key
bubbletea framework (bubbletea, lipgloss, bubbles).  Many things changed from v1.
You will need to refer to docs or v2 upgrade guides.

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
- **Commit message format:** `feat: description` or `fix: description` or `refactor: description`

---

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

**After Edits** Read state of file to ensure edits worked as intended. Numerous problems result from assuming edits worked as intended without double-checking.

**After Editing Python Files** The edit tool frequently breaks Python indentation (flattening nested blocks). ALWAYS run `python3 -m py_compile <file>` immediately after editing any `.py` file to catch indentation errors before they reach runtime. Also re-read the file and verify indentation bytes with:
```bash
python3 -c "
with open('file.py', 'rb') as f:
    for i, line in enumerate(f.readlines()[start-1:end], start=start):
        indent = ''
        for b in line:
            if b == 32: indent += 'SP'
            elif b == 9: indent += 'TAB'
            else: break
        print(f'Line {i}: [{indent}] | {line.rstrip().decode()}')"
```

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

### Manual Testing Checklist

Before committing, verify:

- [ ] Code compiles without errors: `go build ./...`
- [ ] No linting issues: `go vet ./...`
- [ ] Changes to python code (detector.py) need separate testing 
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
| Config Path | `$XDG_CONFIG_HOME/rptui/config.toml` | XDG standard |
| Cache Path | `$XDG_CACHE_HOME/rptui/` | XDG compliance, subdirs: `favorites/`, `blocklist/` |
| MPV Socket | `$XDG_RUNTIME_DIR/mpv/rptui-socket` | Multi-user safe, no stale sockets |
| Logging | `$XDG_STATE_DIR/rptui/rptui.log` | Check for debugging |

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

### Known Bugs/Roadmap

- user generated text files: fixes.txt, roadmap.txt
