package tui

import (
	"fmt"
	"strings"

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

// createListLayout creates a structured 2-column layout without manual spacing
func createListLayout(width, height int, col1, col2 []string, col2Style lipgloss.Style, cursor int) string {
	var lines []string
	
	activeStyle := lipgloss.NewStyle().Foreground(styles.PlexOrange).Bold(true)
	
	for i := 0; i < len(col1); i++ {
		if i >= height {
			break
		}
		
		c1 := col1[i]
		c2 := ""
		if i < len(col2) {
			c2 = col2[i]
		}
		
		// Style active row if cursor matches
		if cursor == i {
			c1 = activeStyle.Render(c1)
			c2 = activeStyle.Render(c2)
		} else {
			c2 = col2Style.Render(c2)
		}
		
		// Calculate padding needed between col1 and col2
		w1 := lipgloss.Width(c1)
		w2 := lipgloss.Width(c2)
		
		padWidth := width - w1 - w2 - 2 // -2 for margins
		if padWidth < 1 {
			padWidth = 1
		}
		
		line := " " + c1 + styles.Pad("", padWidth) + c2 + " "
		lines = append(lines, line)
	}
	
	return strings.Join(lines, "\n")
}

func (m Model) renderDashboardLeftCol(width, height int) string {
	menuHeight := (height * 60) / 100
	shortcutsHeight := height - menuHeight

	// Main Menu
	menuCol1 := []string{
		"Dashboard", "Libraries", "Now Playing", "Discover", "Users", 
		"Activity", "Playlists", "Settings", "Plugins", "Help",
	}
	menuCol2 := []string{
		"", "[L]", "[P]", "[D]", "[U]", 
		"[A]", "[Y]", "[S]", "[G]", "[H]",
	}
	
	cursor := -1
	borderColor := styles.DimGray
	if m.State == StateDashboard {
		cursor = m.SelectedMenuIdx
		borderColor = styles.PlexOrange
	}
	
	keyStyle := lipgloss.NewStyle().Foreground(styles.DimGray)
	menuStr := createListLayout(width-2, menuHeight-2, menuCol1, menuCol2, keyStyle, cursor)
	menuBox := components.RenderBtopBox(" Main Menu ", "", menuStr, width, menuHeight, borderColor)

	// Shortcuts
	scCol1 := []string{
		"Search", "Go to Library", "Refresh", "Global Search", "Command Palette",
	}
	scCol2 := []string{
		"[/]", "[g]", "[r]", "[f]", "[:]",
	}
	
	shortcutsStr := createListLayout(width-2, shortcutsHeight-2, scCol1, scCol2, keyStyle, -1)
	shortcutsBox := components.RenderBtopBox(" Shortcuts ", "", shortcutsStr, width, shortcutsHeight, styles.DimGray)

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
		// TODO: Fetch in-progress media items from playback service
		cwContent = "\n\n  No items in progress"
	}
	cwBox := components.RenderBtopBox(" Continue Watching ", "", cwContent, width, row1Height, styles.Green)

	// Recently Added
	// TODO: Fetch recently added items from library service
	raContent := "\n\n  No recently added items"
	raBox := components.RenderBtopBox(" Recently Added ", "", raContent, halfWidth, row2Height, styles.Blue)

	// Libraries
	var libNames []string
	var libCounts []string
	for i, lib := range m.Libraries {
		if i > 5 {
			break
		}
		count := 0
		if state, ok := m.LibraryStates[lib.ID]; ok {
			count = state.Loaded
		}
		libNames = append(libNames, "• "+lib.Name)
		libCounts = append(libCounts, fmt.Sprintf("%d", count))
	}
	libContent := ""
	if len(libNames) > 0 {
		countStyle := lipgloss.NewStyle().Foreground(styles.DimGray)
		libContent = createListLayout(otherHalfWidth-2, row2Height-2, libNames, libCounts, countStyle, -1)
	} else {
		libContent = "\n\n  No libraries loaded"
	}
	libBox := components.RenderBtopBox(" Libraries ", "", libContent, otherHalfWidth, row2Height, styles.PlexOrange)

	row2 := lipgloss.JoinHorizontal(lipgloss.Top, raBox, libBox)

	// Recent Activity
	actContent := ""
	if m.StatusMsg != "" {
		actContent = "\n\n  \u25cf " + styles.Truncate(m.StatusMsg, halfWidth-10)
	} else {
		// TODO: Fetch server activity logs or user events
		actContent = "\n\n  No recent activity"
	}
	actBox := components.RenderBtopBox(" Recent Activity ", "", actContent, halfWidth, row3Height, styles.PlexOrange)

	// On Deck
	// TODO: Fetch next-up episodes for currently watching series
	odContent := "\n\n  No items on deck"
	odBox := components.RenderBtopBox(" On Deck ", "", odContent, otherHalfWidth, row3Height, styles.Green)

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

	npBox := components.RenderBtopBox(" Now Playing ", "", npContent, width, npHeight, styles.PlexOrange)

	// Quick Actions
	qaCol1 := []string{
		"[R]", "[B]", "[C]", "[U]", "[O]", "[X]",
	}
	qaCol2 := []string{
		"Refresh Libraries", "Backup Database", "Clean Bundles", "Update Libraries", "Optimize Database", "View Logs",
	}
	keyStyle := lipgloss.NewStyle().Foreground(styles.DimGray)
	// TODO: Wire up actual action triggers in keyboard.go
	// Swap columns so keybind is on the left
	qaContent := createListLayout(width-2, qaHeight-2, qaCol1, qaCol2, lipgloss.NewStyle(), -1)
	// We need to style col1, so let's adjust createListLayout or build it directly.
	// Actually, let's just build it quickly here since it's inverted:
	var qaLines []string
	for i := 0; i < len(qaCol1); i++ {
		qaLines = append(qaLines, " "+keyStyle.Render(qaCol1[i])+"  "+qaCol2[i])
	}
	qaContent = strings.Join(qaLines, "\n")
	
	qaBox := components.RenderBtopBox(" Quick Actions ", "", qaContent, width, qaHeight, styles.Blue)

	return lipgloss.JoinVertical(lipgloss.Left, npBox, qaBox)
}
