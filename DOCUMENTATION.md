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
  --audio-forward    Forward audio to client over SSH (requires ssh_audio_server in config)
  --lastfm-auth Run Last.fm OAuth authentication flow and save session key
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

On Linux, the config file generally is located at `~/.config/rptui/config.toml`. It is created automatically on first run with default values.

**To force-update your config file format to the latest version,**
**or to fix formatting problems caused by accidental or erroneous edits,**
**simply change stations from within the app to force an overwrite.**

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

### Audio

| Setting | Type | Description |
|---------|------|-------------|
| `audio.ssh_audio_server` | string | Audio server address for SSH forwarding (e.g., `tcp:localhost:4713`) |

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
| `dj_confidence` | float | Minimum confidence for speech detection (0.1-0.99, default: 0.88) |
| `dj_safety_buffer` | float | Extra seconds to add after detected speech for safe skipping (0-5, default: 0.5) |
| `dj_min_speech_duration` | float | Minimum speech segment duration in seconds to count as DJ talk (5-60, default: 10.0) |

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
The 1150 px width shown below accomodates all optional configurations (Lidarr and RP auth).  
If these are not enabled, smaller values will show the entire footer.
For different terminal font sizes, optimal width and height values can vary widely.

```
bindd = SUPER SHIFT, R, rptui, exec, xdg-terminal-exec --app-id=rptui.medium -e /path/to/rptui/rptui --layout medium
windowrule = match:class rptui.medium, size 1150 460, float on, center on

bindd = SUPER ALT SHIFT, R, rptui, exec, xdg-terminal-exec --app-id=rptui.compact -e /path/to/rptui/rptui --layout compact
windowrule = match:class rptui.compact, size 370 400, float on, center on

bindd = SUPER CTRL SHIFT, R, rptui, exec, xdg-terminal-exec --app-id=rptui.narrow -e /path/to/rptui/rptui --layout narrow
windowrule = match:class rptui.narrow, size 370 750, float on, center on

bindd = SUPER, R, rptui, exec, xdg-terminal-exec --app-id=rptui.large -e /path/to/rptui/rptui --layout large
windowrule = match:class rptui.large, size 1150 850, float on, center on
```

**Linux desktop integration (available from launchers and menus)**

```
# Use your preferred editor
nano ~/.local/share/applications/rptui.desktop
```
**NOTE: Requires xdg-terminal-exec. Available on Arch, OpenSUSE and Ubuntu 24.10+ and derivatives.**
Alternatively, the desktop entry can be edited to use your preferred terminal directly, though
syntax will likely need to be modified.  Check your terminal's options, e.g. `kitty --help`.

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

- Detection runs on **every song** (not just last in block). DJs may speak at the start or end of any song.
- A two-phase approach is used:
  - **Phase 1:** Only the first and last 10 seconds are scanned. If no speech frames are found in either boundary, detection stops (no speech).
  - **Phase 2:** If speech is found at either boundary, scanning expands outward from that boundary until a gap >2.5s with no speech is encountered (natural pause threshold). The DJ speech segment is considered complete.
- Speech segments are processed in 10-second chunks (up from 20s previously) for finer boundary detection.
- Speech segments shorter than `dj_min_speech_duration` (default: 10s) are ignored (filters out brief spoken-vocal moments that aren't DJ talk).
- Detected speech must start within 1.5s of song start OR end within 1.5s of song end — speech far from either boundary is rejected as a false positive (e.g., sung vocals).
- A `dj_safety_buffer` (default: 0.5s) is added before and after the detected speech to ensure the entire segment is skipped (no jarring partial spoken word effect).
- Results are cached per-song, so re-playing a song doesn't re-run detection.
- Detection runs in the background and doesn't block playback or other song-change logic.

### Config

Enable in `config.toml`:

```toml
skip_dj_segments = true
dj_confidence = 0.88
dj_safety_buffer = 0.5
dj_min_speech_duration = 10.0
```

**Config notes**
- Detection scans the first and last 10 seconds of each song (one 10s chunk each). Only if speech is found near a boundary does scanning expand outward to find the full speech segment. This minimizes CPU usage and download time for songs with no DJ speech.
- `dj_confidence`: Almost all confirmed DJ speech segments have very high confidence (>0.95).
However, occasionally a confirmed segment will have confidence ~0.90.  Unfortunately, for songs
with lyrics that "sound spoken", confidence values can be as high as ~0.94.  Lyrics rarely
continue to song boundaries, which the boundary proximity check (1.5s) already filters.
- `dj_safety_buffer`: Speech detection rarely omits the very beginning of the segment.
To avoid hearing a brief speech "blip", leave at 0.5s.  However, this can also skip the last
0.5s of actual music.  To hear as much of the music as possible, change to 0.
- `dj_min_speech_duration`: Shorter DJ speech segments (as brief as ~10s) have been observed.
Reducing this value catches shorter announcements, but may increase false positives from sung
vocals near song boundaries.

All DJ speech testing has been performed and optimized for Main Mix, RockIt!, and Mellow Mix
(all DJ'd by William).  Other stations with other DJs may have different characteristics
(shorter and/or longer DJ speech segments).  Please report.

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

Album download status is shown next to each album in the artist discography:

- ● **downloaded** — all tracks present on disk
- ◐ **partial** — some but not all tracks downloaded
- ○ **wanted** — monitored but no files yet (normal color)
- ○ **unmonitored** — in Lidarr but not monitored, no files (muted color)
- ⊝ **not in Lidarr** — album not in your Lidarr library

## Artist Image Gallery

Discogs consistently provides more artist images than TheAudioDB.
However, to download Discogs images, a Discogs "personal access token" is required.
You may sign up for free at [Discogs](https://www.discogs.com/settings/developers).
Without a Discogs token, the artist image gallery feature will work, but will be limited to 4 images (max returned by TheAudioDB).

## Offline Playback

Saving a pre-recorded cache is required for Offline Mode.  Building/saving this cache will take
approximately the same amount of time as the specified duration.  In other words, if you want 4 hours
of music for listening to on your flight tomorrow, you will need to allow `rptui --cache 4h`
to run for about 4 hours before you go offline.  When the cache has the duration of music specified,
the command will complete.

Unlike the official clients, rptui doesn't have access to the full RP library to rapidly download
an offline cache.  Rather, it simply monitors the specified station and grabs all the songs that are
played, in the order they are played.  It's like running the app normally, but with no TUI and no audio
output, just saved to disk.  Or imagine setting up your cassette recorder next to your
AM/FM clock radio and hitting record before you go to school, so while on your family vacation, you
can listen in the back of the car on your Walkman instead of hearing Dad's oldies or trying to tune-in
a decent radio station in the middle of nowhere.

**How to use:**

1. The night before your trip: `rptui --cache 6h Rock 128k`
- CLI only. Allow to run ~6h

2. While on the plane: `rptui --offline` (will prompt for cache selection)
- Normal TUI, normal listening experience, album art included.  No online integrations (lyrics,
artist info, RP account features, or Lidarr). Scrobbles will be cached and uploaded to your
configured service(s) when back online.

**Disk usage**

Recording at higher bitrates (320k AAC, and especially FLAC) can fill up disk space quickly.
Before the `--cache` command starts recording, it will prompt for confirmation with a disk
space estimate:

```
Cache recording configuration:
  Station: The Main Mix
  Bitrate: 320k AAC
  Target duration: 8h00m
  Estimated size: 1.07 GB

Continue? (y/n):
```

**Already on the plane and forgot to pre-record for offline mode?**

No worries! As long as you have been favoriting songs, Jukebox Mode has you covered.

Just run: `rptui -j`.

## Tmux, SSH, and Tmux+SSH Compatibility

rptui supports rendering images (album art, artist thumbnails, gallery) in
terminals that implement the Kitty graphics protocol. This section documents
which combinations of terminal, connection method, and multiplexer have been
tested.

### Kitty Graphics Protocol Support

The following terminals implement the Kitty graphics protocol and can display
images in rptui:

- **kitty** — native Kitty protocol support
- **rio** — Kitty protocol support
- **WezTerm** — Kitty protocol support
- **ghostty** — Kitty protocol support

### Tested Combinations

| Terminal | Local | SSH | Local + tmux | SSH + tmux |
|----------|:-----:|:---:|:------------:|:----------:|
| kitty | yes | yes | yes | yes |
| rio | yes | yes | yes | yes |
| WezTerm | — | yes | — | yes |
| ghostty | — | — | — | — |

`yes` = tested and working. `—` = not tested (WezTerm and ghostty do not run
on the developer's local GPU; ghostty lacks Ubuntu 22.04 support).

### Other Terminals (VTE-based)

VTE-based terminals (mate-terminal, gnome-terminal, xfce4-terminal, etc.) do
not support Kitty graphics but work with the Halfblocks fallback protocol:

| Terminal | Local | SSH | Local + tmux | SSH + tmux |
|----------|:-----:|:---:|:------------:|:----------:|
| mate-terminal | yes | yes | yes | yes |

Other VTE terminals should behave identically.

### Sixel and iTerm2

Sixel and iTerm2 protocols are not yet tested with SSH or tmux. They work
locally but compatibility under SSH or inside tmux is unconfirmed.

### Known Issues and Workarounds

**Rio and WezTerm over SSH**: The `TERM_PROGRAM` environment variable is not propagated
over SSH by default. Rio and WezTerm both set `TERM=xterm-256color`, which can cause failure
to detect Kitty image protocol support.  There are two options to ensure Kitty image
protocol is used.

1. Configure your terminal (preferred)
- Rio: in `config.toml` add `env-vars = ["TERM=rio"]`.  See [https://rioterm.com/docs/config#env-vars](https://rioterm.com/docs/config#env-vars).
- Wezterm: in `wezterm.lua` add `config.term = 'wezterm'`.  See [https://wezterm.org/config/lua/config/term.html](https://wezterm.org/config/lua/config/term.html).

2. Set `force_protocol = "kitty"` in your rptui config file.

**tmux image rendering**: tmux does not natively support the Kitty image
protocol (current work in progress).  rptui works around this using DCS passthrough sequences to send
Kitty commands directly to the outer terminal. This requires
`allow-passthrough=all` in tmux, which rptui sets automatically. Large gallery
images in tmux are scaled down to stay within tmux's DCS buffer limit, which
may result in slightly smaller images compared to non-tmux rendering.

### SSH Audio Forwarding

When running rptui over SSH, audio plays on the remote server's speakers by default. To hear audio on your local machine instead, use `--audio-forward` with a PulseAudio/PipeWire server address configured in `config.toml`.

This works by setting the `PULSE_SERVER` environment variable for the mpv subprocess, which redirects audio output to a remote PulseAudio or PipeWire server. PipeWire's PulseAudio compatibility layer means this works with both PulseAudio and PipeWire on either end.

#### One-time Local Setup

Your local machine (the SSH client, the one with speakers) needs to accept TCP connections for audio. This is a one-time setup per session.

**PulseAudio** (Ubuntu 22.04 and similar):

```bash
pactl load-module module-native-protocol-tcp port=4713 auth-ip-acl=127.0.0.1
```

Note the module number printed (e.g., `28`). If you run this multiple times, you'll get duplicate modules — check with:
```bash
pactl list modules short | grep native-protocol-tcp
```
To unload a module later: `pactl unload-module <number>`.

**PipeWire** (most common on modern Linux):

```bash
pactl load-module module-native-protocol-tcp port=4713
```

Same note applies — check for duplicates and unload with `pactl unload-module` if needed. PipeWire's PulseAudio compatibility layer handles this the same way.

#### Remote Configuration

Add to `config.toml` on the remote machine:

```toml
[audio]
ssh_audio_server = "tcp:localhost:4713"
```

#### Usage

Connect with SSH port forwarding (the `-R` flag tunnels port 4713 over SSH so no open ports are needed):

```bash
ssh -R 4713:127.0.0.1:4713 <remote-host>
```

Then run rptui with or without audio forwarding:

```bash
# Audio plays on server (default)
rptui

# Audio forwarded to your local machine
rptui --audio-forward
```

When SSH is detected and `ssh_audio_server` is configured but `--audio-forward` is not used, rptui shows a one-time reminder at startup with instructions.

#### Validation

`--audio-forward` will fail with an error message if:
- Not running in an SSH session (`SSH_CONNECTION` not set)
- `ssh_audio_server` is not configured in `config.toml`
- mpv was not built with PulseAudio support (`--ao=pulse` not available)

At runtime, rptui checks if the `PULSE_SERVER` tcp address is reachable. If the SSH tunnel is not set up correctly, a warning is logged to `rptui.log` with instructions.

#### Requirements

- **Both machines**: Linux
- **Local machine (SSH client)**: PulseAudio or PipeWire with TCP listener enabled (port 4713)
  - Run `pactl load-module module-native-protocol-tcp port=4713` once per session
- **Remote machine (SSH server)**: PulseAudio or PipeWire (provides `libpulse` used by mpv), mpv with PulseAudio support
- **SSH connection**: Must include `-R 4713:127.0.0.1:4713` port forwarding
