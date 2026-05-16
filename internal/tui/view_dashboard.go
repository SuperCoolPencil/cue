package tui

import (
	"fmt"
	"strings"

	"github.com/SuperCoolPencil/cue/internal/tui/components"
	"github.com/SuperCoolPencil/cue/internal/tui/styles"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderDashboard() string {
	// Top Header
	header := m.renderDashboardHeader()

	// Available height for the main content
	contentHeight := m.Height - lipgloss.Height(header) - ChromeHeight // ChromeHeight for bottom footer
	if contentHeight < 10 {
		contentHeight = 10
	}

	// Calculate widths
	leftWidth := 28
	rightWidth := 38
	centerWidth := m.Width - leftWidth - rightWidth
	if centerWidth < 40 {
		centerWidth = 40
	}

	// Render Panels
	leftPanel := m.renderDashboardLeftPanel(leftWidth, contentHeight)
	centerPanel := m.renderDashboardCenterPanel(centerWidth, contentHeight)
	rightPanel := m.renderDashboardRightPanel(rightWidth, contentHeight)

	// Combine columns
	content := lipgloss.JoinHorizontal(lipgloss.Top,
		leftPanel,
		centerPanel,
		rightPanel,
	)

	// Footer
	footer := m.renderFooter()

	return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
}

func (m Model) renderDashboardHeader() string {
	cueLogo := ` ██████╗██╗   ██╗███████╗
██╔════╝██║   ██║██╔════╝
██║     ██║   ██║█████╗  
██║     ██║   ██║██╔══╝  
╚██████╗╚██████╔╝███████╗
 ╚═════╝ ╚═════╝ ╚══════╝`

	// Make the logo fit compactly and style it
	logoStyle := lipgloss.NewStyle().Foreground(styles.PlexOrange).Bold(true)
	renderedLogo := logoStyle.Render(cueLogo)

	titleText := lipgloss.NewStyle().Foreground(styles.LightGray).Render(fmt.Sprintf("Cue TUI %s", m.Version))
	welcomeText := lipgloss.NewStyle().Foreground(styles.White).Render("Welcome back, admin")
	serverInfo := lipgloss.NewStyle().Foreground(styles.DimGray).Render(
		fmt.Sprintf("Server: %s | IP: 127.0.0.1 | Time: 14:37 | ", lipgloss.NewStyle().Foreground(styles.Green).Render("media-server")) +
			lipgloss.NewStyle().Foreground(styles.Red).Render("[Q] Quit"),
	)

	// Layout the header
	// [Logo + Title]        [Welcome]        [Server Info]
	logoLines := strings.Split(renderedLogo, "\n")
	logoHeight := len(logoLines)

	// Build a header box of height logoHeight
	var headerLines []string
	for i := 0; i < logoHeight; i++ {
		line := ""
		if i == logoHeight-1 {
			// bottom line gets the title next to it
			logoPart := logoLines[i]
			pad := 2
			leftPart := logoPart + strings.Repeat(" ", pad) + titleText
			
			// Center part
			centerPad := (m.Width / 2) - lipgloss.Width(leftPart) - (lipgloss.Width(welcomeText) / 2)
			if centerPad < 1 {
				centerPad = 1
			}
			midPart := leftPart + strings.Repeat(" ", centerPad) + welcomeText
			
			// Right part
			rightPad := m.Width - lipgloss.Width(midPart) - lipgloss.Width(serverInfo) - 2 // 2 for margin
			if rightPad < 1 {
				rightPad = 1
			}
			line = midPart + strings.Repeat(" ", rightPad) + serverInfo
		} else {
			line = logoLines[i]
		}
		headerLines = append(headerLines, line)
	}

	headerStr := strings.Join(headerLines, "\n")
	return lipgloss.NewStyle().Padding(1, 2).Render(headerStr)
}

func (m Model) renderDashboardLeftPanel(width, height int) string {
	mainMenu := []string{
		styles.HighlightStyle.Render("> Dashboard"),
		"  Libraries",
		"  Now Playing",
		"  Discover",
		"  Users",
		"  Activity",
		"  Playlists",
		"  Settings",
		"  Plugins",
		"  Help",
	}

	shortcuts := []string{
		"Search              /",
		"Go to Library       g",
		"Refresh             r",
		"Global Search       f",
		"Command Palette     :",
	}

	menuStr := strings.Join(mainMenu, "\n\n")
	shortcutsStr := strings.Join(shortcuts, "\n\n")

	// Calculate heights for boxes
	menuHeight := (height / 2) + 2
	shortcutsHeight := height - menuHeight

	menuBox := components.RenderBtopBox("MAIN MENU", "", menuStr, width, menuHeight, styles.SlateLight)
	shortcutsBox := components.RenderBtopBox("SHORTCUTS", "", shortcutsStr, width, shortcutsHeight, styles.SlateLight)

	return lipgloss.JoinVertical(lipgloss.Left, menuBox, shortcutsBox)
}

func (m Model) renderDashboardCenterPanel(width, height int) string {
	// 3 rows: Top (full width), Middle (split), Bottom (split)
	row1Height := height / 3
	row2Height := height / 3
	row3Height := height - row1Height - row2Height

	halfWidth := width / 2
	otherHalfWidth := width - halfWidth

	// Continue Watching
	cwContent := "▷ Dune: Part Two           2024      42:18 / 2:46:00    25% " + styles.RenderProgressBar(25, 15) + "\n\n" +
		"▷ The Bear - S03E05        S03E05    12:07 / 28:15      42% " + styles.RenderProgressBar(42, 15) + "\n\n" +
		"▷ Shogun - S01E10          S01E10    35:42 / 1:02:31    57% " + styles.RenderProgressBar(57, 15)
	cwBox := components.RenderBtopBox("CONTINUE WATCHING", "", cwContent, width, row1Height, styles.Green)

	// Recently Added
	raContent := "+ Furiosa: A Mad Max Saga        2024    4K HDR\n\n" +
		"+ The Fall of the House of Usher 2023    1080p\n\n" +
		"+ Spider-Man: Across the Spider  2023    4K\n\n" +
		"+ Oppenheimer                    2023    4K HDR\n\n" +
		"+ Shogun (2024)                  2024    4K HDR\n\n\n" +
		"... and 8 more"
	raBox := components.RenderBtopBox("RECENTLY ADDED", "", raContent, halfWidth, row2Height, styles.Blue)

	// Libraries
	libContent := "📺 Movies              1,842\n\n" +
		"🎬 TV Shows            312\n\n" +
		"🎞️ 4K Movies            265\n\n" +
		"🎥 Documentaries       127\n\n" +
		"🎵 Music               8,541\n\n" +
		"📸 Photos              12,103\n\n" +
		"Total: 7 Libraries"
	libBox := components.RenderBtopBox("LIBRARIES", "", libContent, otherHalfWidth, row2Height, styles.PlexOrange)

	row2 := lipgloss.JoinHorizontal(lipgloss.Top, raBox, libBox)

	// Recent Activity
	actContent := "● jessica started playing The Bear    14:35\n\n" +
		"● mike started playing Shogun         14:33\n\n" +
		"● alex added Dune: Part Two           14:28\n\n" +
		"● sarah unlocked alex's account       14:21\n\n" +
		"● alex started playing Oppenheimer    14:19\n\n\n" +
		"                 View full activity   [A]"
	actBox := components.RenderBtopBox("RECENT ACTIVITY", "", actContent, halfWidth, row3Height, styles.PlexOrange)

	// On Deck
	odContent := "1  The Bear - S03E06               ▷\n   Fishes\n\n" +
		"2  Shogun - S01E11                 ▷\n   A Dream of a Dream\n\n" +
		"3  Dune: Part Two                  ▷\n   Continue watching\n\n\n" +
		"                 View full on deck [D]"
	odBox := components.RenderBtopBox("ON DECK", "", odContent, otherHalfWidth, row3Height, styles.Green)

	row3 := lipgloss.JoinHorizontal(lipgloss.Top, actBox, odBox)

	return lipgloss.JoinVertical(lipgloss.Left, cwBox, row2, row3)
}

func (m Model) renderDashboardRightPanel(width, height int) string {
	npHeight := (height * 3) / 5
	qaHeight := height - npHeight

	// Now Playing
	npContent := "The Bear - S03E05                 jessica\n" +
		"Napkins\n" +
		"1080p (Local)\n" +
		"                       12:07 / 28:15\n" +
		styles.RenderProgressBar(42, 40) + "\n\n" +
		"Shogun - S01E10                   mike\n" +
		"The Dream of the Fisherman's Wife\n" +
		"1080p (Remote)\n" +
		"                       35:42 / 1:02:31\n" +
		styles.RenderProgressBar(57, 40) + "\n\n" +
		"Dune: Part Two                    admin\n" +
		"2024\n" +
		"4K HDR (Remote)\n" +
		"                       42:18 / 2:46:00\n" +
		styles.RenderProgressBar(25, 40) + "\n\n\n" +
		"                  View all activity [A]"
	npBox := components.RenderBtopBox("NOW PLAYING (3)", "", npContent, width, npHeight, styles.PlexOrange)

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
