package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/lunelson/starbase-cli/internal/config"
	"github.com/lunelson/starbase-cli/internal/database"
	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show dashboard statistics about tracked repositories",
	Long: `Display aggregate statistics about your starred repositories.

Examples:
  starbase stats
  starbase stats --json`,
	RunE: runStats,
}

var statsJSON bool

func init() {
	rootCmd.AddCommand(statsCmd)

	statsCmd.Flags().BoolVar(&statsJSON, "json", false, "Output as JSON")
}

type statsOutput struct {
	TotalRepos     int              `json:"total_repos"`
	ByStatus       statusCounts     `json:"by_status"`
	CloneStatus    cloneCounts      `json:"clone_status"`
	DiskUsageBytes int64            `json:"disk_usage_bytes"`
	DiskUsage      string           `json:"disk_usage"`
	TopLanguages   []languageOutput `json:"top_languages"`
	TopTopics      []topicOutput    `json:"top_topics"`
	LastSyncAt     *time.Time       `json:"last_sync_at"`
}

type statusCounts struct {
	Active      int `json:"active"`
	Archived    int `json:"archived"`
	Unavailable int `json:"unavailable"`
}

type cloneCounts struct {
	Cloned    int `json:"cloned"`
	NotCloned int `json:"not_cloned"`
}

type languageOutput struct {
	Language string `json:"language"`
	Count    int    `json:"count"`
}

type topicOutput struct {
	Topic string `json:"topic"`
	Count int    `json:"count"`
}

func runStats(cmd *cobra.Command, args []string) error {
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

	stats, err := db.GetStats()
	if err != nil {
		return fmt.Errorf("getting stats: %w", err)
	}

	languages, err := db.GetTopLanguages(5)
	if err != nil {
		return fmt.Errorf("getting top languages: %w", err)
	}

	topics, err := db.GetTopTopics(5)
	if err != nil {
		return fmt.Errorf("getting top topics: %w", err)
	}

	diskUsage := calculateDiskUsage(paths.ClonesDir)

	if statsJSON {
		return outputStatsJSON(cmd, stats, languages, topics, diskUsage)
	}
	return outputStatsTable(cmd, stats, languages, topics, diskUsage)
}

func calculateDiskUsage(dir string) int64 {
	var total int64
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func outputStatsJSON(cmd *cobra.Command, stats *database.RepoStats, languages []database.LanguageCount, topics []database.TopicCount, diskUsage int64) error {
	out := statsOutput{
		TotalRepos: stats.Total,
		ByStatus: statusCounts{
			Active:      stats.Active,
			Archived:    stats.Archived,
			Unavailable: stats.Unavailable,
		},
		CloneStatus: cloneCounts{
			Cloned:    stats.Cloned,
			NotCloned: stats.NotCloned,
		},
		DiskUsageBytes: diskUsage,
		DiskUsage:      formatBytes(diskUsage),
		LastSyncAt:     stats.LastSyncAt,
	}

	for _, l := range languages {
		out.TopLanguages = append(out.TopLanguages, languageOutput{
			Language: l.Language,
			Count:    l.Count,
		})
	}

	for _, t := range topics {
		out.TopTopics = append(out.TopTopics, topicOutput{
			Topic: t.Topic,
			Count: t.Count,
		})
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func outputStatsTable(cmd *cobra.Command, stats *database.RepoStats, languages []database.LanguageCount, topics []database.TopicCount, diskUsage int64) error {
	w := cmd.OutOrStdout()

	fmt.Fprintf(w, "╭─────────────────────────────────────╮\n")
	fmt.Fprintf(w, "│         Starbase Statistics         │\n")
	fmt.Fprintf(w, "╰─────────────────────────────────────╯\n\n")

	fmt.Fprintf(w, "Total repos tracked: %d\n\n", stats.Total)

	fmt.Fprintf(w, "── By Status ──\n")
	fmt.Fprintf(w, "  Active:      %d\n", stats.Active)
	fmt.Fprintf(w, "  Archived:    %d\n", stats.Archived)
	fmt.Fprintf(w, "  Unavailable: %d\n\n", stats.Unavailable)

	fmt.Fprintf(w, "── Clone Status ──\n")
	fmt.Fprintf(w, "  Cloned:     %d\n", stats.Cloned)
	fmt.Fprintf(w, "  Not cloned: %d\n\n", stats.NotCloned)

	fmt.Fprintf(w, "── Disk Usage ──\n")
	fmt.Fprintf(w, "  %s\n\n", formatBytes(diskUsage))

	if len(languages) > 0 {
		fmt.Fprintf(w, "── Top Languages ──\n")
		for _, l := range languages {
			fmt.Fprintf(w, "  %-15s %d\n", l.Language, l.Count)
		}
		fmt.Fprintln(w)
	}

	if len(topics) > 0 {
		fmt.Fprintf(w, "── Top Topics ──\n")
		for _, t := range topics {
			fmt.Fprintf(w, "  %-20s %d\n", t.Topic, t.Count)
		}
		fmt.Fprintln(w)
	}

	if stats.LastSyncAt != nil {
		fmt.Fprintf(w, "── Last Sync ──\n")
		fmt.Fprintf(w, "  %s\n", stats.LastSyncAt.Format(time.RFC3339))
	} else {
		fmt.Fprintf(w, "── Last Sync ──\n")
		fmt.Fprintf(w, "  Never synced\n")
	}

	return nil
}

// Ensure w is used even if stats are empty
var _ = os.Stdout
