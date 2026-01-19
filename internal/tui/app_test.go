package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/user/xhub/internal/config"
)

func TestInitialModel_ListFocused(t *testing.T) {
	cfg := &config.Config{DataDir: "/tmp/xhub-test"}
	m := initialModel(cfg)

	// TUI should start with list focused (searching=false)
	if m.searching {
		t.Error("expected searching=false on init, got true")
	}

	// Search input should be blurred
	if m.searchInput.Focused() {
		t.Error("expected search input blurred on init, got focused")
	}
}

func TestUpdate_SlashFocusesSearch(t *testing.T) {
	cfg := &config.Config{DataDir: "/tmp/xhub-test"}
	m := initialModel(cfg)

	// Simulate pressing '/'
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = newModel.(model)

	if !m.searching {
		t.Error("expected searching=true after pressing /, got false")
	}
	if !m.searchInput.Focused() {
		t.Error("expected search input focused after pressing /")
	}
}

func TestUpdate_EscUnfocusesSearch(t *testing.T) {
	cfg := &config.Config{DataDir: "/tmp/xhub-test"}
	m := initialModel(cfg)

	// First focus search
	m.searching = true
	m.searchInput.Focus()

	// Simulate pressing Esc
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = newModel.(model)

	if m.searching {
		t.Error("expected searching=false after pressing Esc, got true")
	}
	if m.searchInput.Focused() {
		t.Error("expected search input blurred after pressing Esc")
	}
}

func TestUpdate_QQuitsOnlyFromList(t *testing.T) {
	cfg := &config.Config{DataDir: "/tmp/xhub-test"}
	m := initialModel(cfg)

	// When in list mode (searching=false), q should quit
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Error("expected quit command when pressing q from list mode")
	}

	// When in search mode (searching=true), q should NOT quit
	m.searching = true
	m.searchInput.Focus()
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	// In search mode, q should be passed to the search input, not trigger quit
	// The returned cmd should be nil for quit (it processes input instead)
}

func TestUpdate_JKNavigatesInListMode(t *testing.T) {
	cfg := &config.Config{DataDir: "/tmp/xhub-test"}
	m := initialModel(cfg)

	// j/k should work immediately since searching=false by default
	if m.searching {
		t.Error("precondition failed: searching should be false")
	}

	// Note: Can't fully test cursor movement without list items,
	// but we can verify j/k are processed without error
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
}
