package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/SuperCoolPencil/cue/internal/config"
	"github.com/SuperCoolPencil/cue/internal/mediaserver"
	"github.com/SuperCoolPencil/cue/internal/tui/styles"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

var plexctlTitleStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(styles.Green).
	MarginBottom(1)

var plexctlItemStyle = lipgloss.NewStyle().PaddingLeft(4)

var plexctlSelectedItemStyle = lipgloss.NewStyle().
	PaddingLeft(2).
	Foreground(styles.Green)

// runDiscover implements `cue discover`: it lists the Plex servers reachable by
// the configured account and switches the active server to the one selected.
// A specific server can be chosen non-interactively with --select <N> (1-based).
func runDiscover(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("discover", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var selectIdx int
	fs.IntVar(&selectIdx, "select", -1, "select server by 1-based index (non-interactive)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: failed to load config: %v\n", err)
		return 1
	}
	if !cfg.IsConfigured() {
		_, _ = fmt.Fprintln(stderr, "Error: cue is not configured. Run cue to set up your server first.")
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	servers, err := mediaserver.DiscoverPlexServers(ctx, cfg)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if len(servers) == 0 {
		_, _ = fmt.Fprintln(stderr, "No Plex servers found for this account.")
		return 1
	}

	var chosen int
	if selectIdx >= 0 {
		if selectIdx < 1 || selectIdx > len(servers) {
			_, _ = fmt.Fprintf(stderr, "Error: --select %d out of range (1-%d)\n", selectIdx, len(servers))
			return 1
		}
		chosen = selectIdx - 1
	} else {
		if term.IsTerminal(int(os.Stdin.Fd())) {
			chosen, err = selectPlexctlStyle(servers, stdout)
		} else {
			chosen, err = -1, nil
		}
		if err != nil || chosen < 0 {
			chosen = promptServerSelection(stdout, stderr, servers)
		}
		if chosen < 0 {
			_, _ = fmt.Fprintln(stderr, "Aborted.")
			return 1
		}
	}

	sel := servers[chosen]
	cfg.Server.URL = sel.URI
	if sel.Token != "" {
		cfg.Server.Token = sel.Token
	}
	cfg.Server.Type = config.SourceTypePlex

	if err := config.SaveConfig(cfg); err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: failed to save config: %v\n", err)
		return 1
	}

	owner := ""
	if !sel.Owned {
		owner = " (shared)"
	}
	_, _ = fmt.Fprintf(stdout, "Switched to server: %s%s (%s)\n", sel.Name, owner, sel.URI)
	return 0
}

// plexctlSelectorItem is a list.Item compatible with bubbletea list.
type plexctlSelectorItem struct {
	index int
	title string
	desc  string
}

func (i plexctlSelectorItem) Title() string       { return i.title }
func (i plexctlSelectorItem) Description() string { return i.desc }
func (i plexctlSelectorItem) FilterValue() string { return i.title + " " + i.desc }

type plexctlSelectorDelegate struct{}

func (d plexctlSelectorDelegate) Height() int                               { return 1 }
func (d plexctlSelectorDelegate) Spacing() int                              { return 0 }
func (d plexctlSelectorDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }
func (d plexctlSelectorDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(plexctlSelectorItem)
	if !ok {
		return
	}
	str := fmt.Sprintf("%d. %s (%s)", index+1, i.title, i.desc)
	fn := plexctlItemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return plexctlSelectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}
	_, _ = fmt.Fprint(w, fn(str))
}

type plexctlSelectorModel struct {
	list     list.Model
	choice   int
	quitting bool
}

func (m plexctlSelectorModel) Init() tea.Cmd { return nil }

func (m plexctlSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width)
		return m, nil
	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "ctrl+c", "q", "esc":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			m.choice = m.list.Index()
			m.quitting = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m plexctlSelectorModel) View() string {
	if m.quitting {
		return ""
	}
	return "\n" + m.list.View()
}

// selectPlexctlStyle presents a plexctl-like interactive list and returns the
// 0-based index of the chosen connection, or -1 if aborted.
func selectPlexctlStyle(servers []mediaserver.DiscoveredServer, stdout io.Writer) (int, error) {
	items := make([]list.Item, 0, len(servers))
	for i, s := range servers {
		desc := s.URI
		if s.Local {
			desc += " (Local)"
		}
		items = append(items, plexctlSelectorItem{index: i, title: s.Name, desc: desc})
	}

	l := list.New(items, plexctlSelectorDelegate{}, 60, 10)
	l.Title = "Select a Plex Server to use"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = plexctlTitleStyle

	m := plexctlSelectorModel{list: l}
	p := tea.NewProgram(m, tea.WithInput(os.Stdin), tea.WithOutput(stdout))
	finalModel, err := p.Run()
	if err != nil {
		return -1, err
	}
	res := finalModel.(plexctlSelectorModel)
	if res.quitting && res.choice >= 0 {
		return res.choice, nil
	}
	return -1, nil
}

// promptServerSelection is the non-interactive fallback used when stdin is not a
// terminal. It prints the discovered server connections and reads a 1-based selection.
func promptServerSelection(stdout, stderr io.Writer, servers []mediaserver.DiscoveredServer) int {
	_, _ = fmt.Fprintln(stdout, "Available Plex server connections:")
	for i, s := range servers {
		owner := ""
		if !s.Owned {
			owner = " (shared)"
		}
		loc := ""
		if s.Local {
			loc = " [local]"
		}
		_, _ = fmt.Fprintf(stdout, "  %d) %s%s\n     %s%s\n", i+1, s.Name, owner, s.URI, loc)
	}

	_, _ = fmt.Fprint(stdout, "Select a server connection [1]: ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "Error: failed to read selection.")
		return -1
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return 0
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(servers) {
		_, _ = fmt.Fprintln(stderr, "Invalid selection.")
		return -1
	}
	return n - 1
}
