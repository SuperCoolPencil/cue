package tui

import (
	"time"

	"github.com/SuperCoolPencil/cue/internal/domain"
	"github.com/SuperCoolPencil/cue/internal/tui/components"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// handleKeyMsg handles keyboard input
func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle state-specific keys
	switch m.State {
	case StateHelp:
		if key.Matches(msg, Keys.Escape, Keys.Help, Keys.Quit) {
			m.State = StateBrowsing
		}
		return m, nil

	case StateConfirmLogout:
		switch {
		case key.Matches(msg, Keys.Confirm):
			// User confirmed logout
			return m, LogoutCmd()
		case key.Matches(msg, Keys.Deny):
			// User cancelled
			m.State = StateBrowsing
		}
		return m, nil
	case StateConfirmResume:
		if m.pendingPlayback == nil {
			m.State = StateBrowsing
			return m, nil
		}
		switch {
		case key.Matches(msg, Keys.Confirm):
			item := *m.pendingPlayback
			playlist := m.pendingPlaylist
			m.pendingPlayback = nil
			m.pendingPlaylist = nil
			m.State = StateBrowsing
			return m, PlayItemCmd(m.PlaybackSvc, item, true, m.UIConfig.Autoplay, playlist...)

		case key.Matches(msg, Keys.Deny):
			item := *m.pendingPlayback
			playlist := m.pendingPlaylist
			m.pendingPlayback = nil
			m.pendingPlaylist = nil
			m.State = StateBrowsing
			return m, PlayItemCmd(m.PlaybackSvc, item, false, m.UIConfig.Autoplay, playlist...)

		case key.Matches(msg, Keys.Escape):
			m.pendingPlayback = nil
			m.pendingPlaylist = nil
			m.State = StateBrowsing
		}
		return m, nil

	case StateConfirmDelete:
		if m.pendingDelete == nil {
			m.State = StateBrowsing
			return m, nil
		}
		switch {
		case key.Matches(msg, Keys.Confirm):
			item := m.pendingDelete
			m.pendingDelete = nil
			m.State = StateBrowsing
			m.StatusMsg = "Deleting " + item.GetTitle() + "..."
			return m, tea.Batch(
				DeleteMediaItemCmd(m.MediaClient, item.GetID(), item.GetLibraryID()),
				ClearStatusCmd(3*time.Second),
			)
		case key.Matches(msg, Keys.Deny), key.Matches(msg, Keys.Escape):
			m.pendingDelete = nil
			m.State = StateBrowsing
		}
		return m, nil

	case StateInspecting:
		if key.Matches(msg, Keys.Back, Keys.Escape) {
			m.State = StateBrowsing
			m.Inspector.Focused = false
			if top := m.ColumnStack.Top(); top != nil {
				top.SetInspectorFocused(false)
			}
			return m, nil
		}
		if key.Matches(msg, Keys.ToggleInspector) {
			return m.handleToggleInspector()
		}
		var cmd tea.Cmd
		if m.ColumnStack.Len() == 1 {
			m.Inspector, cmd = m.Inspector.Update(msg)
		} else if top := m.ColumnStack.Top(); top != nil {
			insp := top.Inspector()
			*insp, cmd = insp.Update(msg)
		}
		return m, cmd
	}

	// Route to active modal if any
	if handled, newModel, cmd := m.routeToModal(msg); handled {
		return newModel, cmd
	}

	// Global keys
	switch {
	case key.Matches(msg, Keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, Keys.Help):
		return m.handleHelp()
	case key.Matches(msg, Keys.Escape):
		return m.handleEscape()
	case key.Matches(msg, Keys.Filter):
		return m.handleFilter()
	case key.Matches(msg, Keys.GlobalSearch):
		return m.handleGlobalSearch()
	case key.Matches(msg, Keys.Sort):
		return m.handleSort()
	case key.Matches(msg, Keys.Back):
		return m.handleBack()
	case key.Matches(msg, Keys.Right):
		return m.handleDrillIn()
	case key.Matches(msg, Keys.Enter):
		return m.handleEnter()
	case key.Matches(msg, Keys.Refresh):
		return m.handleRefresh()
	case key.Matches(msg, Keys.RefreshAll):
		return m.handleRefreshAll()
	case key.Matches(msg, Keys.MarkWatched):
		return m.handleMarkWatched()
	case key.Matches(msg, Keys.MarkUnwatched):
		return m.handleMarkUnwatched()
	case key.Matches(msg, Keys.Play):
		return m.handlePlay()
	case key.Matches(msg, Keys.DirectPlay):
		return m.handleDirectPlay()
	case key.Matches(msg, Keys.ToggleInspector):
		return m.handleToggleInspector()
	case key.Matches(msg, Keys.Logout):
		return m.handleLogout()
	case key.Matches(msg, Keys.PlaylistModal):
		return m.handlePlaylistModal()
	case key.Matches(msg, Keys.Delete):
		return m.handleDelete()
	case key.Matches(msg, Keys.NewPlaylist):
		return m.handleNewPlaylist()
	case key.Matches(msg, Keys.Queue):
		return m.handleQueue()
	case key.Matches(msg, Keys.NextEpisode):
		return m.handleNextEpisode()
	}

	// Let the focused column handle remaining keys (j/k/g/G navigation)
	if top := m.ColumnStack.Top(); top != nil {
		oldCursor := top.SelectedIndex()
		newCol, cmd := top.Update(msg)
		m.ColumnStack.UpdateTop(newCol)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		if oldCursor != top.SelectedIndex() {
			m.updateInspector()
		}
	}

	return m, tea.Batch(cmds...)
}

// routeToModal routes key input to active modals
// Returns (handled, model, cmd) where handled is true if a modal consumed the input
func (m Model) routeToModal(msg tea.KeyMsg) (bool, Model, tea.Cmd) {
	if m.GlobalSearch.IsVisible() {
		newModel, cmd := m.handleGlobalSearchInput(msg)
		return true, newModel, cmd
	}
	if m.SortModal.IsVisible() {
		return m.handleSortModalInput(msg)
	}
	if m.PlaylistModal.IsVisible() {
		return m.handlePlaylistModalInput(msg)
	}
	if m.InputModal.IsVisible() {
		return m.handleInputModalInput(msg)
	}
	if top := m.ColumnStack.Top(); top != nil && top.IsFilterTyping() {
		return m.handleFilterTypingInput(msg)
	}
	return false, m, nil
}

// ----------------------------------------------------------------------------
// Global key handlers
// ----------------------------------------------------------------------------

// handleHelp shows the help screen
func (m Model) handleHelp() (tea.Model, tea.Cmd) {
	m.State = StateHelp
	return m, nil
}

// handleEscape clears active filter or cancels nav plan
func (m Model) handleEscape() (tea.Model, tea.Cmd) {
	if top := m.ColumnStack.Top(); top != nil && top.IsFiltering() {
		top.ClearFilter()
		return m, nil
	}
	if m.navPlan != nil {
		m.clearNavPlan()
		m.StatusMsg = "Navigation cancelled"
		return m, ClearStatusCmd(2 * time.Second)
	}
	return m, nil
}

// handleFilter toggles filter mode in the current column
func (m Model) handleFilter() (tea.Model, tea.Cmd) {
	if top := m.ColumnStack.Top(); top != nil {
		top.ToggleFilter()
	}
	return m, nil
}

// handleGlobalSearch opens the global search modal
func (m Model) handleGlobalSearch() (tea.Model, tea.Cmd) {
	m.GlobalSearch.Show()
	m.GlobalSearch.SetSize(m.Width, m.Height)
	return m, m.GlobalSearch.Init()
}

// handleDrillIn handles drilling into the selected item (l key)
func (m Model) handleDrillIn() (tea.Model, tea.Cmd) {
	top := m.ColumnStack.Top()
	if top == nil {
		return m, nil
	}

	// For the collapsible season+episode column, l acts like Enter
	if top.ColumnType() == components.ColumnTypeSeasonEpisodes {
		return m.handleEnter()
	}

	if !top.CanDrillInto() {
		if item := top.SelectedMediaItem(); item != nil {
			return m.playItemWithContext(top, item, true)
		}

		return m, nil
	}
	return m.drillIntoSelection()
}

// handleEnter handles the enter key press
func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	top := m.ColumnStack.Top()
	if top == nil {
		return m, nil
	}

	// In the collapsible season+episode column, Enter on a season header toggles it
	if top.ColumnType() == components.ColumnTypeSeasonEpisodes {
		if top.SelectedSeasonHeader() != nil {
			needsLoad, seasonID := top.ToggleSelectedSeason()
			if needsLoad {
				m.Loading = true
				return m, LoadEpisodesCmd(m.LibraryService, m.currentLibID, m.currentShowID, seasonID)
			}
			m.updateInspector()
			return m, nil
		}
		// Cursor is on an episode — play it
		if item := top.SelectedMediaItem(); item != nil {
			return m.playItemWithContext(top, item, true)
		}

		return m, nil
	}
	if m.UIConfig.PlayNextOnSelect {
		if item, ok := top.SelectedItem().(domain.ListItem); ok && item.GetItemType() == "show" {
			return m.playNextEpisodeForShow(item)
		}
	}

	if top.CanDrillInto() {
		return m.drillIntoSelection()
	}
	if item := top.SelectedMediaItem(); item != nil {
		return m.playItemWithContext(top, item, true)
	}

	return m, nil
}

// handleSort opens the sort modal for movies/shows columns
func (m Model) handleSort() (tea.Model, tea.Cmd) {
	top := m.ColumnStack.Top()
	if top == nil {
		return m, nil
	}
	var opts []components.SortField
	switch top.ColumnType() {
	case components.ColumnTypeMovies:
		opts = components.MovieSortOptions()
	case components.ColumnTypeShows:
		opts = components.ShowSortOptions()
	case components.ColumnTypeEpisodes:
		opts = components.EpisodeSortOptions()
	case components.ColumnTypeMixed:
		opts = components.MixedSortOptions()
	}
	if opts != nil {
		field, dir := top.SortState()
		m.SortModal.Show(opts, field, dir)
	}
	return m, nil
}

// handleRefresh performs context-sensitive refresh with cascade invalidation.
// At library level: refresh selected library (cascade to seasons/episodes)
// At show level: refresh selected show (cascade to seasons/episodes)
// At season level: refresh selected season (cascade to episodes)
// At episode level: refresh current season's episodes
func (m Model) handleRefresh() (tea.Model, tea.Cmd) {
	top := m.ColumnStack.Top()
	if top == nil {
		return m, nil
	}

	switch top.ColumnType() {
	case components.ColumnTypeLibraries:
		// Refresh selected library
		lib := top.SelectedLibrary()
		if lib == nil || lib.ID == playlistsLibraryID || lib.Type == "cue" || lib.Type == "filter" || lib.Type == "profile" || lib.Type == "config" || lib.Type == "cache" {
			return m, nil
		}
		m.LibraryStates[lib.ID] = components.LibrarySyncState{Status: components.StatusSyncing}
		m.SyncingCount++
		m.Loading = true
		m.MultiLibSync = false
		m.updateLibraryStates()
		// Invalidate then sync
		m.LibraryService.InvalidateLibrary(lib.ID)
		return m, SyncLibraryCmd(m.LibraryService, *lib)

	case components.ColumnTypeMovies, components.ColumnTypeMixed, components.ColumnTypeShows:
		return m.refreshLibraryContent(top)

	case components.ColumnTypeSeasons:
		// Refresh current show's seasons (invalidate seasons + episodes, re-fetch seasons)
		m.LibraryService.InvalidateShow(m.currentLibID, m.currentShowID)
		top.SetItems(nil)
		top.SetLoading(true)
		m.Loading = true
		return m, LoadSeasonsCmd(m.LibraryService, m.currentLibID, m.currentShowID)

	case components.ColumnTypeEpisodes:
		// Refresh current season's episodes
		seasonCol := m.ColumnStack.Get(m.ColumnStack.Len() - 2)
		if seasonCol == nil {
			return m, nil
		}
		season := seasonCol.SelectedSeason()
		if season == nil {
			return m, nil
		}
		m.LibraryService.InvalidateSeason(m.currentLibID, m.currentShowID, season.ID)
		top.SetItems(nil)
		top.SetLoading(true)
		m.Loading = true
		return m, LoadEpisodesCmd(m.LibraryService, m.currentLibID, m.currentShowID, season.ID)

	case components.ColumnTypePlaylists:
		// Refresh playlists
		top.SetItems(nil)
		top.SetLoading(true)
		m.Loading = true
		return m, LoadPlaylistsCmd(m.PlaylistService)

	case components.ColumnTypePlaylistItems:
		// Refresh playlist items
		if m.currentPlaylistID == "" {
			return m, nil
		}
		top.SetItems(nil)
		top.SetLoading(true)
		m.Loading = true
		return m, LoadPlaylistItemsCmd(m.PlaylistService, m.currentPlaylistID)
	}

	return m, nil
}

// refreshLibraryContent refreshes movies, shows, or mixed content in the current library
func (m Model) refreshLibraryContent(top *components.ListColumn) (Model, tea.Cmd) {
	libCol := m.libraryColumn()
	if libCol == nil {
		return m, nil
	}
	lib := libCol.SelectedLibrary()
	if lib == nil {
		return m, nil
	}
	m.LibraryService.InvalidateLibrary(lib.ID)
	top.SetItems(nil)
	top.SetLoading(true)
	m.Loading = true

	switch lib.Type {
	case "movie":
		return m, LoadMoviesCmd(m.LibraryService, lib.ID)
	case "show":
		return m, LoadShowsCmd(m.LibraryService, lib.ID)
	default:
		return m, LoadMixedLibraryCmd(m.LibraryService, lib.ID)
	}
}

// handleRefreshAll refreshes all libraries and resets to library view
func (m Model) handleRefreshAll() (tea.Model, tea.Cmd) {
	m.Loading = true

	// Invalidate all cached data, then re-fetch libraries from the server.
	// This goes through LoadLibrariesCmd -> LibrariesLoadedMsg which rebuilds
	// the full library list and triggers SyncAllLibrariesCmd with fresh data.
	m.LibraryService.InvalidateAll()
	m.PlaylistService.InvalidatePlaylists()
	return m, LoadLibrariesCmd(m.LibraryService)
}

// handleMarkWatched marks the selected item as watched
func (m Model) handleMarkWatched() (tea.Model, tea.Cmd) {
	top := m.ColumnStack.Top()
	if top == nil {
		return m, nil
	}
	item, ok := top.SelectedItem().(domain.ListItem)
	if !ok || item.GetItemType() == "library" {
		return m, nil
	}
	return m, MarkWatchedCmd(m.PlaybackSvc, item.GetLibraryID(), item.GetID(), item.GetTitle())
}

// handleMarkUnwatched marks the selected item as unwatched
func (m Model) handleMarkUnwatched() (tea.Model, tea.Cmd) {
	top := m.ColumnStack.Top()
	if top == nil {
		return m, nil
	}
	item, ok := top.SelectedItem().(domain.ListItem)
	if !ok || item.GetItemType() == "library" {
		return m, nil
	}
	return m, MarkUnwatchedCmd(m.PlaybackSvc, item.GetLibraryID(), item.GetID(), item.GetTitle())
}

// handlePlay plays the selected item from the beginning
func (m Model) handlePlay() (tea.Model, tea.Cmd) {
	top := m.ColumnStack.Top()
	if top == nil {
		return m, nil
	}
	item := top.SelectedMediaItem()
	if item == nil {
		return m, nil
	}
	return m.playItemWithContext(top, item, false)
}

// playItemWithContext handles playback of an item with appropriate playlist context.
// If playing an episode from a non-sequential view (like Continue Watching), it fetches the full season.
func (m Model) playItemWithContext(top *components.ListColumn, item *domain.MediaItem, resume bool) (tea.Model, tea.Cmd) {
	if item.Type == domain.MediaTypeEpisode && (top.ColumnType() == components.ColumnTypeMixed || top.ContentID() == continueLibraryID) {
		m.Loading = true
		return m, LoadSeasonForPlaybackCmd(m.LibraryService, item, resume)
	}

	playlist, _ := top.GetSeasonPlaylist()
	if !resume {
		return m, PlayItemCmd(m.PlaybackSvc, *item, false, m.UIConfig.Autoplay, playlist...)
	}
	return m.playOrConfirmResume(item, playlist)
}

func (m Model) playOrConfirmResume(item *domain.MediaItem, playlist []domain.MediaItem) (tea.Model, tea.Cmd) {
	if item == nil {
		return m, nil
	}
	if item.ShouldResume() {
		m.pendingPlayback = item
		m.pendingPlaylist = playlist
		m.State = StateConfirmResume
		return m, nil
	}
	return m, PlayItemCmd(m.PlaybackSvc, *item, false, m.UIConfig.Autoplay, playlist...)
}

// handleToggleInspector toggles the inspector panel visibility or focuses it
func (m Model) handleToggleInspector() (tea.Model, tea.Cmd) {
	// If already in inspecting state, hide the inspector and go back to browsing
	if m.State == StateInspecting {
		m.ShowInspector = false
		m.State = StateBrowsing
		m.Inspector.Focused = false
		if top := m.ColumnStack.Top(); top != nil {
			top.SetInspectorFocused(false)
		}
		m.updateLayout()
		return m, nil
	}

	// Otherwise, show and focus it
	if !m.ShowInspector {
		m.ShowInspector = true
		m.updateLayout()
	}

	m.State = StateInspecting

	if m.ColumnStack.Len() == 1 {
		m.Inspector.Focused = true
	} else if top := m.ColumnStack.Top(); top != nil {
		top.SetInspectorFocused(true)
	}

	return m, nil
}

// handleLogout shows the logout confirmation
func (m Model) handleLogout() (tea.Model, tea.Cmd) {
	m.State = StateConfirmLogout
	return m, nil
}

// handlePlaylistModal opens the playlist modal for the selected item
func (m Model) handlePlaylistModal() (tea.Model, tea.Cmd) {
	top := m.ColumnStack.Top()
	if top == nil {
		return m, nil
	}
	item := top.SelectedMediaItem()
	if item != nil && m.PlaylistService != nil {
		return m, LoadPlaylistModalDataCmd(m.PlaylistService, item)
	}
	return m, nil
}

// handleDelete handles deletion of playlists or playlist items
func (m Model) handleDelete() (tea.Model, tea.Cmd) {
	top := m.ColumnStack.Top()
	if top == nil {
		return m, nil
	}
	switch top.ColumnType() {
	case components.ColumnTypePlaylistItems:
		item := top.SelectedMediaItem()
		if item != nil && m.currentPlaylistID != "" {
			return m, RemoveFromPlaylistCmd(m.PlaylistService, m.currentPlaylistID, item.ID)
		}
	case components.ColumnTypePlaylists:
		playlist := top.SelectedPlaylist()
		if playlist != nil {
			return m, DeletePlaylistCmd(m.PlaylistService, playlist.ID)
		}
	default:
		if top.ContentID() == queueLibraryID {
			if item := top.SelectedMediaItem(); item != nil {
				return m, RemoveFromQueueCmd(m.PlaylistService, item.ID)
			}
		} else if rawItem := top.SelectedItem(); rawItem != nil {
			// If it's a media item (Movie, Show, Episode), prompt for local file deletion
			if item, ok := rawItem.(domain.ListItem); ok {
				if item.GetItemType() == "movie" || item.GetItemType() == "show" || item.GetItemType() == "episode" {
					m.pendingDelete = item
					m.State = StateConfirmDelete
				}
			}
		}
	}
	return m, nil
}

// handleDirectPlay handles playing the next up episode for a show
func (m Model) handleDirectPlay() (tea.Model, tea.Cmd) {
	top := m.ColumnStack.Top()
	if top == nil {
		return m, nil
	}

	rawItem := top.SelectedItem()
	item, ok := rawItem.(domain.ListItem)
	if !ok || item.GetItemType() != "show" {
		m.StatusMsg = "Direct Play is only available for TV Shows"
		return m, ClearStatusCmd(3 * time.Second)
	}

	return m.playNextEpisodeForShow(item)
}

func (m Model) playNextEpisodeForShow(show domain.ListItem) (tea.Model, tea.Cmd) {
	m.StatusMsg = "Finding next episode for " + show.GetTitle() + "..."
	return m, DirectPlayShowCmd(m.MediaClient, m.PlaybackSvc, show.GetID(), show.GetLibraryID(), m.UIConfig.Autoplay)
}

// handleNewPlaylist shows hint about creating playlists
func (m Model) handleNewPlaylist() (tea.Model, tea.Cmd) {
	top := m.ColumnStack.Top()
	if top != nil && top.ColumnType() == components.ColumnTypePlaylists {
		m.StatusMsg = "Use Space on an item to create a playlist"
		return m, ClearStatusCmd(3 * time.Second)
	}
	return m, nil
}

func (m Model) handleQueue() (tea.Model, tea.Cmd) {
	top := m.ColumnStack.Top()
	if top == nil {
		return m, nil
	}
	if top.ContentID() == queueLibraryID {
		if item := top.SelectedMediaItem(); item != nil {
			return m, RemoveFromQueueCmd(m.PlaylistService, item.ID)
		}
		return m, nil
	}
	item := top.SelectedMediaItem()
	if item == nil {
		return m, nil
	}
	return m, AddToQueueCmd(m.PlaylistService, item)
}

func (m Model) handleNextEpisode() (tea.Model, tea.Cmd) {
	top := m.ColumnStack.Top()
	if top == nil || top.ColumnType() != components.ColumnTypeEpisodes {
		m.StatusMsg = "Open a season to quick-play next unwatched episode"
		return m, ClearStatusCmd(3 * time.Second)
	}
	count := top.ItemCount()
	for i := top.SelectedIndex(); i < count; i++ {
		top.SetSelectedIndex(i)
		if item := top.SelectedMediaItem(); item != nil && !item.IsPlayed {
			playlist, _ := top.GetSeasonPlaylist()
			return m.playOrConfirmResume(item, playlist)
		}

	}

	m.StatusMsg = "No unwatched episode in this season"
	return m, ClearStatusCmd(3 * time.Second)
}

// ----------------------------------------------------------------------------
// Modal input handlers
// ----------------------------------------------------------------------------

// handleGlobalSearchInput handles input when global search is visible
func (m Model) handleGlobalSearchInput(msg tea.KeyMsg) (Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd
	var selected bool

	m.GlobalSearch, cmd, selected = m.GlobalSearch.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	if m.GlobalSearch.QueryChanged() {
		query := m.GlobalSearch.Query()
		results := m.SearchSvc.FilterLocal(query, m.Libraries)
		m.GlobalSearch.SetResults(results)
		if len(results) == 0 && len(query) >= 2 {
			cmds = append(cmds, RemoteSearchCmd(m.SearchSvc, query))
		}
	}

	if selected {
		if result := m.GlobalSearch.Selected(); result != nil {
			m.GlobalSearch.Hide()
			if navCmd := m.navigateToSearchResult(*result); navCmd != nil {
				cmds = append(cmds, navCmd)
			}
		}
	}
	return m, tea.Batch(cmds...)
}

// handleSortModalInput handles input when sort modal is visible
func (m Model) handleSortModalInput(msg tea.KeyMsg) (bool, Model, tea.Cmd) {
	handled, selection := m.SortModal.HandleKeyMsg(msg)
	if handled {
		if selection != nil {
			if top := m.ColumnStack.Top(); top != nil {
				top.ApplySort(selection.Field, selection.Direction)
				m.updateInspector()
			}
		}
		return true, m, nil
	}
	return false, m, nil
}

// handlePlaylistModalInput handles input when playlist modal is visible
func (m Model) handlePlaylistModalInput(msg tea.KeyMsg) (bool, Model, tea.Cmd) {
	handled, shouldClose, shouldCreate := m.PlaylistModal.HandleKeyMsg(msg)
	if !handled {
		return false, m, nil
	}

	if shouldCreate {
		newModel, cmd := m.applyPlaylistCreate()
		return true, newModel, cmd
	}
	if shouldClose {
		newModel, cmd := m.applyPlaylistChanges()
		return true, newModel, cmd
	}
	return true, m, nil
}

// applyPlaylistCreate creates a new playlist and applies checkbox changes
func (m Model) applyPlaylistCreate() (Model, tea.Cmd) {
	title := m.PlaylistModal.NewPlaylistTitle()
	item := m.PlaylistModal.Item()
	changes := m.PlaylistModal.GetChanges()
	m.PlaylistModal.Hide()

	if title == "" || item == nil {
		return m, nil
	}

	cmds := []tea.Cmd{CreatePlaylistCmd(m.PlaylistService, title, []string{item.ID})}
	for _, change := range changes {
		if change.Add {
			cmds = append(cmds, AddToPlaylistCmd(m.PlaylistService, change.PlaylistID, []string{item.ID}))
		} else {
			cmds = append(cmds, RemoveFromPlaylistCmd(m.PlaylistService, change.PlaylistID, item.ID))
		}
	}
	return m, tea.Batch(cmds...)
}

// applyPlaylistChanges applies pending playlist checkbox changes
func (m Model) applyPlaylistChanges() (Model, tea.Cmd) {
	changes := m.PlaylistModal.GetChanges()
	item := m.PlaylistModal.Item()
	m.PlaylistModal.Hide()

	if len(changes) == 0 || item == nil {
		return m, nil
	}

	var cmds []tea.Cmd
	for _, change := range changes {
		if change.Add {
			cmds = append(cmds, AddToPlaylistCmd(m.PlaylistService, change.PlaylistID, []string{item.ID}))
		} else {
			cmds = append(cmds, RemoveFromPlaylistCmd(m.PlaylistService, change.PlaylistID, item.ID))
		}
	}
	return m, tea.Batch(cmds...)
}

// handleInputModalInput handles input when input modal is visible
func (m Model) handleInputModalInput(msg tea.KeyMsg) (bool, Model, tea.Cmd) {
	var cmd tea.Cmd
	var submitted bool

	m.InputModal, cmd, submitted = m.InputModal.Update(msg)
	if submitted {
		title := m.InputModal.Value()
		m.InputModal.Hide()
		if title != "" {
			return true, m, CreatePlaylistCmd(m.PlaylistService, title, []string{})
		}
		return true, m, nil
	}
	if cmd != nil {
		return true, m, cmd
	}
	return true, m, nil
}

// handleFilterTypingInput handles input when filter typing mode is active
func (m Model) handleFilterTypingInput(msg tea.KeyMsg) (bool, Model, tea.Cmd) {
	top := m.ColumnStack.Top()
	if top == nil {
		return false, m, nil
	}
	oldCursor := top.SelectedIndex()
	newCol, _ := top.Update(msg)
	m.ColumnStack.UpdateTop(newCol)
	if oldCursor != top.SelectedIndex() {
		m.updateInspector()
	}
	return true, m, nil
}
