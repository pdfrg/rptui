# RadioParadise Bubbletea TUI - Implementation Status

**Last Updated:** 2026-03-20  
**Framework:** bubbletea v2 / lipgloss v2 / bubbles v2

---

## Current State

Core TUI is functional with bubbletea v2 API used correctly. All major v2 migration
issues have been resolved. Playback, controls, modals, lyrics, artist info, favorites,
blocklist, and progress tracking are working.

---

## Completed Features

### Core
- [x] Block-based playback with gapless URLs
- [x] MPV backend with IPC control (socket in `$XDG_RUNTIME_DIR/mpv/`)
- [x] Config loading/saving (`~/.config/rptui/config.toml`)
- [x] Theme support (Omarchy colors → Catppuccin Mocha fallback)
- [x] XDG-compliant cache paths for favorites/blocklist

### TUI Layout
- [x] Header widget
- [x] Now-playing info (title, artist, album, year, rating, star indicator)
- [x] Progress bar (bubbles/progress, accent-colored, animated)
- [x] Time display (elapsed / total + percentage)
- [x] Connected time display
- [x] Playlist table (bubbles/table, highlighted current song)
- [x] Footer with accent-colored keybindings
- [x] 5-way bottom view toggle (Playlist → Lyrics → Synced Lyrics → Artist → Off)
- [x] Viewport for scrollable content (lyrics, artist info)

### Controls
- [x] Space — Play/Pause
- [x] s — Stop
- [x] n — Skip next (with optional skip warning modal)
- [x] p — Previous song (or restart if at first)
- [x] r — Restart current song
- [x] v — Toggle bottom view
- [x] f — Toggle favorite
- [x] b — Toggle blocklist
- [x] o — Options modal (station/bitrate selection)
- [x] $ — Open RP donate page (placeholder)
- [x] q — Quit (with MPV cleanup)
- [x] Up/Down/j/k — Scroll viewport
- [x] g/G — Scroll to top/bottom

### Modals
- [x] Options modal (station selection, bitrate selection)
- [x] Skip warning modal (y/n confirmation)

### Data Features
- [x] Favorites (add/remove, star indicator, Python metadata.json compat)
- [x] Blocklist (add/remove, indicator)
- [x] Auto-skip blocklisted songs on natural transition
- [x] Lyrics display (plain text via LRCLib)
- [x] Synced lyrics display (time-synced, highlighted current line)
- [x] Artist info (Wikipedia summary + discography)

### Playback Engine
- [x] 1-second progress tick (tea.Tick, always re-armed)
- [x] 5-second poll tick for next-block fetching
- [x] Natural song transition detection (MPV playlist position)
- [x] Next-block polling when on last song of block
- [x] Playback position cached in model (no IPC in View)

### v2 Framework Compliance
- [x] `View()` returns `tea.View` with `.AltScreen = true`
- [x] `tea.EnterAltScreen` removed (v2 uses View field)
- [x] Tick commands use `tea.Tick()` (not `time.Sleep`)
- [x] `tea.KeyPressMsg` (not `tea.KeyMsg`)
- [x] `tea.RequestBackgroundColor` → `tea.BackgroundColorMsg`
- [x] `image/jpeg` and `image/png` decoder imports registered
- [x] Stale async result guards (eventID on fetch messages)

---

## TODO — Remaining Work

### High Priority
- [ ] Album art display — image decoding works now (jpeg/png registered), but
      rendering via go-termimg/sixel needs testing in kitty/sixel-capable terminal
- [ ] Album art — try Kitty protocol first, fall back to Sixel
- [ ] Copy album art to file (config: `copy_album_art` / `album_art_path`)

### Medium Priority
- [ ] Manage favorites modal (list, delete, play from favorites)
- [ ] Auto-playback from favorites (when no block available)
- [ ] Pause handling with 5-minute timeout (Python has this)
- [ ] Open RP donate page (`$` key — needs `xdg-open` integration)
- [ ] MPRIS metadata (via mpv-mpris plugin, should work automatically)
- [ ] Modal overlay — current `placeModal()` replaces base view instead of overlaying

### Low Priority
- [ ] Background color adaptive styling (use `isDark` from BackgroundColorMsg)
- [ ] Responsive layout (adapt now-playing/art widths to terminal size)
- [ ] Error recovery (retry block fetch on network error)
- [ ] Logging to file (structured, with log levels)
- [ ] Unit tests (API clients, config, cache)

---

## Architecture

```
rptui-bubbletea/
├── cmd/rptui/main.go           # Entry point
├── internal/
│   ├── api/                    # RadioParadise, LRCLib, Wikipedia clients
│   ├── cache/                  # Favorites + blocklist management
│   ├── config/                 # Config + theme loading
│   ├── models/                 # Song data model
│   ├── mpv/                    # MPV IPC backend
│   └── tui/                    # Bubbletea app
│       ├── app.go              # Main Model (Init/Update/View)
│       ├── messages.go         # Message types + tick commands
│       ├── modals/             # Options, SkipWarning modals
│       └── widgets/            # Header, Footer, NowPlaying, Playlist
├── AGENTS.md                   # Agent guidelines
├── STATUS.md                   # This file
├── go.mod
└── go.sum
```
