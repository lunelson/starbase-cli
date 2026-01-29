package main

import (
	"context"
	"fmt"

	"github.com/lunelson/starbase-cli/internal/config"
	"github.com/lunelson/starbase-cli/internal/database"
	"github.com/lunelson/starbase-cli/internal/index"
	"github.com/lunelson/starbase-cli/internal/search"
	"github.com/spf13/cobra"
)

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Manage the search index",
	Long:  `Commands for managing the full-text search index.`,
}

var indexRebuildCmd = &cobra.Command{
	Use:   "rebuild",
	Short: "Rebuild the search index from cloned repos",
	Long: `Rebuild the FTS5 search index by re-extracting documents
from all cloned repositories.`,
	RunE: runIndexRebuild,
}

var indexStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show index statistics",
	RunE:  runIndexStats,
}

func init() {
	rootCmd.AddCommand(indexCmd)
	indexCmd.AddCommand(indexRebuildCmd)
	indexCmd.AddCommand(indexStatsCmd)
}

func runIndexRebuild(cmd *cobra.Command, args []string) error {
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

	fmt.Fprintln(cmd.OutOrStdout(), "Rebuilding search index...")

	extractorCfg := index.ExtractorConfig{
		IndexReadme:     cfg.Index.Readme,
		HighSignalFiles: cfg.Index.HighSignalFiles,
		MaxFileSizeKB:   cfg.Index.MaxFileSizeKB,
	}
	if len(extractorCfg.HighSignalFiles) == 0 {
		extractorCfg = index.DefaultExtractorConfig()
	}

	extractor := index.NewExtractor(db, extractorCfg)
	count, err := extractor.RebuildAll(context.Background())
	if err != nil {
		return fmt.Errorf("rebuilding index: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Indexed %d repositories\n", count)
	return nil
}

func runIndexStats(cmd *cobra.Command, args []string) error {
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

	// Count repos
	totalRepos, _ := db.CountRepos()

	// Count cloned repos
	repos, _ := db.ListRepos("")
	cloned := 0
	for _, r := range repos {
		if r.LocalPath != nil {
			cloned++
		}
	}

	// Count indexed entries
	var indexed int
	db.QueryRow("SELECT COUNT(*) FROM search_index").Scan(&indexed)

	// Count documents
	var docs int
	db.QueryRow("SELECT COUNT(*) FROM repo_documents").Scan(&docs)

	fmt.Fprintln(cmd.OutOrStdout(), "Index Statistics:")
	fmt.Fprintf(cmd.OutOrStdout(), "  Total repos:   %d\n", totalRepos)
	fmt.Fprintf(cmd.OutOrStdout(), "  Cloned repos:  %d\n", cloned)
	fmt.Fprintf(cmd.OutOrStdout(), "  Indexed repos: %d\n", indexed)
	fmt.Fprintf(cmd.OutOrStdout(), "  Documents:     %d\n", docs)

	// Test search
	searcher := search.New(db.DB)
	results, _ := searcher.Search("*", 1)
	if len(results) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "  Search: ✓ operational")
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "  Search: no indexed content")
	}

	return nil
}
