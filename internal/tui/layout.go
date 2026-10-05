package tui

import "github.com/SuperCoolPencil/cue/internal/tui/components"

// columnLayout holds calculated column widths for the View
type columnLayout struct {
	grandparentWidth int // 0 if not shown
	parentWidth      int // 0 if not shown
	activeWidth      int
	inspectorWidth   int // 0 if not shown
}

// calculateColumnLayout computes column widths based on stack depth and inspector visibility
func (m Model) calculateColumnLayout(availableWidth int) columnLayout {
	stackLen := m.ColumnStack.Len()
	layout := columnLayout{}

	// Helper to apply minimum width
	applyMin := func(width int) int {
		return max(width, MinColumnWidth)
	}

	switch stackLen {
	case 1:
		// Root level: single column (Libraries)
		// We can still show the horizontal inspector here if desired,
		// as there's no vertical split for the library list itself.
		if m.ShowInspector {
			layout.activeWidth = applyMin(availableWidth * RootColumnPercent / 100)
			layout.inspectorWidth = availableWidth - layout.activeWidth
		} else {
			layout.activeWidth = availableWidth
		}

	case 2:
		// 2 columns: [Library (Full) | Content (Split)]
		// No horizontal inspector — room is used for the two columns.
		layout.parentWidth = applyMin(availableWidth * 30 / 100)
		layout.activeWidth = availableWidth - layout.parentWidth

	default:
		// 3+ columns: [Library (Full) | Parent (Split/Full) | Active (Split)]
		// Proportions: 25% / 35% / 40%
		layout.grandparentWidth = applyMin(availableWidth * 25 / 100)
		layout.parentWidth = applyMin(availableWidth * 35 / 100)
		layout.activeWidth = availableWidth - layout.grandparentWidth - layout.parentWidth
	}

	return layout
}

// updateLayout updates component sizes based on window size
func (m *Model) updateLayout() {
	if m.Width == 0 || m.Height == 0 {
		return
	}

	contentHeight := m.Height - ChromeHeight
	m.GlobalSearch.SetSize(m.Width, m.Height)

	stackLen := m.ColumnStack.Len()
	if stackLen == 0 {
		return
	}

	// Calculate layout using shared logic
	layout := m.calculateColumnLayout(m.Width)
	topIdx := stackLen - 1

	// Calculate list/info heights for split view: 33% list, 66% info
	listHeight := contentHeight / 3
	if listHeight < 4 {
		listHeight = 4
	}

	// Apply calculated sizes to components
	switch stackLen {
	case 1:
		m.ColumnStack.Get(0).SetSize(layout.activeWidth, contentHeight)
		if m.ShowInspector {
			m.Inspector.SetSize(layout.inspectorWidth, contentHeight)
		}

	case 2:
		m.ColumnStack.Get(0).SetSize(layout.parentWidth, contentHeight)
		contentCol := m.ColumnStack.Get(1)
		h := listHeight
		if contentCol.ColumnType() == components.ColumnTypeEpisodes || contentCol.ColumnType() == components.ColumnTypeSeasonEpisodes {
			h = (55 * contentHeight) / 100
		}
		if contentCol.ContentID() == configLibraryID {
			h = contentHeight - configInfoHeight
		}
		// Active column is split in View(), so updateLayout should use appropriate height
		m.ColumnStack.Get(1).SetSize(layout.activeWidth, h)

	default: // 3+ columns
		if layout.grandparentWidth > 0 {
			m.ColumnStack.Get(topIdx-2).SetSize(layout.grandparentWidth, contentHeight)
		}

		parentCol := m.ColumnStack.Get(topIdx - 1)
		activeCol := m.ColumnStack.Get(topIdx)

		// Parent column split vs full depends on View() logic
		ph := listHeight
		if parentCol.ColumnType() == components.ColumnTypeEpisodes || parentCol.ColumnType() == components.ColumnTypeSeasonEpisodes {
			ph = (55 * contentHeight) / 100
		}

		if layout.grandparentWidth > 0 {
			m.ColumnStack.Get(topIdx-1).SetSize(layout.parentWidth, contentHeight)
		} else {
			m.ColumnStack.Get(topIdx-1).SetSize(layout.parentWidth, ph)
		}

		ah := listHeight
		if activeCol.ColumnType() == components.ColumnTypeEpisodes || activeCol.ColumnType() == components.ColumnTypeSeasonEpisodes {
			ah = (55 * contentHeight) / 100
		}
		if activeCol.ContentID() == configLibraryID {
			ah = contentHeight - configInfoHeight
		}
		m.ColumnStack.Get(topIdx).SetSize(layout.activeWidth, ah)
	}
}
