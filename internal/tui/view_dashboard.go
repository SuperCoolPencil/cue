package tui

import (
	"fmt"

	"github.com/SuperCoolPencil/cue/internal/tui/components"
	"github.com/SuperCoolPencil/cue/internal/tui/styles"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderDashboard() string {
	header := m.renderHeader()
	contentHeight := m.Height - ChromeHeight - lipgloss.Height(header)
	if contentHeight < 20 {
		contentHeight = 20
	}

	leftWidth := (m.Width * 20) / 100
	rightWidth := (m.Width * 25) / 100
	centerWidth := m.Width - leftWidth - rightWidth

	leftCol := m.renderDashboardLeftCol(leftWidth, contentHeight)
	centerCol := m.renderDashboardCenterCol(centerWidth, contentHeight)
	rightCol := m.renderDashboardRightCol(rightWidth, contentHeight)

	content := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, centerCol, rightCol)

	footer := m.renderFooter()

	return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
}

func (m Model) renderDashboardLeftCol(width, height int) string {
	menuHeight := (height * 60) / 100
	shortcutsHeight := height - menuHeight

	// Main Menu
	menuStr := "  " + lipgloss.NewStyle().Foreground(styles.PlexOrange).Render("Dashboard") + "\n\n" +
		"  Libraries         [L]\n\n" +
		"  Now Playing       [P]\n\n" +
		"  Discover          [D]\n\n" +
		"  Users             [U]\n\n" +
		"  Activity          [A]\n\n" +
		"  Playlists         [Y]\n\n" +
		"  Settings          [S]\n\n" +
		"  Plugins           [G]\n\n" +
		"  Help              [H]"
	
	menuBox := components.RenderBtopBox("MAIN MENU", "", menuStr, width, menuHeight, styles.DimGray)

	// Shortcuts
	shortcutsStr := "  Search            [/]\n\n" +
		"  Go to Library     [g]\n\n" +
		"  Refresh           [r]\n\n" +
		"  Global Search     [f]\n\n" +
		"  Command Palette   [:]"

	shortcutsBox := components.RenderBtopBox("SHORTCUTS", "", shortcutsStr, width, shortcutsHeight, styles.DimGray)

	return lipgloss.JoinVertical(lipgloss.Left, menuBox, shortcutsBox)
}

func (m Model) renderDashboardCenterCol(width, height int) string {
	row1Height := height / 3
	row2Height := height / 3
	row3Height := height - row1Height - row2Height

	halfWidth := width / 2
	otherHalfWidth := width - halfWidth

	// Continue Watching
	cwContent := ""
	if m.isPlayingTitle != "" {
		cwContent = lipgloss.NewStyle().Foreground(styles.PlexOrange).Render("\u25b7 "+m.isPlayingTitle) + "\n\n  " + styles.RenderProgressBar(50, width-60)
	} else {
		cwContent = "\n\n  No items in progress"
	}
	cwBox := components.RenderBtopBox("CONTINUE WATCHING", "", cwContent, width, row1Height, styles.Green)

	// Recently Added
	raContent := "\n\n  No recently added items"
	raBox := components.RenderBtopBox("RECENTLY ADDED", "", raContent, halfWidth, row2Height, styles.Blue)

	// Libraries
	libContent := ""
	for i, lib := range m.Libraries {
		if i > 5 {
			break
		}
		count := 0
		if state, ok := m.LibraryStates[lib.ID]; ok {
			count = state.Loaded
		}
		libContent += fmt.Sprintf("\u2022 %-20s %d\n\n", lib.Name, count)
	}
	if len(m.Libraries) == 0 {
		libContent = "\n\n  No libraries loaded"
	}
	libBox := components.RenderBtopBox("LIBRARIES", "", libContent, otherHalfWidth, row2Height, styles.PlexOrange)

	row2 := lipgloss.JoinHorizontal(lipgloss.Top, raBox, libBox)

	// Recent Activity
	actContent := ""
	if m.StatusMsg != "" {
		actContent = "\n\n  \u25cf " + m.StatusMsg
	} else {
		actContent = "\n\n  No recent activity"
	}
	actBox := components.RenderBtopBox("RECENT ACTIVITY", "", actContent, halfWidth, row3Height, styles.PlexOrange)

	// On Deck
	odContent := "\n\n  No items on deck"
	odBox := components.RenderBtopBox("ON DECK", "", odContent, otherHalfWidth, row3Height, styles.Green)

	row3 := lipgloss.JoinHorizontal(lipgloss.Top, actBox, odBox)

	return lipgloss.JoinVertical(lipgloss.Left, cwBox, row2, row3)
}

func (m Model) renderDashboardRightCol(width, height int) string {
	npHeight := (height * 60) / 100
	qaHeight := height - npHeight

	// Now Playing
	npContent := ""
	if m.isPlayingTitle != "" {
		npContent = lipgloss.NewStyle().Foreground(styles.White).Render("\n  Now Playing:\n  ") + lipgloss.NewStyle().Foreground(styles.PlexOrange).Render(m.isPlayingTitle)
	} else {
		npContent = "\n\n  Nothing is currently playing"
	}
	
	npBox := components.RenderBtopBox("NOW PLAYING", "", npContent, width, npHeight, styles.PlexOrange)

	// Quick Actions
	qaContent := "[R] Refresh Libraries\n\n" +
		"[B] Backup Database\n\n" +
		"[C] Clean Bundles\n\n" +
		"[U] Update Libraries\n\n" +
		"[O] Optimize Database\n\n" +
		"[X] View Logs"
	qaBox := components.RenderBtopBox("QUICK ACTIONS", "", qaContent, width, qaHeight, styles.Blue)

	return lipgloss.JoinVertical(lipgloss.Left, npBox, qaBox)
}
