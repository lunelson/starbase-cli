package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lunelson/starbase-cli/internal/database"
	"github.com/lunelson/starbase-cli/internal/search"
)

var (
	appStyle = lipgloss.NewStyle().Padding(1, 2)

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFDF5")).
			Background(lipgloss.Color("#25A065")).
			Padding(0, 1)

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#343433", Dark: "#C1C6B2"}).
			Background(lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#353533"})
)

// RepoItem implements list.Item for the TUI list
type RepoItem struct {
	ID        int64
	Forge     string
	FullName  string
	Desc      string
	Language  string
	LocalPath string
	Stars     int
}

func (r RepoItem) Title() string       { return r.FullName }
func (r RepoItem) Description() string { return r.Desc }
func (r RepoItem) FilterValue() string { return r.FullName + " " + r.Desc }

// Model is the main TUI model
type Model struct {
	db       *database.DB
	searcher *search.Searcher

	list        list.Model
	searchInput textinput.Model
	searching   bool
	width       int
	height      int
	err         error
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

	return Model{
		db:          db,
		searcher:    search.New(db.DB),
		list:        l,
		searchInput: ti,
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
		}

		items = append(items, item)
	}

	return reposLoadedMsg{items}
}

type reposLoadedMsg struct {
	items []list.Item
}

type searchResultsMsg struct {
	items []list.Item
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

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c"))):
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("/"))):
			if !m.searching {
				m.searching = true
				m.searchInput.Focus()
				return m, textinput.Blink
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			if m.searching {
				m.searching = false
				m.searchInput.Blur()
				m.searchInput.SetValue("")
				return m, m.loadRepos
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if m.searching && m.searchInput.Value() != "" {
				query := m.searchInput.Value()
				return m, func() tea.Msg {
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
		}

	case reposLoadedMsg:
		m.list.SetItems(msg.items)

	case searchResultsMsg:
		m.list.SetItems(msg.items)
		m.searching = false
		m.searchInput.Blur()

	case errMsg:
		m.err = msg.err
	}

	// Update sub-models
	if m.searching {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// View renders the UI
func (m Model) View() string {
	if m.err != nil {
		return appStyle.Render(fmt.Sprintf("Error: %v\n\nPress q to quit.", m.err))
	}

	var s strings.Builder

	if m.searching {
		s.WriteString("Search: ")
		s.WriteString(m.searchInput.View())
		s.WriteString("\n\n")
	}

	s.WriteString(m.list.View())

	return appStyle.Render(s.String())
}
