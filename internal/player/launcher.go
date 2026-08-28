package player

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/SuperCoolPencil/cue/internal/domain"
)

// Launcher launches media URLs in an external player
type Launcher struct {
	command  string   // configured player command, empty for system default
	args     []string // additional arguments for the player
	seekFlag string   // user-configured seek flag (e.g., "--start=%d"), overrides table lookup
	logger   *slog.Logger
}

// PlayerDef defines a player binary and its seek flag format
type PlayerDef struct {
	Binary   string
	SeekFlag string // Use %d for seconds placeholder, e.g., "--start=%d" or "-ss %d"
}

// Platform-specific player lists, ordered by priority (first match wins)
var linuxPlayers = []PlayerDef{
	{Binary: "mpv", SeekFlag: "--start=%d"},
	{Binary: "vlc", SeekFlag: "--start-time=%d"},
	{Binary: "celluloid", SeekFlag: "--mpv-start=%d"},
	{Binary: "haruna", SeekFlag: "--start=%d"},
	{Binary: "smplayer", SeekFlag: "-ss %d"},
	{Binary: "mplayer", SeekFlag: "-ss %d"},
}

var darwinPlayers = []PlayerDef{
	{Binary: "iina", SeekFlag: "--mpv-start=%d"},
	{Binary: "mpv", SeekFlag: "--start=%d"},
	{Binary: "vlc", SeekFlag: "--start-time=%d"},
}

// Windows detection looks up by base name; exec.LookPath consults PATHEXT
// so "mpv" resolves to mpv.exe (e.g. scoop's shim).
var windowsPlayers = []PlayerDef{
	{Binary: "mpv", SeekFlag: "--start=%d"},
	{Binary: "vlc", SeekFlag: "--start-time=%d"},
}

// Windows-side players reachable from WSL via interop. Probed after the
// native Linux list so a Linux install (e.g. via WSLg) still wins.
var wslPlayers = []PlayerDef{
	{Binary: "PotPlayerMini64.exe", SeekFlag: "/seek=%d"},
	{Binary: "PotPlayerMini.exe", SeekFlag: "/seek=%d"},
	{Binary: "mpv.exe", SeekFlag: "--start=%d"},
	{Binary: "vlc.exe", SeekFlag: "--start-time=%d"},
}

func lookPathOK(binary string) bool {
	_, err := exec.LookPath(binary)
	return err == nil
}

// isWSL reports whether we are running inside Windows Subsystem for Linux.
func isWSL() bool {
	if os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != "" {
		return true
	}
	data, err := os.ReadFile("/proc/sys/kernel/osrelease")
	return err == nil && strings.Contains(strings.ToLower(string(data)), "microsoft")
}

// NewLauncher creates a new Launcher
// seekFlag is optional - if empty, we look up the flag from our known players table
func NewLauncher(command string, args []string, seekFlag string, logger *slog.Logger) *Launcher {
	if logger == nil {
		logger = slog.Default()
	}

	return &Launcher{
		command:  command,
		args:     args,
		seekFlag: seekFlag,
		logger:   logger,
	}
}

// Launch opens one or more media URLs in the configured player or auto-detected player.
func (l *Launcher) Launch(offset time.Duration, playlistStart int, media ...domain.PlayableMedia) (*exec.Cmd, string, error) {
	offsetSecs := int(offset.Seconds())

	// Tier 1: User configured a specific player
	if l.command != "" {
		l.logger.Info("using configured player", "command", l.command)
		return l.launchConfigured(offsetSecs, playlistStart, media...)
	}

	// Tier 2: Auto-detect known players
	if player, found := l.detectPlayer(); found {
		l.logger.Info("auto-detected player", "binary", player.Binary)
		return l.execPlayer(player, offsetSecs, playlistStart, media...)
	}

	// Tier 3: System default fallback (xdg-open/open)
	l.logger.Warn("no video players found, falling back to system default")
	if offsetSecs > 0 {
		l.logger.Warn("resume not supported with system default player - starting from beginning")
	}
	if len(media) > 0 && len(media[0].Subtitles) > 0 {
		l.logger.Warn("external subtitles not supported with system default player - some tracks may be missing")
	}
	if len(media) == 0 {
		return nil, "", fmt.Errorf("no media provided")
	}
	cmd, err := l.launchDefault(media[0].URL)

	return cmd, "", err
}

// subFileArgs returns the player-specific args needed to side-load each external
// subtitle. Returns nil when the binary has no known sub-file flag.
func subFileArgs(binary string, subs []domain.Subtitle) []string {
	if len(subs) == 0 {
		return nil
	}
	bin := strings.ToLower(filepath.Base(binary))
	switch bin {
	case "mpv", "iina", "celluloid", "haruna":
		// mpv, IINA and other mpv-frontends accept multiple --sub-file flags.
		// IINA's CLI is `iina-cli`, but mpv-passthrough flags also work via
		// the `--mpv-` prefix used in seek flags; --sub-file works directly
		// for mpv/celluloid/haruna. IINA accepts `--mpv-sub-file=` too.
		prefix := "--sub-file="
		if bin == "iina" {
			prefix = "--mpv-sub-file="
		}
		args := make([]string, 0, len(subs))
		for _, s := range subs {
			if s.URL == "" {
				continue
			}
			args = append(args, prefix+s.URL)
		}
		return args
	case "vlc":
		// VLC supports only a single :sub-file. If the user has multiple,
		// pick the default (or first) so they at least get one.
		pick := subs[0]
		for _, s := range subs {
			if s.Default {
				pick = s
				break
			}
		}
		if pick.URL == "" {
			return nil
		}
		return []string{":sub-file=" + pick.URL}
	default:
		return nil
	}
}

// detectPlayer returns the first available player from the platform-specific list
func (l *Launcher) detectPlayer() (PlayerDef, bool) {
	var candidates []PlayerDef

	switch runtime.GOOS {
	case "darwin":
		candidates = darwinPlayers
	case "linux":
		candidates = linuxPlayers
		if isWSL() {
			candidates = append(append([]PlayerDef{}, linuxPlayers...), wslPlayers...)
		}
	case "windows":
		candidates = windowsPlayers
	default:
		return PlayerDef{}, false
	}

	for _, p := range candidates {
		if path, err := exec.LookPath(p.Binary); err == nil && path != "" {
			return p, true
		}
	}
	return PlayerDef{}, false
}

// execPlayer launches the detected player with optional seek offset and playlist start
func (l *Launcher) execPlayer(player PlayerDef, offsetSecs int, playlistStart int, media ...domain.PlayableMedia) (*exec.Cmd, string, error) {

	args := []string{}
	var ipcSocket string

	isMpv := player.Binary == "mpv"

	// Enable IPC for mpv
	if isMpv {
		ipcSocket = newMPVSocketPath()
		args = append(args, "--input-ipc-server="+ipcSocket)
		if playlistStart > 0 {
			args = append(args, fmt.Sprintf("--playlist-start=%d", playlistStart))
		}
	}

	// Add seek flag if we have an offset and the player supports it
	if offsetSecs > 0 && player.SeekFlag != "" && !isMpv {
		formattedFlag := fmt.Sprintf(player.SeekFlag, offsetSecs)
		// Split flags like "-ss 10" into separate args
		args = append(args, strings.Fields(formattedFlag)...)
	}

	if !isMpv && len(media) > playlistStart {
		if subArgs := subFileArgs(player.Binary, media[playlistStart].Subtitles); len(subArgs) > 0 {
			args = append(args, subArgs...)
		} else if len(media[playlistStart].Subtitles) > 0 {
			l.logger.Warn("external subtitles not supported by player - skipping",
				"binary", player.Binary, "count", len(media[playlistStart].Subtitles))
		}
	}

	for i, m := range media {
		if isMpv {
			args = append(args, "--{")
			if subArgs := subFileArgs(player.Binary, m.Subtitles); len(subArgs) > 0 {
				args = append(args, subArgs...)
			}
			if offsetSecs > 0 && i == playlistStart && player.SeekFlag != "" {
				formattedFlag := fmt.Sprintf(player.SeekFlag, offsetSecs)
				args = append(args, strings.Fields(formattedFlag)...)
			}
			args = append(args, m.URL)
			args = append(args, "--}")
		} else {
			args = append(args, m.URL)
		}
	}

	l.logger.Debug("executing player", "binary", player.Binary, "args", args)
	cmd := exec.Command(player.Binary, args...)
	if err := cmd.Start(); err != nil {
		return nil, "", err
	}
	return cmd, ipcSocket, nil
}

// launchConfigured launches the media using the user-configured player
func (l *Launcher) launchConfigured(offsetSecs int, playlistStart int, media ...domain.PlayableMedia) (*exec.Cmd, string, error) {

	args := append([]string{}, l.args...)

	bin := strings.ToLower(strings.TrimSuffix(filepath.Base(l.command), filepath.Ext(l.command)))
	isMpv := bin == "mpv"

	// Add seek offset: user-configured flag takes precedence, then table lookup
	if offsetSecs > 0 && !isMpv {
		seekFlag := l.seekFlag
		if seekFlag == "" {
			// Fall back to table lookup for known players
			seekFlag = l.lookupSeekFlag(l.command)
		}

		if seekFlag != "" {
			formattedFlag := fmt.Sprintf(seekFlag, offsetSecs)
			args = append(args, strings.Fields(formattedFlag)...)
		} else {
			l.logger.Warn("cannot set start offset - unknown player, configure start_flag in config",
				"command", l.command, "offset", offsetSecs)
		}
	}

	if !isMpv && len(media) > playlistStart {
		if subArgs := subFileArgs(l.command, media[playlistStart].Subtitles); len(subArgs) > 0 {
			args = append(args, subArgs...)
		} else if len(media[playlistStart].Subtitles) > 0 {
			l.logger.Warn("external subtitles not supported by configured player - skipping",
				"command", l.command, "count", len(media[playlistStart].Subtitles))
		}
	}

	// For mpv, we can also pass the playlist start index
	if isMpv && playlistStart > 0 {
		args = append(args, fmt.Sprintf("--playlist-start=%d", playlistStart))
	}

	for i, m := range media {
		if isMpv {
			args = append(args, "--{")
			if subArgs := subFileArgs(l.command, m.Subtitles); len(subArgs) > 0 {
				args = append(args, subArgs...)
			}
			if offsetSecs > 0 && i == playlistStart {
				seekFlag := l.seekFlag
				if seekFlag == "" {
					seekFlag = l.lookupSeekFlag(l.command)
				}
				if seekFlag != "" {
					formattedFlag := fmt.Sprintf(seekFlag, offsetSecs)
					args = append(args, strings.Fields(formattedFlag)...)
				} else if l.command != "mpv" { // If they specified a custom seek flag or it's standard mpv
					// This warning only fires if they have an unknown binary called mpv
					l.logger.Warn("cannot set start offset - configure start_flag in config",
						"command", l.command, "offset", offsetSecs)
				}
			}
			args = append(args, m.URL)
			args = append(args, "--}")
		} else {
			args = append(args, m.URL)
		}
	}

	l.logger.Debug("launching configured player", "command", l.command, "args", args)

	// On macOS, try 'open -a' if command not in PATH (for GUI apps)
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath(l.command); err != nil {
			cmd, err := l.launchMacOSApp(l.command, args)
			return cmd, "", err
		}
	}

	// For manual config, we check if it's mpv to enable IPC. Match by base
	// name (without extension) so Windows variants like "mpv.exe" and paths
	// with backslashes (`C:\tools\mpv.exe`) are recognised too.
	var ipcSocket string
	if isMpv {
		ipcSocket = newMPVSocketPath()
		args = append([]string{"--input-ipc-server=" + ipcSocket}, args...)
	}

	cmd := exec.Command(l.command, args...)
	if err := cmd.Start(); err != nil {
		return nil, "", err
	}
	return cmd, ipcSocket, nil
}

// lookupSeekFlag finds the seek flag for a known player binary
func (l *Launcher) lookupSeekFlag(binary string) string {
	for _, table := range [][]PlayerDef{linuxPlayers, darwinPlayers, wslPlayers} {
		for _, p := range table {
			if p.Binary == binary {
				return p.SeekFlag
			}
		}
	}
	for _, p := range windowsPlayers {
		if p.Binary == binary {
			return p.SeekFlag
		}
	}
	return ""
}

// launchMacOSApp launches a macOS GUI app using 'open -a'
func (l *Launcher) launchMacOSApp(appName string, playerArgs []string) (*exec.Cmd, error) {
	cmdArgs := []string{"-a", appName}
	if len(playerArgs) > 0 {
		cmdArgs = append(cmdArgs, "--args")
		cmdArgs = append(cmdArgs, playerArgs...)
	}

	l.logger.Debug("using macOS 'open -a'", "app", appName, "args", cmdArgs)
	cmd := exec.Command("open", cmdArgs...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

// launchDefault opens the URL using the system default handler
func (l *Launcher) launchDefault(url string) (*exec.Cmd, error) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		// `start` is a cmd.exe builtin, not a standalone exe. The empty "" is
		// a window title — required because `start` treats the first quoted
		// arg as a title and would otherwise swallow the URL.
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		// Linux and other Unix-like systems
		if isWSL() {
			// WSL distros usually have no xdg-open; hand the URL to Windows.
			// wslview (from wslu) is purpose-built for this. rundll32's
			// FileProtocolHandler is the reliable built-in: explorer.exe
			// chokes on URLs with query strings (?a=1&b=2) and silently
			// opens the Documents folder instead.
			switch {
			case lookPathOK("wslview"):
				cmd = exec.Command("wslview", url)
			case lookPathOK("rundll32.exe"):
				cmd = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url)
			case lookPathOK("explorer.exe"):
				cmd = exec.Command("explorer.exe", url)
			}
		}
		if cmd == nil {
			if _, err := exec.LookPath("xdg-open"); err != nil {
				return nil, fmt.Errorf("no media player found — install mpv (or vlc), or set player.command in config.yaml")
			}
			cmd = exec.Command("xdg-open", url)
		}
	}

	l.logger.Debug("launching with system default", "os", runtime.GOOS, "command", cmd.Path)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

// Service orchestrates playback operations
type Service struct {
	launcher  *Launcher
	playback  domain.PlaybackClient
	scrobbler *Scrobbler
	logger    *slog.Logger
}

// NewService creates a new playback service
func NewService(launcher *Launcher, playback domain.PlaybackClient, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		launcher:  launcher,
		playback:  playback,
		scrobbler: NewScrobbler(playback, logger),
		logger:    logger,
	}
}

// Play starts playback of a media item from the beginning
func (s *Service) Play(ctx context.Context, item domain.MediaItem, playlist ...domain.MediaItem) (PlaybackHandle, error) {
	return s.playItem(ctx, 0, item, playlist...)
}

// Resume starts playback from the saved position
func (s *Service) Resume(ctx context.Context, item domain.MediaItem, playlist ...domain.MediaItem) (PlaybackHandle, error) {
	return s.playItem(ctx, item.ViewOffset, item, playlist...)
}

// playItem resolves URLs and launches player
func (s *Service) playItem(ctx context.Context, offset time.Duration, item domain.MediaItem, playlist ...domain.MediaItem) (PlaybackHandle, error) {
	var allPlaybackItems []domain.MediaItem
	if len(playlist) > 0 {
		allPlaybackItems = playlist
	} else {
		allPlaybackItems = []domain.MediaItem{item}
	}

	playableMedias := make([]domain.PlayableMedia, 0, len(allPlaybackItems))
	filteredPlaybackItems := make([]domain.MediaItem, 0, len(allPlaybackItems))

	for _, pItem := range allPlaybackItems {
		media, err := s.playback.ResolvePlayable(ctx, pItem.ID)
		if err != nil {
			s.logger.Error("failed to resolve playable URL", "error", err, "itemID", pItem.ID)
			// If resolving the first item fails, we abort the whole launch.
			if pItem.ID == item.ID {
				return PlaybackHandle{}, err
			}
			// Skip this item but keep going with the rest of the playlist.
			// We MUST skip the corresponding entry in filteredPlaybackItems too.
			continue
		}
		playableMedias = append(playableMedias, media)
		filteredPlaybackItems = append(filteredPlaybackItems, pItem)
	}

	// Adjust playlistStart if items were skipped before the starting item.
	// Actually, if 'item' is resolved successfully, playlistStart should be
	// recalculated based on its new position in filteredPlaybackItems.
	actualStartIdx := 0
	for i, pItem := range filteredPlaybackItems {
		if pItem.ID == item.ID {
			actualStartIdx = i
			break
		}
	}

	s.logger.Info("launching playback",
		"title", item.Title, "itemID", item.ID, "offset", offset, "playlistSize", len(playableMedias), "startIdx", actualStartIdx)

	cmd, ipcSocket, err := s.launcher.Launch(offset, actualStartIdx, playableMedias...)

	if err != nil {
		return PlaybackHandle{}, err
	}

	// Start monitoring progress for all resolved items
	return s.scrobbler.Monitor(ctx, cmd, ipcSocket, actualStartIdx, filteredPlaybackItems...), nil
}

// MarkWatched marks an item as fully watched
func (s *Service) MarkWatched(ctx context.Context, itemID string) error {
	return s.playback.MarkPlayed(ctx, itemID)
}

// MarkUnwatched marks an item as unwatched
func (s *Service) MarkUnwatched(ctx context.Context, itemID string) error {
	return s.playback.MarkUnplayed(ctx, itemID)
}
