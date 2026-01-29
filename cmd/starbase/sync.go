package main

import (
	"fmt"
	"os"
	"time"

	"github.com/lunelson/starbase-cli/internal/config"
	"github.com/lunelson/starbase-cli/internal/database"
	"github.com/lunelson/starbase-cli/internal/forge"
	"github.com/lunelson/starbase-cli/internal/forge/github"
	"github.com/lunelson/starbase-cli/internal/sync"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync starred repos from forges",
	Long: `Fetch starred repositories from GitHub/GitLab and clone/update them locally.

By default, only syncs stars from the configured time window (default: 30 days).
Use --full to sync all stars.`,
	RunE: runSync,
}

var (
	syncFull         bool
	syncMetadataOnly bool
	syncPullOnly     bool
	syncSince        string
	syncDryRun       bool
	syncConcurrency  int
)

func init() {
	rootCmd.AddCommand(syncCmd)

	syncCmd.Flags().BoolVar(&syncFull, "full", false, "Clone all stars, not just recent window")
	syncCmd.Flags().BoolVar(&syncMetadataOnly, "metadata-only", false, "Skip git operations, update API data only")
	syncCmd.Flags().BoolVar(&syncPullOnly, "pull-only", false, "Only update existing clones")
	syncCmd.Flags().StringVar(&syncSince, "since", "", "Override recency window (e.g., 7d, 2w, 6mo)")
	syncCmd.Flags().BoolVar(&syncDryRun, "dry-run", false, "Show plan without executing")
	syncCmd.Flags().IntVar(&syncConcurrency, "concurrency", 0, "Number of parallel git operations (default: 4, max: 10)")
}

func runSync(cmd *cobra.Command, args []string) error {
	configDir := config.DefaultConfigDir()

	// Load config
	cfg, err := config.Load(configDir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	paths := config.ResolvePaths(cfg)

	// Ensure directories exist
	if err := os.MkdirAll(paths.ClonesDir, 0755); err != nil {
		return fmt.Errorf("creating clones directory: %w", err)
	}

	// Load manifest
	manifest, err := config.LoadManifest(configDir)
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	// Open database
	db, err := database.Open(paths.DBPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	// Initialize forges
	forges := make(map[string]forge.Forge)

	if cfg.Forges.GitHub.Enabled {
		token, err := github.ResolveToken()
		if err != nil {
			return fmt.Errorf("GitHub auth: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Using GitHub token from %s\n", token.Source)
		forges["github"] = github.NewClient(token.Token)
	}

	if len(forges) == 0 {
		return fmt.Errorf("no forges enabled")
	}

	// Parse since flag
	var since *time.Time
	if syncSince != "" {
		dur, err := parseDuration(syncSince)
		if err != nil {
			return fmt.Errorf("invalid --since value: %w", err)
		}
		t := time.Now().Add(-dur)
		since = &t
	}

	// Override concurrency if flag provided
	if syncConcurrency > 0 {
		cfg.Sync.Concurrency = syncConcurrency
	}

	// Build sync options
	opts := sync.Options{
		Full:         syncFull,
		MetadataOnly: syncMetadataOnly,
		PullOnly:     syncPullOnly,
		Since:        since,
		DryRun:       syncDryRun,
		MaxRepos:     cfg.Sync.MaxReposPerSync,
	}

	// Run sync
	syncer := sync.New(cfg, paths, db, forges, manifest, cmd.OutOrStdout())
	result, err := syncer.Run(cmd.Context(), opts)
	if err != nil {
		return err
	}

	// Print summary
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintf(cmd.OutOrStdout(), "Sync complete:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  Fetched: %d\n", result.Fetched)
	fmt.Fprintf(cmd.OutOrStdout(), "  Cloned:  %d\n", result.Cloned)
	fmt.Fprintf(cmd.OutOrStdout(), "  Updated: %d\n", result.Updated)
	fmt.Fprintf(cmd.OutOrStdout(), "  Skipped: %d\n", result.Skipped)

	if result.Errors > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  Errors:  %d\n", result.Errors)
		for _, msg := range result.ErrorMsgs {
			fmt.Fprintf(cmd.OutOrStderr(), "    - %s\n", msg)
		}
	}

	return nil
}

func parseDuration(s string) (time.Duration, error) {
	if len(s) == 0 {
		return 0, fmt.Errorf("empty duration")
	}

	var multiplier time.Duration
	var value string

	// Check for "mo" suffix (months)
	if len(s) >= 3 && s[len(s)-2:] == "mo" {
		multiplier = 30 * 24 * time.Hour // approximate month
		value = s[:len(s)-2]
	} else {
		unit := s[len(s)-1]
		value = s[:len(s)-1]

		switch unit {
		case 'd':
			multiplier = 24 * time.Hour
		case 'w':
			multiplier = 7 * 24 * time.Hour
		case 'h':
			multiplier = time.Hour
		default:
			return 0, fmt.Errorf("unknown duration unit: %c (use h, d, w, or mo)", unit)
		}
	}

	var n int
	if _, err := fmt.Sscanf(value, "%d", &n); err != nil {
		return 0, fmt.Errorf("invalid number: %s", value)
	}

	return time.Duration(n) * multiplier, nil
}
