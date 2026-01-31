package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/lunelson/starbase-cli/internal/config"
	"github.com/lunelson/starbase-cli/internal/database"
	"github.com/lunelson/starbase-cli/internal/search"
	"github.com/spf13/cobra"
)

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove repos that are no longer starred",
	Long: `Delete local clones and database entries for repos that have been unstarred.

By default, only shows what would be pruned. Use --force to actually delete.`,
	RunE: runPrune,
}

var (
	pruneForce     bool
	pruneKeepClone bool
)

func init() {
	rootCmd.AddCommand(pruneCmd)

	pruneCmd.Flags().BoolVar(&pruneForce, "force", false, "Actually delete (default is dry-run)")
	pruneCmd.Flags().BoolVar(&pruneKeepClone, "keep-clone", false, "Remove from DB but keep local clone")
}

func runPrune(cmd *cobra.Command, args []string) error {
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

	repos, err := db.ListRepos("")
	if err != nil {
		return fmt.Errorf("listing repos: %w", err)
	}

	var toPrune []*database.Repo
	for _, repo := range repos {
		if repo.Status == "unstarred" || repo.Status == "removed" {
			toPrune = append(toPrune, repo)
		}
	}

	if len(toPrune) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Nothing to prune.")
		return nil
	}

	if !pruneForce {
		fmt.Fprintf(cmd.OutOrStdout(), "Would prune %d repos (use --force to delete):\n", len(toPrune))
		for _, repo := range toPrune {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s (%s)\n", repo.FullName, repo.Status)
		}
		return nil
	}

	searcher := search.New(db.DB)

	var deleted, errors int
	for _, repo := range toPrune {
		fmt.Fprintf(cmd.OutOrStdout(), "Pruning %s...\n", repo.FullName)

		if repo.LocalPath != nil && !pruneKeepClone {
			if err := os.RemoveAll(*repo.LocalPath); err != nil {
				fmt.Fprintf(cmd.OutOrStderr(), "  Warning: failed to remove clone: %v\n", err)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "  Removed clone: %s\n", filepath.Base(*repo.LocalPath))
			}
		}

		if err := searcher.RemoveRepo(repo.ID); err != nil {
			fmt.Fprintf(cmd.OutOrStderr(), "  Warning: failed to remove from search index: %v\n", err)
		}

		if err := db.DeleteRepo(repo.ID); err != nil {
			fmt.Fprintf(cmd.OutOrStderr(), "  Error: failed to delete from database: %v\n", err)
			errors++
			continue
		}

		deleted++
	}

	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintf(cmd.OutOrStdout(), "Pruned %d repos", deleted)
	if errors > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), " (%d errors)", errors)
	}
	fmt.Fprintln(cmd.OutOrStdout())

	return nil
}
