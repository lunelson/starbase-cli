package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func TestRepoItemInterface(t *testing.T) {
	item := RepoItem{
		ID:       1,
		FullName: "owner/repo",
		Desc:     "A test repository",
		Selected: false,
	}

	if item.Title() != "[ ] owner/repo" {
		t.Errorf("expected '[ ] owner/repo', got %q", item.Title())
	}

	item.Selected = true
	if item.Title() != "[✓] owner/repo" {
		t.Errorf("expected '[✓] owner/repo', got %q", item.Title())
	}

	if item.Description() != "A test repository" {
		t.Errorf("expected 'A test repository', got %q", item.Description())
	}

	if item.FilterValue() != "owner/repo A test repository" {
		t.Errorf("unexpected filter value: %q", item.FilterValue())
	}
}

func TestKeyMapShortHelp(t *testing.T) {
	keys := defaultKeyMap()
	short := keys.ShortHelp()

	if len(short) != 2 {
		t.Errorf("expected 2 short help bindings, got %d", len(short))
	}
}

func TestKeyMapFullHelp(t *testing.T) {
	keys := defaultKeyMap()
	full := keys.FullHelp()

	if len(full) != 4 {
		t.Errorf("expected 4 help groups, got %d", len(full))
	}

	totalBindings := 0
	for _, group := range full {
		totalBindings += len(group)
	}

	if totalBindings < 10 {
		t.Errorf("expected at least 10 bindings, got %d", totalBindings)
	}
}

func TestViewStateTransitions(t *testing.T) {
	tests := []struct {
		name      string
		initial   viewState
		keyPress  string
		wantState viewState
	}{
		{
			name:      "list to search",
			initial:   listView,
			keyPress:  "/",
			wantState: searchView,
		},
		{
			name:      "any to help",
			initial:   listView,
			keyPress:  "?",
			wantState: helpView,
		},
		{
			name:      "help back to list",
			initial:   helpView,
			keyPress:  "?",
			wantState: listView,
		},
		{
			name:      "search back with esc",
			initial:   searchView,
			keyPress:  "esc",
			wantState: listView,
		},
		{
			name:      "detail back with esc",
			initial:   detailView,
			keyPress:  "esc",
			wantState: listView,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := createTestModel()
			m.state = tt.initial
			if tt.initial == helpView {
				m.prevState = listView
			}

			msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.keyPress)}
			if tt.keyPress == "esc" {
				msg = tea.KeyMsg{Type: tea.KeyEscape}
			}

			newModel, _ := m.Update(msg)
			updated := newModel.(Model)

			if updated.state != tt.wantState {
				t.Errorf("expected state %v, got %v", tt.wantState, updated.state)
			}
		})
	}
}

func TestMultiSelect(t *testing.T) {
	m := createTestModel()
	m.list.SetItems([]list.Item{
		RepoItem{ID: 1, FullName: "a/a"},
		RepoItem{ID: 2, FullName: "b/b"},
		RepoItem{ID: 3, FullName: "c/c"},
	})

	if m.SelectedCount() != 0 {
		t.Errorf("expected 0 selected, got %d", m.SelectedCount())
	}

	m.selected[1] = true
	if m.SelectedCount() != 1 {
		t.Errorf("expected 1 selected, got %d", m.SelectedCount())
	}

	for _, item := range m.list.Items() {
		if repo, ok := item.(RepoItem); ok {
			m.selected[repo.ID] = true
		}
	}
	if m.SelectedCount() != 3 {
		t.Errorf("expected 3 selected, got %d", m.SelectedCount())
	}

	items := m.SelectedItems()
	if len(items) != 3 {
		t.Errorf("expected 3 selected items, got %d", len(items))
	}

	m.selected = make(map[int64]bool)
	if m.SelectedCount() != 0 {
		t.Errorf("expected 0 selected after clear, got %d", m.SelectedCount())
	}
}

func TestKeyBindingMatches(t *testing.T) {
	keys := defaultKeyMap()

	tests := []struct {
		keyStr  string
		binding key.Binding
	}{
		{"j", keys.Down},
		{"k", keys.Up},
		{" ", keys.Select},
		{"a", keys.SelectAll},
		{"n", keys.SelectNone},
		{"/", keys.Search},
		{"?", keys.Help},
		{"q", keys.Quit},
		{"e", keys.OpenEditor},
		{"w", keys.OpenWeb},
		{"y", keys.CopyPath},
	}

	for _, tt := range tests {
		t.Run(tt.keyStr, func(t *testing.T) {
			msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.keyStr)}
			if !key.Matches(msg, tt.binding) {
				t.Errorf("key %q should match binding", tt.keyStr)
			}
		})
	}
}

func TestWindowSizeMsg(t *testing.T) {
	m := createTestModel()

	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	newModel, _ := m.Update(msg)
	updated := newModel.(Model)

	if updated.width != 120 {
		t.Errorf("expected width 120, got %d", updated.width)
	}
	if updated.height != 40 {
		t.Errorf("expected height 40, got %d", updated.height)
	}
}

func createTestModel() Model {
	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.CharLimit = 100

	return Model{
		state:          listView,
		keys:           defaultKeyMap(),
		list:           list.New([]list.Item{}, list.NewDefaultDelegate(), 80, 24),
		searchInput:    ti,
		selected:       make(map[int64]bool),
		detailViewport: viewport.New(80, 20),
		width:          80,
		height:         24,
	}
}
