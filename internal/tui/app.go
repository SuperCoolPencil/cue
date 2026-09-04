package tui

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/SuperCoolPencil/cue/internal/config"
	"github.com/SuperCoolPencil/cue/internal/domain"
	"github.com/SuperCoolPencil/cue/internal/library"
	"github.com/SuperCoolPencil/cue/internal/mediaserver"
	"github.com/SuperCoolPencil/cue/internal/player"
	"github.com/SuperCoolPencil/cue/internal/playlist"
	"github.com/SuperCoolPencil/cue/internal/search"
	"github.com/SuperCoolPencil/cue/internal/tui/components"
	tea "github.com/charmbracelet/bubbletea"
)

// authFailedStatusMsg tells the user how to recover from a revoked/expired
// token. Shown persistently (not auto-cleared) since action is required.
const authFailedStatusMsg = "Session expired or revoked — press L to log out, then run cue to sign in again"

func playbackStatusText(item domain.MediaItem, position time.Duration) string {
	if position < 0 {
		position = 0
	}
	title := item.Title
	if item.ShowTitle != "" {
		title += " - " + item.ShowTitle
	}
	totalMinutes := int64(position / time.Minute)
	return fmt.Sprintf("%s (%02d:%02d)", title, totalMinutes/60, totalMinutes%60)
}

// ApplicationState represents the current state of the application
type ApplicationState int

const (
	StateBrowsing ApplicationState = iota
	StateHelp
	StateConfirmLogout
	StateConfirmResume
	StateInspecting
	StateConfirmDelete
)

// Layout proportions for Miller Columns
const (
	// 3-Column Smart Ratios (Inspector visible)
	ParentColumnPercent3   = 25 // Parent context
	ActiveColumnPercent3   = 35 // Active/focused
	InspectorColumnPercent = 30 // Inspector (summary)

	// 3-Column Focus Mode (Inspector hidden) - show more navigation context
	GrandparentColumnPercent = 25 // Grandparent context
	ParentColumnPercent2     = 30 // Parent context
	ActiveColumnPercent2     = 45 // Active/focused

	// Root level (single column + inspector)
	RootColumnPercent    = 40
	RootInspectorPercent = 60

	MinColumnWidth = 15

	// Vertical layout: single footer line
	ChromeHeight = 1

	// Synthetic library entry for playlists
	playlistsLibraryID = "__playlists__"
	continueLibraryID  = "__continue_watching__"
	recentLibraryID    = "__recently_added__"
	queueLibraryID     = "__queue__"
	filtersLibraryID   = "__smart_filters__"
	profilesLibraryID  = "__profiles__"
	configLibraryID    = "__config__"
	cacheLibraryID     = "__cache__"
)

// playlistsLibraryEntry returns the synthetic library entry for playlists
func playlistsLibraryEntry() domain.Library {
	return domain.Library{
		ID:   playlistsLibraryID,
		Name: "Playlists",
		Type: "playlist",
	}
}

func virtualLibraryEntries() []domain.Library {
	return []domain.Library{
		{ID: continueLibraryID, Name: "Continue Watching", Type: "cue"},
		{ID: recentLibraryID, Name: "Recently Added", Type: "cue"},
		{ID: queueLibraryID, Name: "Watch Queue", Type: "cue"},
		{ID: filtersLibraryID, Name: "Smart Filters", Type: "cue"},
		{ID: profilesLibraryID, Name: "Profiles", Type: "cue"},
		{ID: configLibraryID, Name: "Config", Type: "cue"},
		{ID: cacheLibraryID, Name: "Cache", Type: "cue"},
	}
}

// allLibraryEntries returns libraries plus the synthetic Playlists entry
func (m *Model) allLibraryEntries() []domain.Library {
	entries := append([]domain.Library{}, virtualLibraryEntries()...)
	entries = append(entries, m.Libraries...)
	return append(entries, playlistsLibraryEntry())
}

// Model is the main Bubble Tea model for the application
type Model struct {
	// Application state
	State ApplicationState
	Ready bool

	// Cache reads (View-safe)
	Store domain.Store

	// Network coordination (concrete types, not interfaces)
	LibraryService  *library.Service
	PlaylistService *playlist.Service

	// Other services
	SearchSvc   *search.Service
	PlaybackSvc *player.Service

	MediaClient mediaserver.MediaSource

	// UI Components - Miller Columns
	ColumnStack   *ColumnStack             // Stack of navigable list columns
	Inspector     components.Inspector     // View projection (always shows details for middle column selection)
	GlobalSearch  components.GlobalSearch  // Search modal
	SortModal     components.SortModal     // Sort field selector
	PlaylistModal components.PlaylistModal // Playlist management modal
	InputModal    components.InputModal    // Simple text input modal

	// Data
	Libraries []domain.Library

	// Dimensions
	Width  int
	Height int

	// UI state
	StatusMsg      string
	StatusIsErr    bool
	Loading        bool
	SpinnerFrame   int
	ShowInspector  bool   // Toggle inspector visibility (default true)
	isPlayingTitle string // Non-empty while a player process is running

	// Sync state
	LibraryStates map[string]components.LibrarySyncState // Tracks progress per library
	SyncingCount  int                                    // Libraries still syncing
	MultiLibSync  bool                                   // True when syncing multiple libraries (R / startup)
	SyncGen       int                                    // Current sync generation; messages from older generations are dropped

	// Navigation plan for deep linking
	navPlan *NavPlan

	// Playlist navigation context (when viewing playlist items)
	currentPlaylistID string

	// Navigation context for hierarchical cache keys (cascade invalidation)
	currentLibID  string // Set when entering a library
	currentShowID string // Set when entering a show

	// UI preferences from config
	UIConfig  config.UIConfig
	AppConfig *config.Config
	Version   string

	pendingPlayback    *domain.MediaItem
	pendingPlaylist    []domain.MediaItem
	PendingSelectionID string // ID of item to select after load completes
	pendingDelete      domain.ListItem

	// posterItemID tracks the item a poster fetch was last requested for,
	// so stale PosterLoadedMsg results are ignored.
	posterItemID string
	// posterRequestKey identifies the item, URL, and rendered dimensions
	// currently being requested.
	posterRequestKey string
	posterRequestID  uint64
	// posterContent holds the last rendered poster (ASCII art or kitty escape)
	// for posterItemID. It is re-applied to the active column's inspector on
	// every render, since the inspector clears its poster when SetItem is called.
	posterContent string
	// posterPlacement is the virtual kitty placement prepended to the
	// placeholder cells whenever the poster is rendered.
	posterPlacement string
	posterImageID   uint32
	posterOutput    io.Writer
}

// NewModel creates a new application model
func NewModel(
	store domain.Store,
	librarySvc *library.Service,
	playlistSvc *playlist.Service,
	searchSvc *search.Service,
	playbackSvc *player.Service,
	mediaClient mediaserver.MediaSource,
	appConfig *config.Config,
	uiConfig config.UIConfig,
	version string,
) Model {
	return Model{
		State:           StateBrowsing,
		Store:           store,
		LibraryService:  librarySvc,
		PlaylistService: playlistSvc,
		SearchSvc:       searchSvc,
		PlaybackSvc:     playbackSvc,
		MediaClient:     mediaClient,
		AppConfig:       appConfig,
		UIConfig:        uiConfig,
		Version:         version,
		ColumnStack:     NewColumnStack(),
		Inspector:       components.NewInspector(),
		GlobalSearch:    components.NewGlobalSearch(),
		PlaylistModal:   components.NewPlaylistModal(),
		InputModal:      components.NewInputModal(),
		LibraryStates:   make(map[string]components.LibrarySyncState),
		ShowInspector:   true, // Library list (Tab 1) shows horizontal inspector by default
	}
}

// SetOutput configures the synchronized terminal writer used for Kitty image
// uploads. Bubble Tea should be configured with the same writer.
func (m *Model) SetOutput(w io.Writer) {
	m.posterOutput = w
}

// Init initializes the application
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		LoadLibrariesCmd(m.LibraryService),
		TickCmd(100*time.Millisecond),
	)
}

// Update handles all messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.Ready = true
		m.updateLayout()
		return m, m.updateInspector()

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case TickMsg:
		m.SpinnerFrame++
		// Always propagate spinner frame - columns render spinner only when their loading flag is true
		m.ColumnStack.UpdateSpinnerFrame(m.SpinnerFrame)
		return m, TickCmd(100 * time.Millisecond)

	case LibrariesLoadedMsg:
		m.Libraries = msg.Libraries
		m.SyncGen++

		// Initialize all states to Syncing (including playlists)
		m.LibraryStates = make(map[string]components.LibrarySyncState)
		for _, lib := range msg.Libraries {
			m.LibraryStates[lib.ID] = components.LibrarySyncState{Status: components.StatusSyncing}
		}
		m.LibraryStates[playlistsLibraryID] = components.LibrarySyncState{Status: components.StatusSyncing}
		m.SyncingCount = len(msg.Libraries) + 1 // +1 for playlists
		m.MultiLibSync = true

		libraries := m.allLibraryEntries()
		resetStack := !msg.Refresh || m.ColumnStack == nil || m.ColumnStack.Len() == 0 ||
			(m.currentLibID != "" && !slices.ContainsFunc(msg.Libraries, func(lib domain.Library) bool {
				return lib.ID == m.currentLibID
			}))

		var libCol *components.ListColumn
		if resetStack {
			libCol = components.NewLibraryColumn(libraries)
			if m.ColumnStack == nil {
				m.ColumnStack = NewColumnStack()
			}
			m.ColumnStack.Reset(libCol)
			if msg.Refresh {
				m.currentLibID = ""
				m.currentShowID = ""
			}
		} else {
			libCol = m.ColumnStack.Get(0)
			libCol.SetItems(libraries)
		}

		libCol.SetLibraryStates(m.LibraryStates)
		libCol.SetShowWatchStatus(m.UIConfig.ShowWatchStatus)
		libCol.SetShowLibraryCounts(m.UIConfig.ShowLibraryCounts)
		m.Inspector.SetLibraryStates(m.LibraryStates)
		pc := m.updateInspector()

		// Start parallel sync of ALL libraries + playlists
		m.Loading = true
		return m, tea.Batch(
			pc,
			SyncAllLibrariesCmd(m.LibraryService, msg.Libraries, m.SyncGen),
			SyncPlaylistsCmd(m.PlaylistService, playlistsLibraryID, m.SyncGen),
		)

	case MoviesLoadedMsg:
		m.Loading = false

		// If manual load succeeded and library was in error state, clear it
		if state, ok := m.LibraryStates[msg.LibraryID]; ok && state.Status == components.StatusError {
			state.Status = components.StatusIdle
			state.Error = nil
			m.LibraryStates[msg.LibraryID] = state
			m.updateLibraryStates()
		}

		// Validate content ID to prevent race condition
		if !m.validateContentID(msg.LibraryID) {
			return m, nil
		}

		// Update matching column with movies
		if col := m.ColumnStack.FindColumn(msg.LibraryID); col != nil {
			selectedID := m.getSelectedItemID(col)
			col.SetItems(msg.Movies)
			if selectedID != "" {
				col.SetSelectedByID(selectedID)
			}
		}

		pc := m.updateInspector()

		// Advance nav plan if waiting for this load
		if cmd := m.advanceNavPlanAfterLoad(AwaitMovies, msg.LibraryID); cmd != nil {
			return m, tea.Batch(pc, cmd)
		}
		return m, pc

	case ShowsLoadedMsg:
		m.Loading = false

		// If manual load succeeded and library was in error state, clear it
		if state, ok := m.LibraryStates[msg.LibraryID]; ok && state.Status == components.StatusError {
			state.Status = components.StatusIdle
			state.Error = nil
			m.LibraryStates[msg.LibraryID] = state
			m.updateLibraryStates()
		}

		// Validate content ID to prevent race condition
		if !m.validateContentID(msg.LibraryID) {
			return m, nil
		}

		// Update matching column with shows
		if col := m.ColumnStack.FindColumn(msg.LibraryID); col != nil {
			selectedID := m.getSelectedItemID(col)
			col.SetItems(msg.Shows)
			if selectedID != "" {
				col.SetSelectedByID(selectedID)
			}
		}

		pc := m.updateInspector()

		// Advance nav plan if waiting for this load
		if cmd := m.advanceNavPlanAfterLoad(AwaitShows, msg.LibraryID); cmd != nil {
			return m, tea.Batch(pc, cmd)
		}
		return m, pc

	case MixedLibraryLoadedMsg:
		m.Loading = false

		// If manual load succeeded and library was in error state, clear it
		if state, ok := m.LibraryStates[msg.LibraryID]; ok && state.Status == components.StatusError {
			state.Status = components.StatusIdle
			state.Error = nil
			m.LibraryStates[msg.LibraryID] = state
			m.updateLibraryStates()
		}

		// Validate content ID to prevent race condition
		if !m.validateContentID(msg.LibraryID) {
			return m, nil
		}

		// Update matching column with mixed content
		if col := m.ColumnStack.FindColumn(msg.LibraryID); col != nil {
			selectedID := m.getSelectedItemID(col)
			col.SetItems(msg.Items)
			if selectedID != "" {
				col.SetSelectedByID(selectedID)
			}
		}

		pc := m.updateInspector()

		// Advance nav plan if waiting for this load
		if cmd := m.advanceNavPlanAfterLoad(AwaitMixed, msg.LibraryID); cmd != nil {
			return m, tea.Batch(pc, cmd)
		}
		return m, pc

	case SeasonsLoadedMsg:
		m.Loading = false

		// Validate content ID to prevent race condition
		if !m.validateContentID(msg.ShowID) {
			return m, nil
		}

		top := m.ColumnStack.Top()
		if top == nil {
			return m, nil
		}

		if top.ColumnType() == components.ColumnTypeSeasonEpisodes {
			// New path: build collapsible season groups
			groups := make([]components.SeasonGroup, len(msg.Seasons))
			for i, s := range msg.Seasons {
				groups[i] = components.SeasonGroup{
					Header: &components.SeasonHeader{Season: s},
				}
			}
			top.SetSeasonGroups(groups)
			pc := m.updateInspector()

			// Auto-expand first season and kick off its episode load
			if needsLoad, seasonID := top.ExpandFirstSeason(); needsLoad {
				m.Loading = true
				return m, tea.Batch(pc, LoadEpisodesCmd(m.LibraryService, m.currentLibID, m.currentShowID, seasonID))
			}
			return m, pc
		}

		// Classic path: populate seasons column
		selectedID := m.getSelectedItemID(top)
		top.SetItems(msg.Seasons)
		if selectedID != "" {
			top.SetSelectedByID(selectedID)
		}
		pc := m.updateInspector()

		// Advance nav plan if waiting for this load
		if cmd := m.advanceNavPlanAfterLoad(AwaitSeasons, msg.ShowID); cmd != nil {
			return m, tea.Batch(pc, cmd)
		}
		return m, pc

	case EpisodesLoadedMsg:
		m.Loading = false

		top := m.ColumnStack.Top()
		if top == nil {
			return m, nil
		}

		if top.ColumnType() == components.ColumnTypeSeasonEpisodes {
			// New path: insert episodes into the matching season group
			top.AddSeasonEpisodes(msg.SeasonID, msg.Episodes)
			pc := m.updateInspector()
			return m, pc
		}

		// Classic path: validate and populate a standalone episodes column
		if !m.validateContentID(msg.SeasonID) {
			return m, nil
		}
		selectedID := m.getSelectedItemID(top)
		top.SetItems(msg.Episodes)
		if selectedID != "" {
			top.SetSelectedByID(selectedID)
		}
		pc := m.updateInspector()

		// Advance nav plan if waiting for this load
		if cmd := m.advanceNavPlanAfterLoad(AwaitEpisodes, msg.SeasonID); cmd != nil {
			return m, tea.Batch(pc, cmd)
		}
		return m, pc

	case PlaybackStartedMsg:
		m.isPlayingTitle = playbackStatusText(msg.Item, msg.Item.ViewOffset)
		m.StatusMsg = ""
		return m, tea.Batch(
			WaitForPlaybackCmd(msg.Handle.ResultCh),
			ListenForPlaybackStatusCmd(msg.Handle.StatusCh),
		)

	case PlaybackStatusMsg:
		m.isPlayingTitle = playbackStatusText(msg.Status.Item, time.Duration(msg.Status.PositionMs)*time.Millisecond)
		return m, ListenForPlaybackStatusCmd(msg.StatusCh)

	case PlaybackFinishedMsg:
		if msg.Err != nil {
			m.StatusMsg = "Playback error: " + msg.Err.Error()
			m.StatusIsErr = true
		} else if msg.AutoMarked {
			m.StatusMsg = "✓ Finished & marked watched: " + msg.Title
		} else {
			m.StatusMsg = "Finished: " + msg.Title
		}

		// Only clear status if we're NOT playing another title
		if m.isPlayingTitle != "" {
			m.isPlayingTitle = ""
			cmds = append(cmds, ClearStatusCmd(3*time.Second))
		}

		// Refresh view to update watch status indicators
		cmds = append(cmds, m.refreshCurrentView())
		return m, tea.Batch(cmds...)

	case MarkWatchedMsg:
		m.StatusMsg = "Marked watched: " + msg.Title
		m.applyWatchState(msg.ItemID, true)
		cmds = append(cmds, ClearStatusCmd(3*time.Second))
		return m, tea.Batch(cmds...)

	case MarkUnwatchedMsg:
		m.StatusMsg = "Marked unwatched: " + msg.Title
		m.applyWatchState(msg.ItemID, false)
		cmds = append(cmds, ClearStatusCmd(3*time.Second))
		return m, tea.Batch(cmds...)

	case ErrMsg:
		m.clearNavPlan()
		m.StatusIsErr = true
		m.Loading = false
		if errors.Is(msg.Err, domain.ErrAuthFailed) {
			// Actionable, persistent message: the token was revoked/expired
			// and the user must re-authenticate
			m.StatusMsg = authFailedStatusMsg
			return m, nil
		}
		m.StatusMsg = msg.Error()
		cmds = append(cmds, ClearStatusCmd(5*time.Second))
		return m, tea.Batch(cmds...)

	case StatusMsg:
		m.StatusMsg = msg.Message
		m.StatusIsErr = msg.IsError
		cmds = append(cmds, ClearStatusCmd(3*time.Second))
		return m, tea.Batch(cmds...)

	case ClearStatusMsg:
		m.StatusMsg = ""
		m.StatusIsErr = false
		return m, nil

	case LibrarySyncProgressMsg:
		if msg.Generation != m.SyncGen {
			return m, nil
		}
		state := m.LibraryStates[msg.LibraryID]

		if msg.Error != nil {
			state.Status = components.StatusError
			state.Error = msg.Error
			m.SyncingCount--
			slog.Error("library sync failed", "libraryID", msg.LibraryID, "error", msg.Error)
			if errors.Is(msg.Error, domain.ErrAuthFailed) {
				m.StatusMsg = authFailedStatusMsg
				m.StatusIsErr = true
			}
		} else {
			state.Loaded = msg.Loaded
			state.Total = msg.Total
			state.FromCache = msg.FromCache

			if msg.Done {
				state.Status = components.StatusSynced
				m.SyncingCount--

				// Trigger delayed cleanup
				cmds = append(cmds, ClearLibraryStatusCmd(msg.LibraryID, 2*time.Second))

			}
		}

		m.LibraryStates[msg.LibraryID] = state
		m.updateLibraryStates()

		// If there's a continuation command, run it
		if msg.NextCmd != nil {
			cmds = append(cmds, msg.NextCmd)
		}

		// Check if all done
		if m.SyncingCount == 0 {
			m.Loading = false
		}

		return m, tea.Batch(cmds...)

	case ClearLibraryStatusMsg:
		if state, ok := m.LibraryStates[msg.LibraryID]; ok {
			if state.Status == components.StatusSynced {
				state.Status = components.StatusIdle
				m.LibraryStates[msg.LibraryID] = state
				m.updateLibraryStates()
			}
		}
		return m, nil

	case LogoutCompleteMsg:
		if msg.Error != nil {
			m.StatusMsg = fmt.Sprintf("Logout failed: %v", msg.Error)
			m.StatusIsErr = true
			m.State = StateBrowsing
			return m, ClearStatusCmd(5 * time.Second)
		}
		// Logout successful - quit the application
		return m, tea.Quit

	case PlaylistsLoadedMsg:
		m.Loading = false
		if top := m.ColumnStack.Top(); top != nil {
			top.SetItems(msg.Playlists)
		}
		pc := m.updateInspector()
		return m, pc

	case PlaylistItemsLoadedMsg:
		m.Loading = false

		// Validate content ID to prevent race condition
		if !m.validateContentID(msg.PlaylistID) {
			return m, nil
		}

		m.currentPlaylistID = msg.PlaylistID
		if top := m.ColumnStack.Top(); top != nil {
			top.SetItems(msg.Items)
		}
		pc := m.updateInspector()
		return m, pc

	case ContinueWatchingLoadedMsg:
		m.Loading = false
		if top := m.ColumnStack.Top(); top != nil && top.ContentID() == continueLibraryID {
			top.SetItems(msg.Items)
		}
		pc := m.updateInspector()
		return m, pc

	case PosterLoadedMsg:
		if msg.RequestID != m.posterRequestID || msg.ItemID != m.posterItemID {
			deleteKittyImage(m.posterOutput, msg.ImageID)
			return m, nil
		}
		if m.posterImageID != msg.ImageID {
			deleteKittyImage(m.posterOutput, m.posterImageID)
		}
		m.posterContent = msg.Content
		m.posterPlacement = msg.Placement
		m.posterImageID = msg.ImageID
		return m, nil

	case SeasonForPlaybackLoadedMsg:
		m.Loading = false
		playlist := make([]domain.MediaItem, len(msg.Episodes))
		for i, ep := range msg.Episodes {
			playlist[i] = *ep
		}
		if !msg.Resume {
			return m, PlayItemCmd(m.PlaybackSvc, *msg.Item, false, m.UIConfig.Autoplay, playlist...)
		}
		return m.playOrConfirmResume(msg.Item, playlist)

	case PlaylistModalDataMsg:
		m.PlaylistModal.Show(msg.Playlists, msg.Membership, msg.Item)
		m.PlaylistModal.SetSize(m.Width, m.Height)
		return m, nil

	case RemoteSearchLoadedMsg:
		if msg.Error != nil {
			m.StatusMsg = fmt.Sprintf("Remote search failed: %v", msg.Error)
			m.StatusIsErr = true
			return m, ClearStatusCmd(4 * time.Second)
		}
		if m.GlobalSearch.IsVisible() && msg.Query == m.GlobalSearch.Query() && len(msg.Results) > 0 {
			m.GlobalSearch.SetResults(msg.Results)
		}
		return m, nil

	case QueueUpdatedMsg:
		if msg.Error != nil {
			m.StatusMsg = fmt.Sprintf("Queue update failed: %v", msg.Error)
			m.StatusIsErr = true
		} else {
			m.StatusMsg = msg.Message
		}
		if top := m.ColumnStack.Top(); top != nil && top.ContentID() == queueLibraryID {
			top.SetItems(m.PlaylistService.QueueItems())
		}
		return m, ClearStatusCmd(3 * time.Second)

	case PlaylistUpdatedMsg:
		if msg.Error != nil {
			m.StatusMsg = fmt.Sprintf("Playlist update failed: %v", msg.Error)
			m.StatusIsErr = true
		} else {
			m.StatusMsg = "Playlist updated"
			// Refresh playlist items if viewing a playlist
			if m.currentPlaylistID != "" {
				return m, LoadPlaylistItemsCmd(m.PlaylistService, m.currentPlaylistID)
			}
		}
		cmds = append(cmds, ClearStatusCmd(3*time.Second))
		return m, tea.Batch(cmds...)

	case PlaylistCreatedMsg:
		if msg.Error != nil {
			m.StatusMsg = fmt.Sprintf("Failed to create playlist: %v", msg.Error)
			m.StatusIsErr = true
		} else {
			m.StatusMsg = fmt.Sprintf("Created playlist: %s", msg.Playlist.Title)
			// Refresh playlists if viewing playlists
			if top := m.ColumnStack.Top(); top != nil && top.ColumnType() == components.ColumnTypePlaylists {
				return m, LoadPlaylistsCmd(m.PlaylistService)
			}
		}
		cmds = append(cmds, ClearStatusCmd(3*time.Second))
		return m, tea.Batch(cmds...)

	case PlaylistDeletedMsg:
		if msg.Error != nil {
			m.StatusMsg = fmt.Sprintf("Failed to delete playlist: %v", msg.Error)
			m.StatusIsErr = true
			cmds = append(cmds, ClearStatusCmd(3*time.Second))
		} else {
			m.StatusMsg = "Playlist deleted"
			// Clear current playlist ID and refresh the playlists
			m.currentPlaylistID = ""
			cmds = append(cmds, LoadPlaylistsCmd(m.PlaylistService))
			cmds = append(cmds, ClearStatusCmd(3*time.Second))
		}
		return m, tea.Batch(cmds...)

	case RefreshCurrentMsg:
		return m, m.refreshAfterStatusChange(msg.LibraryID)

	case MediaItemDeletedMsg:
		// Instead of just refreshing the UI, we invalidate and reload.
		// Find the column that has this item and remove it from view instantly,
		// and also trigger a background refresh.
		m.StatusMsg = "Deleted successfully"
		cmds = append(cmds, ClearStatusCmd(3*time.Second))
		return m, tea.Batch(append(cmds, m.refreshCurrentView())...)
	}

	// Update the focused column (top of stack)
	if top := m.ColumnStack.Top(); top != nil {
		oldCursor := top.SelectedIndex()
		newCol, cmd := top.Update(msg)
		m.ColumnStack.columns[len(m.ColumnStack.columns)-1] = newCol
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		if oldCursor != top.SelectedIndex() {
			if pc := m.updateInspector(); pc != nil {
				cmds = append(cmds, pc)
			}
		}
	}

	return m, tea.Batch(cmds...)
}

// libraryColumn returns the library column (index 0) or nil if not available
func (m *Model) libraryColumn() *components.ListColumn {
	return m.ColumnStack.Get(0)
}

// validateContentID checks if the top column has the expected content ID.
// Returns false if the column doesn't match (user navigated away before async load completed).
func (m *Model) validateContentID(expectedID string) bool {
	top := m.ColumnStack.Top()
	return top != nil && top.ContentID() == expectedID
}

// updateLibraryStates updates the library states in the library column and inspector
func (m *Model) updateLibraryStates() {
	if libCol := m.libraryColumn(); libCol != nil {
		libCol.SetLibraryStates(m.LibraryStates)
	}
	m.Inspector.SetLibraryStates(m.LibraryStates)
}

// applyWatchState updates every cached and currently rendered copy of an item.
// Keeping the cache in sync is important: otherwise a subsequent cached load
// can immediately restore the item's old watch status after the server update.
func (m *Model) applyWatchState(itemID string, played bool) {
	m.LibraryService.SetWatchState(itemID, played)

	var patched *domain.MediaItem
	flipped := false
	for i := 0; i < m.ColumnStack.Len(); i++ {
		if col := m.ColumnStack.Get(i); col != nil {
			if item, changed := col.ApplyWatchState(itemID, played); item != nil {
				patched = item
				flipped = flipped || changed
			}
		}
	}

	if flipped && patched != nil && patched.ShowID != "" {
		delta := 1
		if played {
			delta = -1
		}
		for i := 0; i < m.ColumnStack.Len(); i++ {
			if col := m.ColumnStack.Get(i); col != nil {
				col.AdjustUnwatchedCounts(patched.ShowID, patched.ParentID, delta)
			}
		}
	}

	m.updateInspector()
}

// refreshCurrentView refreshes the current view
func (m *Model) refreshCurrentView() tea.Cmd {
	m.invalidatePoster()
	if m.currentLibID != "" {
		m.LibraryService.InvalidateLibrary(m.currentLibID)
	} else {
		m.LibraryService.InvalidateAll()
	}
	m.Loading = true

	top := m.ColumnStack.Top()
	if top == nil {
		return LoadLibrariesCmd(m.LibraryService)
	}

	var cmds []tea.Cmd

	// Get context from column stack to reload
	switch top.ColumnType() {
	case components.ColumnTypeMovies:
		if libCol := m.libraryColumn(); libCol != nil {
			if lib := libCol.SelectedLibrary(); lib != nil {
				cmds = append(cmds, LoadMoviesCmd(m.LibraryService, *lib))
			}
		}
	case components.ColumnTypeShows:
		if libCol := m.libraryColumn(); libCol != nil {
			if lib := libCol.SelectedLibrary(); lib != nil {
				cmds = append(cmds, LoadShowsCmd(m.LibraryService, *lib))
			}
		}
	case components.ColumnTypeSeasons:
		// Get show from parent column - needs libID for hierarchical cache
		if showCol := m.ColumnStack.Get(m.ColumnStack.Len() - 2); showCol != nil {
			if show := showCol.SelectedShow(); show != nil {
				cmds = append(cmds, LoadSeasonsCmd(m.LibraryService, m.currentLibID, show.ID))
			}
		}
	case components.ColumnTypeEpisodes:
		// Get season from parent column - needs full ancestry for hierarchical cache
		if seasonCol := m.ColumnStack.Get(m.ColumnStack.Len() - 2); seasonCol != nil {
			if season := seasonCol.SelectedSeason(); season != nil {
				cmds = append(cmds, LoadEpisodesCmd(m.LibraryService, m.currentLibID, m.currentShowID, season.ID))
			}
		}
		// ALSO reload the parent (Shows) to update counters
		if showCol := m.ColumnStack.Get(m.ColumnStack.Len() - 3); showCol != nil && showCol.ColumnType() == components.ColumnTypeShows {
			if lib := m.findLibrary(m.currentLibID); lib != nil {
				cmds = append(cmds, LoadShowsCmd(m.LibraryService, *lib))
			}
		}
	case components.ColumnTypeSeasonEpisodes:
		// Collapsible view: reload seasons for the show
		cmds = append(cmds, LoadSeasonsCmd(m.LibraryService, m.currentLibID, m.currentShowID))
		// ALSO reload the parent (Shows) to update counters
		if showCol := m.ColumnStack.Get(m.ColumnStack.Len() - 2); showCol != nil && showCol.ColumnType() == components.ColumnTypeShows {
			if lib := m.findLibrary(m.currentLibID); lib != nil {
				cmds = append(cmds, LoadShowsCmd(m.LibraryService, *lib))
			}
		}
	case components.ColumnTypeMixed:
		if libCol := m.libraryColumn(); libCol != nil {
			if lib := libCol.SelectedLibrary(); lib != nil {
				cmds = append(cmds, LoadMixedLibraryCmd(m.LibraryService, *lib))
			}
		}
	}

	if len(cmds) > 0 {
		return tea.Batch(cmds...)
	}

	return LoadLibrariesCmd(m.LibraryService)
}

// refreshAfterStatusChange handles refreshing the UI after a watch status change.
// It invalidates the appropriate caches and triggers a reload of the current view.
func (m *Model) refreshAfterStatusChange(libID string) tea.Cmd {
	if libID != "" {
		m.LibraryService.InvalidateLibrary(libID)
	}

	// Targeted refresh of current view
	return m.refreshCurrentView()
}

// propagateWatchStatus updates parent items in the column stack when an episode's status changes
func (m *Model) propagateWatchStatus(item *domain.MediaItem, watched bool) {
	if item.Type != domain.MediaTypeEpisode {
		return
	}

	delta := 1
	if watched {
		delta = -1
	}

	for i := 0; i < m.ColumnStack.Len(); i++ {
		col := m.ColumnStack.Get(i)
		for _, listItem := range col.Items() {
			switch v := listItem.(type) {
			case *domain.Show:
				if v.ID == item.ShowID {
					v.UnwatchedCount += delta
					if v.UnwatchedCount < 0 {
						v.UnwatchedCount = 0
					}
					if v.UnwatchedCount > v.EpisodeCount {
						v.UnwatchedCount = v.EpisodeCount
					}
				}
			case *domain.Season:
				if v.ID == item.ParentID {
					v.UnwatchedCount += delta
					if v.UnwatchedCount < 0 {
						v.UnwatchedCount = 0
					}
					if v.UnwatchedCount > v.EpisodeCount {
						v.UnwatchedCount = v.EpisodeCount
					}
				}
			}
		}
	}
}

// findLibrary finds a library by ID
func (m Model) findLibrary(id string) *domain.Library {
	for _, lib := range m.Libraries {
		if lib.ID == id {
			return &lib
		}
	}
	return nil
}

// updateInspector updates the inspector with the selected item from middle column
func (m *Model) updateInspector() tea.Cmd {
	var top *components.ListColumn
	if m.ColumnStack != nil {
		top = m.ColumnStack.Top()
	}
	if top == nil {
		m.Inspector.SetItem(nil)
		if m.hasPosterState() {
			m.invalidatePoster()
		}
		return nil
	}

	item := top.SelectedItem()
	m.Inspector.SetItem(item)

	// The inspector follows the active column, while the preview follows the
	// nearest movie/show browsing column. Opening seasons or episodes must not
	// discard the artwork for the selected parent show.
	posterCol := m.posterSourceColumn()
	if posterCol == nil {
		if m.hasPosterState() {
			m.invalidatePoster()
		}
		return nil
	}
	posterItem := posterCol.SelectedItem()
	id := m.getSelectedItemID(posterCol)
	url := PosterURL(posterItem)
	if id == "" || (url == "" && !posterMetadataFallback(posterItem)) {
		if m.hasPosterState() {
			m.invalidatePoster()
		}
		return nil
	}

	width := m.posterPreviewWidth()
	maxHeight := m.posterMaxHeight()
	requestKey := strings.Join([]string{id, url, fmt.Sprint(width), fmt.Sprint(maxHeight)}, "\x00")
	if requestKey == m.posterRequestKey {
		return nil
	}

	m.posterRequestID++
	requestID := m.posterRequestID
	m.posterRequestKey = requestKey
	m.posterItemID = id
	return FetchPosterCmd(m.MediaClient, m.posterOutput, requestID, id, url, width, maxHeight)
}

func (m Model) posterSourceColumn() *components.ListColumn {
	if m.ColumnStack == nil {
		return nil
	}
	for idx := m.ColumnStack.Len() - 1; idx >= 0; idx-- {
		col := m.ColumnStack.Get(idx)
		if col != nil && showsPosterForColumn(col.ColumnType()) {
			return col
		}
	}
	return nil
}

func showsPosterForColumn(columnType components.ColumnType) bool {
	switch columnType {
	case components.ColumnTypeMovies, components.ColumnTypeShows, components.ColumnTypeMixed:
		return true
	default:
		return false
	}
}

func (m Model) posterPreviewWidth() int {
	if m.Width <= 0 {
		return posterMinWidth
	}
	layout := m.calculateColumnLayout(m.Width)
	width := layout.parentWidth
	if m.ColumnStack != nil && m.ColumnStack.Len() >= 3 && layout.grandparentWidth > 0 {
		width = layout.grandparentWidth
	}
	width -= 4 // preview border and a small horizontal margin
	if width > posterPreviewWidth {
		width = posterPreviewWidth
	}
	if width < 1 {
		return 1
	}
	return width
}

func posterMetadataFallback(item interface{}) bool {
	_, ok := item.(*domain.MediaItem)
	return ok
}

func (m Model) posterMaxHeight() int {
	if m.Height <= ChromeHeight {
		return 0
	}

	contentHeight := m.Height - ChromeHeight
	previewHeight := contentHeight - contentHeight/2
	// Preview border, title, and the blank line below the title.
	maxHeight := previewHeight - 4
	if maxHeight < 1 {
		maxHeight = 1
	}
	return maxHeight
}

func (m *Model) hasPosterState() bool {
	return m.posterRequestKey != "" || m.posterItemID != "" || m.posterContent != "" ||
		m.posterPlacement != "" || m.posterImageID != 0
}

// clearPosterState removes the currently rendered Kitty image and clears the
// displayed poster without invalidating the request generation.
func (m *Model) clearPosterState() {
	deleteKittyImage(m.posterOutput, m.posterImageID)
	m.posterItemID = ""
	m.posterContent = ""
	m.posterPlacement = ""
	m.posterImageID = 0
}

// invalidatePoster makes every in-flight poster result stale.
func (m *Model) invalidatePoster() {
	m.posterRequestID++
	m.posterRequestKey = ""
	m.clearPosterState()
}

// getSelectedItemID returns the ID of the selected item in a column
func (m Model) getSelectedItemID(c *components.ListColumn) string {
	if c == nil {
		return ""
	}
	item, ok := c.SelectedItem().(domain.ListItem)
	if !ok {
		return ""
	}
	return item.GetID()
}
