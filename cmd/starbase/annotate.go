package main

import (
	"fmt"
	"strings"

	"github.com/lunelson/starbase-cli/internal/config"
	"github.com/lunelson/starbase-cli/internal/database"
	"github.com/lunelson/starbase-cli/internal/search"
	"github.com/spf13/cobra"
)

var noteCmd = &cobra.Command{
	Use:   "note <query> [text]",
	Short: "Add or view notes on a repository",
	Long: `Add or view notes on repositories matching the query.

Without text, shows existing notes.
With text, sets or updates the note.

Examples:
  starbase note bubbletea "Great TUI framework"
  starbase note bubbletea                          # Show note`,
	Args: cobra.MinimumNArgs(1),
	RunE: runNote,
}

var pinCmd = &cobra.Command{
	Use:   "pin <query>",
	Short: "Pin repositories to prevent pruning",
	Long: `Pin repositories matching the query.
Pinned repos will not be removed during pruning operations.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runPin,
}

var unpinCmd = &cobra.Command{
	Use:   "unpin <query>",
	Short: "Unpin repositories",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runUnpin,
}

func init() {
	rootCmd.AddCommand(noteCmd)
	rootCmd.AddCommand(pinCmd)
	rootCmd.AddCommand(unpinCmd)
}

func runNote(cmd *cobra.Command, args []string) error {
	query := args[0]
	var noteText string
	if len(args) > 1 {
		noteText = strings.Join(args[1:], " ")
	}

	configDir := config.DefaultConfigDir()
	cfg, err := config.Load(configDir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	manifest, err := config.LoadManifest(configDir)
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	paths := config.ResolvePaths(cfg)
	db, err := database.Open(paths.DBPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	// Search for repos
	searcher := search.New(db.DB)
	results, err := searcher.Search(query, 10)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No repos matched the query.")
		return nil
	}

	// For multiple results, show all if viewing notes
	if noteText == "" {
		for _, r := range results {
			repo := manifest.GetRepo(r.Forge, r.Owner, r.Name)
			if repo != nil && repo.Notes != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", r.FullName, repo.Notes)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: (no note)\n", r.FullName)
			}
		}
		return nil
	}

	// Set note on first result
	r := results[0]
	repo := manifest.GetRepo(r.Forge, r.Owner, r.Name)
	if repo == nil {
		manifest.AddRepo(config.ManifestRepo{
			Forge: r.Forge,
			Owner: r.Owner,
			Name:  r.Name,
			Notes: noteText,
		})
	} else {
		repo.Notes = noteText
	}

	if err := config.SaveManifest(configDir, manifest); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Note set for %s\n", r.FullName)
	return nil
}

func runPin(cmd *cobra.Command, args []string) error {
	query := strings.Join(args, " ")
	return setPinned(cmd, query, true)
}

func runUnpin(cmd *cobra.Command, args []string) error {
	query := strings.Join(args, " ")
	return setPinned(cmd, query, false)
}

func setPinned(cmd *cobra.Command, query string, pinned bool) error {
	configDir := config.DefaultConfigDir()
	cfg, err := config.Load(configDir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	manifest, err := config.LoadManifest(configDir)
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	paths := config.ResolvePaths(cfg)
	db, err := database.Open(paths.DBPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	searcher := search.New(db.DB)
	results, err := searcher.Search(query, 100)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No repos matched the query.")
		return nil
	}

	count := 0
	for _, r := range results {
		repo := manifest.GetRepo(r.Forge, r.Owner, r.Name)
		if repo == nil {
			manifest.AddRepo(config.ManifestRepo{
				Forge:  r.Forge,
				Owner:  r.Owner,
				Name:   r.Name,
				Pinned: pinned,
			})
			count++
		} else if repo.Pinned != pinned {
			repo.Pinned = pinned
			count++
		}
	}

	if err := config.SaveManifest(configDir, manifest); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}

	action := "Pinned"
	if !pinned {
		action = "Unpinned"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s %d repos\n", action, count)
	return nil
}
