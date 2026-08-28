package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/SuperCoolPencil/cue/internal/config"
	"github.com/SuperCoolPencil/cue/internal/domain"
	"github.com/SuperCoolPencil/cue/internal/search"
	"github.com/SuperCoolPencil/cue/internal/tui/components"
	tea "github.com/charmbracelet/bubbletea"
)

// NavAwaitKind specifies what async load the plan is waiting for
type NavAwaitKind int

const (
	AwaitNone     NavAwaitKind = iota
	AwaitMovies                // AwaitID = LibraryID
	AwaitShows                 // AwaitID = LibraryID
	AwaitMixed                 // AwaitID = LibraryID (mixed content library)
	AwaitSeasons               // AwaitID = ShowID
	AwaitEpisodes              // AwaitID = SeasonID
)

// NavTarget represents a single navigation step
type NavTarget struct {
	ID string // item ID to select (empty = no-op, just land)
}

// NavPlan represents a multi-step navigation flow
type NavPlan struct {
	Targets     []NavTarget
	CurrentStep int
	AwaitKind   NavAwaitKind
	AwaitID     string
}

func (p *NavPlan) IsComplete() bool {
	return p == nil || p.CurrentStep >= len(p.Targets)
}

func (p *NavPlan) Current() *NavTarget {
	if p.IsComplete() {
		return nil
	}
	return &p.Targets[p.CurrentStep]
}

func (p *NavPlan) Advance() {
	if p != nil {
		p.CurrentStep++
	}
}

// drillResult contains the result of drilling into an item
type drillResult struct {
	AwaitKind NavAwaitKind
	AwaitID   string
	Cmd       tea.Cmd
}

// columnLoadSpec contains everything needed to push and load a library column
type columnLoadSpec struct {
	colType   components.ColumnType
	name      string
	awaitKind NavAwaitKind
	awaitID   string
	getCached func() interface{} // Returns nil if not cached, otherwise a slice for SetItems
	loadCmd   tea.Cmd
}

// pushAndLoadColumn pushes a column and either populates from cache or triggers async load.
// This consolidates the repeated cache-check-and-load pattern used throughout navigation.
func (m *Model) pushAndLoadColumn(spec columnLoadSpec, cursor int) *drillResult {
	col := components.NewListColumn(spec.colType, spec.name)
	col.SetShowWatchStatus(m.UIConfig.ShowWatchStatus)
	col.SetContentID(spec.awaitID)
	m.ColumnStack.Push(col, cursor)
	m.updateLayout()

	if cached := spec.getCached(); cached != nil {
		col.SetItems(cached)
		m.updateInspector()
		if m.navPlan != nil {
			return &drillResult{
				AwaitKind: spec.awaitKind,
				AwaitID:   spec.awaitID,
				Cmd:       m.advanceNavPlanAfterLoad(spec.awaitKind, spec.awaitID),
			}
		}
		return &drillResult{AwaitKind: AwaitNone}
	}

	col.SetLoading(true)
	m.Loading = true
	return &drillResult{
		AwaitKind: spec.awaitKind,
		AwaitID:   spec.awaitID,
		Cmd:       spec.loadCmd,
	}
}

// navigateToMixedLibraryItem navigates to an item in a mixed library using NavPlan.
// This consolidates the 3 near-identical mixed library navigation blocks.
func (m *Model) navigateToMixedLibraryItem(lib *domain.Library, targets []NavTarget) tea.Cmd {
	m.navPlan = &NavPlan{
		Targets:     targets,
		CurrentStep: 0,
		AwaitKind:   AwaitMixed,
		AwaitID:     lib.ID,
	}

	mixedCol := components.NewListColumn(components.ColumnTypeMixed, lib.Name)
	mixedCol.SetShowWatchStatus(m.UIConfig.ShowWatchStatus)
	mixedCol.SetContentID(lib.ID)

	if cached, ok := m.Store.GetMixedContent(lib.ID); ok {
		mixedCol.SetItems(cached)
		m.ColumnStack.Push(mixedCol, 0)
		m.updateLayout()
		m.currentLibID = lib.ID // Track context for hierarchical caching
		return m.advanceNavPlanAfterLoad(AwaitMixed, lib.ID)
	}

	mixedCol.SetLoading(true)
	m.ColumnStack.Push(mixedCol, 0)
	m.Loading = true
	m.updateLayout()
	return LoadMixedLibraryCmd(m.LibraryService, *lib)
}

// NavigationContext contains information needed to navigate to an item
// This is purely a TUI concern - the service layer provides FilterItem with LibraryID,
// and the TUI decides how to navigate based on that.
type NavigationContext struct {
	LibraryID   string
	LibraryName string
	MovieID     string
	ShowID      string
	ShowTitle   string
	SeasonID    string
	EpisodeID   string
}

// buildNavContext constructs navigation context from a filter result
func (m *Model) buildNavContext(item search.FilterItem) NavigationContext {
	lib := m.findLibrary(item.LibraryID)
	libName := ""
	if lib != nil {
		libName = lib.Name
	}

	ctx := NavigationContext{
		LibraryID:   item.LibraryID,
		LibraryName: libName,
	}

	switch item.Type {
	case domain.MediaTypeMovie:
		ctx.MovieID = item.Item.GetID()
	case domain.MediaTypeShow:
		ctx.ShowID = item.Item.GetID()
		if show, ok := item.Item.(*domain.Show); ok {
			ctx.ShowTitle = show.Title
		}
	}

	return ctx
}

// clearNavPlan clears the current navigation plan
func (m *Model) clearNavPlan() {
	m.navPlan = nil
}

// drillSelected pushes a new column for the selected item and returns await info
func (m *Model) drillSelected() *drillResult {
	top := m.ColumnStack.Top()
	if top == nil || !top.CanDrillInto() {
		return nil
	}
	item := top.SelectedItem()
	if item == nil {
		return nil
	}
	cursor := top.SelectedIndex()

	switch v := item.(type) {
	case domain.Library:
		if result := m.drillVirtualLibrary(v, cursor); result != nil {
			return result
		}
		// Handle synthetic "Playlists" entry
		if v.ID == playlistsLibraryID {
			col := components.NewListColumn(components.ColumnTypePlaylists, "Playlists")
			col.SetShowWatchStatus(m.UIConfig.ShowWatchStatus)
			col.SetContentID(playlistsLibraryID)
			m.ColumnStack.Push(col, cursor)
			m.updateLayout()

			// Check cache first
			if cached, ok := m.Store.GetPlaylists(); ok {
				col.SetItems(cached)
				m.updateInspector()
				return &drillResult{AwaitKind: AwaitNone}
			}

			col.SetLoading(true)
			m.Loading = true
			return &drillResult{
				AwaitKind: AwaitNone,
				Cmd:       LoadPlaylistsCmd(m.PlaylistService),
			}
		}

		// Track library context for hierarchical caching
		m.currentLibID = v.ID
		m.currentShowID = "" // Reset show context when entering a library

		// Build column spec based on library type
		var spec columnLoadSpec
		switch v.Type {
		case "movie":
			spec = columnLoadSpec{
				colType:   components.ColumnTypeMovies,
				name:      v.Name,
				awaitKind: AwaitMovies,
				awaitID:   v.ID,
				getCached: func() interface{} {
					if c, ok := m.Store.GetMovies(v.ID); ok {
						return c
					}
					return nil
				},
				loadCmd: LoadMoviesCmd(m.LibraryService, v),
			}
		case "show":
			spec = columnLoadSpec{
				colType:   components.ColumnTypeShows,
				name:      v.Name,
				awaitKind: AwaitShows,
				awaitID:   v.ID,
				getCached: func() interface{} {
					if c, ok := m.Store.GetShows(v.ID); ok {
						return c
					}
					return nil
				},
				loadCmd: LoadShowsCmd(m.LibraryService, v),
			}
		case "mixed":
			spec = columnLoadSpec{
				colType:   components.ColumnTypeMixed,
				name:      v.Name,
				awaitKind: AwaitMixed,
				awaitID:   v.ID,
				getCached: func() interface{} {
					if c, ok := m.Store.GetMixedContent(v.ID); ok {
						return c
					}
					return nil
				},
				loadCmd: LoadMixedLibraryCmd(m.LibraryService, v),
			}
		default:
			// Unknown library type - treat as mixed
			spec = columnLoadSpec{
				colType:   components.ColumnTypeMixed,
				name:      v.Name,
				awaitKind: AwaitMixed,
				awaitID:   v.ID,
				getCached: func() interface{} {
					if c, ok := m.Store.GetMixedContent(v.ID); ok {
						return c
					}
					return nil
				},
				loadCmd: LoadMixedLibraryCmd(m.LibraryService, v),
			}
		}
		return m.pushAndLoadColumn(spec, cursor)

	case *domain.Show:
		// Track show context for hierarchical caching (episodes need showID)
		m.currentShowID = v.ID

		libID := m.currentLibID
		showID := v.ID

		// Push a collapsible season+episode column directly (Tab 3 for TV shows).
		// Seasons are fetched first; episodes are loaded lazily per season.
		col := components.NewListColumn(components.ColumnTypeSeasonEpisodes, v.Title)
		col.SetShowWatchStatus(m.UIConfig.ShowWatchStatus)
		col.SetContentID(v.ID) // contentID = showID so SeasonsLoadedMsg can validate
		col.SetLoading(true)
		m.ColumnStack.Push(col, cursor)
		m.Loading = true
		m.updateLayout()
		return &drillResult{
			AwaitKind: AwaitSeasons,
			AwaitID:   showID,
			Cmd:       LoadSeasonsCmd(m.LibraryService, libID, showID),
		}

	case *domain.Season:
		title := v.ShowTitle
		if v.SeasonNum == 0 {
			title += " - Specials"
		} else {
			title += fmt.Sprintf(" - S%02d", v.SeasonNum)
		}

		libID := m.currentLibID
		showID := m.currentShowID
		seasonID := v.ID
		spec := columnLoadSpec{
			colType:   components.ColumnTypeEpisodes,
			name:      title,
			awaitKind: AwaitEpisodes,
			awaitID:   v.ID,
			getCached: func() interface{} {
				if c, ok := m.Store.GetEpisodes(libID, showID, seasonID); ok {
					return c
				}
				return nil
			},
			loadCmd: LoadEpisodesCmd(m.LibraryService, libID, showID, seasonID),
		}
		return m.pushAndLoadColumn(spec, cursor)

	case *domain.Playlist:
		col := components.NewListColumn(components.ColumnTypePlaylistItems, v.Title)
		col.SetShowWatchStatus(m.UIConfig.ShowWatchStatus)
		col.SetContentID(v.ID)
		m.ColumnStack.Push(col, cursor)
		m.currentPlaylistID = v.ID
		m.updateLayout()

		// Check cache first
		if cached, ok := m.Store.GetPlaylistItems(v.ID); ok {
			col.SetItems(cached)
			m.updateInspector()
			return &drillResult{AwaitKind: AwaitNone}
		}

		col.SetLoading(true)
		m.Loading = true
		return &drillResult{
			AwaitKind: AwaitNone, // Playlists don't use the NavPlan system
			AwaitID:   v.ID,
			Cmd:       LoadPlaylistItemsCmd(m.PlaylistService, v.ID),
		}
	}
	return nil
}

func (m *Model) drillVirtualLibrary(v domain.Library, cursor int) *drillResult {
	var items interface{}
	title := v.Name
	contentID := v.ID

	switch v.ID {
	case continueLibraryID:
		col := components.NewListColumn(components.ColumnTypeMixed, title)
		col.SetShowWatchStatus(m.UIConfig.ShowWatchStatus)
		col.SetShowShowTitle(true)
		col.SetContentID(contentID)
		col.SetLoading(true)
		m.ColumnStack.Push(col, cursor)
		m.Loading = true
		m.updateLayout()
		return &drillResult{
			AwaitKind: AwaitNone,
			Cmd:       LoadContinueWatchingCmd(m.LibraryService),
		}
	case recentLibraryID:
		items = m.LibraryService.RecentlyAdded(0)
	case queueLibraryID:
		items = m.PlaylistService.QueueItems()
	case filtersLibraryID:
		items = smartFilterEntries()
	case profilesLibraryID:
		items = m.profileEntries()
	case configLibraryID:
		items = m.configEntries()
	case cacheLibraryID:
		items = m.cacheEntries()
	case "__config_watch__":
		m.UIConfig.ShowWatchStatus = !m.UIConfig.ShowWatchStatus
		if m.AppConfig != nil {
			m.AppConfig.UI.ShowWatchStatus = m.UIConfig.ShowWatchStatus
			if err := config.SaveConfig(m.AppConfig); err != nil {
				m.StatusMsg = fmt.Sprintf("Failed to save config: %v", err)
				m.StatusIsErr = true
				return &drillResult{AwaitKind: AwaitNone}
			}
		}
		if top := m.ColumnStack.Top(); top != nil {
			top.SetItems(m.configEntries())
		}
		m.StatusMsg = "Saved watch indicator setting"
		return &drillResult{AwaitKind: AwaitNone}
	case "__config_counts__":
		m.UIConfig.ShowLibraryCounts = !m.UIConfig.ShowLibraryCounts
		if m.AppConfig != nil {
			m.AppConfig.UI.ShowLibraryCounts = m.UIConfig.ShowLibraryCounts
			if err := config.SaveConfig(m.AppConfig); err != nil {
				m.StatusMsg = fmt.Sprintf("Failed to save config: %v", err)
				m.StatusIsErr = true
				return &drillResult{AwaitKind: AwaitNone}
			}
		}
		if top := m.ColumnStack.Top(); top != nil {
			top.SetItems(m.configEntries())
		}
		m.StatusMsg = "Saved library count setting"
		return &drillResult{AwaitKind: AwaitNone}
	case "__config_hide_watched__":
		m.UIConfig.HideWatched = !m.UIConfig.HideWatched
		if m.AppConfig != nil {
			m.AppConfig.UI.HideWatched = m.UIConfig.HideWatched
			if err := config.SaveConfig(m.AppConfig); err != nil {
				m.StatusMsg = fmt.Sprintf("Failed to save config: %v", err)
				m.StatusIsErr = true
				return &drillResult{AwaitKind: AwaitNone}
			}
		}
		// Apply to ALL columns in the stack
		for i := 0; i < m.ColumnStack.Len(); i++ {
			if col := m.ColumnStack.Get(i); col != nil {
				col.SetHideWatched(m.UIConfig.HideWatched)
			}
		}
		if top := m.ColumnStack.Top(); top != nil {
			top.SetItems(m.configEntries())
		}
		m.StatusMsg = "Saved hide watched setting"
		return &drillResult{AwaitKind: AwaitNone}
	case "__config_autoplay__":
		m.UIConfig.Autoplay = !m.UIConfig.Autoplay
		if m.AppConfig != nil {
			m.AppConfig.UI.Autoplay = m.UIConfig.Autoplay
			if err := config.SaveConfig(m.AppConfig); err != nil {
				m.StatusMsg = fmt.Sprintf("Failed to save config: %v", err)
				m.StatusIsErr = true
				return &drillResult{AwaitKind: AwaitNone}
			}
		}
		if top := m.ColumnStack.Top(); top != nil {
			top.SetItems(m.configEntries())
		}
		m.StatusMsg = "Saved autoplay setting"
		return &drillResult{AwaitKind: AwaitNone}
	case "__config_play_next_on_select__":
		m.UIConfig.PlayNextOnSelect = !m.UIConfig.PlayNextOnSelect
		if m.AppConfig != nil {
			m.AppConfig.UI.PlayNextOnSelect = m.UIConfig.PlayNextOnSelect
			if err := config.SaveConfig(m.AppConfig); err != nil {
				m.StatusMsg = fmt.Sprintf("Failed to save config: %v", err)
				m.StatusIsErr = true
				return &drillResult{AwaitKind: AwaitNone}
			}
		}
		if top := m.ColumnStack.Top(); top != nil {
			top.SetItems(m.configEntries())
		}
		m.StatusMsg = "Saved play-next-on-select setting"
		return &drillResult{AwaitKind: AwaitNone}
	case "__cache_clear__":
		m.LibraryService.InvalidateAll()
		m.PlaylistService.ClearQueue()
		m.PlaylistService.InvalidatePlaylists()
		if err := config.ClearCache(); err != nil {
			m.StatusMsg = fmt.Sprintf("Failed to clear cache: %v", err)
			m.StatusIsErr = true
			return &drillResult{AwaitKind: AwaitNone}
		}
		if top := m.ColumnStack.Top(); top != nil {
			top.SetItems(m.cacheEntries())
		}
		m.StatusMsg = "Cache cleared"
		return &drillResult{AwaitKind: AwaitNone}
	default:
		// __profile_current__ is display-only — no action on drill
		if v.ID == "__profile_current__" {
			return &drillResult{AwaitKind: AwaitNone}
		}
		if strings.HasPrefix(v.ID, "__profile_") {
			name := strings.TrimPrefix(v.ID, "__profile_")
			if m.AppConfig != nil {
				m.AppConfig.CurrentProfile = name
				m.AppConfig.ApplyProfileForUI(name)
				if err := config.SaveConfig(m.AppConfig); err != nil {
					m.StatusMsg = fmt.Sprintf("Failed to save profile: %v", err)
					m.StatusIsErr = true
					return &drillResult{AwaitKind: AwaitNone}
				}
			}
			m.StatusMsg = "Switched profile. Restart Cue to reconnect."
			if top := m.ColumnStack.Top(); top != nil {
				top.SetItems(m.profileEntries())
			}
			return &drillResult{AwaitKind: AwaitNone}
		}
		// Config/cache read-only entries have no drill action
		if v.ID == "__config_player__" || v.ID == "__config_os__" ||
			v.ID == "__cache_libraries__" || v.ID == "__cache_queue__" || v.ID == "__cache_refresh__" {
			return &drillResult{AwaitKind: AwaitNone}
		}
		if key := filterKey(v.ID); key != "" {
			items = m.LibraryService.SmartFiltered(key, 0)
			contentID = v.ID
		} else {
			// If it's a real library (not a virtual/cue/profile entry), return nil
			// so the caller (drillSelected) can handle it as a standard media library.
			if v.Type != "cue" && v.Type != "profile" && !strings.HasPrefix(v.ID, "__") {
				return nil
			}
			// Unknown or no-op virtual ID — do nothing rather than sending a bad API request
			return &drillResult{AwaitKind: AwaitNone}
		}
	}

	colType := components.ColumnTypeMixed
	if _, ok := items.([]domain.Library); ok {
		colType = components.ColumnTypeLibraries
	}
	col := components.NewListColumn(colType, title)
	col.SetShowWatchStatus(m.UIConfig.ShowWatchStatus)
	col.SetContentID(contentID)
	col.SetItems(items)
	m.ColumnStack.Push(col, cursor)
	m.updateLayout()
	m.updateInspector()
	return &drillResult{AwaitKind: AwaitNone}
}

// drillIntoSelection pushes a new column for the selected item
func (m Model) drillIntoSelection() (tea.Model, tea.Cmd) {
	result := m.drillSelected()
	if result == nil {
		return m, nil
	}
	return m, result.Cmd
}

// handleBack handles navigation back (h/backspace)
func (m Model) handleBack() (tea.Model, tea.Cmd) {
	// Manual navigation cancels any pending search-navigation plan
	m.clearNavPlan()
	if !m.ColumnStack.CanGoBack() {
		return m, nil
	}

	// Check if we're leaving playlist items view
	if top := m.ColumnStack.Top(); top != nil && top.ColumnType() == components.ColumnTypePlaylistItems {
		m.currentPlaylistID = ""
	}

	// Track context when navigating back for hierarchical caching
	if top := m.ColumnStack.Top(); top != nil {
		switch top.ColumnType() {
		case components.ColumnTypeEpisodes:
			// Leaving episodes - clear nothing (still in show context)
		case components.ColumnTypeSeasons:
			// Leaving seasons - clear show context
			m.currentShowID = ""
		case components.ColumnTypeMovies, components.ColumnTypeShows, components.ColumnTypeMixed:
			// Leaving library content - clear both contexts
			m.currentLibID = ""
			m.currentShowID = ""
		}
	}

	_, savedCursor := m.ColumnStack.Pop()

	// Restore cursor position on the new top
	if top := m.ColumnStack.Top(); top != nil {
		top.SetSelectedIndex(savedCursor)
	}

	m.updateLayout()
	m.updateInspector()
	return m, nil
}

// advanceNavPlanAfterLoad advances the navigation plan after an async load completes
func (m *Model) advanceNavPlanAfterLoad(kind NavAwaitKind, id string) tea.Cmd {
	p := m.navPlan
	if p == nil || p.IsComplete() {
		m.navPlan = nil
		return nil
	}
	// Only advance if this is the awaited load
	if p.AwaitKind != kind || p.AwaitID != id {
		return nil
	}

	top := m.ColumnStack.Top()
	if top == nil {
		m.clearNavPlan()
		return nil
	}

	target := p.Current()
	if target == nil {
		m.clearNavPlan()
		return nil
	}

	// Apply ID selection if requested
	if target.ID != "" {
		if !top.SetSelectedByID(target.ID) {
			m.clearNavPlan()
			m.StatusMsg = "Item not found (library may have changed)"
			m.StatusIsErr = true
			return ClearStatusCmd(5 * time.Second)
		}
	}

	p.Advance()

	if p.IsComplete() {
		m.clearNavPlan()
		m.updateInspector()
		return nil
	}

	// More steps: drill to next level
	result := m.drillSelected()
	if result == nil {
		m.clearNavPlan()
		m.StatusMsg = "Navigation failed"
		m.StatusIsErr = true
		return ClearStatusCmd(5 * time.Second)
	}
	// Update navPlan with await info for next load
	m.navPlan.AwaitKind = result.AwaitKind
	m.navPlan.AwaitID = result.AwaitID
	return result.Cmd
}

// navigateToSearchResult navigates to a search result item in its library context.
// Called when a user selects an item from global search results in the omnibar.
func (m *Model) navigateToSearchResult(item search.FilterItem) tea.Cmd {
	navCtx := m.buildNavContext(item)

	// Reset stack to library level first
	libCol := components.NewLibraryColumn(m.allLibraryEntries())
	libCol.SetLibraryStates(m.LibraryStates)
	libCol.SetShowWatchStatus(m.UIConfig.ShowWatchStatus)
	libCol.SetShowLibraryCounts(m.UIConfig.ShowLibraryCounts)
	m.Inspector.SetLibraryStates(m.LibraryStates)

	// Find and select the library
	virtualOffset := len(virtualLibraryEntries())
	for i, lib := range m.Libraries {
		if lib.ID == navCtx.LibraryID {
			libCol.SetSelectedIndex(i + virtualOffset)
			break
		}
	}
	m.ColumnStack.Reset(libCol)

	lib := m.findLibrary(navCtx.LibraryID)
	if lib == nil {
		return nil
	}

	// Build navigation targets based on media type
	var targets []NavTarget
	switch item.Type {
	case domain.MediaTypeMovie:
		targets = []NavTarget{{ID: navCtx.MovieID}}
	case domain.MediaTypeShow:
		show, ok := item.Item.(*domain.Show)
		if !ok {
			return nil
		}
		targets = []NavTarget{{ID: show.ID}, {}} // Select show, land on seasons
	case domain.MediaTypeEpisode:
		targets = []NavTarget{
			{ID: navCtx.ShowID},
			{ID: navCtx.SeasonID},
			{ID: navCtx.EpisodeID},
		}
	default:
		return nil
	}

	// Mixed libraries use their own navigation path
	if lib.Type == "mixed" {
		return m.navigateToMixedLibraryItem(lib, targets)
	}

	// Typed libraries (movie/show)
	return m.navigateToTypedLibraryItem(lib, navCtx, targets, item.Type)
}

// navigateToTypedLibraryItem navigates to an item in a typed (movie/show) library.
func (m *Model) navigateToTypedLibraryItem(lib *domain.Library, navCtx NavigationContext, targets []NavTarget, mediaType domain.MediaType) tea.Cmd {
	// Track library context for hierarchical caching
	m.currentLibID = lib.ID
	m.currentShowID = "" // Reset show context

	var spec columnLoadSpec

	if mediaType == domain.MediaTypeMovie {
		m.navPlan = &NavPlan{
			Targets:     targets,
			CurrentStep: 0,
			AwaitKind:   AwaitMovies,
			AwaitID:     lib.ID,
		}
		spec = columnLoadSpec{
			colType:   components.ColumnTypeMovies,
			name:      lib.Name,
			awaitKind: AwaitMovies,
			awaitID:   lib.ID,
			getCached: func() interface{} {
				if c, ok := m.Store.GetMovies(lib.ID); ok {
					return c
				}
				return nil
			},
			loadCmd: LoadMoviesCmd(m.LibraryService, *lib),
		}
	} else {
		// Shows and episodes both start from the shows column
		m.navPlan = &NavPlan{
			Targets:     targets,
			CurrentStep: 0,
			AwaitKind:   AwaitShows,
			AwaitID:     lib.ID,
		}
		spec = columnLoadSpec{
			colType:   components.ColumnTypeShows,
			name:      lib.Name,
			awaitKind: AwaitShows,
			awaitID:   lib.ID,
			getCached: func() interface{} {
				if c, ok := m.Store.GetShows(lib.ID); ok {
					return c
				}
				return nil
			},
			loadCmd: LoadShowsCmd(m.LibraryService, *lib),
		}
	}

	return m.pushAndLoadColumn(spec, 0).Cmd
}
