# RadioParadise TUI

<img src="assets/rptui-icon.png" alt="rptui icon" width="200" align="right" style="margin-left: 20px; margin-top: -10px;">

**The ultimate (terminal) client for Radio Paradise.**

A fast, beautiful Go + Bubble Tea TUI designed for terminal lovers who want more from Radio Paradise than the official web or Android clients.



## Why rptui?

While Radio Paradise offers the absolute best internet radio stations, human-curated and completely commercial-free, the official clients are heavy and limited, while generic internet radio players lack depth and suffer from buffering.  rptui delivers full audio file playback, beautiful visuals, real offline capability, tight Radio Paradise integration, and extensive customization in one fast, light package.


![rptui in default layout](assets/rptui-default-playlist-view.png)


## Features

- **All RP Stations/Qualities**: Play any RadioParadise channel (Main Mix, Mellow Mix, RockIt!, The Globe, Beyond..., Serenity, KFAT) at your choice of bitrate (64k/128k/320k/FLAC).
- **No Stream Buffering**: Uses same full audio file based playback as the official clients.
- **Skip/Previous/Seek/Restart Song**: Same functionality as provided by local music players.
- **Favorites**: Mark songs as favorites. Auto-queues favorites for playback when skipped ahead of livestream.  Random play favorites in jukebox mode.  Search favorites with `/`, play immediately or enqueue.
- **Blocklist**: Add songs to blocklist.  Auto-skips blocklisted songs.
- **Lyrics**: Fetch lyrics from [LRCLib](https://lrclib.net/) — plain and synced (when available).
- **Artist Info**: Smart query for artist bios and album descriptions from [TheAudioDB](https://www.theaudiodb.com/), [Discogs](https://www.discogs.com/), and Wikipedia.
- **Artist Images**: Smart query for artist thumbnails and image galleries from Discogs (requires user token) and TheAudioDB (no token required).
- **Artist Discographies**: All official studio albums from [MusicBrainz](https://musicbrainz.org/)
- **Album Art**: Smart terminal support via go-termimg (Kitty, iTerm2, Sixel, Unicode fallback).  Terminals with Kitty image protocol support recommended.
- **Visualizations**: 9 real-time audio visualizations (bars, braille, wave, rain, classic, stars, etc.).  Full terminal window toggle, true fullscreen when terminal maximized.
- **Scrobbling**: [Last.fm](https://www.last.fm/) and [ListenBrainz](https://listenbrainz.org/) support.
- **Themes**: Automatic theme detection from Omarchy with live-reloads, 6 built-in themes, custom colors.toml support.  Smart parsing of Omarchy themes for optimum color choices (tested on 70 themes).
- **Jukebox Mode**: Random play all favorites, option to re-shuffle and repeat all for endless playback.  Works offline.
- **Offline Mode**: Cache and play any station from offline recordings.
- **Network Status Handling**: Smart prompts offer to change modes when network change detected, so that music keeps playing. 
- **Desktop Integration**: MPRIS metadata, media key support, desktop notifications.
- **Four Smart Layouts**: `--layout large` (default, full dashboard with multiple bottom views available), `medium`, `compact`, and `narrow` (perfect vertical sidebar).
- **Keyboard Navigation**: Hotkeys and RP stations shown in footer.
- **Sleep Timer/Alarm**: Fall asleep or wake to the sounds of RP.  

## RP Account Support
- **Ratings**: Displays all your user ratings.  Submit ratings (1-10), just as in the official clients.
- **Comments**: Show song comments.  Loads 20 comments at a time with pagination.
- **My Paradise**: Appears as additional station.  Stream all songs above rating threshold (set in RP account, default 7+) without need to download.
- **Auto-Download Favorites**: Configurable setting (default = false).  Grabs all songs with user rating above threshold when they appear on the RP playlist.
- **Auto-Add to Blocklist**: Configurable setting (default = false).  Auto-skip all songs with user rating below configurable threshhold (default <4).

## Screenshots

**Visualizer**
![Visualizer](assets/rptui-visualizer-fullscreen.png)

rptui offers four unique layouts and multiple views (playlist, lyrics, synced lyrics, artist info, artist image gallery, song comments, visualizer, and full-window visualizer).
See [SCREENSHOTS.md](SCREENSHOTS.md) for the full gallery, including all built-in themes.

## Installation

### Prerequisites

- **mpv** — Required for audio playback
- **Go 1.22+** — To build from source

### Recommended Dependencies

- **mpv-mpris** — MPRIS support for media keys and desktop integration
- **notify-send** — Desktop notifications on song changes with optional album art
- **PipeWire** (with pipewire-alsa) — Required for real audio visualization

### Quick Installation

```bash
# Recommended: install via Go
# Both go install and go build require last.fm API account for scrobbling, see DOCUMENTATION.md)
go install github.com/pdfrg/rptui@latest
```
### Build from Source

```bash
git clone https://github.com/pdfrg/rptui.git
cd rptui
go build -o rptui ./cmd/rptui
```

A pre-built binary for Linux/x86 with last.fm support baked-in is downloadable from releases.  Only a last.fm user account is required.  See DOCUMENTATION/Scrobbling for details.

## Attribution

Audio visualizations: [cliamp](https://github.com/bjarneo/cliamp).  Awesome music player with retro Winamp style in the terminal.

## Documentation

For detailed configuration options and advanced usage, see [DOCUMENTATION.md](DOCUMENTATION.md).

## License

MIT
