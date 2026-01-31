package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/lunelson/starbase-cli/internal/config"
	"github.com/lunelson/starbase-cli/internal/database"
	"github.com/lunelson/starbase-cli/internal/forge"
	"github.com/lunelson/starbase-cli/internal/forge/github"
	"github.com/lunelson/starbase-cli/internal/git"
	"github.com/lunelson/starbase-cli/internal/index"
	"github.com/lunelson/starbase-cli/internal/search"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add [repo]",
	Short: "Star and clone a repository",
	Long: `Add a repository to your starred repos and clone it locally.

If no repo is specified, launches interactive mode to select from
your GitHub stars that aren't yet tracked in starbase.

If already starred, skips starring. If already cloned, skips cloning.
Removes the repo from tombstones if previously removed.

Accepts various URL formats:
  owner/repo                         (assumes github.com)
  https://github.com/owner/repo
  https://github.com/owner/repo/tree/main/src/file.ts
  git@github.com:owner/repo.git

This is the inverse of 'rm'.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAdd,
}

func init() {
	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	configDir := config.DefaultConfigDir()
	cfg, err := config.Load(configDir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	paths := config.ResolvePaths(cfg)

	if err := os.MkdirAll(paths.ClonesDir, 0755); err != nil {
		return fmt.Errorf("creating clones directory: %w", err)
	}

	db, err := database.Open(paths.DBPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	manifest, err := config.LoadManifest(configDir)
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	token, err := github.ResolveToken()
	if err != nil {
		return fmt.Errorf("GitHub auth: %w", err)
	}

	client := github.NewClient(token.Token)

	if len(args) == 0 {
		return runAddInteractive(cmd, cfg, paths, db, manifest, client, configDir)
	}

	parsed, err := forge.ParseRepoURL(args[0])
	if err != nil {
		return err
	}

	return addRepo(cmd, cfg, paths, db, manifest, client, configDir, parsed)
}

func runAddInteractive(cmd *cobra.Command, cfg *config.Config, paths config.Paths, db *database.DB, manifest *config.Manifest, client *github.Client, configDir string) error {
	fmt.Fprintln(cmd.OutOrStdout(), "Fetching starred repos from GitHub...")

	stars, err := fetchAllStars(cmd.Context(), client)
	if err != nil {
		return fmt.Errorf("fetching stars: %w", err)
	}

	existingRepos, err := db.ListRepos("")
	if err != nil {
		return fmt.Errorf("listing repos: %w", err)
	}

	existingSet := make(map[string]bool)
	for _, r := range existingRepos {
		existingSet[r.ForgeID] = true
	}

	var untracked []forge.StarredRepo
	for _, star := range stars {
		if !existingSet[star.ForgeID] {
			untracked = append(untracked, star)
		}
	}

	if len(untracked) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "All starred repos are already tracked.")
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Found %d untracked starred repos.\n", len(untracked))

	options := make([]huh.Option[int], 0, len(untracked))
	for i, star := range untracked {
		label := star.FullName
		if star.Description != "" {
			if len(star.Description) > 50 {
				label = fmt.Sprintf("%s - %s...", star.FullName, star.Description[:50])
			} else {
				label = fmt.Sprintf("%s - %s", star.FullName, star.Description)
			}
		}
		options = append(options, huh.NewOption(label, i))
	}

	var selectedIndices []int
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[int]().
				Title("Select repositories to add").
				Description("Space to select, Enter to confirm").
				Options(options...).
				Value(&selectedIndices).
				Filterable(true).
				Height(15),
		),
	)

	if err := form.Run(); err != nil {
		return fmt.Errorf("selection cancelled")
	}

	if len(selectedIndices) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No repositories selected.")
		return nil
	}

	for _, idx := range selectedIndices {
		star := untracked[idx]
		parsed := &forge.ParsedRepo{
			Host:     "github.com",
			Owner:    star.Owner,
			Name:     star.Name,
			CloneURL: star.CloneURL,
		}

		if err := addRepoFromStar(cmd, cfg, paths, db, manifest, configDir, star, parsed); err != nil {
			fmt.Fprintf(cmd.OutOrStderr(), "Error adding %s: %v\n", star.FullName, err)
		}
	}

	return nil
}

func fetchAllStars(ctx context.Context, client *github.Client) ([]forge.StarredRepo, error) {
	var allStars []forge.StarredRepo

	opts := forge.ListOptions{
		Page:    1,
		PerPage: 100,
	}

	for {
		result, err := client.ListStars(ctx, opts)
		if err != nil {
			return nil, err
		}

		allStars = append(allStars, result.Repos...)

		if result.NextPage == 0 {
			break
		}
		opts.Page = result.NextPage
	}

	return allStars, nil
}

func addRepo(cmd *cobra.Command, cfg *config.Config, paths config.Paths, db *database.DB, manifest *config.Manifest, client *github.Client, configDir string, parsed *forge.ParsedRepo) error {
	forgeName := forge.ForgeFromHost(parsed.Host)
	if forgeName != "github" {
		return fmt.Errorf("only GitHub is currently supported")
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Fetching %s...\n", parsed.FullName())
	ghRepo, err := client.GetRepo(cmd.Context(), parsed.Owner, parsed.Name)
	if err != nil {
		return fmt.Errorf("fetching repo: %w", err)
	}
	if ghRepo == nil {
		return fmt.Errorf("repo not found: %s", parsed.FullName())
	}

	starred, err := client.IsStarred(cmd.Context(), parsed.Owner, parsed.Name)
	if err != nil {
		return fmt.Errorf("checking star status: %w", err)
	}

	if starred {
		fmt.Fprintf(cmd.OutOrStdout(), "Already starred: %s\n", parsed.FullName())
	} else {
		if err := client.Star(cmd.Context(), parsed.Owner, parsed.Name); err != nil {
			return fmt.Errorf("starring repo: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Starred: %s\n", parsed.FullName())
	}

	star := forge.StarredRepo{
		ForgeID:     ghRepo.ForgeID,
		Owner:       ghRepo.Owner,
		Name:        ghRepo.Name,
		FullName:    ghRepo.FullName,
		CloneURL:    ghRepo.CloneURL,
		WebURL:      ghRepo.WebURL,
		Description: ghRepo.Description,
		Language:    ghRepo.Language,
		Topics:      ghRepo.Topics,
		StarsCount:  ghRepo.StarsCount,
		ForksCount:  ghRepo.ForksCount,
		IsArchived:  ghRepo.IsArchived,
		IsPrivate:   ghRepo.IsPrivate,
	}

	return addRepoFromStar(cmd, cfg, paths, db, manifest, configDir, star, parsed)
}

func addRepoFromStar(cmd *cobra.Command, cfg *config.Config, paths config.Paths, db *database.DB, manifest *config.Manifest, configDir string, star forge.StarredRepo, parsed *forge.ParsedRepo) error {
	forgeName := forge.ForgeFromHost(parsed.Host)

	now := time.Now()
	repo := &database.Repo{
		Forge:     forgeName,
		ForgeID:   star.ForgeID,
		Owner:     star.Owner,
		Name:      star.Name,
		FullName:  star.FullName,
		CloneURL:  star.CloneURL,
		WebURL:    star.WebURL,
		StarredAt: &now,
		SyncedAt:  &now,
		Status:    "active",
	}

	repoID, err := db.InsertRepo(repo)
	if err != nil {
		return fmt.Errorf("inserting repo: %w", err)
	}

	desc := star.Description
	lang := star.Language
	meta := &database.RepoMetadata{
		RepoID:      repoID,
		Description: strPtr(desc),
		Language:    strPtr(lang),
		Topics:      star.Topics,
		StarsCount:  intPtr(star.StarsCount),
		ForksCount:  intPtr(star.ForksCount),
		IsArchived:  star.IsArchived,
		IsPrivate:   star.IsPrivate,
	}

	if err := db.UpsertMetadata(meta); err != nil {
		return fmt.Errorf("saving metadata: %w", err)
	}

	localPath := filepath.Join(paths.ClonesDir, forgeName, parsed.Owner, parsed.Name)

	if git.IsGitRepo(localPath) {
		fmt.Fprintf(cmd.OutOrStdout(), "Already cloned: %s\n", localPath)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Cloning %s...\n", parsed.FullName())
		cloneOpts := git.CloneOptions{
			Depth:          cfg.Clone.Depth,
			SingleBranch:   cfg.Clone.SingleBranch,
			SkipSubmodules: cfg.Clone.SkipSubmodules,
			SkipLFS:        cfg.Clone.SkipLFS,
		}
		if err := git.Clone(cmd.Context(), star.CloneURL, localPath, cloneOpts); err != nil {
			return fmt.Errorf("cloning repo: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Cloned: %s\n", localPath)
	}

	if err := db.UpdateRepoLocalPath(repoID, localPath, time.Now()); err != nil {
		return fmt.Errorf("updating local path: %w", err)
	}

	extractorCfg := index.DefaultExtractorConfig()
	extractor := index.NewExtractor(db, extractorCfg)
	if err := extractor.ExtractAndIndex(cmd.Context(), repoID, localPath); err != nil {
		fmt.Fprintf(cmd.OutOrStderr(), "Warning: indexing failed: %v\n", err)
	}

	if _, err := search.New(db.DB).RebuildIndex(); err != nil {
		fmt.Fprintf(cmd.OutOrStderr(), "Warning: failed to rebuild search index: %v\n", err)
	}

	manifest.RemoveTombstone(parsed.ID())
	if err := config.SaveManifest(configDir, manifest); err != nil {
		fmt.Fprintf(cmd.OutOrStderr(), "Warning: failed to save manifest: %v\n", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Added %s\n", parsed.FullName())
	return nil
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func intPtr(i int) *int {
	return &i
}
