package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lunelson/starbase-cli/internal/config"
	"github.com/lunelson/starbase-cli/internal/database"
	"github.com/lunelson/starbase-cli/internal/forge/github"
	"github.com/lunelson/starbase-cli/internal/search"
	"github.com/spf13/cobra"
)

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Manage individual repos",
}

var repoAddCmd = &cobra.Command{
	Use:   "add <owner/name>",
	Short: "Add a repo to the index",
	Long: `Add a repository to starbase without starring it on GitHub.

The repo will be fetched from the forge API and indexed.
Use --clone to also clone it locally.`,
	Args: cobra.ExactArgs(1),
	RunE: runRepoAdd,
}

var repoRemoveCmd = &cobra.Command{
	Use:   "remove <owner/name>",
	Short: "Remove a repo from the index",
	Long: `Remove a repository from starbase.

By default keeps the local clone. Use --delete-clone to remove it.`,
	Args: cobra.ExactArgs(1),
	RunE: runRepoRemove,
}

var (
	repoAddClone       bool
	repoRemoveDelClone bool
)

func init() {
	rootCmd.AddCommand(repoCmd)

	repoCmd.AddCommand(repoAddCmd)
	repoAddCmd.Flags().BoolVar(&repoAddClone, "clone", false, "Also clone the repo locally")

	repoCmd.AddCommand(repoRemoveCmd)
	repoRemoveCmd.Flags().BoolVar(&repoRemoveDelClone, "delete-clone", false, "Also delete the local clone")
}

func runRepoAdd(cmd *cobra.Command, args []string) error {
	fullName := args[0]

	parts := strings.Split(fullName, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo format: use owner/name")
	}

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

	existing, err := db.GetRepoByFullName("github", fullName)
	if err != nil {
		return fmt.Errorf("checking existing repo: %w", err)
	}
	if existing != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Repo %s already exists (ID: %d)\n", fullName, existing.ID)
		return nil
	}

	token, err := github.ResolveToken()
	if err != nil {
		return fmt.Errorf("GitHub auth: %w", err)
	}

	client := github.NewClient(token.Token)

	fmt.Fprintf(cmd.OutOrStdout(), "Fetching %s...\n", fullName)

	ghRepo, err := client.GetRepo(cmd.Context(), parts[0], parts[1])
	if err != nil {
		return fmt.Errorf("fetching repo: %w", err)
	}

	now := time.Now()
	repo := &database.Repo{
		Forge:    "github",
		ForgeID:  ghRepo.ForgeID,
		Owner:    ghRepo.Owner,
		Name:     ghRepo.Name,
		FullName: ghRepo.FullName,
		CloneURL: ghRepo.CloneURL,
		WebURL:   ghRepo.WebURL,
		SyncedAt: &now,
		Status:   "manual",
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

	readme, err := client.GetReadme(cmd.Context(), parts[0], parts[1])
	if err == nil && readme != "" {
		now := time.Now()
		doc := &database.RepoDocument{
			RepoID:      repoID,
			DocType:     "readme",
			Filename:    "README.md",
			Content:     &readme,
			ContentHash: ptrString(hashContent(readme)),
			ExtractedAt: &now,
		}
		if err := db.UpsertDocument(doc); err != nil {
			fmt.Fprintf(cmd.OutOrStderr(), "Warning: failed to save README: %v\n", err)
		}
	}

	if _, err := search.New(db.DB).RebuildIndex(); err != nil {
		fmt.Fprintf(cmd.OutOrStderr(), "Warning: failed to rebuild search index: %v\n", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Added %s (ID: %d)\n", fullName, repoID)

	if repoAddClone {
		fmt.Fprintf(cmd.OutOrStdout(), "Cloning is not yet implemented in add command\n")
	}

	return nil
}

func runRepoRemove(cmd *cobra.Command, args []string) error {
	fullName := args[0]

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

	repo, err := db.GetRepoByFullName("github", fullName)
	if err != nil {
		return fmt.Errorf("looking up repo: %w", err)
	}
	if repo == nil {
		return fmt.Errorf("repo not found: %s", fullName)
	}

	if repoRemoveDelClone && repo.LocalPath != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Deleting clone at %s...\n", *repo.LocalPath)
		if err := removeDir(*repo.LocalPath); err != nil {
			fmt.Fprintf(cmd.OutOrStderr(), "Warning: failed to delete clone: %v\n", err)
		}
	}

	if err := search.New(db.DB).RemoveRepo(repo.ID); err != nil {
		fmt.Fprintf(cmd.OutOrStderr(), "Warning: failed to remove from search index: %v\n", err)
	}

	if err := db.DeleteRepo(repo.ID); err != nil {
		return fmt.Errorf("deleting repo: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", fullName)
	return nil
}

func ptrString(s string) *string {
	return &s
}

func hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

func removeDir(path string) error {
	return os.RemoveAll(path)
}
