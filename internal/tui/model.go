package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/lunelson/starbase-cli/internal/database"
	"github.com/lunelson/starbase-cli/internal/search"
)

// viewState represents the current view
type viewState int

const (
	listView viewState = iota
	searchView
	detailView
	helpView
)

// RepoItem implements list.Item for the TUI list
type RepoItem struct {
	ID        int64
	Forge     string
	FullName  string
	Desc      string
	Language  string
	LocalPath string
	WebURL    string
	Stars     int
	Topics    []string
	Selected  bool
}

func (r RepoItem) Title() string {
	checkbox := "[ ] "
	if r.Selected {
		checkbox := "[✓] "
		return checkbox + r.FullName
	}
	return checkbox + r.FullName
}

func (r RepoItem) Description() string { return r.Desc }
func (r RepoItem) FilterValue() string { return r.FullName + " " + r.Desc }

// Model is the main TUI model
type Model struct {
	db       *database.DB
	searcher *search.Searcher

	// View state
	state     viewState
	prevState viewState // for returning from help overlay
	keys      keyMap

	// List view
	list        list.Model
	searchInput textinput.Model
	selected    map[int64]bool

	// Detail view
	detailRepo     *RepoItem
	detailViewport viewport.Model
	detailContent  string

	// Dimensions
	width  int
	height int
	err    error
}

// New creates a new TUI model
func New(db *database.DB) Model {
	delegate := list.NewDefaultDelegate()
	l := list.New([]list.Item{}, delegate, 0, 0)
	l.Title = "starbase"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = titleStyle

	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.CharLimit = 100

	vp := viewport.New(0, 0)

	return Model{
		db:             db,
		searcher:       search.New(db.DB),
		state:          listView,
		keys:           defaultKeyMap(),
		list:           l,
		searchInput:    ti,
		selected:       make(map[int64]bool),
		detailViewport: vp,
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return m.loadRepos
}

func (m Model) loadRepos() tea.Msg {
	repos, err := m.db.ListRepos("")
	if err != nil {
		return errMsg{err}
	}

	items := make([]list.Item, 0, len(repos))
	for _, r := range repos {
		item := RepoItem{
			ID:       r.ID,
			Forge:    r.Forge,
			FullName: r.FullName,
			WebURL:   r.WebURL,
		}
		if r.LocalPath != nil {
			item.LocalPath = *r.LocalPath
		}

		meta, _ := m.db.GetMetadata(r.ID)
		if meta != nil {
			if meta.Description != nil {
				item.Desc = *meta.Description
			}
			if meta.Language != nil {
				item.Language = *meta.Language
			}
			if meta.StarsCount != nil {
				item.Stars = *meta.StarsCount
			}
			item.Topics = meta.Topics
		}

		items = append(items, item)
	}

	return reposLoadedMsg{items}
}

// Message types
type reposLoadedMsg struct {
	items []list.Item
}

type searchResultsMsg struct {
	items []list.Item
}

type detailLoadedMsg struct {
	readme string
}

type errMsg struct {
	err error
}

// Update handles events
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		h, v := appStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
		m.detailViewport.Width = msg.Width - h - 4
		m.detailViewport.Height = msg.Height - v - 8

	case tea.KeyMsg:
		cmd := m.handleKeyMsg(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case reposLoadedMsg:
		m.list.SetItems(msg.items)

	case searchResultsMsg:
		m.list.SetItems(msg.items)
		m.state = listView
		m.searchInput.Blur()

	case detailLoadedMsg:
		m.detailContent = m.renderDetailContent(msg.readme)
		m.detailViewport.SetContent(m.detailContent)

	case errMsg:
		m.err = msg.err
	}

	// Update sub-models based on state
	switch m.state {
	case searchView:
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		cmds = append(cmds, cmd)
	case detailView:
		var cmd tea.Cmd
		m.detailViewport, cmd = m.detailViewport.Update(msg)
		cmds = append(cmds, cmd)
	case listView:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) handleKeyMsg(msg tea.KeyMsg) tea.Cmd {
	// Global keys (work in any state)
	switch {
	case key.Matches(msg, m.keys.Quit):
		if m.state == helpView {
			m.state = m.prevState
			return nil
		}
		if m.state == detailView {
			m.state = listView
			m.detailRepo = nil
			return nil
		}
		if m.state == searchView {
			m.state = listView
			m.searchInput.Blur()
			m.searchInput.SetValue("")
			return nil
		}
		return tea.Quit

	case key.Matches(msg, m.keys.Help):
		if m.state == helpView {
			m.state = m.prevState
		} else {
			m.prevState = m.state
			m.state = helpView
		}
		return nil

	case key.Matches(msg, m.keys.Back):
		switch m.state {
		case helpView:
			m.state = m.prevState
		case detailView:
			m.state = listView
			m.detailRepo = nil
		case searchView:
			m.state = listView
			m.searchInput.Blur()
			m.searchInput.SetValue("")
			return m.loadRepos
		}
		return nil
	}

	// State-specific keys
	switch m.state {
	case listView:
		return m.handleListKeys(msg)
	case searchView:
		return m.handleSearchKeys(msg)
	case detailView:
		return m.handleDetailKeys(msg)
	}

	return nil
}

func (m *Model) handleListKeys(msg tea.KeyMsg) tea.Cmd {
	switch {
	case key.Matches(msg, m.keys.Search):
		m.state = searchView
		m.searchInput.Focus()
		return textinput.Blink

	case key.Matches(msg, m.keys.Enter):
		if item, ok := m.list.SelectedItem().(RepoItem); ok {
			m.detailRepo = &item
			m.state = detailView
			return m.loadDetail
		}

	case key.Matches(msg, m.keys.Select):
		if item, ok := m.list.SelectedItem().(RepoItem); ok {
			m.selected[item.ID] = !m.selected[item.ID]
			m.updateItemSelection()
		}

	case key.Matches(msg, m.keys.SelectAll):
		for _, item := range m.list.Items() {
			if repo, ok := item.(RepoItem); ok {
				m.selected[repo.ID] = true
			}
		}
		m.updateItemSelection()

	case key.Matches(msg, m.keys.SelectNone):
		m.selected = make(map[int64]bool)
		m.updateItemSelection()
	}

	return nil
}

func (m *Model) handleSearchKeys(msg tea.KeyMsg) tea.Cmd {
	if key.Matches(msg, m.keys.Enter) && m.searchInput.Value() != "" {
		query := m.searchInput.Value()
		return func() tea.Msg {
			results, err := m.searcher.Search(query, 100)
			if err != nil {
				return errMsg{err}
			}

			items := make([]list.Item, 0, len(results))
			for _, r := range results {
				item := RepoItem{
					ID:       r.RepoID,
					Forge:    r.Forge,
					FullName: r.FullName,
					WebURL:   r.WebURL,
				}
				if r.Description != nil {
					item.Desc = *r.Description
				}
				if r.Language != nil {
					item.Language = *r.Language
				}
				if r.LocalPath != nil {
					item.LocalPath = *r.LocalPath
				}
				if r.StarsCount != nil {
					item.Stars = *r.StarsCount
				}
				items = append(items, item)
			}
			return searchResultsMsg{items}
		}
	}
	return nil
}

func (m *Model) handleDetailKeys(msg tea.KeyMsg) tea.Cmd {
	// Detail view uses viewport for scrolling, handled in Update
	return nil
}

func (m *Model) loadDetail() tea.Msg {
	if m.detailRepo == nil {
		return nil
	}

	// Load README from database
	docs, err := m.db.GetDocumentsByRepo(m.detailRepo.ID)
	if err != nil {
		return errMsg{err}
	}

	var readme string
	for _, doc := range docs {
		if doc.DocType == "readme" && doc.Content != nil {
			readme = *doc.Content
			break
		}
	}

	return detailLoadedMsg{readme: readme}
}

func (m *Model) renderDetailContent(readme string) string {
	var s strings.Builder

	repo := m.detailRepo
	if repo == nil {
		return ""
	}

	// Metadata section
	s.WriteString(detailTitleStyle.Render(repo.FullName))
	s.WriteString("\n\n")

	// Info rows
	if repo.Desc != "" {
		s.WriteString(repo.Desc)
		s.WriteString("\n\n")
	}

	s.WriteString(detailLabelStyle.Render("Language:"))
	s.WriteString(detailValueStyle.Render(repo.Language))
	s.WriteString("\n")

	s.WriteString(detailLabelStyle.Render("Stars:"))
	s.WriteString(detailValueStyle.Render(fmt.Sprintf("%d", repo.Stars)))
	s.WriteString("\n")

	if len(repo.Topics) > 0 {
		s.WriteString(detailLabelStyle.Render("Topics:"))
		s.WriteString(detailValueStyle.Render(strings.Join(repo.Topics, ", ")))
		s.WriteString("\n")
	}

	s.WriteString(detailLabelStyle.Render("URL:"))
	s.WriteString(detailValueStyle.Render(repo.WebURL))
	s.WriteString("\n")

	if repo.LocalPath != "" {
		s.WriteString(detailLabelStyle.Render("Local:"))
		s.WriteString(detailValueStyle.Render(repo.LocalPath))
		s.WriteString("\n")
	}

	// README section
	if readme != "" {
		s.WriteString("\n")
		s.WriteString(detailTitleStyle.Render("README"))
		s.WriteString("\n\n")

		// Render markdown with glamour
		renderer, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(m.detailViewport.Width),
		)
		if err == nil {
			rendered, err := renderer.Render(readme)
			if err == nil {
				s.WriteString(rendered)
			} else {
				s.WriteString(readme)
			}
		} else {
			s.WriteString(readme)
		}
	}

	return s.String()
}

// updateItemSelection syncs the Selected field on all items with the selected map
func (m *Model) updateItemSelection() {
	items := m.list.Items()
	updated := make([]list.Item, len(items))
	for i, item := range items {
		if repo, ok := item.(RepoItem); ok {
			repo.Selected = m.selected[repo.ID]
			updated[i] = repo
		} else {
			updated[i] = item
		}
	}
	m.list.SetItems(updated)
}

// SelectedItems returns all currently selected RepoItems
func (m Model) SelectedItems() []RepoItem {
	var result []RepoItem
	for _, item := range m.list.Items() {
		if repo, ok := item.(RepoItem); ok && m.selected[repo.ID] {
			result = append(result, repo)
		}
	}
	return result
}

// SelectedCount returns the number of selected items
func (m Model) SelectedCount() int {
	count := 0
	for _, v := range m.selected {
		if v {
			count++
		}
	}
	return count
}

// View renders the UI
func (m Model) View() string {
	if m.err != nil {
		return appStyle.Render(fmt.Sprintf("Error: %v\n\nPress q to quit.", m.err))
	}

	var content string

	switch m.state {
	case listView:
		content = m.viewList()
	case searchView:
		content = m.viewSearch()
	case detailView:
		content = m.viewDetail()
	case helpView:
		content = m.viewHelp()
	}

	return appStyle.Render(content)
}

func (m Model) viewList() string {
	var s strings.Builder
	s.WriteString(m.list.View())

	if count := m.SelectedCount(); count > 0 {
		s.WriteString(statusStyle.Render(fmt.Sprintf(" %d selected", count)))
	}

	return s.String()
}

func (m Model) viewSearch() string {
	var s strings.Builder
	s.WriteString("Search: ")
	s.WriteString(m.searchInput.View())
	s.WriteString("\n\n")
	s.WriteString(m.list.View())
	return s.String()
}

func (m Model) viewDetail() string {
	if m.detailRepo == nil {
		return "No repo selected"
	}

	var s strings.Builder
	s.WriteString(m.detailViewport.View())
	s.WriteString("\n")
	s.WriteString(statusStyle.Render("↑/↓ scroll • esc back • ? help"))

	return s.String()
}

func (m Model) viewHelp() string {
	var s strings.Builder

	s.WriteString(helpTitleStyle.Render("Keyboard Shortcuts"))
	s.WriteString("\n\n")

	bindings := m.keys.FullHelp()
	for _, row := range bindings {
		for _, b := range row {
			help := b.Help()
			s.WriteString(helpKeyStyle.Render(fmt.Sprintf("%-12s", help.Key)))
			s.WriteString(helpDescStyle.Render(help.Desc))
			s.WriteString("\n")
		}
		s.WriteString("\n")
	}

	s.WriteString(statusStyle.Render("Press ? or esc to close"))

	// Center the help overlay
	helpContent := helpStyle.Render(s.String())
	return lipgloss.Place(m.width-4, m.height-4, lipgloss.Center, lipgloss.Center, helpContent)
}
