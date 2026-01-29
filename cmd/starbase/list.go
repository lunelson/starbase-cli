package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lunelson/starbase-cli/internal/config"
	"github.com/lunelson/starbase-cli/internal/database"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all tracked repositories",
	Long: `List all repositories tracked in the starbase database.

Examples:
  starbase list
  starbase list --status=active
  starbase list --json`,
	RunE: runList,
}

var (
	listStatus string
	listJSON   bool
	listYAML   bool
	listPaths  bool
)

func init() {
	rootCmd.AddCommand(listCmd)

	listCmd.Flags().StringVar(&listStatus, "status", "", "Filter by status (active, archived, unavailable)")
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Output as JSON")
	listCmd.Flags().BoolVar(&listYAML, "yaml", false, "Output as YAML")
	listCmd.Flags().BoolVar(&listPaths, "paths", false, "Output only local paths (for piping)")
}

func runList(cmd *cobra.Command, args []string) error {
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

	repos, err := db.ListRepos(listStatus)
	if err != nil {
		return fmt.Errorf("listing repos: %w", err)
	}

	if len(repos) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No repositories found.")
		fmt.Fprintln(cmd.OutOrStdout(), "Run 'starbase sync' to fetch starred repos.")
		return nil
	}

	switch {
	case listPaths:
		return outputListPaths(cmd, repos)
	case listJSON:
		return outputListJSON(cmd, repos, db)
	case listYAML:
		return outputListYAML(cmd, repos, db)
	default:
		return outputListTable(cmd, repos, db)
	}
}

func outputListPaths(cmd *cobra.Command, repos []*database.Repo) error {
	for _, r := range repos {
		if r.LocalPath != nil {
			fmt.Fprintln(cmd.OutOrStdout(), *r.LocalPath)
		}
	}
	return nil
}

type listOutput struct {
	Forge       string   `json:"forge" yaml:"forge"`
	FullName    string   `json:"full_name" yaml:"full_name"`
	WebURL      string   `json:"web_url" yaml:"web_url"`
	LocalPath   string   `json:"local_path,omitempty" yaml:"local_path,omitempty"`
	Status      string   `json:"status" yaml:"status"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Language    string   `json:"language,omitempty" yaml:"language,omitempty"`
	Topics      []string `json:"topics,omitempty" yaml:"topics,omitempty"`
	Stars       int      `json:"stars,omitempty" yaml:"stars,omitempty"`
}

func outputListJSON(cmd *cobra.Command, repos []*database.Repo, db *database.DB) error {
	out := make([]listOutput, 0, len(repos))
	for _, r := range repos {
		item := listOutput{
			Forge:    r.Forge,
			FullName: r.FullName,
			WebURL:   r.WebURL,
			Status:   r.Status,
		}
		if r.LocalPath != nil {
			item.LocalPath = *r.LocalPath
		}

		// Get metadata
		meta, _ := db.GetMetadata(r.ID)
		if meta != nil {
			if meta.Description != nil {
				item.Description = *meta.Description
			}
			if meta.Language != nil {
				item.Language = *meta.Language
			}
			item.Topics = meta.Topics
			if meta.StarsCount != nil {
				item.Stars = *meta.StarsCount
			}
		}

		out = append(out, item)
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func outputListYAML(cmd *cobra.Command, repos []*database.Repo, db *database.DB) error {
	out := make([]listOutput, 0, len(repos))
	for _, r := range repos {
		item := listOutput{
			Forge:    r.Forge,
			FullName: r.FullName,
			WebURL:   r.WebURL,
			Status:   r.Status,
		}
		if r.LocalPath != nil {
			item.LocalPath = *r.LocalPath
		}

		meta, _ := db.GetMetadata(r.ID)
		if meta != nil {
			if meta.Description != nil {
				item.Description = *meta.Description
			}
			if meta.Language != nil {
				item.Language = *meta.Language
			}
			item.Topics = meta.Topics
			if meta.StarsCount != nil {
				item.Stars = *meta.StarsCount
			}
		}

		out = append(out, item)
	}

	enc := yaml.NewEncoder(cmd.OutOrStdout())
	return enc.Encode(out)
}

func outputListTable(cmd *cobra.Command, repos []*database.Repo, db *database.DB) error {
	fmt.Fprintf(cmd.OutOrStdout(), "Tracked repositories: %d\n\n", len(repos))

	// Group by status
	byStatus := make(map[string][]*database.Repo)
	for _, r := range repos {
		byStatus[r.Status] = append(byStatus[r.Status], r)
	}

	for _, status := range []string{"active", "archived", "unavailable", "pending"} {
		repoList := byStatus[status]
		if len(repoList) == 0 {
			continue
		}

		fmt.Fprintf(cmd.OutOrStdout(), "── %s (%d) ──\n", strings.ToUpper(status), len(repoList))

		for _, r := range repoList {
			meta, _ := db.GetMetadata(r.ID)

			// Repo name
			fmt.Fprintf(cmd.OutOrStdout(), "  %s/%s", r.Forge, r.FullName)

			// Language
			if meta != nil && meta.Language != nil && *meta.Language != "" {
				fmt.Fprintf(cmd.OutOrStdout(), " [%s]", *meta.Language)
			}

			// Cloned indicator
			if r.LocalPath != nil {
				fmt.Fprintf(cmd.OutOrStdout(), " ✓")
			}

			fmt.Fprintln(cmd.OutOrStdout())
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}

	// Summary
	cloned := 0
	for _, r := range repos {
		if r.LocalPath != nil {
			cloned++
		}
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Cloned: %d/%d\n", cloned, len(repos))

	return nil
}
