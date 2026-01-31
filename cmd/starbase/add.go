package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lunelson/starbase-cli/internal/config"
	"github.com/lunelson/starbase-cli/internal/database"
	"github.com/lunelson/starbase-cli/internal/forge/github"
	"github.com/lunelson/starbase-cli/internal/git"
	"github.com/lunelson/starbase-cli/internal/index"
	"github.com/lunelson/starbase-cli/internal/search"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add <owner/name>",
	Short: "Star and clone a repository",
	Long: `Add a repository to your starred repos and clone it locally.

If already starred, skips starring. If already cloned, skips cloning.
This is the inverse of 'rm'.`,
	Args: cobra.ExactArgs(1),
	RunE: runAdd,
}

func init() {
	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
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

	if err := os.MkdirAll(paths.ClonesDir, 0755); err != nil {
		return fmt.Errorf("creating clones directory: %w", err)
	}

	db, err := database.Open(paths.DBPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	token, err := github.ResolveToken()
	if err != nil {
		return fmt.Errorf("GitHub auth: %w", err)
	}

	client := github.NewClient(token.Token)

	// Check if repo exists on GitHub
	fmt.Fprintf(cmd.OutOrStdout(), "Fetching %s...\n", fullName)
	ghRepo, err := client.GetRepo(cmd.Context(), owner, name)
	if err != nil {
		return fmt.Errorf("fetching repo: %w", err)
	}
	if ghRepo == nil {
		return fmt.Errorf("repo not found: %s", fullName)
	}

	// Star the repo if not already starred
	starred, err := client.IsStarred(cmd.Context(), owner, name)
	if err != nil {
		return fmt.Errorf("checking star status: %w", err)
	}

	if starred {
		fmt.Fprintf(cmd.OutOrStdout(), "Already starred: %s\n", fullName)
	} else {
		if err := client.Star(cmd.Context(), owner, name); err != nil {
			return fmt.Errorf("starring repo: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Starred: %s\n", fullName)
	}

	// Add to database
	now := time.Now()
	repo := &database.Repo{
		Forge:     "github",
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

	// Save metadata
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

	// Clone if not already cloned
	localPath := filepath.Join(paths.ClonesDir, "github", owner, name)

	if git.IsGitRepo(localPath) {
		fmt.Fprintf(cmd.OutOrStdout(), "Already cloned: %s\n", localPath)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Cloning %s...\n", fullName)
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

	// Update local path in database
	if err := db.UpdateRepoLocalPath(repoID, localPath, time.Now()); err != nil {
		return fmt.Errorf("updating local path: %w", err)
	}

	// Extract and index
	extractorCfg := index.DefaultExtractorConfig()
	extractor := index.NewExtractor(db, extractorCfg)
	if err := extractor.ExtractAndIndex(cmd.Context(), repoID, localPath); err != nil {
		fmt.Fprintf(cmd.OutOrStderr(), "Warning: indexing failed: %v\n", err)
	}

	// Rebuild search index
	if _, err := search.New(db.DB).RebuildIndex(); err != nil {
		fmt.Fprintf(cmd.OutOrStderr(), "Warning: failed to rebuild search index: %v\n", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Added %s\n", fullName)
	return nil
}
