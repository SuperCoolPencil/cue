package components

import (
	"fmt"
	"strings"

	"github.com/SuperCoolPencil/cue/internal/domain"
	"github.com/SuperCoolPencil/cue/internal/search"
	"github.com/SuperCoolPencil/cue/internal/tui/styles"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// GlobalSearch is the fuzzy search modal component
type GlobalSearch struct {
	input     textinput.Model
	results   []search.FilterResult
	cursor    int
	offset    int
	visible   bool
	width     int
	height    int
	loading   bool
	prevQuery string
}

// NewGlobalSearch creates a new global search component
func NewGlobalSearch() GlobalSearch {
	ti := textinput.New()
	ti.Placeholder = "Type to search movies, TV shows, episodes..."
	ti.CharLimit = 100
	ti.Prompt = "🔍 "
	ti.PromptStyle = styles.AccentStyle()
	ti.TextStyle = lipgloss.NewStyle().Foreground(styles.ActiveTheme().FgBright)
	ti.PlaceholderStyle = styles.DimStyle()

	return GlobalSearch{
		input: ti,
	}
}

// Show makes the global search visible and focuses the input
func (o *GlobalSearch) Show() {
	o.visible = true
	o.input.Focus()
	o.input.SetValue("")
	o.input.Placeholder = "Type to search movies, TV shows, episodes..."
	o.input.Prompt = "🔍 "
	o.results = nil
	o.cursor = 0
	o.offset = 0
	o.loading = false
	o.prevQuery = ""
}

// Hide hides the global search
func (o *GlobalSearch) Hide() {
	o.visible = false
	o.input.Blur()
}

// IsVisible returns true if the global search is visible
func (o GlobalSearch) IsVisible() bool {
	return o.visible
}

// SetResults sets the search results with match highlighting data
func (o *GlobalSearch) SetResults(results []search.FilterResult) {
	o.results = results
	o.cursor = 0
	o.offset = 0
	o.loading = false
}

// SetSize updates the component dimensions
func (o *GlobalSearch) SetSize(width, height int) {
	o.width = width
	o.height = height

	modalWidth := (width * 65) / 100
	if modalWidth < 50 {
		modalWidth = 50
	}
	if modalWidth > 110 {
		modalWidth = 110
	}
	o.input.Width = modalWidth - 10
}

// Query returns the current search query
func (o GlobalSearch) Query() string {
	return o.input.Value()
}

// QueryChanged returns true if the query changed since last check and updates prevQuery
func (o *GlobalSearch) QueryChanged() bool {
	current := o.input.Value()
	if current != o.prevQuery {
		o.prevQuery = current
		return true
	}
	return false
}

// Selected returns the selected result's FilterItem
func (o GlobalSearch) Selected() *search.FilterItem {
	if len(o.results) == 0 || o.cursor >= len(o.results) {
		return nil
	}
	return &o.results[o.cursor].FilterItem
}

// ResultCount returns the number of results
func (o GlobalSearch) ResultCount() int {
	return len(o.results)
}

// Init initializes the component
func (o GlobalSearch) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages
func (o GlobalSearch) Update(msg tea.Msg) (GlobalSearch, tea.Cmd, bool) {
	if !o.visible {
		return o, nil, false
	}

	var cmd tea.Cmd
	resultCount := o.ResultCount()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, GlobalSearchKeys.Escape):
			o.Hide()
			return o, nil, false

		case key.Matches(msg, GlobalSearchKeys.Enter):
			if resultCount > 0 {
				return o, nil, true // Selected
			}
			return o, nil, false

		case key.Matches(msg, GlobalSearchKeys.Down):
			if o.cursor < resultCount-1 {
				o.cursor++
				o.ensureVisible(10)
			}
			return o, nil, false

		case key.Matches(msg, GlobalSearchKeys.Up):
			if o.cursor > 0 {
				o.cursor--
				o.ensureVisible(10)
			}
			return o, nil, false

		default:
			// Pass to text input
			o.input, cmd = o.input.Update(msg)
			return o, cmd, false
		}
	}

	// Handle other messages
	o.input, cmd = o.input.Update(msg)
	return o, cmd, false
}

func (o *GlobalSearch) ensureVisible(maxVisible int) {
	if o.cursor < o.offset {
		o.offset = o.cursor
	}
	if o.cursor >= o.offset+maxVisible {
		o.offset = o.cursor - maxVisible + 1
	}
}

// View renders the component
func (o GlobalSearch) View() string {
	if !o.visible {
		return ""
	}

	// Dynamic modal dimensions based on terminal width and height
	modalWidth := (o.width * 65) / 100
	if modalWidth < 50 {
		modalWidth = 50
	}
	if modalWidth > 110 {
		modalWidth = 110
	}
	contentWidth := modalWidth - 4

	maxResults := (o.height - 12) / 2
	if maxResults < 5 {
		maxResults = 5
	}
	if maxResults > 12 {
		maxResults = 12
	}

	var b strings.Builder
	theme := styles.ActiveTheme()

	// Title / Header
	header := lipgloss.NewStyle().
		Foreground(theme.Accent).
		Bold(true).
		Render("GLOBAL SEARCH")
	b.WriteString(header)
	b.WriteString("\n\n")

	// Input box container (sleek inner panel)
	inputBox := lipgloss.NewStyle().
		Background(theme.BgMid).
		Foreground(theme.FgBright).
		Padding(0, 1).
		Width(contentWidth).
		Render(o.input.View())
	b.WriteString(inputBox)
	b.WriteString("\n\n")

	// Results area
	if o.loading {
		spinner := styles.SpinnerStyle().Render("⠋ Searching library...")
		b.WriteString(lipgloss.NewStyle().Width(contentWidth).Align(lipgloss.Center).Render(spinner))
		b.WriteString("\n")
	} else {
		o.renderResults(&b, contentWidth, maxResults)
	}

	b.WriteString("\n")

	// Footer bar with shortcuts & count
	footerStyle := lipgloss.NewStyle().Width(contentWidth)
	hints := styles.DimStyle().Render("↑/↓ navigate  •  enter select  •  esc cancel")
	var countStr string
	if len(o.results) > 0 {
		countStr = styles.DimStyle().Render(fmt.Sprintf("%d of %d", o.cursor+1, len(o.results)))
	}

	gapLen := contentWidth - lipgloss.Width(hints) - lipgloss.Width(countStr)
	if gapLen < 1 {
		gapLen = 1
	}
	footerRow := hints + strings.Repeat(" ", gapLen) + countStr
	b.WriteString(footerStyle.Render(footerRow))

	// Outer Modal Box
	content := lipgloss.NewStyle().
		Width(contentWidth).
		Render(b.String())

	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Accent).
		Padding(1, 2).
		Background(theme.BgDark).
		Width(modalWidth).
		Render(content)

	// Center horizontally and vertically
	return lipgloss.Place(
		o.width,
		o.height,
		lipgloss.Center,
		lipgloss.Center,
		modal,
	)
}

// highlightMatches renders text with matched characters highlighted cleanly using theme styles
func highlightMatches(text string, matchedIndexes []int, selected bool) string {
	if len(text) == 0 {
		return ""
	}
	if len(matchedIndexes) == 0 {
		if selected {
			return lipgloss.NewStyle().Foreground(styles.ActiveTheme().FgBright).Background(styles.ActiveTheme().BgMid).Render(text)
		}
		return lipgloss.NewStyle().Foreground(styles.ActiveTheme().FgMid).Render(text)
	}

	matchSet := make(map[int]bool, len(matchedIndexes))
	for _, idx := range matchedIndexes {
		matchSet[idx] = true
	}

	theme := styles.ActiveTheme()
	var normalStyle, matchStyle lipgloss.Style

	if selected {
		normalStyle = lipgloss.NewStyle().Foreground(theme.FgBright).Background(theme.BgMid)
		matchStyle = lipgloss.NewStyle().Foreground(theme.Accent).Background(theme.BgMid).Bold(true)
	} else {
		normalStyle = lipgloss.NewStyle().Foreground(theme.FgMid)
		matchStyle = lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	}

	runes := []rune(text)
	var result strings.Builder
	i := 0
	for i < len(runes) {
		isMatch := matchSet[i]

		var batch strings.Builder
		for i < len(runes) && matchSet[i] == isMatch {
			batch.WriteRune(runes[i])
			i++
		}

		if isMatch {
			result.WriteString(matchStyle.Render(batch.String()))
		} else {
			result.WriteString(normalStyle.Render(batch.String()))
		}
	}

	return result.String()
}

// renderResults renders the search results list
func (o GlobalSearch) renderResults(b *strings.Builder, contentWidth, maxResults int) {
	theme := styles.ActiveTheme()

	if len(o.results) == 0 && o.input.Value() != "" {
		emptyMsg := styles.DimStyle().Render(fmt.Sprintf("No matches found for %q", o.input.Value()))
		b.WriteString(lipgloss.NewStyle().Width(contentWidth).Align(lipgloss.Center).Render(emptyMsg))
		b.WriteString("\n")
		return
	}
	if len(o.results) == 0 {
		placeholderMsg := styles.DimStyle().Render("Start typing to search movies, TV shows, and episodes...")
		b.WriteString(lipgloss.NewStyle().Width(contentWidth).Align(lipgloss.Center).Render(placeholderMsg))
		b.WriteString("\n")
		return
	}

	displayCount := len(o.results) - o.offset
	if displayCount > maxResults {
		displayCount = maxResults
	}

	for i := o.offset; i < o.offset+displayCount; i++ {
		result := o.results[i]
		selected := i == o.cursor

		var line strings.Builder

		// Cursor indicator
		if selected {
			line.WriteString(lipgloss.NewStyle().Foreground(theme.Accent).Background(theme.BgMid).Bold(true).Render("▸ "))
		} else {
			line.WriteString("  ")
		}

		// Type badge
		var badgeStr string
		switch result.Type {
		case domain.MediaTypeMovie:
			badgeStr = "MOVIE"
		case domain.MediaTypeShow:
			badgeStr = "SHOW"
		case domain.MediaTypeEpisode:
			badgeStr = "EPISODE"
		default:
			badgeStr = "ITEM"
		}

		if selected {
			line.WriteString(lipgloss.NewStyle().
				Foreground(theme.FgBright).
				Background(theme.Accent).
				Bold(true).
				Padding(0, 1).
				Render(badgeStr))
		} else {
			line.WriteString(lipgloss.NewStyle().
				Foreground(theme.FgMid).
				Background(theme.BgMid).
				Padding(0, 1).
				Render(badgeStr))
		}
		line.WriteString(" ")

		// Build display title
		title := result.Title
		matchedIndexes := result.MatchedIndexes
		maxTitleWidth := contentWidth - 18
		if maxTitleWidth < 15 {
			maxTitleWidth = 15
		}

		switch result.Type {
		case domain.MediaTypeEpisode:
			if item, ok := result.Item.(*domain.MediaItem); ok {
				title = fmt.Sprintf("%s - %s %s", item.ShowTitle, item.EpisodeCode(), item.Title)
				matchedIndexes = nil
			}
		case domain.MediaTypeMovie:
			if item, ok := result.Item.(*domain.MediaItem); ok && item.Year > 0 {
				title = fmt.Sprintf("%s (%d)", item.Title, item.Year)
			}
		}
		title = styles.Truncate(title, maxTitleWidth)

		// Title with match highlighting
		line.WriteString(highlightMatches(title, matchedIndexes, selected))

		// Full-width line wrapping with consistent background on selection
		rowStr := line.String()
		rowWidth := lipgloss.Width(rowStr)
		if rowWidth < contentWidth {
			padding := strings.Repeat(" ", contentWidth-rowWidth)
			if selected {
				rowStr += lipgloss.NewStyle().Background(theme.BgMid).Render(padding)
			} else {
				rowStr += padding
			}
		}

		b.WriteString(rowStr)
		b.WriteString("\n")
	}

	if remaining := len(o.results) - (o.offset + displayCount); remaining > 0 {
		b.WriteString(styles.DimStyle().Render(fmt.Sprintf("  ... and %d more", remaining)))
		b.WriteString("\n")
	}
}
