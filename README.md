# Cue

![Cue Media Player](assets/screenshot1.png)

> A fast terminal client for browsing and playing media from Plex and Jellyfin servers

## Features

- **Fast browsing:** Navigate large media libraries with a keyboard-first interface.
- **Unified TV view:** Explore seasons and episodes in one collapsible tree.
- **Playback tracking:** Sync progress and watched status with Plex and Jellyfin through mpv IPC.
- **Binge watching:** Play full seasons as gapless native mpv playlists.
- **Search and filtering:** Find titles globally, filter the current view, or hide watched media.
- **Artwork and metadata:** View posters, media details, and progress in the inspector.
- **Playlists and queue:** Build playlists and manage what to watch next.
- **Plex server discovery:** Find and switch servers without entering their addresses manually.
- **Local caching:** Browse and search a responsive local library cache.

## Quick Start

Download a binary from [Releases](https://github.com/SuperCoolPencil/cue/releases), or install Cue with Go:

```bash
go install github.com/SuperCoolPencil/cue@latest
```

Launch Cue:

```bash
cue
```

Cue asks for your server URL, detects Plex or Jellyfin, and guides you through authentication.

## Usage

### Switch Plex Servers

After configuring Cue with Plex, list the servers available to your account:

```bash
cue discover
```

In the picker:

| Key | Action |
|-----|--------|
| `↑` / `↓`, `j` / `k` | Move between connections |
| `g` / `G` | Jump to the first / last connection |
| `Enter` | Switch to the selected connection |
| `q` / `Esc` | Cancel |

For non-interactive use, select a server by its 1-based position in the discovery list:

```bash
cue discover --select 2
```

Discovery is Plex-only. Jellyfin continues to use the URL configured during setup.

### Artwork Preview

Cue uses the Kitty graphics protocol when running directly in Kitty. Other terminals—and Kitty sessions inside tmux, Zellij, or GNU screen—use the ASCII fallback automatically.

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `↑` `↓` `j` `k` | Navigate up/down |
| `←` `→` `h` `l` | Navigate left/right (columns) |
| `Enter` | Open a collection, or play/resume an item |
| `p` | Play from start |
| `.` | Play the next unplayed episode of a selected show, or the next episode in an open season |
| `w` / `u` | Mark watched / unwatched |
| `f` | Global search |
| `/` | Local filter (current column) |
| `Space` | Manage playlists |
| `a` | Add to / remove from queue |
| `x` | Delete playlist / remove item |
| `n` | Create new playlist (in Playlists view) |
| `s` | Sort options |
| `i` | Toggle inspector panel |
| `o` | Open selected item in server web browser |
| `r` / `R` | Refresh the current library / all libraries |
| `g` / `G` | Jump to top / bottom |
| `Ctrl+u` / `d` | Page up / half-page down |
| `Autoplay` | Toggle automatic next episode in Config menu |
| `Hide watched` | Toggle visibility of watched items in Config menu |
| `L` | Logout |
| `?` | Show help |
| `q` | Quit or go back |

### Intro and Outro Skipping

In mpv, Cue displays “Press x to skip intro/outro” while a known segment plays.
Press `Ctrl+x` to skip, or `Alt+x` to undo. The keys are configurable under `player.skip`;
user mpv keybindings take precedence. The Config menu cycles intro/outro skipping
through off, manual, and auto for subsequent playback sessions.

Cue uses Plex intro/credits markers, Jellyfin media segments (including those
published by compatible Intro Skipper plugins), and explicitly named embedded
chapters such as Intro, OP, Ending, ED, or Credits. When a stream has no chapters,
Cue adds runtime chapters for known intervals. Existing chapters are preserved;
original media files are modified only by an explicit `chapters --write` command.

Cue automatically inventories and analyzes all shows in the background at startup,
using one worker so browsing and playback can start immediately. It reuses existing
fingerprints and only repeats matching when a season changes. Disable this with
`player.skip.analysis_at_startup: false`.

You can also explicitly analyze a season using its server season ID:

```bash
cue analyze --season <season-id> --intro-window 600 --window 300 --audio-track 0
```

This requires at least three episodes and an FFmpeg build with the Chromaprint
muxer (`ffmpeg -h muxer=chromaprint`). Analysis fingerprints the beginning and end
of each episode and caches results by server, media source, and revision. Remote
analysis can transfer substantial media data. Startup analysis can continue while
you watch; it never blocks playback or library refresh. Repeated audio is only a candidate intro/outro: these experimental
results always require a manual skip, even in auto mode. Content after a bounded
outro remains playable.

Per-show `settings.json`, analysis JSON files, and generated Lua scripts share
`~/.config/cue/playback-settings/<server-account>/<show>/` (IDs are hashed).
Generated launch scripts are removed when playback ends; analysis persists.
After a complete successful inventory, Cue removes directories for absent series
and analysis files for removed episodes. Failed or incomplete inventories never
trigger cleanup, and other server profiles are left intact.

To export known segments as Matroska chapter XML, use
`cue chapters --item <item-id> > chapters.xml`. To embed them into a local original,
use `cue chapters --item <item-id> --write --file /path/to/original.mkv`.
Embedding requires `ffprobe` and `mkvpropedit`, refuses files with existing chapters,
stages a full copy (requiring free disk space), and retains the original as
`original.mkv.cue-chapters.bak`. Review experimental detection boundaries before
embedding them. The supplied file must correspond to the selected server source.

### Playback

Cue supports mpv, VLC, IINA, PotPlayer, and other system players. mpv is recommended because native playlists, resume tracking, and real-time scrobbling depend on its IPC support.

When playing a TV show with mpv, Cue sends the season as a native playlist. This enables:

- Gapless transitions between episodes.
- Starting at the selected episode or saved position.
- Updating progress throughout the playlist session.
- Marking preceding episodes as watched when you skip ahead.
- Marking an episode watched after it reaches the 90% threshold.
- Restoring the audio and subtitle tracks and timing offsets last selected for that show, including a disabled subtitle track.

On WSL, Cue detects Windows players from both `PATH` and Windows App Paths. Native Windows builds use the same detection.

If no supported player is found, Cue tries to open the raw media URL with the system's default handler. Resume is unavailable in this mode, and some MKV files or audio codecs may not work correctly.

## Configuration

Cue creates its configuration file on first run:

```text
~/.config/cue/config.yaml
```

## Attribution

Cue is forked from [Kino](https://github.com/mmcdole/kino), originally created by Matthew McDole. The original MIT license notice is preserved in `LICENSE`.

## License

MIT

Intro analysis scans the first 25% of each episode, capped at ten minutes by default.
Set `player.skip.intro_window_seconds` (30–900 seconds) to change the startup cap,
or use `--intro-window` for an explicit season scan. `--window` controls the outro
scan separately. Changed intro windows refresh intro fingerprints and reuse unchanged
outro fingerprints. Each episode needs a confident match; season completion does not
mean every episode has a detected intro.
