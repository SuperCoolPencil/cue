package components

import (
	"strings"

	"github.com/SuperCoolPencil/cue/internal/domain"
	"github.com/SuperCoolPencil/cue/internal/tui/styles"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PlaylistChange represents a pending change to playlist membership
type PlaylistChange struct {
	PlaylistID string
	Add        bool // true = add to playlist, false = remove from playlist
}

// PlaylistModal is a modal for managing playlist membership
type PlaylistModal struct {
	visible    bool
	item       *domain.MediaItem
	playlists  []*domain.Playlist
	membership map[string]bool // Current membership: playlist ID -> is member
	pending    map[string]bool // Toggled state: playlist ID -> should be member

	cursor     int
	createMode bool
	newTitle   textinput.Model

	width  int
	height int
}

// NewPlaylistModal creates a new playlist modal
func NewPlaylistModal() PlaylistModal {
	ti := textinput.New()
	ti.Placeholder = "Playlist name..."
	ti.Prompt = "> "
	ti.CharLimit = 50

	return PlaylistModal{
		membership: make(map[string]bool),
		pending:    make(map[string]bool),
		newTitle:   ti,
	}
}

// Show displays the modal with the given playlists and item
func (m *PlaylistModal) Show(playlists []*domain.Playlist, membership map[string]bool, item *domain.MediaItem) {
	m.visible = true
	m.playlists = playlists
	m.item = item
	m.membership = membership
	m.cursor = 0
	m.createMode = false
	m.newTitle.SetValue("")
	m.newTitle.Blur()

	// Initialize pending with current membership
	m.pending = make(map[string]bool)
	for id, isMember := range membership {
		m.pending[id] = isMember
	}
}

// Hide dismisses the modal
func (m *PlaylistModal) Hide() {
	m.visible = false
	m.createMode = false
	m.newTitle.Blur()
}

// IsVisible returns whether the modal is shown
func (m *PlaylistModal) IsVisible() bool {
	return m.visible
}

// IsCreateMode returns whether we're creating a new playlist
func (m *PlaylistModal) IsCreateMode() bool {
	return m.createMode
}

// Item returns the media item being managed
func (m *PlaylistModal) Item() *domain.MediaItem {
	return m.item
}

// NewPlaylistTitle returns the title entered for new playlist creation
func (m *PlaylistModal) NewPlaylistTitle() string {
	return m.newTitle.Value()
}

// SetSize sets the modal dimensions
func (m *PlaylistModal) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// GetChanges returns the list of changes to apply (added/removed playlists)
func (m *PlaylistModal) GetChanges() []PlaylistChange {
	var changes []PlaylistChange
	for id, shouldBeMember := range m.pending {
		wasMember := m.membership[id]
		if shouldBeMember != wasMember {
			changes = append(changes, PlaylistChange{
				PlaylistID: id,
				Add:        shouldBeMember,
			})
		}
	}
	return changes
}

// HandleKeyMsg processes a key message, returns (handled, shouldClose, shouldCreate)
func (m *PlaylistModal) HandleKeyMsg(msg tea.KeyMsg) (handled bool, shouldClose bool, shouldCreate bool) {
	if !m.visible {
		return false, false, false
	}

	// Handle create mode (text input active)
	if m.createMode {
		switch {
		case key.Matches(msg, PlaylistModalKeys.Escape):
			m.createMode = false
			m.newTitle.Blur()
			m.newTitle.SetValue("")
			return true, false, false
		case key.Matches(msg, PlaylistModalKeys.Enter):
			if m.newTitle.Value() != "" {
				m.createMode = false
				m.newTitle.Blur()
				return true, false, true // Signal to create playlist
			}
			return true, false, false
		default:
			// Route to textinput
			m.newTitle, _ = m.newTitle.Update(msg)
			return true, false, false
		}
	}

	// Normal navigation mode
	switch {
	case key.Matches(msg, PlaylistModalKeys.Down):
		// +1 for the "Create new" option at the end
		maxIdx := len(m.playlists)
		if m.cursor < maxIdx {
			m.cursor++
		}
		return true, false, false
	case key.Matches(msg, PlaylistModalKeys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
		return true, false, false
	case key.Matches(msg, PlaylistModalKeys.Toggle):
		// Toggle playlist membership or enter create mode
		if m.cursor < len(m.playlists) {
			playlist := m.playlists[m.cursor]
			current := m.pending[playlist.ID]
			m.pending[playlist.ID] = !current
		} else {
			// "Create new" selected
			m.createMode = true
			m.newTitle.Focus()
		}
		return true, false, false
	case key.Matches(msg, PlaylistModalKeys.Create):
		// Quick shortcut to create new
		m.createMode = true
		m.newTitle.Focus()
		return true, false, false
	case key.Matches(msg, PlaylistModalKeys.Enter):
		// Select/toggle or confirm
		if m.cursor < len(m.playlists) {
			playlist := m.playlists[m.cursor]
			current := m.pending[playlist.ID]
			m.pending[playlist.ID] = !current
		} else {
			m.createMode = true
			m.newTitle.Focus()
		}
		return true, false, false
	case key.Matches(msg, PlaylistModalKeys.Escape) || msg.String() == "q":
		return true, true, false
	}

	return true, false, false // Consume all keys when visible
}

// HandleMouse handles mouse input for the playlist modal.
// Returns (modal, handled, dismissed) where dismissed=true if user wants to close.
func (m *PlaylistModal) HandleMouse(msg tea.MouseMsg, screenW, screenH int) (*PlaylistModal, bool, bool) {
	if !m.visible {
		return m, false, false
	}

	// Approximate modal dimensions
	modalWidth := 40
	if m.width > 0 && m.width < 60 {
		modalWidth = m.width - 10
	}
	// Height: title(1) + blank(1) + playlists + blank(1) + create(1) + blank(1) + help(1) + padding(2) + border(2)
	modalHeight := 1 + 1 + len(m.playlists) + 1 + 1 + 1 + 1 + 2 + 2
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

	insideModal := msg.X >= modalX && msg.X < modalX+modalWidth &&
		msg.Y >= modalY && msg.Y < modalY+modalHeight

	totalCount := len(m.playlists) + 1 // +1 for "Create new" option

	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		if m.cursor > 0 {
			m.cursor--
		}
		return m, true, false

	case msg.Button == tea.MouseButtonWheelDown:
		if m.cursor < totalCount-1 {
			m.cursor++
		}
		return m, true, false

	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonRight:
		m.Hide()
		return m, true, true

	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		if insideModal {
			// Items start after border(1)+padding(1)+title(1)+blank(1) = modalY + 4
			itemIdx := msg.Y - modalY - 4
			if itemIdx >= 0 && itemIdx < len(m.playlists) {
				// Toggle playlist membership
				playlist := m.playlists[itemIdx]
				current := m.pending[playlist.ID]
				m.pending[playlist.ID] = !current
				m.cursor = itemIdx
				return m, true, false
			}
			// A blank line separates the playlists from the create option.
			if itemIdx == len(m.playlists)+1 {
				// "Create new" option
				m.createMode = true
				m.newTitle.Focus()
				m.cursor = len(m.playlists)
				return m, true, false
			}
			return m, true, false
		}
		// Click outside modal — dismiss and close
		m.Hide()
		return m, true, true
	}

	return m, false, false
}

// View renders the playlist modal
func (m *PlaylistModal) View() string {
	if !m.visible {
		return ""
	}

	// Modal width
	modalWidth := 40
	if m.width > 0 && m.width < 60 {
		modalWidth = m.width - 10
	}

	var lines []string

	// Title
	title := "Manage Playlists"
	if m.item != nil {
		itemTitle := m.item.Title
		if len(itemTitle) > 25 {
			itemTitle = itemTitle[:22] + "..."
		}
		title = "Add to Playlist: " + itemTitle
	}
	titleLine := styles.ModalTitleStyle.Render(title)
	lines = append(lines, titleLine)
	lines = append(lines, "")

	// Playlist items with checkboxes
	for i, playlist := range m.playlists {
		selected := i == m.cursor
		isMember := m.pending[playlist.ID]

		// Checkbox
		checkbox := "[ ]"
		if isMember {
			checkbox = "[x]"
		}

		line := checkbox + " " + playlist.Title

		if selected {
			line = lipgloss.NewStyle().
				Foreground(styles.White).
				Background(styles.SlateLight).
				Render(styles.Pad(line, modalWidth-4))
		} else if isMember {
			line = lipgloss.NewStyle().
				Foreground(styles.PlexOrange).
				Render(styles.Pad(line, modalWidth-4))
		} else {
			line = lipgloss.NewStyle().
				Foreground(styles.LightGray).
				Render(styles.Pad(line, modalWidth-4))
		}
		lines = append(lines, "  "+line)
	}

	// "Create new playlist" option
	createSelected := m.cursor == len(m.playlists)
	createLine := "[+] Create new playlist..."
	if m.createMode {
		createLine = m.newTitle.View()
	}
	if createSelected && !m.createMode {
		createLine = lipgloss.NewStyle().
			Foreground(styles.White).
			Background(styles.SlateLight).
			Render(styles.Pad(createLine, modalWidth-4))
	} else {
		createLine = lipgloss.NewStyle().
			Foreground(styles.DimGray).
			Render(styles.Pad(createLine, modalWidth-4))
	}
	lines = append(lines, "")
	lines = append(lines, "  "+createLine)

	// Help text
	lines = append(lines, "")
	helpText := styles.DimStyle.Render("Space: Toggle  n: New  Esc: Done")
	lines = append(lines, helpText)

	content := strings.Join(lines, "\n")

	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.PlexOrange).
		Background(styles.SlateDark).
		Padding(1, 2).
		Width(modalWidth).
		Render(content)

	return modal
}
