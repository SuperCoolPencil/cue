package tui

import (
	"github.com/SuperCoolPencil/cue/internal/domain"
	"github.com/SuperCoolPencil/cue/internal/player"
	"github.com/SuperCoolPencil/cue/internal/search"
	tea "github.com/charmbracelet/bubbletea"
)

// Message types for the TUI

// ErrMsg represents an error
type ErrMsg struct {
	Err     error
	Context string
}

// Error implements the error interface
func (e ErrMsg) Error() string {
	if e.Context != "" {
		return e.Context + ": " + e.Err.Error()
	}
	return e.Err.Error()
}

// LibrariesLoadedMsg signals that libraries have been loaded
type LibrariesLoadedMsg struct {
	Libraries []domain.Library
	Refresh   bool // true for refresh-all: keep the navigation stack if possible
}

// MoviesLoadedMsg signals that movies have been loaded
type MoviesLoadedMsg struct {
	Movies    []*domain.MediaItem
	LibraryID string
}

// ShowsLoadedMsg signals that shows have been loaded
type ShowsLoadedMsg struct {
	Shows     []*domain.Show
	LibraryID string
}

// MixedLibraryLoadedMsg signals that mixed library content has been loaded
type MixedLibraryLoadedMsg struct {
	Items     []domain.ListItem
	LibraryID string
}

// SeasonsLoadedMsg signals that seasons have been loaded
type SeasonsLoadedMsg struct {
	Seasons []*domain.Season
	ShowID  string
}

// EpisodesLoadedMsg signals that episodes have been loaded
type EpisodesLoadedMsg struct {
	Episodes []*domain.MediaItem
	SeasonID string
}

// ContinueWatchingLoadedMsg signals that continue watching items have been loaded
type ContinueWatchingLoadedMsg struct {
	Items []*domain.MediaItem
}

// SeasonForPlaybackLoadedMsg signals that a full season has been loaded for playback
type SeasonForPlaybackLoadedMsg struct {
	Item     *domain.MediaItem
	Episodes []*domain.MediaItem
	Resume   bool
}

// PlaybackStartedMsg signals that playback has started (player launched)
type PlaybackStartedMsg struct {
	Item   domain.MediaItem
	Handle player.PlaybackHandle
}

// PlaybackStatusMsg signals a real-time status update during playback
type PlaybackStatusMsg struct {
	Message  string
	StatusCh <-chan string
}

// PlaybackFinishedMsg signals that playback has ended
type PlaybackFinishedMsg struct {
	Item       domain.MediaItem
	Title      string
	AutoMarked bool // true if auto-scrobbled as watched
	Err        error
}

// MarkWatchedMsg signals a request to mark an item as watched
type MarkWatchedMsg struct {
	ItemID    string
	Title     string
	LibraryID string
}

// MarkUnwatchedMsg signals a request to mark an item as unwatched
type MarkUnwatchedMsg struct {
	ItemID    string
	Title     string
	LibraryID string
}

// TickMsg is a general tick message for animations
type TickMsg struct{}

// ClearStatusMsg clears the status bar message
type ClearStatusMsg struct{}

// StatusMsg sets a temporary status message
type StatusMsg struct {
	Message string
	IsError bool
}

// LibrarySyncProgressMsg sent for each chunk during streaming sync
type LibrarySyncProgressMsg struct {
	LibraryID   string
	LibraryType string
	Generation  int // Sync generation; stale generations are dropped
	Loaded      int
	Total       int
	Done        bool
	FromCache   bool
	Error       error
	NextCmd     tea.Cmd // Continuation command for streaming
}

// ClearLibraryStatusMsg signals that the success indicator should be removed
type ClearLibraryStatusMsg struct {
	LibraryID string
}

// LogoutCompleteMsg signals that logout has been completed
type LogoutCompleteMsg struct {
	Error error
}

// PlaylistsLoadedMsg signals that playlists have been loaded
type PlaylistsLoadedMsg struct {
	Playlists []*domain.Playlist
}

// PlaylistItemsLoadedMsg signals that playlist items have been loaded
type PlaylistItemsLoadedMsg struct {
	Items      []*domain.MediaItem
	PlaylistID string
}

// PlaylistUpdatedMsg signals that a playlist was updated (item added/removed)
type PlaylistUpdatedMsg struct {
	PlaylistID string
	Error      error
}

// PlaylistCreatedMsg signals that a new playlist was created
type PlaylistCreatedMsg struct {
	Playlist *domain.Playlist
	Error    error
}

// PlaylistDeletedMsg signals that a playlist was deleted
type PlaylistDeletedMsg struct {
	PlaylistID string
	Error      error
}

type MediaItemDeletedMsg struct {
	ItemID    string
	LibraryID string
}

// PlaylistModalDataMsg contains data for the playlist modal
type PlaylistModalDataMsg struct {
	Playlists  []*domain.Playlist
	Membership map[string]bool
	Item       *domain.MediaItem
}

// RemoteSearchLoadedMsg carries server-side fallback search results.
type RemoteSearchLoadedMsg struct {
	Query   string
	Results []search.FilterResult
	Error   error
}

// QueueUpdatedMsg signals local queue changes.
type QueueUpdatedMsg struct {
	Message string
	Error   error
}

// RefreshCurrentMsg triggers a refresh of the current view
type RefreshCurrentMsg struct {
	LibraryID string
}
