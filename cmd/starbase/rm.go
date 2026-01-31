package main

import (
	"fmt"

	"github.com/lunelson/starbase-cli/internal/config"
	"github.com/lunelson/starbase-cli/internal/database"
	"github.com/lunelson/starbase-cli/internal/forge"
	"github.com/lunelson/starbase-cli/internal/forge/github"
	"github.com/lunelson/starbase-cli/internal/search"
	"github.com/spf13/cobra"
)

var rmCmd = &cobra.Command{
	Use:     "rm <repo>",
	Aliases: []string{"remove"},
	Short:   "Remove a repository from starbase",
	Long: `Remove a repository from starbase tracking.

By default, keeps the local clone and star on GitHub. Use flags to change:
  --delete   Delete the local clone from disk
  --unstar   Unstar the repo on GitHub

Accepts various URL formats:
  owner/repo                         (assumes github.com)
  https://github.com/owner/repo
  git@github.com:owner/repo.git

Removed repos are tombstoned to prevent re-syncing. Use 'add' to un-tombstone.`,
	Args: cobra.ExactArgs(1),
	RunE: runRm,
}

var (
	rmDelete bool
	rmUnstar bool
)

func init() {
	rootCmd.AddCommand(rmCmd)
	rmCmd.Flags().BoolVarP(&rmDelete, "delete", "d", false, "Delete local clone from disk")
	rmCmd.Flags().BoolVar(&rmUnstar, "unstar", false, "Also unstar the repo on GitHub")
}

func runRm(cmd *cobra.Command, args []string) error {
	parsed, err := forge.ParseRepoURL(args[0])
	if err != nil {
		return err
	}

	forgeName := forge.ForgeFromHost(parsed.Host)

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

	manifest, err := config.LoadManifest(configDir)
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	repo, err := db.GetRepoByFullName(forgeName, parsed.FullName())
	if err != nil {
		return fmt.Errorf("looking up repo: %w", err)
	}
	if repo == nil {
		return fmt.Errorf("repo not found in starbase: %s", parsed.FullName())
	}

	if rmDelete && repo.LocalPath != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Deleting clone: %s\n", *repo.LocalPath)
		if err := removeDir(*repo.LocalPath); err != nil {
			fmt.Fprintf(cmd.OutOrStderr(), "Warning: failed to delete clone: %v\n", err)
		}
	}

	if err := search.New(db.DB).RemoveRepo(repo.ID); err != nil {
		fmt.Fprintf(cmd.OutOrStderr(), "Warning: failed to remove from search index: %v\n", err)
	}

	if err := db.DeleteRepo(repo.ID); err != nil {
		return fmt.Errorf("deleting from database: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Removed from starbase: %s\n", parsed.FullName())

	manifest.AddTombstone(parsed.ID())
	manifest.RemoveRepo(forgeName, parsed.Owner, parsed.Name)
	if err := config.SaveManifest(configDir, manifest); err != nil {
		fmt.Fprintf(cmd.OutOrStderr(), "Warning: failed to save manifest: %v\n", err)
	}

	if rmUnstar {
		if forgeName != "github" {
			fmt.Fprintf(cmd.OutOrStderr(), "Warning: unstar only supported for GitHub\n")
		} else {
			token, err := github.ResolveToken()
			if err != nil {
				return fmt.Errorf("GitHub auth: %w", err)
			}

			client := github.NewClient(token.Token)
			if err := client.Unstar(cmd.Context(), parsed.Owner, parsed.Name); err != nil {
				return fmt.Errorf("unstarring repo: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Unstarred on GitHub: %s\n", parsed.FullName())
		}
	}

	return nil
}
