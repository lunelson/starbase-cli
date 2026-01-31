package main

import (
	"fmt"
	"strings"

	"github.com/lunelson/starbase-cli/internal/config"
	"github.com/lunelson/starbase-cli/internal/database"
	"github.com/lunelson/starbase-cli/internal/forge/github"
	"github.com/lunelson/starbase-cli/internal/search"
	"github.com/spf13/cobra"
)

var rmCmd = &cobra.Command{
	Use:     "rm <owner/name>",
	Aliases: []string{"remove"},
	Short:   "Remove a repository from local clones",
	Long: `Remove a repository from starbase and delete its local clone.

By default, keeps the repo starred on GitHub. Use --unstar to also
unstar it on GitHub.

This is the inverse of 'add'.`,
	Args: cobra.ExactArgs(1),
	RunE: runRm,
}

var rmUnstar bool

func init() {
	rootCmd.AddCommand(rmCmd)
	rmCmd.Flags().BoolVar(&rmUnstar, "unstar", false, "Also unstar the repo on GitHub")
}

func runRm(cmd *cobra.Command, args []string) error {
	fullName := args[0]

	parts := strings.Split(fullName, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo format: use owner/name")
	}
	owner, name := parts[0], parts[1]

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

	// Find repo in database
	repo, err := db.GetRepoByFullName("github", fullName)
	if err != nil {
		return fmt.Errorf("looking up repo: %w", err)
	}
	if repo == nil {
		return fmt.Errorf("repo not found in starbase: %s", fullName)
	}

	// Delete local clone if it exists
	if repo.LocalPath != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Deleting clone: %s\n", *repo.LocalPath)
		if err := removeDir(*repo.LocalPath); err != nil {
			fmt.Fprintf(cmd.OutOrStderr(), "Warning: failed to delete clone: %v\n", err)
		}
	}

	// Remove from search index
	if err := search.New(db.DB).RemoveRepo(repo.ID); err != nil {
		fmt.Fprintf(cmd.OutOrStderr(), "Warning: failed to remove from search index: %v\n", err)
	}

	// Remove from database
	if err := db.DeleteRepo(repo.ID); err != nil {
		return fmt.Errorf("deleting from database: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Removed from starbase: %s\n", fullName)

	// Unstar on GitHub if requested
	if rmUnstar {
		token, err := github.ResolveToken()
		if err != nil {
			return fmt.Errorf("GitHub auth: %w", err)
		}

		client := github.NewClient(token.Token)

		if err := client.Unstar(cmd.Context(), owner, name); err != nil {
			return fmt.Errorf("unstarring repo: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Unstarred on GitHub: %s\n", fullName)
	}

	return nil
}
