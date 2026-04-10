# RadioParadise TUI - Documentation

## Command Line Reference

```
Radio Paradise TUI - A terminal UI for Radio Paradise

USAGE:
    rptui [FLAGS]

FLAGS:
    -h, --help              Show this help message and exit
    -v, --version           Show version information and exit
    -j, --jukebox           Launch in jukebox mode (random favorites playback)
        --layout LAYOUT     Set UI layout: large, medium, compact, narrow
                            large: full layout with all elements (default)
                            medium: no bottom view (no playlist/lyrics/visualizer)
                            compact: no album art, no bottom view, mini footer
                            narrow: album art top-left, now playing below, mini footer

OFFLINE CACHE:
    --cache <DURATION> [STATION] [BITRATE]
                            Record audio cache for offline playback
                            DURATION: recording length (e.g., 2h, 3.5h)
                            STATION: station name or number (default: from config)
                            BITRATE: bitrate name or number (default: from config)
                            Example: rptui --cache 2h "Rock Mix" FLAC

    --offline [CACHE_NAME]  Launch TUI in offline playback mode
                            If CACHE_NAME omitted, prompts for selection
                            Example: rptui --offline 2024-01-15_main_mix_320k

    --list-caches           List all available offline caches and exit

    --delete-cache <NAME>   Delete a named offline cache (prompts for confirmation)

ACTIONS:
    --lastfm-auth           Run Last.fm OAuth authentication flow and save session key
    --rp-auth               Authenticate with Radio Paradise account
                            Enables user ratings, comments, favorites sync, and My Paradise channel
                            (optional — all features work without an RP account)

EXAMPLES:
    rptui                   Launch with default settings
    rptui -j                Launch in jukebox mode
    rptui --cache 4h        Record 4 hours of current station/bitrate
    rptui --offline         Play back a previously recorded cache
    rptui --list-caches     See what caches are available
    rptui --create-colors-file > ~/.config/rptui/colors.toml

STATIONS:
    0 - The Main Mix  1 - Mellow Mix    2 - RockIt!
    3 - The Globe     42 - Serenity     5 - Beyond...
    945 - KFAT

BITRATES:
    1 - 64k AAC   2 - 128k AAC   3 - 320k AAC   4 - FLAC

CONFIGURATION:
    Config file: $XDG_CONFIG_HOME/rptui/config.toml (default: ~/.config/rptui/config.toml)
    Cache dir:   $XDG_CACHE_HOME/rptui/ (default: ~/.cache/rptui/)
    Log file:    $XDG_STATE_HOME/rptui/rptui.log (default: ~/.local/state/rptui/)
```

## Configuration File

The config file is located at `~/.config/rptui/config.toml`. It is created automatically on first run with default values.

### General Settings

| Setting | Type | Description |
|---------|------|-------------|
| `channel` | int | Default station (0=Main, 1=Mellow, 2=Rock, 3=Globe, 42=Serenity, 5=Beyond, 945=KFAT) |
| `bitrate` | int | Audio quality (1=64k, 2=128k, 3=320k, 4=FLAC) |
| `layout` | string | UI layout: `large`, `medium`, `compact`, `narrow` |
| `show_album_art` | bool | Display album art (auto-fallback: kitty > iterm2 > sixel > unicode) |
| `copy_album_art` | bool | Save album art to file |
| `album_art_path` | string | Path for copied album art (default: `/tmp/cover.jpg`) |
| `colors_file` | string | Custom colors.toml file path |
| `theme` | string | Built-in theme: `catppuccin-mocha`, `gruvbox-dark`, `dark-red`, `osaka-jade`, `synth`, `basic` |
| `notifications_enabled` | bool | Show desktop notifications |
| `notifications_show_art` | bool | Include album art in notifications |

### Favorites & Blocklist

| Setting | Type | Description |
|---------|------|-------------|
| `favorites_dir` | string | Directory for favorites (default: `$XDG_CACHE_HOME/rptui/favorites`) |
| `max_favorites` | int | Maximum favorites to store |
| `min_favorites` | int | Minimum favorites to enable autoplay |
| `show_skip_warning` | bool | Warn when skipping ahead of livestream, disabled when number of favorites > min_favorites |

### RadioParadise Account

| Setting | Type | Description |
|---------|------|-------------|
| `rp_auth.username` | string | Your RP account username |
| `rp_auth.password` | string | Your RP account password |
| `auto_download_rp_favorites` | bool | Auto-download songs from your RP favorites |
| `auto_blocklist_rp_enabled` | bool | Auto-blocklist songs based on RP ratings |
| `auto_blocklist_rp_threshold` | int | Rating threshold for auto-blocklist (1-4) |

### Last.fm Scrobbling

| Setting | Type | Description |
|---------|------|-------------|
| `lastfm.enabled` | bool | Enable Last.fm scrobbling |
| `lastfm.session_key` | string | Session key from `rptui --lastfm-auth` |

### ListenBrainz Scrobbling

| Setting | Type | Description |
|---------|------|-------------|
| `listenbrainz.enabled` | bool | Enable ListenBrainz scrobbling |
| `listenbrainz.token` | string | User token from https://listenbrainz.org/profile/ |

### Discogs API

| Setting | Type | Description |
|---------|------|-------------|
| `discogs_token` | string | Personal access token from https://www.discogs.com/settings/developers |
| `discogs_key` | string | Consumer key (alternative to token) |
| `discogs_secret` | string | Consumer secret (alternative to token) |

### Visualizer

| Setting | Type | Description |
|---------|------|-------------|
| `visualizer.mode` | string | Visualizer style: `Bars`, `Braille`, `ClassicPeak`, `Wave`, `Stars`, `BrailleBars`, `Rain`, `Segmented`, `Binary` |
| `visualizer.show_info` | string | Song info overlay: `fade`, `on`, `off` |
| `visualizer.info_duration` | int | Seconds to show song info (default: 5) |
| `visualizer.real_audio` | bool | Use PipeWire audio capture (requires `pw-record`) |

### Jukebox Mode

| Setting | Type | Description |
|---------|------|-------------|
| `jukebox.min_faves` | int | Minimum favorites required |
| `jukebox.repeat` | bool | Repeat after playing all favorites |
| `jukebox.crossfade_duration` | float | Crossfade duration in seconds (0 to disable) |

## Theme Files

Custom themes can be provided via a `colors.toml` file. To use a custom theme, add to your `config.toml`:

```toml
colors_file = "/path/to/your/colors.toml"
```

Example `colors.toml`:

```toml
# RadioParadise TUI Color Theme
# Place in ~/.config/rptui/colors.toml and reference in config.toml
# Priority: colors_file > theme > Omarchy > Catppuccin Mocha fallback

# [base] - Core UI colors (required)
# [colors] - ANSI 256-color palette (optional, used for fallbacks)

[base]
# Main UI colors
background = "#1e1e2e"  # Window background, panels
foreground = "#cdd6f4"  # Primary text, song info
accent = "#89b4fa"     # Song titles, hotkeys, progress bar gradient, current selection
muted = "#6c7086"      # Secondary text, borders, inactive elements
cursor = "#f5c2e7"     # Playback position indicator, current playlist item

[colors]
# ANSI 256-color palette (colors 0-7 standard, 8-15 bright)
# Used as fallbacks when accent/cursor need to differ from foreground
color0  = "#45475a"    # black
color1  = "#f38ba8"    # red
color2  = "#a6e3a1"    # green
color3  = "#f9e2af"    # yellow
color4  = "#89b4fa"    # blue
color5  = "#f5c2e7"    # magenta
color6  = "#94e2d5"    # cyan
color7  = "#bac2de"    # white
color8  = "#585b70"    # bright black (gray)
color9  = "#f38ba8"    # bright red
color10 = "#a6e3a1"    # bright green
color11 = "#f9e2af"    # bright yellow
color12 = "#89b4fa"    # bright blue
color13 = "#f5c2e7"    # bright magenta
color14 = "#94e2d5"    # bright cyan
color15 = "#a6adc8"    # bright white
```

**Tip:** Generate a starter template with `rptui --create-colors-file`.

## Keyboard Shortcuts

### Playback

| Key | Action |
|-----|--------|
| `Space` | Play/Pause |
| `s` | Stop playback |
| `n` | Skip to next song |
| `p` | Previous song (or restart if at beginning) |
| `r` | Restart current song |
| `←` | Seek backward 10 seconds |
| `→` | Seek forward 10 seconds |
| `q` | Quit |

### Views & Navigation

| Key | Action |
|-----|--------|
| `v` | Cycle bottom view (Playlist → Lyrics → Synced Lyrics → Artist → Comments → Visualizer → Off) |
| `F` | Toggle fullscreen visualizer (when in visualizer view) |
| `Up/Down` or `j/k` | Scroll viewport / Cycle visualizer modes |
| `g` | Scroll to top |
| `G` | Scroll to bottom |
| `u` | Update to current song, when viewing previous song lyrics, artist info, or comments |

### Song Actions

| Key | Action |
|-----|--------|
| `f` | Toggle favorite |
| `b` | Toggle blocklist |
| `R` | Open rating modal (requires RP account) |
| `1-9, 0` | Rate current song 1-10 |

### Modals

| Key | Action |
|-----|--------|
| `o` | Open options modal |
| `m` | Open favorites modal |
| `i` | Open artist gallery (when viewing artist with images) |

### Comments (when in comments view)

| Key | Action |
|-----|--------|
| `l` | Load more comments / Next page |
| `P` | Previous page |

### Other

| Key | Action |
|-----|--------|
| `J` | Toggle jukebox mode |
| `c` | Copy current song info to clipboard |
| `$` | Open RP donate page |
| `Ctrl+C` | Quit |

## File Paths

The app follows the XDG Base Directory Specification:

| Path | Default | Env Variable |
|------|---------|--------------|
| Config | `~/.config/rptui/` | `$XDG_CONFIG_HOME` |
| Cache | `~/.cache/rptui/` | `$XDG_CACHE_HOME` |
| State | `~/.local/state/rptui/` | `$XDG_STATE_HOME` |

- **Config**: `$XDG_CONFIG_HOME/rptui/config.toml`
- **Auth**: `$XDG_CONFIG_HOME/rptui/auth.toml`
- **Favorites**: `$XDG_CACHE_HOME/rptui/favorites/`
- **Blocklist**: `$XDG_CACHE_HOME/rptui/blocklist/`
- **Offline cache**: `$XDG_CACHE_HOME/rptui/offline/`
- **Log**: `$XDG_STATE_HOME/rptui/rptui.log`

## External Dependencies

### Required
- **mpv** - Audio playback

### Recommended
- **mpv-mpris** - MPRIS support for media keys
- **notify-send** - Desktop notifications
- **PipeWire** (with pipewire-alsa) - Audio visualization
- **pw-record** - Real-time audio capture for visualization

## Hyprland (Omarchy) integration

Add launchers for each layout

```
# place in ~/.config/hypr/bindings.conf
# customize command to point to the rptui binary on your system, or just 'rptui' if in PATH
# size is in the format (size width height), adjust to your preference, sizes below are recommeneded minimums

bindd = SUPER SHIFT, R, rptui medium, exec, ghostty --class=rptui.medium --command="/path/to/rptui --layout medium"
windowrule = match:class rptui.medium, size 975 460, float on, center on

bindd = SUPER ALT SHIFT, R, rptui compact, exec, ghostty --class=rptui.compact --command="/path/to/rptui --layout compact"
windowrule = match:class rptui.compact, size 370 400, float on, center on

bindd = SUPER CTRL SHIFT, R, rptui narrow, exec, ghostty --class=rptui.narrow --command="/path/to/rptui --layout narrow"
windowrule = match:class rptui.narrow, size 370 750, float on, center on

bindd = SUPER, R, rptui, exec, ghostty --class=rptui.large --command="/path/to/rptui --layout large"
windowrule = match:class rptui.large, size 975 850, float on, center on
```

## UI Explained

6.2  │  🔑 --

Community rating average | User rating
**Key icon only displayed when RP account configured and successfully authorized**
When no user rating set, displays "--"

󰒮 5  󰒭 3  ⭐ 12/2 ✅ <12>

5: Songs in playlist before current (available uses of "play previous")
3: Songs in playlist after current (available uses of "play next")
Useful for alternate layouts where playlist is not visible
12: Total number of favorites
2: Minimum number of favorites to auto-queue favorites when needed and disable skip warning
<12>: Number of favorites remaining to auto-queue (no repeats).  When all used, will re-shuffle and reset to total favorites

[fm], [lb], or [fm+lb]

Only visible when scrobbling is enabled and successfully authorized.
At song completion, will display in accent color for 5 seconds on success, flash 5 seconds on failure.  Check log on failure.


