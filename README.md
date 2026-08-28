# Cue

![Cue Media Player](assets/screenshot1.png)

> A fast terminal client for browsing and playing media from Plex and Jellyfin servers

## Features

-  **Lightning Fast Browsing**: Instant, keyboard-driven navigation across massive media libraries.
-  **Unified TV Show View**: Explore seasons and episodes in a single, collapsible tree view.
-  **Native Binge-Watching**: Seamless, gapless playback for TV shows using native mpv playlists.
-  **Bulk Scrobbling**: Automatically marks previous episodes as watched when skipping ahead or finishing a season.
-  **Smart Filtering**: Hide watched movies, shows, and seasons with a single setting for a cleaner library view.
-  **Smart Scrobbling**: Real-time playback progress and watch status sync with Plex & Jellyfin via mpv IPC.
-  **Deep Metadata**: View rich details, media info, and progress bars in a dedicated inspector.
-  **Global Fuzzy Search**: Instantly find any movie or show with just a few keystrokes.
-  **Vim-Style Navigation**: Efficient, keyboard-first interface using familiar `h/j/k/l` bindings.
-  **Live Status Display**: Persistent 'Now Playing' and scrobble status in the footer.
-  **Playlist & Queue**: Manage your watch queue and playlists directly from the terminal.
-  **High-Performance Caching**: Snappy, progressive loading for a smooth browsing experience.

## Quick Start

### Installation

**Download** from [Releases](https://github.com/SuperCoolPencil/cue/releases) or install with Go:

```bash
go install github.com/SuperCoolPencil/cue@latest
```

### First Run

Launch Cue and follow the interactive setup:

```bash
cue
```

You'll be prompted to enter your server URL. Cue automatically detects whether it's a Plex or Jellyfin server and guides you through the appropriate authentication.

## Usage

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `↑` `↓` `j` `k` | Navigate up/down |
| `←` `→` `h` `l` | Navigate left/right (columns) |
| `Enter` | Play/Resume item |
| `p` | Play from start |
| `Shift+Enter` | Play the next unplayed episode of a selected show |
| `w` / `u` | Mark watched / unwatched |
| `f` | Global search |
| `/` | Local filter (current column) |
| `Space` | Manage playlists |
| `a` | Add to / remove from queue |
| `x` | Delete playlist / remove item |
| `n` | Create new playlist (in Playlists view) |
| `N` | Play next unwatched episode |
| `s` | Sort options |
| `i` | Toggle inspector panel |
| `r` / `R` | Refresh library / all |
| `g` / `G` | Jump to top / bottom |
| `Ctrl+u` / `d` | Page up / half-page down |
| `Autoplay` | Toggle automatic next episode in Config menu |
| `Play next episode on select` | Make Enter on a show play its next episode instead of opening seasons |
| `Hide watched` | Toggle visibility of watched items in Config menu |
| `L` | Logout |
| `?` | Show help |
| `q` | Quit/Back |

## Configuration

Config file: `~/.config/cue/config.yaml` (created on first run).

### Binge Watching & Native Playlists
Cue handles TV show playback by sending the entire season to mpv as a native playlist. This provides several benefits:
- **Gapless Transitions**: mpv handles the transition between episodes internally, ensuring zero delay.
- **Smart Start**: Playback always starts at your selected episode (or saved position) while keeping the rest of the season accessible in the player's playlist.
- **Bulk Progress Sync**: 
    - When you transition to a new episode, Cue automatically marks all *preceding* episodes in the playlist as watched on your server.
    - Reaching the 90% threshold on an episode automatically marks it and all previous unwatched episodes as played.
- **IPC Integration**: Real-time progress monitoring continues across the entire playlist session.

Other players (VLC, IINA, etc.) are supported for basic playback, but the native playlist and real-time scrobbling features require `mpv`.


## Attribution

Cue is forked from [Kino](https://github.com/mmcdole/kino), originally created by Matthew McDole. The original MIT license notice is preserved in `LICENSE`.

On WSL, Cue detects Windows-side players (PotPlayer, mpv.exe, VLC) from both `PATH` and Windows App Paths, so normal GUI installations work without extra configuration. Native Windows builds use the same detection.

If no media player is available, Cue opens the raw media URL with the platform's default URL handler. This is a best-effort browser fallback: resume is unavailable and some MKV/audio-codec combinations may play without audio. Installing mpv, VLC, or PotPlayer is recommended for reliable playback.

## License

MIT
