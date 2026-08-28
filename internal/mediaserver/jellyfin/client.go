package jellyfin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/SuperCoolPencil/cue/internal/domain"
)

const (
	defaultTimeout = 60 * time.Second
	maxRetries     = 3
	baseRetryDelay = 500 * time.Millisecond
)

// Client implements the MediaSource interface for Jellyfin
type Client struct {
	baseURL    string
	token      string
	userID     string
	deviceID   string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewClient creates a new Jellyfin API client
func NewClient(baseURL, token, userID, deviceID string, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		token:    token,
		userID:   userID,
		deviceID: deviceID,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		logger: logger,
	}
}

// do performs an authenticated HTTP request to the Jellyfin API. All error
// mapping lives here: 401 → domain.ErrAuthFailed, transport failures →
// domain.ErrServerOffline (wrapped with the cause), any 2xx → success.
// Idempotent requests (retry=true) are retried on network errors and 5xx
// responses with exponential backoff.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, jsonBody interface{}, retry bool) ([]byte, error) {
	reqURL := fmt.Sprintf("%s%s", c.baseURL, path)
	if query != nil {
		reqURL = fmt.Sprintf("%s?%s", reqURL, query.Encode())
	}

	var bodyBytes []byte
	if jsonBody != nil {
		var err error
		bodyBytes, err = json.Marshal(jsonBody)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
	}

	attempts := 1
	if retry {
		attempts = maxRetries + 1
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		// Check context before each attempt
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Wait before retry (exponential backoff)
		if attempt > 0 {
			delay := baseRetryDelay * time.Duration(1<<(attempt-1)) // 500ms, 1s, 2s
			c.logger.Debug("retrying request", "attempt", attempt, "delay", delay, "url", reqURL)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		var reqBody io.Reader
		if bodyBytes != nil {
			reqBody = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, reqURL, reqBody)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Emby-Authorization", buildAuthHeader(c.token, c.deviceID))
		if bodyBytes != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		c.logger.Debug("jellyfin request", "method", method, "path", path, "attempt", attempt)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%w: %v", domain.ErrServerOffline, err)
			c.logger.Warn("jellyfin request failed", "error", err, "method", method, "path", path, "attempt", attempt)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		switch {
		case resp.StatusCode == http.StatusUnauthorized:
			return nil, domain.ErrAuthFailed
		case resp.StatusCode >= 500:
			lastErr = fmt.Errorf("server error: %d - %s", resp.StatusCode, truncateForLog(body))
			c.logger.Warn("jellyfin server error",
				"status", resp.StatusCode,
				"attempt", attempt,
				"method", method,
				"path", path,
			)
			continue
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			return body, nil
		default:
			c.logger.Error("jellyfin request error", "status", resp.StatusCode, "path", path, "body", truncateForLog(body))
			return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
	}

	c.logger.Error("jellyfin request failed", "error", lastErr, "method", method, "path", path)
	return nil, lastErr
}

// doRequest performs an idempotent (retried) GET-style request
func (c *Client) doRequest(ctx context.Context, method, path string, query url.Values) ([]byte, error) {
	return c.do(ctx, method, path, query, nil, true)
}

// truncateForLog bounds response bodies before they reach the log file
// (a reverse proxy's 502 page can be arbitrarily large)
func truncateForLog(body []byte) string {
	const max = 512
	if len(body) > max {
		return string(body[:max]) + "...(truncated)"
	}
	return string(body)
}

// GetLibraries returns all available libraries (Views)
func (c *Client) GetLibraries(ctx context.Context) ([]domain.Library, error) {
	path := fmt.Sprintf("/Users/%s/Views", c.userID)
	body, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var resp ItemsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return MapLibraries(resp.Items), nil
}

// GetMovies returns paginated movies from a movie library
func (c *Client) GetMovies(ctx context.Context, libID string, offset, limit int) ([]*domain.MediaItem, int, error) {
	query := url.Values{}
	query.Set("ParentId", libID)
	query.Set("IncludeItemTypes", "Movie")
	query.Set("Recursive", "true")
	query.Set("Fields", "Overview,DateCreated,MediaSources,MediaStreams")
	query.Set("StartIndex", strconv.Itoa(offset))
	if limit > 0 {
		query.Set("Limit", strconv.Itoa(limit))
	}
	query.Set("SortBy", "SortName")
	query.Set("SortOrder", "Ascending")

	path := fmt.Sprintf("/Users/%s/Items", c.userID)
	body, err := c.doRequest(ctx, http.MethodGet, path, query)
	if err != nil {
		return nil, 0, err
	}

	var resp ItemsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, fmt.Errorf("failed to parse response: %w", err)
	}

	movies := MapMovies(resp.Items, c.baseURL)
	// Set library ID for all movies
	for _, m := range movies {
		m.LibraryID = libID
	}

	return movies, resp.TotalRecordCount, nil
}

// GetShows returns paginated TV shows from a show library
func (c *Client) GetShows(ctx context.Context, libID string, offset, limit int) ([]*domain.Show, int, error) {
	query := url.Values{}
	query.Set("ParentId", libID)
	query.Set("IncludeItemTypes", "Series")
	query.Set("Recursive", "true")
	query.Set("Fields", "Overview,ChildCount,RecursiveItemCount,DateCreated,DateLastMediaAdded,MediaSources,MediaStreams")
	query.Set("StartIndex", strconv.Itoa(offset))
	if limit > 0 {
		query.Set("Limit", strconv.Itoa(limit))
	}
	query.Set("SortBy", "SortName")
	query.Set("SortOrder", "Ascending")

	path := fmt.Sprintf("/Users/%s/Items", c.userID)
	body, err := c.doRequest(ctx, http.MethodGet, path, query)
	if err != nil {
		return nil, 0, err
	}

	var resp ItemsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, fmt.Errorf("failed to parse response: %w", err)
	}

	shows := MapShows(resp.Items, c.baseURL)
	// Set library ID for all shows
	for _, s := range shows {
		s.LibraryID = libID
	}

	return shows, resp.TotalRecordCount, nil
}

// GetMixedContent returns paginated content (movies AND shows) from a mixed library.
// This fetches both types in a single API call with server-side sorting.
func (c *Client) GetMixedContent(ctx context.Context, libID string, offset, limit int) ([]domain.ListItem, int, error) {
	query := url.Values{}
	query.Set("ParentId", libID)
	query.Set("IncludeItemTypes", "Movie,Series")
	query.Set("Recursive", "true")
	query.Set("Fields", "Overview,ChildCount,RecursiveItemCount,DateCreated,DateLastMediaAdded,MediaSources,MediaStreams")
	query.Set("StartIndex", strconv.Itoa(offset))
	if limit > 0 {
		query.Set("Limit", strconv.Itoa(limit))
	}
	query.Set("SortBy", "SortName")
	query.Set("SortOrder", "Ascending")

	path := fmt.Sprintf("/Users/%s/Items", c.userID)
	body, err := c.doRequest(ctx, http.MethodGet, path, query)
	if err != nil {
		return nil, 0, err
	}

	var resp ItemsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, fmt.Errorf("failed to parse response: %w", err)
	}

	items := MapLibraryContent(resp.Items, c.baseURL)
	// Set library ID for all items
	for _, item := range items {
		switch v := item.(type) {
		case *domain.MediaItem:
			v.LibraryID = libID
		case *domain.Show:
			v.LibraryID = libID
		}
	}

	return items, resp.TotalRecordCount, nil
}

// GetLibraryItemCount returns the total item count for a library without
// fetching the items. Limit=1 keeps the response tiny while still populating
// TotalRecordCount.
func (c *Client) GetLibraryItemCount(ctx context.Context, libID, libType string) (int, error) {
	query := url.Values{}
	query.Set("ParentId", libID)
	switch libType {
	case "movie":
		query.Set("IncludeItemTypes", "Movie")
	case "show":
		query.Set("IncludeItemTypes", "Series")
	default: // mixed
		query.Set("IncludeItemTypes", "Movie,Series")
	}
	query.Set("Recursive", "true")
	query.Set("Limit", "1")
	query.Set("EnableTotalRecordCount", "true")

	path := fmt.Sprintf("/Users/%s/Items", c.userID)
	body, err := c.doRequest(ctx, http.MethodGet, path, query)
	if err != nil {
		return 0, err
	}

	var resp ItemsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("failed to parse response: %w", err)
	}

	return resp.TotalRecordCount, nil
}

// GetSeasons returns all seasons for a TV show
func (c *Client) GetSeasons(ctx context.Context, showID string) ([]*domain.Season, error) {
	query := url.Values{}
	query.Set("Fields", "ChildCount,RecursiveItemCount")

	path := fmt.Sprintf("/Shows/%s/Seasons", showID)
	body, err := c.doRequest(ctx, http.MethodGet, path, query)
	if err != nil {
		return nil, err
	}

	var resp ItemsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return MapSeasons(resp.Items, c.baseURL), nil
}

// GetEpisodes returns all episodes for a season.
// Queried by ParentId directly — no need for the extra round-trip that
// looked up the season's series ID first.
func (c *Client) GetEpisodes(ctx context.Context, seasonID string) ([]*domain.MediaItem, error) {
	query := url.Values{}
	query.Set("ParentId", seasonID)
	query.Set("Fields", "Overview,MediaSources,MediaStreams,DateCreated")
	query.Set("SortBy", "IndexNumber")
	query.Set("SortOrder", "Ascending")

	path := fmt.Sprintf("/Users/%s/Items", c.userID)
	body, err := c.doRequest(ctx, http.MethodGet, path, query)
	if err != nil {
		return nil, err
	}

	var resp ItemsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return MapEpisodes(resp.Items, c.baseURL), nil
}

// Search performs a search across all libraries
func (c *Client) Search(ctx context.Context, query string) ([]*domain.MediaItem, error) {
	params := url.Values{}
	params.Set("searchTerm", query)
	params.Set("IncludeItemTypes", "Movie,Episode,Series")
	params.Set("Limit", "50")

	path := "/Search/Hints"
	body, err := c.doRequest(ctx, http.MethodGet, path, params)
	if err != nil {
		return nil, err
	}

	var resp SearchHintsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return MapSearchResults(resp.SearchHints, c.baseURL), nil
}

// ResolvePlayable returns a direct playback URL plus any external subtitle tracks
// for an item.
func (c *Client) ResolvePlayable(ctx context.Context, itemID string) (domain.PlayableMedia, error) {
	// Get playback info to get the stream URL
	query := url.Values{}
	query.Set("UserId", c.userID)
	query.Set("MaxStreamingBitrate", "140000000") // High bitrate for direct play

	path := fmt.Sprintf("/Items/%s/PlaybackInfo", itemID)
	body, err := c.doRequest(ctx, http.MethodGet, path, query)
	if err != nil {
		return domain.PlayableMedia{}, err
	}

	var resp PlaybackInfoResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return domain.PlayableMedia{}, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(resp.MediaSources) == 0 {
		return domain.PlayableMedia{}, domain.ErrItemNotFound
	}

	source := resp.MediaSources[0]

	// Build direct stream URL
	// Format: /Videos/{itemId}/stream.{container}?Static=true&api_key={token}
	streamURL := fmt.Sprintf("%s/Videos/%s/stream.%s?Static=true&api_key=%s",
		c.baseURL, itemID, source.Container, c.token)

	subs := c.collectExternalSubtitles(itemID, source)
	if len(subs) > 0 {
		c.logger.Debug("resolved external subtitles", "itemID", itemID, "count", len(subs))
	}

	return domain.PlayableMedia{URL: streamURL, Subtitles: subs}, nil
}

// collectExternalSubtitles builds a list of side-loadable subtitle tracks
// from the media source's stream list. Embedded tracks are skipped because
// the player picks them up from the container directly.
func (c *Client) collectExternalSubtitles(itemID string, source MediaSource) []domain.Subtitle {
	if len(source.MediaStreams) == 0 {
		return nil
	}

	subs := make([]domain.Subtitle, 0)
	for _, stream := range source.MediaStreams {
		if stream.Type != "Subtitle" {
			continue
		}
		// Embedded subtitles travel inside the container and don't need a sub-file.
		if !stream.IsExternal && !strings.EqualFold(stream.DeliveryMethod, "External") {
			continue
		}

		codec := strings.ToLower(stream.Codec)
		ext := codec
		if ext == "" {
			ext = "srt"
		}

		// Prefer the server-provided DeliveryUrl when present; otherwise build the
		// canonical /Videos/{item}/{src}/Subtitles/{idx}/Stream.{codec} URL.
		var subURL string
		switch {
		case stream.DeliveryURL != "":
			subURL = c.absolutize(stream.DeliveryURL)
			subURL = appendAPIKey(subURL, c.token)
		default:
			subURL = fmt.Sprintf("%s/Videos/%s/%s/Subtitles/%d/Stream.%s?api_key=%s",
				c.baseURL, itemID, source.ID, stream.Index, ext, c.token)
		}

		subs = append(subs, domain.Subtitle{
			URL:      subURL,
			Language: stream.Language,
			Title:    firstNonEmpty(stream.DisplayTitle, stream.Title),
			Codec:    codec,
			Default:  stream.IsDefault,
			Forced:   stream.IsForced,
		})
	}
	return subs
}

func (c *Client) absolutize(u string) string {
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	if !strings.HasPrefix(u, "/") {
		u = "/" + u
	}
	return c.baseURL + u
}

func appendAPIKey(u, token string) string {
	if token == "" {
		return u
	}
	sep := "?"
	if strings.Contains(u, "?") {
		sep = "&"
	}
	return u + sep + "api_key=" + token
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// GetMediaItem returns detailed metadata for a specific item
func (c *Client) GetMediaItem(ctx context.Context, itemID string) (*domain.MediaItem, error) {
	query := url.Values{}
	query.Set("Fields", "Overview,MediaSources,MediaStreams,DateCreated")

	path := fmt.Sprintf("/Users/%s/Items/%s", c.userID, itemID)
	body, err := c.doRequest(ctx, http.MethodGet, path, query)
	if err != nil {
		return nil, err
	}

	var item Item
	if err := json.Unmarshal(body, &item); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var result domain.MediaItem
	switch item.Type {
	case "Movie":
		result = mapMovie(item, c.baseURL)
	case "Episode":
		result = mapEpisode(item, c.baseURL)
	default:
		return nil, domain.ErrItemNotFound
	}

	return &result, nil
}

// DeleteMediaItem deletes a media item from the server's disk
func (c *Client) DeleteMediaItem(ctx context.Context, itemID string) error {
	path := fmt.Sprintf("/Items/%s", itemID)
	_, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		if strings.Contains(err.Error(), "status code: 400") || strings.Contains(err.Error(), "status code: 401") || strings.Contains(err.Error(), "status code: 403") {
			return fmt.Errorf("not allowed (ensure user has permission to delete media)")
		}
		return err
	}
	return nil
}

// GetNextUp returns the next unwatched episode for a show
func (c *Client) GetNextUp(ctx context.Context, showID string) (*domain.MediaItem, error) {
	query := url.Values{}
	query.Set("SeriesId", showID)
	query.Set("UserId", c.userID)
	query.Set("Fields", "Overview,MediaSources,MediaStreams,DateCreated")
	query.Set("Limit", "1")

	path := "/Shows/NextUp"
	body, err := c.doRequest(ctx, http.MethodGet, path, query)
	if err != nil {
		return nil, err
	}

	var resp ItemsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(resp.Items) == 0 {
		return nil, domain.ErrItemNotFound
	}

	episode := mapEpisode(resp.Items[0], c.baseURL)
	return &episode, nil
}

// MarkPlayed marks an item as fully watched
func (c *Client) MarkPlayed(ctx context.Context, itemID string) error {
	path := fmt.Sprintf("/Users/%s/PlayedItems/%s", c.userID, itemID)
	if _, err := c.do(ctx, http.MethodPost, path, nil, nil, false); err != nil {
		return fmt.Errorf("failed to mark as played: %w", err)
	}
	return nil
}

// MarkUnplayed marks an item as unwatched
func (c *Client) MarkUnplayed(ctx context.Context, itemID string) error {
	path := fmt.Sprintf("/Users/%s/PlayedItems/%s", c.userID, itemID)
	if _, err := c.do(ctx, http.MethodDelete, path, nil, nil, false); err != nil {
		return fmt.Errorf("failed to mark as unplayed: %w", err)
	}
	return nil
}

// UpdateProgress reports the current playback position to the server.
func (c *Client) UpdateProgress(ctx context.Context, itemID string, positionMs int64) error {
	payload := map[string]interface{}{
		"ItemId": itemID, "PositionTicks": positionMs * 10000,
	}
	if _, err := c.do(ctx, http.MethodPost, "/Sessions/Playing/Progress", nil, payload, false); err != nil {
		return fmt.Errorf("failed to report progress: %w", err)
	}
	return nil
}

// ReportTimeline is a no-op for Jellyfin: cue already reports progress through
// the session-aware /Sessions/Playing/Progress endpoint (see UpdateProgress),
// and Jellyfin tracks the live session itself without a separate timeline call.
func (c *Client) ReportTimeline(ctx context.Context, state, ratingKey string, timeMs, durationMs int64) error {
	return nil
}

// GetPlaylists returns all user playlists
func (c *Client) GetPlaylists(ctx context.Context) ([]*domain.Playlist, error) {
	query := url.Values{}
	query.Set("IncludeItemTypes", "Playlist")
	query.Set("Recursive", "true")
	query.Set("Fields", "ChildCount,DateCreated")

	path := fmt.Sprintf("/Users/%s/Items", c.userID)
	body, err := c.doRequest(ctx, http.MethodGet, path, query)
	if err != nil {
		return nil, err
	}

	var resp ItemsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return MapPlaylists(resp.Items, c.baseURL), nil
}

// GetPlaylistItems returns all items in a playlist
func (c *Client) GetPlaylistItems(ctx context.Context, playlistID string) ([]*domain.MediaItem, error) {
	query := url.Values{}
	query.Set("UserId", c.userID)
	query.Set("Fields", "Overview,MediaSources,DateCreated")

	path := fmt.Sprintf("/Playlists/%s/Items", playlistID)
	body, err := c.doRequest(ctx, http.MethodGet, path, query)
	if err != nil {
		return nil, err
	}

	var resp ItemsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Map items (could be movies or episodes)
	items := make([]*domain.MediaItem, 0, len(resp.Items))
	for _, item := range resp.Items {
		switch item.Type {
		case "Movie", "Video", "MusicVideo":
			movie := mapMovie(item, c.baseURL)
			items = append(items, &movie)
		case "Episode":
			episode := mapEpisode(item, c.baseURL)
			items = append(items, &episode)
		}
	}

	return items, nil
}

// CreatePlaylist creates a new playlist with the given title and optional initial items
func (c *Client) CreatePlaylist(ctx context.Context, title string, itemIDs []string) (*domain.Playlist, error) {
	reqBody := map[string]interface{}{
		"Name":   title,
		"UserId": c.userID,
	}
	if len(itemIDs) > 0 {
		reqBody["Ids"] = itemIDs
	}

	respBody, err := c.do(ctx, http.MethodPost, "/Playlists", nil, reqBody, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create playlist: %w", err)
	}

	// Parse the response to get the created playlist
	var createResp struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(respBody, &createResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Return a minimal playlist object - caller can refresh for full details
	return &domain.Playlist{
		ID:           createResp.ID,
		Title:        title,
		PlaylistType: "video",
		ItemCount:    len(itemIDs),
	}, nil
}

// AddToPlaylist adds items to an existing playlist
func (c *Client) AddToPlaylist(ctx context.Context, playlistID string, itemIDs []string) error {
	if len(itemIDs) == 0 {
		return nil
	}

	query := url.Values{}
	query.Set("Ids", strings.Join(itemIDs, ","))
	query.Set("UserId", c.userID)

	path := fmt.Sprintf("/Playlists/%s/Items", playlistID)
	if _, err := c.do(ctx, http.MethodPost, path, query, nil, false); err != nil {
		return fmt.Errorf("failed to add items to playlist: %w", err)
	}
	return nil
}

// RemoveFromPlaylist removes an item from a playlist.
// Jellyfin's EntryIds parameter takes the playlist-specific entry ID
// (PlaylistItemId), not the media item's ID — passing an item ID is silently
// ignored (204 with no change). Resolve the entry ID first.
func (c *Client) RemoveFromPlaylist(ctx context.Context, playlistID string, itemID string) error {
	entryID, err := c.resolvePlaylistEntryID(ctx, playlistID, itemID)
	if err != nil {
		return err
	}

	query := url.Values{}
	query.Set("EntryIds", entryID)

	path := fmt.Sprintf("/Playlists/%s/Items", playlistID)
	if _, err := c.do(ctx, http.MethodDelete, path, query, nil, false); err != nil {
		return fmt.Errorf("failed to remove item from playlist: %w", err)
	}
	return nil
}

// resolvePlaylistEntryID fetches the playlist's items and returns the
// playlist entry ID (PlaylistItemId) for the given media item ID.
func (c *Client) resolvePlaylistEntryID(ctx context.Context, playlistID, itemID string) (string, error) {
	query := url.Values{}
	query.Set("UserId", c.userID)

	path := fmt.Sprintf("/Playlists/%s/Items", playlistID)
	body, err := c.doRequest(ctx, http.MethodGet, path, query)
	if err != nil {
		return "", err
	}

	var resp ItemsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	for _, item := range resp.Items {
		if item.ID == itemID {
			if item.PlaylistItemID != "" {
				return item.PlaylistItemID, nil
			}
			// Very old servers predate PlaylistItemId; fall back to the
			// item ID, which those versions accepted
			return itemID, nil
		}
	}

	return "", fmt.Errorf("item %s not found in playlist %s", itemID, playlistID)
}

// DeletePlaylist deletes a playlist
func (c *Client) DeletePlaylist(ctx context.Context, playlistID string) error {
	path := fmt.Sprintf("/Items/%s", playlistID)
	if _, err := c.do(ctx, http.MethodDelete, path, nil, nil, false); err != nil {
		return fmt.Errorf("failed to delete playlist: %w", err)
	}
	return nil
}

// GetContinueWatching returns items that are currently in progress
func (c *Client) GetContinueWatching(ctx context.Context) ([]*domain.MediaItem, error) {
	query := url.Values{}
	query.Set("Fields", "Overview,MediaSources,MediaStreams,DateCreated")
	query.Set("Limit", "20")
	query.Set("Recursive", "true")

	path := fmt.Sprintf("/Users/%s/Items/Resume", c.userID)
	body, err := c.doRequest(ctx, http.MethodGet, path, query)
	if err != nil {
		return nil, err
	}

	var resp ItemsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// items could be movies or episodes
	items := make([]*domain.MediaItem, 0, len(resp.Items))
	for _, item := range resp.Items {
		switch item.Type {
		case "Movie":
			movie := mapMovie(item, c.baseURL)
			items = append(items, &movie)
		case "Episode":
			episode := mapEpisode(item, c.baseURL)
			items = append(items, &episode)
		}
	}
	return items, nil
}
