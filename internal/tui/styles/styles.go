package styles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Theme definition
// ---------------------------------------------------------------------------

// Theme holds all semantic color tokens for a colour scheme.
type Theme struct {
	Name      string
	Accent    lipgloss.Color // primary highlight: borders, focused items, spinner
	AccentAlt lipgloss.Color // secondary accent: in-progress watch dot
	BgDark    lipgloss.Color // deepest background (modal bg)
	BgMid     lipgloss.Color // selected-item background
	FgBright  lipgloss.Color // primary text
	FgMid     lipgloss.Color // secondary / normal text
	FgDim     lipgloss.Color // dim / help text
	Success   lipgloss.Color // watched check, success messages
	Error     lipgloss.Color // error messages
	Info      lipgloss.Color // informational blue tones
}

// ---------------------------------------------------------------------------
// Built-in presets
// ---------------------------------------------------------------------------

var themePlex = Theme{
	Name:      "plex",
	Accent:    lipgloss.Color("#E5A00D"),
	AccentAlt: lipgloss.Color("#E5A00D"),
	BgDark:    lipgloss.Color("#1F2937"),
	BgMid:     lipgloss.Color("#374151"),
	FgBright:  lipgloss.Color("#F9FAFB"),
	FgMid:     lipgloss.Color("#9CA3AF"),
	FgDim:     lipgloss.Color("#6B7280"),
	Success:   lipgloss.Color("#10B981"),
	Error:     lipgloss.Color("#EF4444"),
	Info:      lipgloss.Color("#3B82F6"),
}

var themeDracula = Theme{
	Name:      "dracula",
	Accent:    lipgloss.Color("#BD93F9"),
	AccentAlt: lipgloss.Color("#FF79C6"),
	BgDark:    lipgloss.Color("#282A36"),
	BgMid:     lipgloss.Color("#44475A"),
	FgBright:  lipgloss.Color("#F8F8F2"),
	FgMid:     lipgloss.Color("#BFBFBF"),
	FgDim:     lipgloss.Color("#6272A4"),
	Success:   lipgloss.Color("#50FA7B"),
	Error:     lipgloss.Color("#FF5555"),
	Info:      lipgloss.Color("#8BE9FD"),
}

var themeNord = Theme{
	Name:      "nord",
	Accent:    lipgloss.Color("#88C0D0"),
	AccentAlt: lipgloss.Color("#81A1C1"),
	BgDark:    lipgloss.Color("#2E3440"),
	BgMid:     lipgloss.Color("#3B4252"),
	FgBright:  lipgloss.Color("#ECEFF4"),
	FgMid:     lipgloss.Color("#D8DEE9"),
	FgDim:     lipgloss.Color("#4C566A"),
	Success:   lipgloss.Color("#A3BE8C"),
	Error:     lipgloss.Color("#BF616A"),
	Info:      lipgloss.Color("#5E81AC"),
}

var themeCatppuccin = Theme{
	Name:      "catppuccin",
	Accent:    lipgloss.Color("#CBA6F7"),
	AccentAlt: lipgloss.Color("#F5C2E7"),
	BgDark:    lipgloss.Color("#1E1E2E"),
	BgMid:     lipgloss.Color("#313244"),
	FgBright:  lipgloss.Color("#CDD6F4"),
	FgMid:     lipgloss.Color("#BAC2DE"),
	FgDim:     lipgloss.Color("#6C7086"),
	Success:   lipgloss.Color("#A6E3A1"),
	Error:     lipgloss.Color("#F38BA8"),
	Info:      lipgloss.Color("#89DCEB"),
}

var themeGruvbox = Theme{
	Name:      "gruvbox",
	Accent:    lipgloss.Color("#D79921"),
	AccentAlt: lipgloss.Color("#D65D0E"),
	BgDark:    lipgloss.Color("#282828"),
	BgMid:     lipgloss.Color("#3C3836"),
	FgBright:  lipgloss.Color("#EBDBB2"),
	FgMid:     lipgloss.Color("#D5C4A1"),
	FgDim:     lipgloss.Color("#928374"),
	Success:   lipgloss.Color("#B8BB26"),
	Error:     lipgloss.Color("#CC241D"),
	Info:      lipgloss.Color("#458588"),
}

// allThemes is the ordered list of built-in presets.
var allThemes = []Theme{
	themePlex,
	themeDracula,
	themeNord,
	themeCatppuccin,
	themeGruvbox,
}

// ---------------------------------------------------------------------------
// Active-theme API
// ---------------------------------------------------------------------------

var active = themePlex

// SetTheme switches the active theme by name. Returns false if unknown.
func SetTheme(name string) bool {
	for _, t := range allThemes {
		if t.Name == name {
			active = t
			return true
		}
	}
	return false
}

// ActiveTheme returns the currently active theme.
func ActiveTheme() Theme {
	return active
}

// ThemeNames returns all built-in theme names in display order.
func ThemeNames() []string {
	names := make([]string, len(allThemes))
	for i, t := range allThemes {
		names[i] = t.Name
	}
	return names
}

// NextThemeName returns the name that follows the given theme name (wraps around).
func NextThemeName(current string) string {
	names := ThemeNames()
	for i, n := range names {
		if n == current {
			return names[(i+1)%len(names)]
		}
	}
	return names[0]
}

// ---------------------------------------------------------------------------
// Style constructors (read active theme at call time)
// ---------------------------------------------------------------------------

// Borders

func ActiveBorder() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(active.Accent)
}

func InactiveBorder() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(active.FgDim)
}

func NoBorder() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.HiddenBorder())
}

// Text styles

func TitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(active.FgBright).
		Bold(true)
}

func SubtitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(active.FgMid)
}

func DimStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(active.FgDim)
}

func AccentStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(active.Accent)
}

func ErrorStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(active.Error)
}

func SuccessStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(active.Success)
}

func HighlightStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(active.FgBright).
		Background(active.Accent).
		Padding(0, 1)
}

// Raw watch status characters (unstyled)
const (
	UnplayedChar   = "○"
	InProgressChar = "◐"
	PlayedChar     = "✓"
)

// Watch status indicator styles

func UnplayedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(active.Accent)
}

func InProgressStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(active.AccentAlt)
}

func PlayedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(active.Success)
}

// Pre-rendered watch status indicators

func UnplayedDot() string   { return UnplayedStyle().Render(UnplayedChar) }
func InProgressDot() string { return InProgressStyle().Render(InProgressChar) }
func PlayedCheck() string   { return PlayedStyle().Render(PlayedChar) }

// List item styles

func SelectedItemStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(active.FgBright).
		Background(active.BgMid).
		Padding(0, 1)
}

func NormalItemStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(active.FgMid).
		Padding(0, 1)
}

func FocusedItemStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(active.Accent).
		Bold(true).
		Padding(0, 1)
}

// Modal styles

func ModalStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(active.Accent).
		Padding(1, 2).
		Background(active.BgDark)
}

func ModalTitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(active.FgBright).
		Bold(true).
		MarginBottom(1)
}

// Help styles

func HelpKeyStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(active.Accent)
}

func HelpDescStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(active.FgDim)
}

// Progress bar styles

func ProgressFullStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(active.Accent)
}

func ProgressEmptyStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(active.FgDim)
}

// Badge styles

func BadgeStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(active.FgBright).
		Background(active.Accent).
		Padding(0, 1)
}

func DimBadgeStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(active.FgMid).
		Background(active.BgMid).
		Padding(0, 1)
}

// Spinner style

func SpinnerStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(active.Accent)
}

// SpinnerFrames contains the animation frames for the loading spinner
var SpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Filter styles

func FilterStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(active.Accent)
}

func FilterPromptStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(active.Accent).
		Bold(true)
}

// Match highlight styles for search results

func MatchHighlightStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(active.Accent).
		Bold(true)
}

func MatchHighlightSelectedStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(active.Accent).
		Background(active.BgMid).
		Bold(true)
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// Truncate truncates a string to the given width with ellipsis
func Truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if len(s) <= width {
		return s
	}
	if width <= 3 {
		if width > len(s) {
			return s
		}
		return s[:width]
	}
	return s[:width-3] + "..."
}

// Pad pads a string to the given width
func Pad(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return s + spaces(width-len(s))
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}

// InsetLeft adds one printable cell inside a pane without changing the pane's
// border frame. Keeping this separate from border-style padding avoids terminal
// wrapping differences in incremental full-screen renders.
func InsetLeft(content string) string {
	return " " + strings.ReplaceAll(content, "\n", "\n ")
}

// RenderProgressBar renders a progress bar
func RenderProgressBar(percent float64, width int) string {
	if width < 3 {
		return ""
	}

	filled := int(float64(width) * percent / 100)
	if filled > width {
		filled = width
	}

	bar := ""
	for i := 0; i < filled; i++ {
		bar += ProgressFullStyle().Render("━")
	}
	for i := filled; i < width; i++ {
		bar += ProgressEmptyStyle().Render("─")
	}

	return bar
}

// RenderListRow renders a complete list row with uniform background when selected.
// This function styles each part explicitly to avoid ANSI reset code issues.
// parts is a slice of {text, fgColor} pairs. Use nil for default foreground.
func RenderListRow(parts []RowPart, selected bool, width int) string {
	bg := active.BgMid
	defaultFg := active.FgMid
	selectedFg := active.FgBright

	var result string
	visibleLen := 0

	for _, part := range parts {
		style := lipgloss.NewStyle()
		if part.Foreground != nil {
			style = style.Foreground(*part.Foreground)
		} else if selected {
			style = style.Foreground(selectedFg)
		} else {
			style = style.Foreground(defaultFg)
		}
		if selected {
			style = style.Background(bg)
		}
		if part.Bold {
			style = style.Bold(true)
		}
		result += style.Render(part.Text)
		visibleLen += lipgloss.Width(part.Text)
	}

	// Add padding to fill width (subtract 2 for left/right margin)
	paddingNeeded := width - visibleLen - 2
	if paddingNeeded > 0 {
		padStyle := lipgloss.NewStyle()
		if selected {
			padStyle = padStyle.Background(bg)
		}
		result += padStyle.Render(spaces(paddingNeeded))
	}

	// Add margins
	marginStyle := lipgloss.NewStyle()
	if selected {
		marginStyle = marginStyle.Background(bg)
	}
	margin := marginStyle.Render(" ")

	return margin + result + margin
}

// RowPart represents a part of a row with optional foreground color
type RowPart struct {
	Text       string
	Foreground *lipgloss.Color
	Bold       bool
}
