package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/user/xhub/internal/config"
	"github.com/user/xhub/internal/db"
	"github.com/user/xhub/internal/indexer"
)

type model struct {
	cfg         *config.Config
	store       *db.Store
	searchInput textinput.Model
	list        list.Model
	bookmarks   []db.Bookmark
	sources     map[string]bool // Source filter toggles
	width       int
	height      int
	searching   bool
	err         error
}

type bookmarkItem struct {
	bookmark db.Bookmark
}

func (b bookmarkItem) Title() string {
	icon := sourceIcon(b.bookmark.Source)
	return fmt.Sprintf("%s %s", icon, b.bookmark.Title)
}

func (b bookmarkItem) Description() string {
	if b.bookmark.Summary != "" {
		summary := b.bookmark.Summary
		if len(summary) > 80 {
			summary = summary[:80] + "..."
		}
		return summary
	}
	return b.bookmark.URL
}

func (b bookmarkItem) FilterValue() string {
	return b.bookmark.Title + " " + b.bookmark.Summary + " " + b.bookmark.Keywords
}

func sourceIcon(source string) string {
	switch source {
	case "x":
		return "[X]"
	case "raindrop":
		return "[R]"
	case "github":
		return "[G]"
	case "manual":
		return "[M]"
	default:
		return "[?]"
	}
}

func initialModel(cfg *config.Config) model {
	ti := textinput.New()
	ti.Placeholder = "Search bookmarks..."
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 50

	delegate := list.NewDefaultDelegate()
	l := list.New([]list.Item{}, delegate, 0, 0)
	l.Title = "XHub"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(true)

	return model{
		cfg:         cfg,
		searchInput: ti,
		list:        l,
		sources: map[string]bool{
			"x":        true,
			"raindrop": true,
			"github":   true,
			"manual":   true,
		},
		searching: true,
	}
}

type initMsg struct {
	store     *db.Store
	bookmarks []db.Bookmark
	err       error
}

type searchMsg struct {
	bookmarks []db.Bookmark
	err       error
}

type refreshMsg struct {
	err error
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.initStore,
	)
}

func (m model) initStore() tea.Msg {
	store, err := db.NewStore(m.cfg.DataDir)
	if err != nil {
		return initMsg{err: err}
	}

	// Check if refresh needed
	lastRefresh, _ := store.GetMetadata("last_refresh_at")
	needsRefresh := true
	if lastRefresh != "" {
		if t, err := time.Parse(time.RFC3339, lastRefresh); err == nil {
			needsRefresh = time.Since(t) >= 24*time.Hour
		}
	}

	if needsRefresh {
		// Run refresh in background
		go func() {
			indexer.Fetch(m.cfg, false)
		}()
	}

	bookmarks, err := store.List(nil, 100)
	if err != nil {
		return initMsg{store: store, err: err}
	}

	return initMsg{store: store, bookmarks: bookmarks}
}

func (m model) doSearch(query string) tea.Cmd {
	return func() tea.Msg {
		if m.store == nil {
			return searchMsg{err: fmt.Errorf("store not initialized")}
		}

		bookmarks, err := m.store.Search(query, 50)
		return searchMsg{bookmarks: bookmarks, err: err}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if !m.searching {
				return m, tea.Quit
			}
		case "esc":
			if m.searching {
				m.searching = false
				m.searchInput.Blur()
			}
		case "/":
			if !m.searching {
				m.searching = true
				m.searchInput.Focus()
				return m, textinput.Blink
			}
		case "enter":
			if m.searching {
				m.searching = false
				m.searchInput.Blur()
				return m, m.doSearch(m.searchInput.Value())
			}
			// Open edit modal (placeholder)
		case "j", "down":
			if !m.searching {
				m.list.CursorDown()
				return m, nil
			}
		case "k", "up":
			if !m.searching {
				m.list.CursorUp()
				return m, nil
			}
		case "g":
			if !m.searching {
				m.list.Select(0)
				return m, nil
			}
		case "G":
			if !m.searching {
				items := m.list.Items()
				if len(items) > 0 {
					m.list.Select(len(items) - 1)
				}
				return m, nil
			}
		case "o":
			if !m.searching {
				if item, ok := m.list.SelectedItem().(bookmarkItem); ok {
					openBrowser(item.bookmark.URL)
				}
			}
		case "d":
			if !m.searching {
				// Delete with confirmation (placeholder)
			}
		case "1":
			m.sources["x"] = !m.sources["x"]
			return m, m.filterResults
		case "2":
			m.sources["raindrop"] = !m.sources["raindrop"]
			return m, m.filterResults
		case "3":
			m.sources["github"] = !m.sources["github"]
			return m, m.filterResults
		case "4":
			m.sources["manual"] = !m.sources["manual"]
			return m, m.filterResults
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height-6)
		m.searchInput.Width = msg.Width - 20

	case initMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.store = msg.store
		m.bookmarks = msg.bookmarks
		m.list.SetItems(m.bookmarksToItems(msg.bookmarks))

	case searchMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.bookmarks = msg.bookmarks
		m.list.SetItems(m.bookmarksToItems(msg.bookmarks))

	case refreshMsg:
		if msg.err != nil {
			m.err = msg.err
		}
		return m, m.doSearch(m.searchInput.Value())
	}

	if m.searching {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		cmds = append(cmds, cmd)

		// Live search on input change
		if len(m.searchInput.Value()) > 0 {
			cmds = append(cmds, m.doSearch(m.searchInput.Value()))
		}
	} else {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m model) filterResults() tea.Msg {
	filtered := make([]db.Bookmark, 0)
	for _, b := range m.bookmarks {
		if m.sources[b.Source] {
			filtered = append(filtered, b)
		}
	}
	return searchMsg{bookmarks: filtered}
}

func (m model) bookmarksToItems(bookmarks []db.Bookmark) []list.Item {
	items := make([]list.Item, 0, len(bookmarks))
	for _, b := range bookmarks {
		if m.sources[b.Source] {
			items = append(items, bookmarkItem{bookmark: b})
		}
	}
	return items
}

func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress q to quit.", m.err)
	}

	var b strings.Builder

	// Header with search and filters
	searchStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1)

	filterStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	activeFilter := lipgloss.NewStyle().
		Foreground(lipgloss.Color("86")).
		Bold(true)

	inactiveFilter := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	filters := []string{}
	for _, s := range []struct{ key, label string }{
		{"x", "[X]"},
		{"raindrop", "[R]"},
		{"github", "[G]"},
		{"manual", "[M]"},
	} {
		if m.sources[s.key] {
			filters = append(filters, activeFilter.Render(s.label))
		} else {
			filters = append(filters, inactiveFilter.Render(s.label))
		}
	}

	searchBox := searchStyle.Render(m.searchInput.View())
	filterBar := filterStyle.Render(strings.Join(filters, " "))

	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, searchBox, "  ", filterBar))
	b.WriteString("\n\n")

	// List
	b.WriteString(m.list.View())

	// Help
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		MarginTop(1)

	help := "[j/k]nav [g/G]top/end [/]search [o]pen [Enter]edit [d]elete [1-4]filters [q]uit"
	b.WriteString(helpStyle.Render(help))

	return b.String()
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	}
	if cmd != nil {
		cmd.Start()
	}
}

// Run starts the TUI application
func Run(cfg *config.Config) error {
	p := tea.NewProgram(initialModel(cfg), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
