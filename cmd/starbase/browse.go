package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lunelson/starbase-cli/internal/config"
	"github.com/lunelson/starbase-cli/internal/database"
	"github.com/lunelson/starbase-cli/internal/tui"
	"github.com/spf13/cobra"
)

var browseCmd = &cobra.Command{
	Use:   "browse",
	Short: "Launch interactive TUI",
	Long: `Launch an interactive terminal UI to browse, search, and manage
your starred repositories.

Key bindings:
  j/↓    Move down
  k/↑    Move up
  /      Search
  Enter  Select / Execute search
  Esc    Cancel search
  q      Quit`,
	RunE: runBrowse,
}

func init() {
	rootCmd.AddCommand(browseCmd)
}

func runBrowse(cmd *cobra.Command, args []string) error {
	configDir := config.DefaultConfigDir()
	cfg, err := config.Load(configDir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	paths := config.ResolvePaths(cfg)

	db, err := database.Open(paths.DBPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	m := tui.New(db)
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("running TUI: %w", err)
	}

	return nil
}
