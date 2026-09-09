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
| `Enter` | Play or resume an item |
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
| `r` / `R` | Refresh the current library / all libraries |
| `g` / `G` | Jump to top / bottom |
| `Ctrl+u` / `d` | Page up / half-page down |
| `Autoplay` | Toggle automatic next episode in Config menu |
| `Play next episode on select` | Make Enter on a show play its next episode instead of opening seasons |
| `Hide watched` | Toggle visibility of watched items in Config menu |
| `L` | Logout |
| `?` | Show help |
| `q` | Quit or go back |

### Playback

Cue supports mpv, VLC, IINA, PotPlayer, and other system players. mpv is recommended because native playlists, resume tracking, and real-time scrobbling depend on its IPC support.

When playing a TV show with mpv, Cue sends the season as a native playlist. This enables:

- Gapless transitions between episodes.
- Starting at the selected episode or saved position.
- Updating progress throughout the playlist session.
- Marking preceding episodes as watched when you skip ahead.
- Marking an episode watched after it reaches the 90% threshold.

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
