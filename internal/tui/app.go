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
	allBookmarks []db.Bookmark   // Unfiltered search results
	sources      map[string]bool // Source filter toggles
	width        int
	height       int
	searching    bool
	err          error

	// Edit modal state
	editing       bool
	editBookmark  *db.Bookmark
	editFields    []textinput.Model // 0=title, 1=summary, 2=keywords, 3=notes
	editFocusIdx  int

	// Delete confirmation state
	deleting       bool
	deleteBookmark *db.Bookmark
}

type bookmarkItem struct {
	bookmark db.Bookmark
}

func (b bookmarkItem) Title() string {
	icon := sourceIcon(b.bookmark.Source)
	title := sanitizeLine(b.bookmark.Title)
	return fmt.Sprintf("%s %s", icon, title)
}

// sanitizeLine removes newlines and collapses whitespace to ensure single-line display
func sanitizeLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	// Collapse multiple spaces
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

func (b bookmarkItem) Description() string {
	if b.bookmark.Summary != "" {
		summary := sanitizeLine(b.bookmark.Summary)
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
	// Start with list focused, not search input
	ti.Blur()
	ti.CharLimit = 256
	ti.Width = 50

	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(2) // Fixed height: title + description
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
		searching: false, // Start with list focused
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

type editSaveMsg struct {
	bookmark *db.Bookmark
	err      error
}

type deleteMsg struct {
	id  string
	err error
}

func (m model) Init() tea.Cmd {
	return m.initStore
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
		// If showing error, only handle quit
		if m.err != nil {
			if msg.String() == "q" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "q":
			if !m.searching {
				return m, tea.Quit
			}
		case "esc":
			if m.editing {
				m.editing = false
				m.editBookmark = nil
				m.editFields = nil
				return m, nil
			}
			if m.searching {
				m.searching = false
				m.searchInput.Blur()
				return m, nil
			}
		case "tab":
			if m.editing {
				m.editFields[m.editFocusIdx].Blur()
				m.editFocusIdx = (m.editFocusIdx + 1) % len(m.editFields)
				m.editFields[m.editFocusIdx].Focus()
				return m, textinput.Blink
			}
		case "shift+tab":
			if m.editing {
				m.editFields[m.editFocusIdx].Blur()
				m.editFocusIdx = (m.editFocusIdx - 1 + len(m.editFields)) % len(m.editFields)
				m.editFields[m.editFocusIdx].Focus()
				return m, textinput.Blink
			}
		case "/":
			if !m.searching && !m.editing {
				m.searching = true
				m.searchInput.Focus()
				return m, nil
			}
		case "enter":
			if m.editing {
				// Save and close edit modal
				return m, m.saveEdit()
			}
			if m.searching {
				m.searching = false
				m.searchInput.Blur()
				return m, m.doSearch(m.searchInput.Value())
			}
			// Open edit modal for selected bookmark
			if item, ok := m.list.SelectedItem().(bookmarkItem); ok {
				m.editing = true
				bm := item.bookmark
				m.editBookmark = &bm
				m.editFields = m.createEditFields(&bm)
				m.editFocusIdx = 0
				m.editFields[0].Focus()
				return m, textinput.Blink
			}
		case "j", "down":
			if !m.searching && !m.editing {
				m.list.CursorDown()
				return m, nil
			}
		case "k", "up":
			if !m.searching && !m.editing {
				m.list.CursorUp()
				return m, nil
			}
		case "g":
			if !m.searching && !m.editing {
				m.list.Select(0)
				return m, nil
			}
		case "G":
			if !m.searching && !m.editing {
				items := m.list.Items()
				if len(items) > 0 {
					m.list.Select(len(items) - 1)
				}
				return m, nil
			}
		case "o":
			if !m.searching && !m.editing {
				if item, ok := m.list.SelectedItem().(bookmarkItem); ok {
					openBrowser(item.bookmark.URL)
				}
			}
		case "d":
			if !m.searching && !m.editing && !m.deleting {
				if item, ok := m.list.SelectedItem().(bookmarkItem); ok {
					m.deleting = true
					bm := item.bookmark
					m.deleteBookmark = &bm
				}
			}
		case "y":
			if m.deleting && m.deleteBookmark != nil {
				return m, m.doDelete(m.deleteBookmark.ID)
			}
		case "n":
			if m.deleting {
				m.deleting = false
				m.deleteBookmark = nil
			}
		case "1":
			if !m.editing {
				m.sources["x"] = !m.sources["x"]
				return m, m.filterResults
			}
		case "2":
			if !m.editing {
				m.sources["raindrop"] = !m.sources["raindrop"]
				return m, m.filterResults
			}
		case "3":
			if !m.editing {
				m.sources["github"] = !m.sources["github"]
				return m, m.filterResults
			}
		case "4":
			if !m.editing {
				m.sources["manual"] = !m.sources["manual"]
				return m, m.filterResults
			}
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
		m.allBookmarks = msg.bookmarks
		m.list.SetItems(m.bookmarksToItems(msg.bookmarks))
		return m, nil

	case searchMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.allBookmarks = msg.bookmarks
		m.list.SetItems(m.bookmarksToItems(msg.bookmarks))
		return m, nil

	case refreshMsg:
		if msg.err != nil {
			m.err = msg.err
		}
		return m, m.doSearch(m.searchInput.Value())

	case filterMsg:
		// Re-filter from allBookmarks (non-destructive)
		m.list.SetItems(m.bookmarksToItems(m.allBookmarks))
		return m, nil

	case editSaveMsg:
		m.editing = false
		m.editBookmark = nil
		m.editFields = nil
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		// Update bookmark in allBookmarks list
		for i, b := range m.allBookmarks {
			if b.ID == msg.bookmark.ID {
				m.allBookmarks[i] = *msg.bookmark
				break
			}
		}
		m.list.SetItems(m.bookmarksToItems(m.allBookmarks))
		return m, nil

	case deleteMsg:
		m.deleting = false
		m.deleteBookmark = nil
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		// Remove bookmark from allBookmarks list
		newBookmarks := make([]db.Bookmark, 0, len(m.allBookmarks)-1)
		for _, b := range m.allBookmarks {
			if b.ID != msg.id {
				newBookmarks = append(newBookmarks, b)
			}
		}
		m.allBookmarks = newBookmarks
		m.list.SetItems(m.bookmarksToItems(m.allBookmarks))
		return m, nil
	}

	if m.editing {
		// Update focused edit field
		if m.editFields != nil && m.editFocusIdx < len(m.editFields) {
			var cmd tea.Cmd
			m.editFields[m.editFocusIdx], cmd = m.editFields[m.editFocusIdx].Update(msg)
			cmds = append(cmds, cmd)
		}
	} else if m.searching {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		cmds = append(cmds, cmd)

		// Live search on input change (including when empty to restore full list)
		cmds = append(cmds, m.doSearch(m.searchInput.Value()))
	} else {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

type filterMsg struct{}

func (m model) filterResults() tea.Msg {
	return filterMsg{}
}

func (m model) createEditFields(b *db.Bookmark) []textinput.Model {
	fields := make([]textinput.Model, 4)

	// Title
	fields[0] = textinput.New()
	fields[0].Placeholder = "Title"
	fields[0].SetValue(b.Title)
	fields[0].CharLimit = 256
	fields[0].Width = 60

	// Summary
	fields[1] = textinput.New()
	fields[1].Placeholder = "Summary"
	fields[1].SetValue(b.Summary)
	fields[1].CharLimit = 500
	fields[1].Width = 60

	// Keywords
	fields[2] = textinput.New()
	fields[2].Placeholder = "Keywords (comma-separated)"
	fields[2].SetValue(b.Keywords)
	fields[2].CharLimit = 256
	fields[2].Width = 60

	// Notes
	fields[3] = textinput.New()
	fields[3].Placeholder = "Notes"
	fields[3].SetValue(b.Notes)
	fields[3].CharLimit = 500
	fields[3].Width = 60

	return fields
}

func (m model) saveEdit() tea.Cmd {
	// Capture values before closure to avoid race conditions
	if m.editBookmark == nil || m.store == nil || m.editFields == nil {
		return func() tea.Msg {
			return editSaveMsg{err: fmt.Errorf("no bookmark to save")}
		}
	}

	// Copy values from fields
	bm := *m.editBookmark
	bm.Title = m.editFields[0].Value()
	bm.Summary = m.editFields[1].Value()
	bm.Keywords = m.editFields[2].Value()
	bm.Notes = m.editFields[3].Value()
	store := m.store

	return func() tea.Msg {
		err := store.Update(&bm)
		if err != nil {
			return editSaveMsg{err: err}
		}
		return editSaveMsg{bookmark: &bm}
	}
}

func (m model) doDelete(id string) tea.Cmd {
	store := m.store
	return func() tea.Msg {
		if store == nil {
			return deleteMsg{err: fmt.Errorf("store not initialized")}
		}
		err := store.Delete(id)
		if err != nil {
			return deleteMsg{err: err}
		}
		return deleteMsg{id: id}
	}
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

	// Edit modal overlay
	if m.editing && m.editBookmark != nil {
		return m.renderEditModal()
	}

	// Delete confirmation overlay
	if m.deleting && m.deleteBookmark != nil {
		return m.renderDeleteConfirm()
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

func (m model) renderEditModal() string {
	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(70)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")).
		MarginBottom(1)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Width(12)

	focusedLabel := lipgloss.NewStyle().
		Foreground(lipgloss.Color("86")).
		Bold(true).
		Width(12)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		MarginTop(1)

	var content strings.Builder

	content.WriteString(titleStyle.Render("Edit Bookmark"))
	content.WriteString("\n")
	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(m.editBookmark.URL))
	content.WriteString("\n\n")

	labels := []string{"Title:", "Summary:", "Keywords:", "Notes:"}
	for i, field := range m.editFields {
		var label string
		if i == m.editFocusIdx {
			label = focusedLabel.Render(labels[i])
		} else {
			label = labelStyle.Render(labels[i])
		}
		content.WriteString(label)
		content.WriteString(field.View())
		content.WriteString("\n")
	}

	content.WriteString(helpStyle.Render("[Tab]next [Shift+Tab]prev [Enter]save [Esc]cancel"))

	return modalStyle.Render(content.String())
}

func (m model) renderDeleteConfirm() string {
	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("196")). // Red border for delete
		Padding(1, 2).
		Width(60)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("196")).
		MarginBottom(1)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		MarginTop(1)

	var content strings.Builder

	content.WriteString(titleStyle.Render("Delete Bookmark?"))
	content.WriteString("\n\n")

	title := m.deleteBookmark.Title
	if len(title) > 50 {
		title = title[:50] + "..."
	}
	content.WriteString(title)
	content.WriteString("\n")
	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(m.deleteBookmark.URL))
	content.WriteString("\n\n")

	content.WriteString(helpStyle.Render("[y]es [n]o"))

	return modalStyle.Render(content.String())
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
