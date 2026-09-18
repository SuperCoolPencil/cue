package components

import (
	"github.com/SuperCoolPencil/cue/internal/tui/styles"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// InputModal is a simple text input modal
type InputModal struct {
	visible bool
	title   string
	input   textinput.Model
}

// NewInputModal creates a new input modal
func NewInputModal() InputModal {
	ti := textinput.New()
	ti.Placeholder = "Enter name..."
	ti.CharLimit = 50
	ti.Width = 30
	ti.Prompt = ""
	ti.TextStyle = lipgloss.NewStyle().Foreground(styles.ActiveTheme().FgBright)
	ti.PlaceholderStyle = styles.DimStyle()

	return InputModal{
		input: ti,
	}
}

// Show displays the modal with a title
func (m *InputModal) Show(title string) {
	m.visible = true
	m.title = title
	m.input.SetValue("")
	m.input.Focus()
}

// Hide dismisses the modal
func (m *InputModal) Hide() {
	m.visible = false
	m.input.Blur()
}

// IsVisible returns whether the modal is shown
func (m InputModal) IsVisible() bool {
	return m.visible
}

// Value returns the current input value
func (m InputModal) Value() string {
	return m.input.Value()
}

// Update handles input events, returns (modal, cmd, submitted)
func (m InputModal) Update(msg tea.Msg) (InputModal, tea.Cmd, bool) {
	if !m.visible {
		return m, nil, false
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "enter":
			return m, nil, true
		case "esc":
			m.Hide()
			return m, nil, false
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd, false
}

// View renders the input modal
func (m InputModal) View() string {
	if !m.visible {
		return ""
	}

	const modalWidth = 36

	titleStyle := lipgloss.NewStyle().
		Foreground(styles.ActiveTheme().FgBright).
		Bold(true).
		Width(modalWidth).
		Background(styles.ActiveTheme().BgDark)

	inputStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Background(styles.ActiveTheme().BgDark)

	spacer := lipgloss.NewStyle().
		Width(modalWidth).
		Background(styles.ActiveTheme().BgDark).
		Render("")

	content := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render(m.title),
		spacer,
		inputStyle.Render(m.input.View()),
	)

	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ActiveTheme().Accent).
		Background(styles.ActiveTheme().BgDark).
		Padding(1, 2).
		Render(content)

	return modal
}
