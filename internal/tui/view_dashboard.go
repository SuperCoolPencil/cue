package tui

import (
	"fmt"

	"github.com/SuperCoolPencil/cue/internal/tui/components"
	"github.com/SuperCoolPencil/cue/internal/tui/styles"
	"github.com/charmbracelet/bubbles/table"
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

func createDashboardTable(width, height int, columns []table.Column, rows []table.Row, cursor int) string {
	// Truncate rows to prevent overflow
	for i := range rows {
		for j := range rows[i] {
			if lipgloss.Width(rows[i][j]) > columns[j].Width {
				rows[i][j] = styles.Truncate(rows[i][j], columns[j].Width)
			}
		}
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithHeight(height-3),
	)
	s := table.DefaultStyles()
	s.Header = lipgloss.NewStyle().Height(0).Margin(0).Padding(0)
	s.Cell = lipgloss.NewStyle().PaddingLeft(2)
	
	if cursor >= 0 {
		s.Selected = lipgloss.NewStyle().Foreground(styles.PlexOrange).PaddingLeft(2).Bold(true)
		t.SetCursor(cursor)
	} else {
		s.Selected = s.Cell
		t.SetStyles(s)
	}
	t.SetStyles(s)
	
	return t.View()
}

func (m Model) renderDashboardLeftCol(width, height int) string {
	menuHeight := (height * 60) / 100
	shortcutsHeight := height - menuHeight

	// Main Menu
	menuCols := []table.Column{{Title: "", Width: width - 8}, {Title: "", Width: 4}}
	menuRows := []table.Row{
		{"Dashboard", ""},
		{"Libraries", "[L]"},
		{"Now Playing", "[P]"},
		{"Discover", "[D]"},
		{"Users", "[U]"},
		{"Activity", "[A]"},
		{"Playlists", "[Y]"},
		{"Settings", "[S]"},
		{"Plugins", "[G]"},
		{"Help", "[H]"},
	}
	
	// Determine if the menu is focused
	cursor := -1
	borderColor := styles.DimGray
	if m.State == StateDashboard {
		cursor = m.SelectedMenuIdx
		borderColor = styles.PlexOrange
	}
	
	menuStr := createDashboardTable(width, menuHeight, menuCols, menuRows, cursor)
	menuBox := components.RenderBtopBox("MAIN MENU", "", menuStr, width, menuHeight, borderColor)

	// Shortcuts
	scCols := []table.Column{{Title: "", Width: width - 8}, {Title: "", Width: 4}}
	scRows := []table.Row{
		{"Search", "[/]"},
		{"Go to Library", "[g]"},
		{"Refresh", "[r]"},
		{"Global Search", "[f]"},
		{"Command Palette", "[:]"},
	}
	shortcutsStr := createDashboardTable(width, shortcutsHeight, scCols, scRows, -1)
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
		cwContent = lipgloss.NewStyle().Foreground(styles.PlexOrange).Render("\u25b7 "+styles.Truncate(m.isPlayingTitle, width-10)) + "\n\n  " + styles.RenderProgressBar(50, width-60)
	} else {
		cwContent = "\n\n  No items in progress"
	}
	cwBox := components.RenderBtopBox("CONTINUE WATCHING", "", cwContent, width, row1Height, styles.Green)

	// Recently Added
	raContent := "\n\n  No recently added items"
	raBox := components.RenderBtopBox("RECENTLY ADDED", "", raContent, halfWidth, row2Height, styles.Blue)

	// Libraries
	libCols := []table.Column{{Title: "", Width: otherHalfWidth - 8}, {Title: "", Width: 4}}
	var libRows []table.Row
	for i, lib := range m.Libraries {
		if i > 5 {
			break
		}
		count := 0
		if state, ok := m.LibraryStates[lib.ID]; ok {
			count = state.Loaded
		}
		libRows = append(libRows, table.Row{"\u2022 " + lib.Name, fmt.Sprintf("%d", count)})
	}
	libContent := ""
	if len(libRows) > 0 {
		libContent = createDashboardTable(otherHalfWidth, row2Height, libCols, libRows, -1)
	} else {
		libContent = "\n\n  No libraries loaded"
	}
	libBox := components.RenderBtopBox("LIBRARIES", "", libContent, otherHalfWidth, row2Height, styles.PlexOrange)

	row2 := lipgloss.JoinHorizontal(lipgloss.Top, raBox, libBox)

	// Recent Activity
	actContent := ""
	if m.StatusMsg != "" {
		actContent = "\n\n  \u25cf " + styles.Truncate(m.StatusMsg, halfWidth-10)
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
		npContent = lipgloss.NewStyle().Foreground(styles.White).Render("\n  Now Playing:\n  ") + lipgloss.NewStyle().Foreground(styles.PlexOrange).Render(styles.Truncate(m.isPlayingTitle, width-6))
	} else {
		npContent = "\n\n  Nothing is currently playing"
	}
	
	npBox := components.RenderBtopBox("NOW PLAYING", "", npContent, width, npHeight, styles.PlexOrange)

	// Quick Actions
	qaCols := []table.Column{{Title: "", Width: width - 4}}
	qaRows := []table.Row{
		{"[R] Refresh Libraries"},
		{"[B] Backup Database"},
		{"[C] Clean Bundles"},
		{"[U] Update Libraries"},
		{"[O] Optimize Database"},
		{"[X] View Logs"},
	}
	qaContent := createDashboardTable(width, qaHeight, qaCols, qaRows, -1)
	qaBox := components.RenderBtopBox("QUICK ACTIONS", "", qaContent, width, qaHeight, styles.Blue)

	return lipgloss.JoinVertical(lipgloss.Left, npBox, qaBox)
}
