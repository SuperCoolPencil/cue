package mediaserver

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/SuperCoolPencil/cue/internal/config"
	"github.com/SuperCoolPencil/cue/internal/domain"
	"github.com/SuperCoolPencil/cue/internal/mediaserver/jellyfin"
	"github.com/SuperCoolPencil/cue/internal/mediaserver/plex"
)

// MediaSource combines all client interfaces that a media server backend must implement.
// This is the unified interface for browsing, playback, search, and playlist operations.
type MediaSource interface {
	domain.LibraryClient  // Browsing: GetLibraries, GetMovies, GetShows, GetSeasons, GetEpisodes
	domain.PlaybackClient // Playback: ResolvePlayableURL, MarkPlayed/Unplayed
	domain.SearchClient   // Search: Search(query) across all libraries
	domain.PlaylistClient // Playlists: GetPlaylists, CreatePlaylist, AddToPlaylist, etc.

	// GetMediaItem fetches full metadata for a single item.
	// Kept on MediaSource (not in a domain interface) as it's only used
	// by specific features that need detailed item metadata.
	GetMediaItem(ctx context.Context, itemID string) (*domain.MediaItem, error)

	// DeleteMediaItem deletes the media item from the server's disk.
	DeleteMediaItem(ctx context.Context, itemID string) error

	// GetNextUp returns the next unwatched episode for a show.
	GetNextUp(ctx context.Context, showID string) (*domain.MediaItem, error)

	// GetImage fetches raw image bytes (poster/artwork) from an absolute URL,
	// authenticated against the media server. Used for ASCII/terminal image rendering.
	GetImage(ctx context.Context, url string) ([]byte, error)

	// GetWebURL returns the web interface URL for a given item.
	GetWebURL(ctx context.Context, itemID string) (string, error)
}

// NewClient creates a new MediaSource based on the server type.
// This factory function abstracts away the specific backend implementation.
func NewClient(cfg *config.Config, logger *slog.Logger) (MediaSource, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	if cfg.Server.URL == "" {
		return nil, fmt.Errorf("server URL is required")
	}

	if cfg.Server.Token == "" {
		return nil, fmt.Errorf("server token is required")
	}

	switch cfg.Server.Type {
	case config.SourceTypePlex:
		return plex.NewClient(cfg.Server.URL, cfg.Server.Token, cfg.Server.DeviceID, logger), nil

	case config.SourceTypeJellyfin:
		if cfg.Server.UserID == "" {
			return nil, fmt.Errorf("jellyfin requires user ID")
		}
		return jellyfin.NewClient(cfg.Server.URL, cfg.Server.Token, cfg.Server.UserID, cfg.Server.DeviceID, logger), nil

	default:
		return nil, fmt.Errorf("unknown server type: %s", cfg.Server.Type)
	}
}
