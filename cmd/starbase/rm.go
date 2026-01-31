package main

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/lunelson/starbase-cli/internal/config"
	"github.com/lunelson/starbase-cli/internal/database"
	"github.com/lunelson/starbase-cli/internal/forge"
	"github.com/lunelson/starbase-cli/internal/forge/github"
	"github.com/lunelson/starbase-cli/internal/search"
	"github.com/spf13/cobra"
)

var rmCmd = &cobra.Command{
	Use:     "rm [repo]",
	Aliases: []string{"remove"},
	Short:   "Remove a repository from starbase",
	Long: `Remove a repository from starbase tracking.

If no repo is specified, launches interactive multiselect mode.

By default, keeps the local clone and star on GitHub. Use flags to change:
  --delete   Delete the local clone from disk
  --unstar   Unstar the repo on GitHub

Accepts various URL formats:
  owner/repo                         (assumes github.com)
  https://github.com/owner/repo
  git@github.com:owner/repo.git

Removed repos are tombstoned to prevent re-syncing. Use 'add' to un-tombstone.`,
	Args: cobra.MaximumNArgs(1),
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

	if len(args) == 0 {
		return runRmInteractive(cmd, db, manifest, configDir)
	}

	parsed, err := forge.ParseRepoURL(args[0])
	if err != nil {
		return err
	}

	return removeRepo(cmd, db, manifest, configDir, parsed)
}

func runRmInteractive(cmd *cobra.Command, db *database.DB, manifest *config.Manifest, configDir string) error {
	repos, err := db.ListRepos("")
	if err != nil {
		return fmt.Errorf("listing repos: %w", err)
	}

	if len(repos) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No repositories in starbase.")
		return nil
	}

	options := make([]huh.Option[int64], 0, len(repos))
	for _, r := range repos {
		options = append(options, huh.NewOption(r.FullName, r.ID))
	}

	var selectedIDs []int64
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[int64]().
				Title("Select repositories to remove").
				Description("Space to select, Enter to confirm").
				Options(options...).
				Value(&selectedIDs).
				Filterable(true).
				Height(15),
		),
	)

	if err := form.Run(); err != nil {
		return fmt.Errorf("selection cancelled")
	}

	if len(selectedIDs) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No repositories selected.")
		return nil
	}

	var confirm bool
	confirmMsg := fmt.Sprintf("Remove %d repository(ies)?", len(selectedIDs))
	if rmDelete {
		confirmMsg = fmt.Sprintf("Remove %d repository(ies) and delete from disk?", len(selectedIDs))
	}

	confirmForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(confirmMsg).
				Value(&confirm),
		),
	)

	if err := confirmForm.Run(); err != nil || !confirm {
		fmt.Fprintln(cmd.OutOrStdout(), "Cancelled.")
		return nil
	}

	repoMap := make(map[int64]*database.Repo)
	for _, r := range repos {
		repoMap[r.ID] = r
	}

	for _, id := range selectedIDs {
		repo := repoMap[id]
		if repo == nil {
			continue
		}

		parsed := &forge.ParsedRepo{
			Host:  forge.HostFromForge(repo.Forge),
			Owner: repo.Owner,
			Name:  repo.Name,
		}

		if err := removeRepo(cmd, db, manifest, configDir, parsed); err != nil {
			fmt.Fprintf(cmd.OutOrStderr(), "Error removing %s: %v\n", repo.FullName, err)
		}
	}

	return nil
}

func removeRepo(cmd *cobra.Command, db *database.DB, manifest *config.Manifest, configDir string, parsed *forge.ParsedRepo) error {
	forgeName := forge.ForgeFromHost(parsed.Host)

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
