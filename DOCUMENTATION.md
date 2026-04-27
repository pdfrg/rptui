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
                            Example: rptui --cache 2h Rock FLAC (station substrings accepted)

    --offline [CACHE_NAME]  Launch TUI in offline playback mode
                            If CACHE_NAME omitted, prompts for selection
                            Example: rptui --offline 2024-01-15_main_mix_320k

    --list-caches           List all available offline caches and exit

    --delete-cache <NAME>   Delete a named offline cache (prompts for confirmation)

SLEEP TIMER / ALARM:
    --sleep <DURATION>       Start sleep timer (e.g., 20m, 1.5h)
                            App pauses and quits after timer expires
    --alarm <TIME>           Schedule alarm (e.g., 7:20am, 7:20 a.m., 19:20)
                            App starts at specified time
ACTIONS:
--lastfm-auth          Run Last.fm OAuth authentication flow and save session key
--rp-auth              Authenticate with Radio Paradise account
                       Enables user ratings, comments, favorites sync, and My Paradise channel
                       (optional — all features work without an RP account)
--setup-dj-skip        Download TVSM model for DJ speech skipping
                       (~2.5GB Python dependencies, 10-20 min install time)
--create-colors-file   Print color theme template to stdout
--test-terminal-colors Query and display terminal color information

EXAMPLES:
    rptui                   Launch with default settings
    rptui -j                Launch in jukebox mode
    rptui --cache 4h        Record 4 hours of current station/bitrate
    rptui --offline         Play back a previously recorded cache
    rptui --list-caches     See what caches are available
    rptui --create-colors-file > ~/.config/rptui/colors.toml
    rptui --test-terminal-colors  Check terminal color support and palette

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
| `force_protocol` | string | Force image protocol: `kitty`, `sixel`, `iterm2`, `halfblocks` (default: auto-detect) |
| `show_album_art` | bool | Display album art (auto-fallback: kitty > iterm2 > sixel > unicode) |
| `copy_album_art` | bool | Save album art to file |
| `album_art_path` | string | Path for copied album art (default: `/tmp/cover.jpg`) |
| `colors_file` | string | Custom colors.toml file path |
| `theme` | string | Built-in theme: `catppuccin-mocha`, `gruvbox-dark`, `dark-red`, `osaka-jade`, `synth`, `basic` |
| `transparent_background` | bool | Use terminal's default background color |
| `disable_theme` | bool | Disable all theming, use terminal's default colors |
| `terminal_palette.cursor` | int | Palette index for cursor color (0-15, default: 2, green) |
| `terminal_palette.accent` | int | Palette index for accent color (0-15, default: 4, blue) |
| `terminal_palette.muted` | int | Palette index for muted color (0-15, default: 8, gray) |
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
| `visualizer.real_audio` | bool | Use real audio capture for visualizer (Linux: PipeWire/PulseAudio, Windows: WASAPI, macOS: SoX+BlackHole). Default: true. |

### Jukebox Mode

| Setting | Type | Description |
|---------|------|-------------|
| `jukebox.min_faves` | int | Minimum favorites required |
| `jukebox.repeat` | bool | Repeat after playing all favorites |
| `jukebox.crossfade_duration` | float | (Pseudo) crossfade duration in seconds (0 to disable) |

### DJ Speech Skipping

| Setting | Type | Description |
|---------|------|-------------|
| `skip_dj_segments` | bool | Enable automatic skipping of DJ speech at end of songs |
| `dj_check_seconds` | int | Seconds from end of song to check for DJ speech (5-120, default: 80) |
| `dj_confidence` | float | Minimum confidence for speech detection (0.1-0.99, default: 0.88) |
| `dj_safety_buffer` | float | Extra seconds to add after detected speech for safe skipping (0-5, default: 0.5) |
| `dj_min_speech_duration` | float | Minimum speech segment duration in seconds to count as DJ talk (5-60, default: 15.0) |

### Lidarr Integration

| Setting | Type | Description |
|---------|------|-------------|
| `lidarr.enabled` | bool | Enable Lidarr integration (default: false) |
| `lidarr.url` | string | Lidarr base URL (e.g., `http://localhost:8686`) |
| `lidarr.api_key` | string | API key from Lidarr Settings > General |

## Themes

There are 6 built in themes: basic, catppuccin-mocha, dark-red, gruvbox-dark, osaka-jade, synth.
View them all here: [SCREENSHOTS.md](SCREENSHOTS.md)

Reads current Omarchy theme from `~/.config/omarchy/current/theme/colors.toml`

**To use your Omarchy theme, make sure to leave `theme = ''` and `colors_file = ''` (both defaults).**

**For non-Omarchy users, default settings will fallback to Catppuccin Mocha.  To turn off themes entirely, use `disable_theme = true`.**

Custom themes can be provided via a `colors.toml` file. To use a custom theme, add the path to your `config.toml`:

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
| `p` | Previous song (or restart if current song is first in playlist) |
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
| `f` | Toggle favorite. Hourglass icon appears in playlist while download active. Star when download complete. |
| `b` | Toggle blocklist. Block icon appears in playlist. |
| `R` | Open rating modal (requires RP account) |
| `L` | Open current artist in Lidarr (when configured) |

### Modals

| Key | Action |
|-----|--------|
| `o` | Open options modal |
| `m` | Open favorites modal |
| `i` | Open artist gallery (when viewing artist with images) |

### Comments

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
- **DJ venv**: `$XDG_CACHE_HOME/rptui/env/`
- **DJ model**: `$XDG_CACHE_HOME/rptui/tvsm_models/`
- **DJ detection cache**: `$XDG_CACHE_HOME/rptui/smad/cache/`
- **Log**: `$XDG_STATE_HOME/rptui/rptui.log`

On first run, a default configuration file is created.

## Hyprland (Omarchy) integration

**Add a Hyprland launcher for each layout**

Place in `~/.config/hypr/bindings.conf`.
Customize command to point to the rptui binary on your system, or just 'rptui' if in PATH.
Size is in the format (width height). Adjust to your preferences.
Sizes below are recommended minimums, based on use of 11pt font in the terminal.
For large (default) and medium, more narrow will work, but some keybindings won't be shown in the footer.
For different terminal font sizes, optimal width and height values can vary widely.

```
bindd = SUPER SHIFT, R, rptui, exec, xdg-terminal-exec --app-id=rptui.medium -e /path/to/rptui/rptui --layout medium
windowrule = match:class rptui.medium, size 1060 460, float on, center on

bindd = SUPER ALT SHIFT, R, rptui, exec, xdg-terminal-exec --app-id=rptui.compact -e /path/to/rptui/rptui --layout compact
windowrule = match:class rptui.compact, size 370 400, float on, center on

bindd = SUPER CTRL SHIFT, R, rptui, exec, xdg-terminal-exec --app-id=rptui.narrow -e /path/to/rptui/rptui --layout narrow
windowrule = match:class rptui.narrow, size 370 750, float on, center on

bindd = SUPER, R, rptui, exec, xdg-terminal-exec --app-id=rptui.large -e /path/to/rptui/rptui --layout large
windowrule = match:class rptui.large, size 1060 850, float on, center on
```

**Desktop integration (available from launchers and menus)**

```
# Use your preferred editor
nano ~/.local/share/applications/rptui.desktop
```

Copy / paste / edit paths as needed:

```
[Desktop Entry]
Version=1.0
Name=rptui
Comment=RadioParadise TUI
Exec=xdg-terminal-exec --app-id=rptui.large -e /path/to/rptui/rptui --layout large
Terminal=false
Type=Application
Icon=/path/to/rptui/assets/rptui-icon.png
StartupNotify=true
```

You can add additional desktop entries for each preferred layout.

Be sure to name each `.desktop` file with a different name, e.g. `rptui-narrow.desktop`.
Then change the `Exec` line so that the layout matches in  `--app-id=rptui.<layout>` and `--layout <layout>`.


## TUI Elements Explained

`6.2  │  🔑 --`

Community rating average / User rating when available

**Key icon is only displayed when RP account configured and successfully authorized.**

When no user is rating set, displays "--"

`󰒮 5  󰒭 3  ⭐ 12/2 ✅ <12>`

5: Songs in playlist before current (available uses of "play previous").

3: Songs in playlist after current (available uses of "play next").

Useful for alternate layouts or views where playlist is not visible.

12: Total number of favorites.

2: Minimum number of favorites required to auto-queue favorites when needed and disable skip warning.

**Green checkmark icon is only displayed when number of favorites >= `min_favorites`.**

When present, a random favorite will be enqueued at playlist end while awaiting new songs from RP.
Process repeats until new songs are received.
Very useful when you wish to skip ahead.  If you never skip songs, "favorites mode" will not activate (only used on an as-needed basis).
You may choose to play or enqueue favorites anytime from the `Manage` modal.

<12>: Number of favorites remaining to auto-queue (no repeats).  When all are used (value = 0), favorites will be re-shuffled and re-enabled.


`[fm], [lb], or [fm][lb]`

Only visible in the footer when scrobbling is enabled and successfully authorized.
At song completion, will display in accent color for 5 seconds on success, flash 5 seconds on failure.  Check log on failure.
When both configured, each updates independently.

`[L]`

Only visible in the footer when Lidarr is enabled.
For artists not in your Lidarr library, displays in muted text.
For artists in Lidarr AND monitored, displays in accent colored text (same color as hotkeys).
For artists in Lidarr and NOT monitored, displays in normal (foreground) text (same color as inactive playlist entries).


## Scrobbling (Optional)

To enable scrobbling, you will need to configure at least one service:

**Last.fm**: Two options...
1. Build from source or go install.
Requires your own last.fm API account -- free, easy sign-up at [last.fm](https://www.last.fm/api/account/create).
Pass API key and shared secret as build flags.

```bash
go build -ldflags "-s -w -X rptui/internal/api.LastFMAPIKey=YOUR_KEY -X rptui/internal/api.LastFMSharedSecret=YOUR_SECRET" -o rptui ./cmd/rptui
```
2. Download binary with API key and shared secret built-in.

For both methods (1) and (2): run `rptui --lastfm-auth`.
Will open default browser to Last.fm login page to authorize app.
Session key will be automatically added to rptui config file.
Session key does not expire.

**ListenBrainz**: Get a free token from https://listenbrainz.org/settings/

Set via config file:
```toml
listenbrainz_token = "your-token"
```

## DJ Speech Skipping (Optional)

Uses a TVSM CRNN neural network to detect DJ speech within songs and automatically seeks past it.

### Setup

Run `rptui --setup-dj-skip`. This will:

1. Create an isolated Python virtual environment
2. Install PyTorch + audio libraries (~2.5GB download, 10-20 min)
3. Download the TVSM speech detection model (~11MB)
4. Convert the model to runtime format

A confirmation prompt is shown before proceeding. If setup is already complete, the command exits immediately.

### How It Works

- Only the last `dj_check_seconds` (default: 80) of each song are analyzed. RP DJ interludes are always at song end.
- Detection only runs on the **last song in each RP programming block** — DJs only speak at block boundaries. Favorites and jukebox songs are always checked since they lack block context, but results are cached permanently so detection only runs once per song.
- Speech segments shorter than 15 seconds are ignored (filters out brief spoken-vocal moments that aren't DJ talk). Some speech segments are >60s.
- Detected speech must end within 1.5s of the song's end — RP DJs talk at the very end of a track, so speech ending further from the end is rejected as a false positive (e.g., sung vocals).
- A `dj_safety_buffer` (default: 0.5s) is added before and after the detected speech to ensure segment is skipped (no jarring partial spoken word effect).
- Results are cached per-song, so re-playing a song doesn't re-run detection.
- Detection runs in the background and doesn't block playback or other song-change logic.

### Config

Enable in `config.toml`:

```toml
skip_dj_segments = true
dj_check_seconds = 80
dj_confidence = 0.88
dj_safety_buffer = 0.5
dj_min_speech_duration = 15.0
```

## Lidarr Integration (Optional)

Shows artist and album monitoring status from your [Lidarr](https://lidarr.audio/) music collection manager.

### Setup

Add to `config.toml`:

```toml
[lidarr]
enabled = true
url = "http://localhost:8686"
api_key = "your-api-key"
```

Get the API key from Lidarr > Settings > General.

### How It Works

When a song changes, rptui looks up the current artist in Lidarr via their MusicBrainz ID:

- **If the artist IS in your Lidarr library**: Shows monitored (●) or not monitored (○) status in the artist view and footer. Press `L` to open the artist page in Lidarr.
- **If the artist is NOT in your Lidarr library**: Shows "not in Lidarr" (⊝) status. Press `L` to open the "Add New Artist" search page in Lidarr. Artists are never added automatically. The Lidarr page opens for your review and confirmation.

Album monitoring status is also shown in the artist discography when available.

## Artist Image Gallery

Discogs consistently provides more artist images than TheAudioDB.
However, to download Discogs images, a Discogs "personal access token" is required.
You may sign up for free at [Discogs](https://www.discogs.com/settings/developers).
Without a Discogs token, the artist image gallery feature will work, but will be limited to 4 images (max returned by TheAudioDB).



