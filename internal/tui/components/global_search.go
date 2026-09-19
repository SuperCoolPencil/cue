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
	poster    string
}

// NewGlobalSearch creates a new global search component
func NewGlobalSearch() GlobalSearch {
	ti := textinput.New()
	ti.Placeholder = "Type to search movies, TV shows, episodes..."
	ti.CharLimit = 100
	ti.Prompt = "⌕ "
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
	o.input.Prompt = "⌕ "
	o.input.PromptStyle = styles.AccentStyle()
	o.input.TextStyle = lipgloss.NewStyle().Foreground(styles.ActiveTheme().FgBright)
	o.input.PlaceholderStyle = styles.DimStyle()
	o.results = nil
	o.cursor = 0
	o.offset = 0
	o.loading = false
	o.prevQuery = ""
	o.poster = ""
}

// Hide hides the global search
func (o *GlobalSearch) Hide() {
	o.visible = false
	o.input.Blur()
}

// SetPoster sets the artwork content for previewing the selected item
func (o *GlobalSearch) SetPoster(poster string) {
	o.poster = poster
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
	o.poster = ""
}

// SetSize updates the component dimensions
func (o *GlobalSearch) SetSize(width, height int) {
	o.width = width
	o.height = height

	modalWidth := (width * 80) / 100
	if modalWidth < 70 {
		modalWidth = 70
	}
	if modalWidth > 115 {
		modalWidth = 115
	}
	leftWidth := (modalWidth - 7) * 55 / 100
	if leftWidth < 30 {
		leftWidth = 30
	}
	o.input.Width = leftWidth - 6
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
				o.poster = ""
			}
			return o, nil, false

		case key.Matches(msg, GlobalSearchKeys.Up):
			if o.cursor > 0 {
				o.cursor--
				o.ensureVisible(10)
				o.poster = ""
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

const posterPlacementMarker = "\x00"

// View renders the component
func (o GlobalSearch) View() string {
	if !o.visible {
		return ""
	}

	theme := styles.ActiveTheme()

	modalWidth := (o.width * 80) / 100
	if modalWidth < 70 {
		modalWidth = 70
	}
	if modalWidth > 115 {
		modalWidth = 115
	}
	if modalWidth > o.width-2 {
		modalWidth = o.width - 2
	}

	// Fixed stable modal height so the box never jumps when posters load
	modalHeight := (o.height * 70) / 100
	if modalHeight < 22 {
		modalHeight = 22
	}
	if modalHeight > 32 {
		modalHeight = 32
	}
	if modalHeight > o.height-4 {
		modalHeight = o.height - 4
	}

	leftWidth := (modalWidth - 7) * 55 / 100
	if leftWidth < 30 {
		leftWidth = 30
	}
	rightWidth := modalWidth - 5 - leftWidth
	if rightWidth < 25 {
		rightWidth = 25
	}

	maxResults := (modalHeight - 6) / 2
	if maxResults < 4 {
		maxResults = 4
	}
	if maxResults > 10 {
		maxResults = 10
	}

	// 1. Left Column (Header + Input + Results + Footer hints)
	var bLeft strings.Builder
	bLeft.WriteString(styles.AccentStyle().Bold(true).Render("GLOBAL SEARCH"))
	bLeft.WriteString("\n\n")

	bLeft.WriteString(o.input.View())
	bLeft.WriteString("\n\n")

	if o.loading {
		spinner := lipgloss.NewStyle().Width(leftWidth - 2).Align(lipgloss.Center).Render(styles.SpinnerStyle().Render("⠋ Searching library..."))
		listAreaHeight := modalHeight - 5
		if listAreaHeight < 3 {
			listAreaHeight = 3
		}
		topPad := (listAreaHeight - 1) / 2
		bLeft.WriteString(strings.Repeat("\n", topPad))
		bLeft.WriteString(spinner)
		bLeft.WriteString("\n")
	} else if len(o.results) == 0 {
		var rawMsg string
		if o.input.Value() != "" {
			rawMsg = fmt.Sprintf("No matches found for %q", o.input.Value())
		} else {
			rawMsg = "Start typing to search..."
		}
		msg := lipgloss.NewStyle().Width(leftWidth - 2).Align(lipgloss.Center).Render(styles.DimStyle().Render(rawMsg))
		listAreaHeight := modalHeight - 5
		if listAreaHeight < 3 {
			listAreaHeight = 3
		}
		topPad := (listAreaHeight - 1) / 2
		bLeft.WriteString(strings.Repeat("\n", topPad))
		bLeft.WriteString(msg)
		bLeft.WriteString("\n")
	} else {
		o.renderResults(&bLeft, leftWidth-2, maxResults)
		bLeft.WriteString("\n")
	}

	hints := styles.RenderKeyHint("↑/↓", "navigate") + styles.DimStyle().Render(" • ") +
		styles.RenderKeyHint("enter", "select") + styles.DimStyle().Render(" • ") +
		styles.RenderKeyHint("esc", "cancel")
	var countStr string
	if len(o.results) > 0 {
		countStr = styles.DimStyle().Render(fmt.Sprintf("%d of %d", o.cursor+1, len(o.results)))
	}

	gapLen := (leftWidth - 2) - lipgloss.Width(hints) - lipgloss.Width(countStr)
	if gapLen < 1 {
		gapLen = 1
	}
	footerRow := hints + strings.Repeat(" ", gapLen) + countStr

	usedLines := strings.Count(bLeft.String(), "\n")
	paddingLines := modalHeight - usedLines - 1
	if paddingLines > 0 {
		bLeft.WriteString(strings.Repeat("\n", paddingLines))
	}
	bLeft.WriteString(footerRow)

	leftBox := lipgloss.NewStyle().Width(leftWidth).Height(modalHeight).Render(bLeft.String())

	// 2. Right Column (Header + Preview Card)
	var bRight strings.Builder
	headerStr := lipgloss.NewStyle().Width(rightWidth).Align(lipgloss.Center).Render(styles.AccentStyle().Bold(true).Render("PREVIEW"))
	bRight.WriteString(headerStr)
	bRight.WriteString("\n\n")

	previewContent := o.renderPreview(rightWidth, modalHeight-2)
	bRight.WriteString(previewContent)

	rightBox := lipgloss.NewStyle().Width(rightWidth).Height(modalHeight).Render(bRight.String())

	// 3. Full-height vertical divider running between columns (blank at top row 0 for header clearance)
	dividerLines := make([]string, modalHeight)
	if modalHeight > 0 {
		dividerLines[0] = " "
	}
	for i := 1; i < modalHeight; i++ {
		dividerLines[i] = styles.DimStyle().Render("│")
	}
	divider := strings.Join(dividerLines, "\n")

	content := lipgloss.JoinHorizontal(lipgloss.Top, leftBox, "  "+divider+"  ", rightBox)

	// Outer Modal Box
	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Accent).
		Padding(1, 2).
		Render(content)

	return lipgloss.Place(
		o.width,
		o.height,
		lipgloss.Center,
		lipgloss.Center,
		modal,
	)
}

func (o GlobalSearch) renderPreview(width, maxHeight int) string {
	theme := styles.ActiveTheme()
	selected := o.Selected()
	if selected == nil || selected.Item == nil {
		emptyMsg := styles.DimStyle().Render("No item selected")
		return lipgloss.Place(width, maxHeight-2, lipgloss.Center, lipgloss.Center, emptyMsg)
	}

	var b strings.Builder

	var title, showTitle, epCode, yearStr, ratingStr, contentRating, durationStr, resolutionStr, codecsStr, summary string
	var mediaType domain.MediaType

	switch v := selected.Item.(type) {
	case *domain.MediaItem:
		mediaType = v.Type
		title = v.Title
		if v.Type == domain.MediaTypeEpisode {
			epCode = v.EpisodeCode()
			showTitle = v.ShowTitle
		}
		if v.AirDate != "" {
			yearStr = v.AirDate
		} else if v.Year > 0 {
			yearStr = fmt.Sprintf("%d", v.Year)
		}
		if v.Rating > 0 {
			ratingStr = fmt.Sprintf("★ %.1f", v.Rating)
		}
		contentRating = v.ContentRating
		durationStr = v.FormattedDuration()
		resolutionStr = v.Resolution()
		if v.VideoCodec != "" || v.AudioCodec != "" {
			var codecs []string
			if v.VideoCodec != "" {
				codecs = append(codecs, v.VideoCodec)
			}
			if v.AudioCodec != "" {
				codecs = append(codecs, v.AudioCodec)
			}
			codecsStr = strings.Join(codecs, " · ")
		}
		summary = v.Summary

	case *domain.Show:
		mediaType = domain.MediaTypeShow
		title = v.Title
		if v.Year > 0 {
			yearStr = fmt.Sprintf("%d", v.Year)
		}
		if v.Rating > 0 {
			ratingStr = fmt.Sprintf("★ %.1f", v.Rating)
		}
		contentRating = v.ContentRating
		durationStr = v.GetDescription()
		summary = v.Summary
	}

	// Title
	if epCode != "" {
		b.WriteString(styles.TitleStyle().Render(styles.Truncate(fmt.Sprintf("%s - %s", epCode, title), width)))
		b.WriteString("\n")
	} else {
		b.WriteString(styles.TitleStyle().Render(styles.Truncate(title, width)))
		b.WriteString("\n")
	}

	if showTitle != "" {
		b.WriteString(styles.SubtitleStyle().Render(styles.Truncate(showTitle, width)))
		b.WriteString("\n")
	}

	// Type Badge & Rating
	var badgeStr string
	switch mediaType {
	case domain.MediaTypeMovie:
		badgeStr = "MOVIE"
	case domain.MediaTypeShow:
		badgeStr = "SHOW"
	case domain.MediaTypeEpisode:
		badgeStr = "EPISODE"
	default:
		badgeStr = "ITEM"
	}
	badge := styles.DimBadgeStyle().Render(badgeStr)

	var metaLine []string
	metaLine = append(metaLine, badge)

	if ratingStr != "" {
		var rStyle lipgloss.Style
		if selected.Item.GetRating() >= 7.0 {
			rStyle = lipgloss.NewStyle().Foreground(theme.Success).Bold(true)
		} else if selected.Item.GetRating() >= 5.0 {
			rStyle = lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
		} else {
			rStyle = lipgloss.NewStyle().Foreground(theme.Error).Bold(true)
		}
		metaLine = append(metaLine, rStyle.Render(ratingStr))
	}

	b.WriteString(strings.Join(metaLine, "  "))
	b.WriteString("\n")

	// Meta details: Year • Duration • Content Rating • Resolution
	var details []string
	if yearStr != "" {
		details = append(details, yearStr)
	}
	if durationStr != "" && durationStr != "0m" {
		details = append(details, durationStr)
	}
	if contentRating != "" {
		details = append(details, contentRating)
	}
	if resolutionStr != "" {
		details = append(details, resolutionStr)
	}
	if len(details) > 0 {
		b.WriteString(styles.DimStyle().Render(strings.Join(details, " · ")))
		b.WriteString("\n")
	}

	if codecsStr != "" {
		b.WriteString(styles.DimStyle().Render(codecsStr))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Render Poster if loaded, or reserve poster area so space does not jump
	if o.poster != "" {
		b.WriteString(posterPlacementMarker + o.poster)
		b.WriteString("\n\n")
	} else if selected.Item != nil {
		var placeholderLines []string
		for k := 0; k < 8; k++ {
			placeholderLines = append(placeholderLines, "")
		}
		b.WriteString(strings.Join(placeholderLines, "\n"))
		b.WriteString("\n\n")
	}

	// Plot Synopsis / Description with dynamic line calculation & ellipsis truncation
	linesUsedSoFar := strings.Count(b.String(), "\n")
	availSummaryLines := maxHeight - linesUsedSoFar

	if summary != "" && availSummaryLines > 0 {
		wrapped := wordWrap(summary, width)
		summaryLines := strings.Split(wrapped, "\n")
		if len(summaryLines) > availSummaryLines {
			if availSummaryLines == 1 {
				summaryLines = []string{styles.DimStyle().Render(styles.Truncate(summaryLines[0], width-3) + "...")}
			} else {
				summaryLines = summaryLines[:availSummaryLines-1]
				summaryLines = append(summaryLines, styles.DimStyle().Render("..."))
			}
		}
		b.WriteString(styles.SubtitleStyle().Render(strings.Join(summaryLines, "\n")))
	}

	resultLines := strings.Split(b.String(), "\n")
	if len(resultLines) > maxHeight {
		resultLines = resultLines[:maxHeight]
	}
	return strings.Join(resultLines, "\n")
}

// highlightMatches renders text with matched characters highlighted cleanly using theme styles (no background fills)
func highlightMatches(text string, matchedIndexes []int, selected bool) string {
	if len(text) == 0 {
		return ""
	}
	if len(matchedIndexes) == 0 {
		if selected {
			return lipgloss.NewStyle().Foreground(styles.ActiveTheme().FgBright).Bold(true).Render(text)
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
		normalStyle = lipgloss.NewStyle().Foreground(theme.FgBright).Bold(true)
		matchStyle = lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Underline(true)
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

// renderResults renders the search results list without background boxes
func (o GlobalSearch) renderResults(b *strings.Builder, contentWidth, maxResults int) {
	theme := styles.ActiveTheme()

	if len(o.results) == 0 {
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
			line.WriteString(lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Render("▸ "))
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
			line.WriteString(styles.DimBadgeStyle().Render(badgeStr))
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

		b.WriteString(line.String())
		b.WriteString("\n")
	}

	if remaining := len(o.results) - (o.offset + displayCount); remaining > 0 {
		b.WriteString(styles.DimStyle().Render(fmt.Sprintf("  ... and %d more", remaining)))
		b.WriteString("\n")
	}
}
