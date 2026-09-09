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
	ti.Placeholder = "Search..."
	ti.CharLimit = 100
	ti.Width = 40
	ti.Prompt = "/ "
	ti.PromptStyle = styles.AccentStyle
	ti.TextStyle = lipgloss.NewStyle().Foreground(styles.White)
	ti.PlaceholderStyle = styles.DimStyle

	return GlobalSearch{
		input: ti,
	}
}

// Show makes the global search visible and focuses the input
func (o *GlobalSearch) Show() {
	o.visible = true
	o.input.Focus()
	o.input.SetValue("")
	o.input.Placeholder = "Type to search..."
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
	o.input.Width = width - 10
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

// HandleMouse handles mouse input for the global search modal.
// screenW, screenH are the terminal dimensions.
// Returns (globalSearch, handled, resultClicked) where:
//
//	handled = true if the mouse event was consumed (search remains open)
//	resultClicked = true if the click landed on a result row (for double-click gating)
func (o GlobalSearch) HandleMouse(msg tea.MouseMsg, screenW, screenH int) (GlobalSearch, bool, bool) {
	if !o.visible {
		return o, false, false
	}

	// modal dimensions (same as View())
	modalWidth := o.width * 2 / 3
	if modalWidth < 40 {
		modalWidth = 40
	}
	if modalWidth > 80 {
		modalWidth = 80
	}

	// Approximate content height: title(1) + blank(1) + input(1) + blank(1) + results
	maxResults := 10
	resultCount := o.ResultCount()
	displayCount := resultCount - o.offset
	if displayCount > maxResults {
		displayCount = maxResults
	}
	if displayCount < 0 {
		displayCount = 0
	}

	contentHeight := 4 + displayCount // title+blank+input+blank+results
	if remaining := resultCount - (o.offset + displayCount); remaining > 0 {
		contentHeight++ // "and X more" line
	}
	// ModalStyle has Border(RoundedBorder) + Padding(1,2) = 4 vertical frame
	modalHeight := contentHeight + 4
	if modalHeight > screenH {
		modalHeight = screenH
	}

	modalX := (screenW - modalWidth) / 2
	if modalX < 0 {
		modalX = 0
	}
	modalY := (screenH - modalHeight) / 2
	if modalY < 0 {
		modalY = 0
	}

	// Check if click is inside the modal rect
	insideModal := msg.X >= modalX && msg.X < modalX+modalWidth &&
		msg.Y >= modalY && msg.Y < modalY+modalHeight

	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		if o.cursor > 0 {
			o.cursor--
			o.ensureVisible(10)
		}
		return o, true, false

	case msg.Button == tea.MouseButtonWheelDown:
		if o.cursor < resultCount-1 {
			o.cursor++
			o.ensureVisible(10)
		}
		return o, true, false

	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		if insideModal {
			// Results start after border(1)+padding(1)+title(1)+blank(1)+input(1)+blank(1)
			// = modalY + 6
			firstResultLine := modalY + 6
			resultIdx := msg.Y - firstResultLine
			if resultIdx >= 0 && resultIdx < displayCount {
				o.cursor = o.offset + resultIdx
				return o, true, true // result row clicked — app gates double-click on this
			}
			// Let other areas (input field) pass through, but mark as handled
			// so the outside-click dismiss doesn't trigger
			return o, true, false
		}
		// Click outside modal — dismiss
		o.Hide()
		return o, true, false

	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonRight:
		o.Hide()
		return o, true, false
	}

	return o, false, false
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

	// Modal dimensions
	modalWidth := o.width * 2 / 3
	if modalWidth < 40 {
		modalWidth = 40
	}
	if modalWidth > 80 {
		modalWidth = 80
	}
	maxResults := 10

	var b strings.Builder

	// Title
	b.WriteString("Global Search")
	b.WriteString("\n\n")

	// Input field
	b.WriteString(o.input.View())
	b.WriteString("\n\n")

	// Results
	if o.loading {
		b.WriteString(styles.SpinnerStyle.Render("Searching..."))
	} else {
		o.renderResults(&b, modalWidth, maxResults)
	}

	// Center the modal
	content := lipgloss.NewStyle().
		Width(modalWidth - 4).
		Render(b.String())

	modal := styles.ModalStyle.
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

// highlightMatches renders text with matched characters highlighted
// Uses ANSI codes directly to avoid lipgloss padding issues
func highlightMatches(text string, matchedIndexes []int, selected bool) string {
	if len(matchedIndexes) == 0 {
		if selected {
			return styles.SelectedItemStyle.Render(text)
		}
		return styles.NormalItemStyle.Render(text)
	}

	// Create a set of matched indexes for O(1) lookup
	matchSet := make(map[int]bool)
	for _, idx := range matchedIndexes {
		matchSet[idx] = true
	}

	// ANSI escape codes for inline styling (no padding)
	// Orange/bold for matches, gray for normal text
	const (
		reset      = "\033[0m"
		orange     = "\033[38;5;208m" // PlexOrange approximate
		orangeBold = "\033[38;5;208;1m"
		gray       = "\033[38;5;250m" // LightGray approximate
		white      = "\033[38;5;255m"
		bgSlate    = "\033[48;5;238m" // SlateLight approximate
	)

	var matchStart, matchEnd, normalStart, normalEnd string
	if selected {
		// Selected: white bg for normal, orange+bold+bg for match
		normalStart = white + bgSlate
		normalEnd = reset
		matchStart = orangeBold + bgSlate
		matchEnd = reset
	} else {
		// Not selected: gray for normal, orange+bold for match
		normalStart = gray
		normalEnd = reset
		matchStart = orangeBold
		matchEnd = reset
	}

	// Batch consecutive characters with the same style
	var result strings.Builder
	runes := []rune(text)
	i := 0
	for i < len(runes) {
		isMatch := matchSet[i]

		// Collect consecutive characters with the same match state
		var batch strings.Builder
		for i < len(runes) && matchSet[i] == isMatch {
			batch.WriteRune(runes[i])
			i++
		}

		// Render the batch with ANSI codes
		if isMatch {
			result.WriteString(matchStart)
			result.WriteString(batch.String())
			result.WriteString(matchEnd)
		} else {
			result.WriteString(normalStart)
			result.WriteString(batch.String())
			result.WriteString(normalEnd)
		}
	}

	return result.String()
}

// renderResults renders the search results
func (o GlobalSearch) renderResults(b *strings.Builder, modalWidth, maxResults int) {
	if len(o.results) == 0 && o.input.Value() != "" {
		b.WriteString(styles.DimStyle.Render("No matches found"))
		return
	}
	if len(o.results) == 0 {
		// Don't show anything when empty - placeholder already guides the user
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

		// Type badge with library context
		switch result.Type {
		case domain.MediaTypeMovie:
			line.WriteString(styles.DimBadgeStyle.Render("MOV"))
		case domain.MediaTypeShow:
			line.WriteString(styles.DimBadgeStyle.Render("SHOW"))
		case domain.MediaTypeEpisode:
			line.WriteString(styles.DimBadgeStyle.Render("EP"))
		}
		line.WriteString(" ")

		// Build display title
		title := result.Title
		matchedIndexes := result.MatchedIndexes
		maxTitleWidth := modalWidth - 25
		switch result.Type {
		case domain.MediaTypeEpisode:
			// For episodes, show: ShowTitle - S01E01 Title
			if item, ok := result.Item.(*domain.MediaItem); ok {
				title = fmt.Sprintf("%s - %s %s", item.ShowTitle, item.EpisodeCode(), item.Title)
				// Reset matched indexes since the title format changed
				matchedIndexes = nil
			}
		case domain.MediaTypeMovie:
			// For movies, show: Title (Year)
			if item, ok := result.Item.(*domain.MediaItem); ok && item.Year > 0 {
				title = fmt.Sprintf("%s (%d)", item.Title, item.Year)
				// Matched indexes still apply to the title portion
			}
		}
		title = styles.Truncate(title, maxTitleWidth)

		// Apply highlighting to the title
		line.WriteString(highlightMatches(title, matchedIndexes, selected))

		b.WriteString(line.String())
		b.WriteString("\n")
	}

	if remaining := len(o.results) - (o.offset + displayCount); remaining > 0 {
		b.WriteString(styles.DimStyle.Render(fmt.Sprintf("... and %d more", remaining)))
	}
}
