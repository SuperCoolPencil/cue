package tui

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/SuperCoolPencil/cue/internal/config"
	"github.com/SuperCoolPencil/cue/internal/domain"
	"github.com/SuperCoolPencil/cue/internal/library"
	"github.com/SuperCoolPencil/cue/internal/mediaserver"
	"github.com/SuperCoolPencil/cue/internal/player"
	"github.com/SuperCoolPencil/cue/internal/playlist"
	"github.com/SuperCoolPencil/cue/internal/search"
	tea "github.com/charmbracelet/bubbletea"
)

// syncChannelSize is the buffer size for sync progress channels
const syncChannelSize = 100

// Command factories for async operations

// LoadLibrariesCmd loads all available libraries
func LoadLibrariesCmd(svc *library.Service) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		libraries, err := svc.FetchLibraries(ctx)
		if err != nil {
			slog.Error("failed to load libraries", "error", err)
			return ErrMsg{Err: err, Context: "loading libraries"}
		}
		return LibrariesLoadedMsg{Libraries: libraries}
	}
}

// LoadMoviesCmd loads movies from a library
func LoadMoviesCmd(svc *library.Service, libID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		movies, err := svc.FetchMovies(ctx, libID, nil)
		if err != nil {
			return ErrMsg{Err: err, Context: "loading movies"}
		}
		return MoviesLoadedMsg{Movies: movies, LibraryID: libID}
	}
}

// LoadShowsCmd loads TV shows from a library
func LoadShowsCmd(svc *library.Service, libID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		shows, err := svc.FetchShows(ctx, libID, nil)
		if err != nil {
			return ErrMsg{Err: err, Context: "loading shows"}
		}
		return ShowsLoadedMsg{Shows: shows, LibraryID: libID}
	}
}

// LoadMixedLibraryCmd loads content (movies AND shows) from a mixed library
func LoadMixedLibraryCmd(svc *library.Service, libID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		items, err := svc.FetchMixedContent(ctx, libID, nil)
		if err != nil {
			return ErrMsg{Err: err, Context: "loading library content"}
		}
		return MixedLibraryLoadedMsg{Items: items, LibraryID: libID}
	}
}

// LoadSeasonsCmd loads seasons for a show
func LoadSeasonsCmd(svc *library.Service, libID, showID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		seasons, err := svc.FetchSeasons(ctx, libID, showID)
		if err != nil {
			return ErrMsg{Err: err, Context: "loading seasons"}
		}
		return SeasonsLoadedMsg{Seasons: seasons, ShowID: showID}
	}
}

// LoadEpisodesCmd loads episodes for a season
func LoadEpisodesCmd(svc *library.Service, libID, showID, seasonID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		episodes, err := svc.FetchEpisodes(ctx, libID, showID, seasonID)
		if err != nil {
			return ErrMsg{Err: err, Context: "loading episodes"}
		}
		return EpisodesLoadedMsg{Episodes: episodes, SeasonID: seasonID}
	}
}

// LoadContinueWatchingCmd loads items currently in progress
func LoadContinueWatchingCmd(svc *library.Service) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		items, err := svc.FetchContinueWatching(ctx)
		if err != nil {
			return ErrMsg{Err: err, Context: "loading continue watching"}
		}
		return ContinueWatchingLoadedMsg{Items: items}
	}
}

// LoadSeasonForPlaybackCmd loads all episodes for an episode's season to build a full playlist
func LoadSeasonForPlaybackCmd(svc *library.Service, item *domain.MediaItem, resume bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// We need ShowID and ParentID (SeasonID)
		if item.ShowID == "" || item.ParentID == "" {
			return ErrMsg{Err: fmt.Errorf("missing show or season ID for binge-watching"), Context: "loading season"}
		}

		episodes, err := svc.FetchEpisodes(ctx, item.LibraryID, item.ShowID, item.ParentID)
		if err != nil {
			return ErrMsg{Err: err, Context: "loading season for playback"}
		}
		return SeasonForPlaybackLoadedMsg{Item: item, Episodes: episodes, Resume: resume}
	}
}

// PlayItemCmd starts playback of an item, optionally with a playlist
func PlayItemCmd(svc *player.Service, item domain.MediaItem, resume bool, autoplay bool, playlist ...domain.MediaItem) tea.Cmd {

	return func() tea.Msg {
		ctx := context.Background()

		// This command handles two collapsed concerns:
		// 1. Single-item playback (if autoplay is off) vs full-season playlist.
		// 2. Play from beginning (Play) vs resume from saved offset (Resume).
		// The logic below ensures that if autoplay is disabled, we always launch a
		// single-item playlist, regardless of whether it's a fresh play or a resume.
		actualPlaylist := playlist
		if !autoplay && len(playlist) > 0 {
			actualPlaylist = []domain.MediaItem{item}
		}

		var handle player.PlaybackHandle
		var err error
		if resume {
			handle, err = svc.Resume(ctx, item, actualPlaylist...)
		} else {
			handle, err = svc.Play(ctx, item, actualPlaylist...)
		}

		if err != nil {
			return ErrMsg{Err: err, Context: "starting playback"}
		}
		return PlaybackStartedMsg{Item: item, Handle: handle}
	}
}

// WaitForPlaybackCmd waits for the playback to finish and returns a message
func WaitForPlaybackCmd(resultCh <-chan player.ScrobbleResult) tea.Cmd {
	return func() tea.Msg {
		result, ok := <-resultCh
		if !ok {
			return nil
		}
		return PlaybackFinishedMsg{
			Item:       result.Item,
			Title:      result.Title,
			AutoMarked: result.AutoMarked,
			Err:        result.Err,
		}
	}
}

// ListenForPlaybackStatusCmd waits for status updates during playback
func ListenForPlaybackStatusCmd(statusCh <-chan string) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-statusCh
		if !ok {
			return nil
		}
		return PlaybackStatusMsg{
			Message:  msg,
			StatusCh: statusCh,
		}
	}
}

// MarkWatchedCmd marks an item as watched
func MarkWatchedCmd(svc *player.Service, libID, itemID, title string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := svc.MarkWatched(ctx, itemID); err != nil {
			return ErrMsg{Err: err, Context: "marking as watched"}
		}
		return MarkWatchedMsg{Title: title, LibraryID: libID}
	}
}

// MarkUnwatchedCmd marks an item as unwatched
func MarkUnwatchedCmd(svc *player.Service, libID, itemID, title string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := svc.MarkUnwatched(ctx, itemID); err != nil {
			return ErrMsg{Err: err, Context: "marking as unwatched"}
		}
		return MarkUnwatchedMsg{Title: title, LibraryID: libID}
	}
}

// TickCmd returns a command that sends a tick after a delay
func TickCmd(delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(t time.Time) tea.Msg {
		return TickMsg{}
	})
}

// ClearStatusCmd returns a command that clears status after a delay
func ClearStatusCmd(delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(t time.Time) tea.Msg {
		return ClearStatusMsg{}
	})
}

// ClearLibraryStatusCmd returns a command that clears library status after delay
func ClearLibraryStatusCmd(libID string, delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(t time.Time) tea.Msg {
		return ClearLibraryStatusMsg{LibraryID: libID}
	})
}

// SyncLibraryCmd performs smart sync with streaming progress updates
func SyncLibraryCmd(svc *library.Service, lib domain.Library) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)

		progressCh := make(chan syncProgress, syncChannelSize)

		go func() {
			defer cancel()
			defer close(progressCh)

			onProgress := func(loaded, total int) {
				select {
				case progressCh <- syncProgress{loaded: loaded, total: total}:
				default:
				}
			}

			result, err := svc.SyncLibrary(ctx, lib, onProgress)

			// Send final message
			select {
			case progressCh <- syncProgress{
				loaded:    result.Count,
				total:     result.Count,
				done:      true,
				fromCache: result.FromCache,
				err:       err,
			}:
			default:
			}
		}()

		return readSyncProgress(lib, progressCh)
	}
}

// syncProgress is an internal type for channel communication
type syncProgress struct {
	loaded    int
	total     int
	done      bool
	fromCache bool
	err       error
}

// readSyncProgress reads one message from the channel and creates a LibrarySyncProgressMsg
func readSyncProgress(lib domain.Library, progressCh <-chan syncProgress) tea.Msg {
	progress, ok := <-progressCh
	if !ok {
		return LibrarySyncProgressMsg{
			LibraryID:   lib.ID,
			LibraryType: lib.Type,
			Done:        true,
			Error:       fmt.Errorf("sync cancelled"),
		}
	}

	msg := LibrarySyncProgressMsg{
		LibraryID:   lib.ID,
		LibraryType: lib.Type,
		Loaded:      progress.loaded,
		Total:       progress.total,
		Done:        progress.done,
		FromCache:   progress.fromCache,
		Error:       progress.err,
	}

	if !progress.done && progress.err == nil {
		msg.NextCmd = listenToSyncCmd(lib, progressCh)
	}

	return msg
}

// listenToSyncCmd returns a command that reads the next message from the progress channel
func listenToSyncCmd(lib domain.Library, progressCh <-chan syncProgress) tea.Cmd {
	return func() tea.Msg {
		return readSyncProgress(lib, progressCh)
	}
}

// SyncAllLibrariesCmd syncs all libraries in parallel
func SyncAllLibrariesCmd(svc *library.Service, libraries []domain.Library) tea.Cmd {
	teaCmds := make([]tea.Cmd, len(libraries))
	for i, lib := range libraries {
		teaCmds[i] = SyncLibraryCmd(svc, lib)
	}
	return tea.Batch(teaCmds...)
}

// SyncPlaylistsCmd syncs playlists and their items (two levels deep, like library sync).
func SyncPlaylistsCmd(svc *playlist.Service, playlistsID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)

		progressCh := make(chan syncProgress, syncChannelSize)

		go func() {
			defer cancel()
			defer close(progressCh)

			// SyncPlaylists fetches playlists AND items for each
			playlists, err := svc.SyncPlaylists(ctx)

			// Send final message
			select {
			case progressCh <- syncProgress{
				loaded:    len(playlists),
				total:     len(playlists),
				done:      true,
				fromCache: false,
				err:       err,
			}:
			default:
			}
		}()

		return readPlaylistSyncProgress(playlistsID, progressCh)
	}
}

// readPlaylistSyncProgress reads sync progress for playlists
func readPlaylistSyncProgress(playlistsID string, progressCh <-chan syncProgress) tea.Msg {
	progress, ok := <-progressCh
	if !ok {
		return LibrarySyncProgressMsg{
			LibraryID:   playlistsID,
			LibraryType: "playlist",
			Done:        true,
			Error:       fmt.Errorf("sync cancelled"),
		}
	}

	return LibrarySyncProgressMsg{
		LibraryID:   playlistsID,
		LibraryType: "playlist",
		Loaded:      progress.loaded,
		Total:       progress.total,
		Done:        progress.done,
		FromCache:   progress.fromCache,
		Error:       progress.err,
	}
}

// LogoutCmd clears server config and cache, then signals completion
func LogoutCmd() tea.Cmd {
	return func() tea.Msg {
		if err := config.ClearServerConfig(); err != nil {
			return LogoutCompleteMsg{Error: err}
		}
		if err := config.ClearCache(); err != nil {
			return LogoutCompleteMsg{Error: err}
		}
		return LogoutCompleteMsg{Error: nil}
	}
}

// LoadPlaylistsCmd loads all playlists
func LoadPlaylistsCmd(svc *playlist.Service) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		playlists, err := svc.FetchPlaylists(ctx)
		if err != nil {
			return ErrMsg{Err: err, Context: "loading playlists"}
		}
		return PlaylistsLoadedMsg{Playlists: playlists}
	}
}

// LoadPlaylistItemsCmd loads items from a playlist
func LoadPlaylistItemsCmd(svc *playlist.Service, playlistID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		items, err := svc.FetchPlaylistItems(ctx, playlistID)
		if err != nil {
			return ErrMsg{Err: err, Context: "loading playlist items"}
		}
		return PlaylistItemsLoadedMsg{Items: items, PlaylistID: playlistID}
	}
}

// CreatePlaylistCmd creates a new playlist
func CreatePlaylistCmd(svc *playlist.Service, title string, itemIDs []string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		playlist, err := svc.CreatePlaylist(ctx, title, itemIDs)
		if err != nil {
			return PlaylistCreatedMsg{Error: err}
		}
		return PlaylistCreatedMsg{Playlist: playlist}
	}
}

// AddToPlaylistCmd adds items to a playlist
func AddToPlaylistCmd(svc *playlist.Service, playlistID string, itemIDs []string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := svc.AddToPlaylist(ctx, playlistID, itemIDs)
		if err != nil {
			return PlaylistUpdatedMsg{PlaylistID: playlistID, Error: err}
		}
		return PlaylistUpdatedMsg{PlaylistID: playlistID}
	}
}

// RemoveFromPlaylistCmd removes an item from a playlist
func RemoveFromPlaylistCmd(svc *playlist.Service, playlistID, itemID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := svc.RemoveFromPlaylist(ctx, playlistID, itemID)
		if err != nil {
			return PlaylistUpdatedMsg{PlaylistID: playlistID, Error: err}
		}
		return PlaylistUpdatedMsg{PlaylistID: playlistID}
	}
}

// DeletePlaylistCmd deletes a playlist
func DeletePlaylistCmd(svc *playlist.Service, playlistID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := svc.DeletePlaylist(ctx, playlistID)
		if err != nil {
			return PlaylistDeletedMsg{PlaylistID: playlistID, Error: err}
		}
		return PlaylistDeletedMsg{PlaylistID: playlistID}
	}
}

// DeleteMediaItemCmd deletes a media item from the server
func DeleteMediaItemCmd(svc mediaserver.MediaSource, itemID, libID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := svc.DeleteMediaItem(ctx, itemID)
		if err != nil {
			return ErrMsg{Err: err, Context: "deleting media item"}
		}
		return MediaItemDeletedMsg{ItemID: itemID, LibraryID: libID}
	}
}

// DirectPlayShowCmd finds the first unplayed episode after the latest played
// episode and starts it. Episodes left unplayed before the latest played one
// are intentionally skipped, so users can permanently skip episodes.
func DirectPlayShowCmd(svc mediaserver.MediaSource, playerSvc *player.Service, showID, libraryID string, autoplay bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		seasons, err := svc.GetSeasons(ctx, showID)
		if err != nil {
			return ErrMsg{Err: err, Context: "loading seasons for next episode"}
		}
		var episodes []*domain.MediaItem
		for _, season := range seasons {
			// Season zero contains specials, which should not interrupt regular
			// episode progression.
			if season == nil || season.SeasonNum == 0 {
				continue
			}
			seasonEpisodes, err := svc.GetEpisodes(ctx, season.ID)
			if err != nil {
				return ErrMsg{Err: err, Context: "loading episodes for next episode"}
			}
			for _, episode := range seasonEpisodes {
				if episode == nil {
					continue
				}
				if episode.LibraryID == "" {
					episode.LibraryID = libraryID
				}
			}
			episodes = append(episodes, seasonEpisodes...)
		}

		episode := domain.NextUnplayedEpisodeAfterLastPlayed(episodes)
		if episode == nil {
			return ErrMsg{Err: domain.ErrItemNotFound, Context: "finding next episode to play"}
		}

		// Return a command that plays the found episode
		return PlayItemCmd(playerSvc, *episode, episode.ShouldResume(), autoplay)()
	}
}

// LoadPlaylistModalDataCmd loads data for the playlist management modal
func LoadPlaylistModalDataCmd(svc *playlist.Service, item *domain.MediaItem) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		playlists, err := svc.FetchPlaylists(ctx)
		if err != nil {
			return ErrMsg{Err: err, Context: "loading playlists for modal"}
		}

		membership, err := svc.GetPlaylistMembership(ctx, item.ID)
		if err != nil {
			return ErrMsg{Err: err, Context: "checking playlist membership"}
		}

		return PlaylistModalDataMsg{
			Playlists:  playlists,
			Membership: membership,
			Item:       item,
		}
	}
}

func RemoteSearchCmd(svc *search.Service, query string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		results, err := svc.SearchRemote(ctx, query)
		return RemoteSearchLoadedMsg{Query: query, Results: results, Error: err}
	}
}

func AddToQueueCmd(svc *playlist.Service, item *domain.MediaItem) tea.Cmd {
	return func() tea.Msg {
		if err := svc.AddToQueue(item); err != nil {
			return QueueUpdatedMsg{Error: err}
		}
		if item == nil {
			return QueueUpdatedMsg{Message: "Queue unchanged"}
		}
		return QueueUpdatedMsg{Message: "Queued: " + item.Title}
	}
}

func RemoveFromQueueCmd(svc *playlist.Service, itemID string) tea.Cmd {
	return func() tea.Msg {
		if err := svc.RemoveFromQueue(itemID); err != nil {
			return QueueUpdatedMsg{Error: err}
		}
		return QueueUpdatedMsg{Message: "Removed from queue"}
	}
}
