package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap defines all key bindings with help text
type keyMap struct {
	Up           key.Binding
	Down         key.Binding
	Select       key.Binding
	SelectAll    key.Binding
	SelectNone   key.Binding
	Search       key.Binding
	FilterLang   key.Binding
	FilterTopic  key.Binding
	ClearFilters key.Binding
	Enter        key.Binding
	Back         key.Binding
	Help         key.Binding
	Quit         key.Binding
	OpenEditor   key.Binding
	OpenWeb      key.Binding
	CopyPath     key.Binding
}

// defaultKeyMap returns the default key bindings
func defaultKeyMap() keyMap {
	return keyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "move down"),
		),
		Select: key.NewBinding(
			key.WithKeys(" "),
			key.WithHelp("space", "toggle select"),
		),
		SelectAll: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "select all"),
		),
		SelectNone: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "select none"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		FilterLang: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "filter by language"),
		),
		FilterTopic: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "filter by topic"),
		),
		ClearFilters: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "clear filters"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "view details"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		OpenEditor: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "open in $EDITOR"),
		),
		OpenWeb: key.NewBinding(
			key.WithKeys("w"),
			key.WithHelp("w", "open in browser"),
		),
		CopyPath: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "copy path"),
		),
	}
}

// ShortHelp returns key bindings for the short help view
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit}
}

// FullHelp returns key bindings for the full help view
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Select, k.SelectAll, k.SelectNone},
		{k.Search, k.FilterLang, k.FilterTopic, k.ClearFilters},
		{k.Enter, k.Back, k.OpenEditor, k.OpenWeb, k.CopyPath},
		{k.Help, k.Quit},
	}
}
