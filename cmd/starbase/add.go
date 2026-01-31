package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

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
	Use:   "add <repo>",
	Short: "Star and clone a repository",
	Long: `Add a repository to your starred repos and clone it locally.

If already starred, skips starring. If already cloned, skips cloning.
Removes the repo from tombstones if previously removed.

Accepts various URL formats:
  owner/repo                         (assumes github.com)
  https://github.com/owner/repo
  https://github.com/owner/repo/tree/main/src/file.ts
  git@github.com:owner/repo.git

This is the inverse of 'rm'.`,
	Args: cobra.ExactArgs(1),
	RunE: runAdd,
}

func init() {
	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	parsed, err := forge.ParseRepoURL(args[0])
	if err != nil {
		return err
	}

	forgeName := forge.ForgeFromHost(parsed.Host)
	if forgeName != "github" {
		return fmt.Errorf("only GitHub is currently supported")
	}

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

	now := time.Now()
	repo := &database.Repo{
		Forge:     forgeName,
		ForgeID:   ghRepo.ForgeID,
		Owner:     ghRepo.Owner,
		Name:      ghRepo.Name,
		FullName:  ghRepo.FullName,
		CloneURL:  ghRepo.CloneURL,
		WebURL:    ghRepo.WebURL,
		StarredAt: &now,
		SyncedAt:  &now,
		Status:    "active",
	}

	repoID, err := db.InsertRepo(repo)
	if err != nil {
		return fmt.Errorf("inserting repo: %w", err)
	}

	desc := ghRepo.Description
	lang := ghRepo.Language
	branch := ghRepo.DefaultBranch
	meta := &database.RepoMetadata{
		RepoID:        repoID,
		Description:   &desc,
		Language:      &lang,
		Topics:        ghRepo.Topics,
		StarsCount:    &ghRepo.StarsCount,
		ForksCount:    &ghRepo.ForksCount,
		DefaultBranch: &branch,
		IsArchived:    ghRepo.IsArchived,
		IsPrivate:     ghRepo.IsPrivate,
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
		if err := git.Clone(cmd.Context(), ghRepo.CloneURL, localPath, cloneOpts); err != nil {
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
