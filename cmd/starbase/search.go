package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lunelson/starbase-cli/internal/config"
	"github.com/lunelson/starbase-cli/internal/database"
	"github.com/lunelson/starbase-cli/internal/search"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search tracked repositories",
	Long: `Search your starred repositories using full-text search.

Examples:
  starbase search "tui framework"
  starbase search "machine learning" --limit=50
  starbase search "go cli" --json`,
	Args: cobra.MinimumNArgs(1),
	RunE: runSearch,
}

var (
	searchLimit    int
	searchJSON     bool
	searchYAML     bool
	searchMarkdown bool
)

func init() {
	rootCmd.AddCommand(searchCmd)

	searchCmd.Flags().IntVarP(&searchLimit, "limit", "n", 20, "Maximum number of results")
	searchCmd.Flags().BoolVar(&searchJSON, "json", false, "Output as JSON")
	searchCmd.Flags().BoolVar(&searchYAML, "yaml", false, "Output as YAML")
	searchCmd.Flags().BoolVar(&searchMarkdown, "markdown", false, "Output as Markdown")
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := strings.Join(args, " ")

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

	searcher := search.New(db.DB)
	results, err := searcher.Search(query, searchLimit)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No results found.")
		return nil
	}

	// Output based on format
	switch {
	case searchJSON:
		return outputJSON(cmd, results)
	case searchYAML:
		return outputYAML(cmd, results)
	case searchMarkdown:
		return outputMarkdown(cmd, results)
	default:
		return outputTable(cmd, results)
	}
}

type searchOutput struct {
	Forge       string   `json:"forge" yaml:"forge"`
	FullName    string   `json:"full_name" yaml:"full_name"`
	WebURL      string   `json:"web_url" yaml:"web_url"`
	LocalPath   string   `json:"local_path,omitempty" yaml:"local_path,omitempty"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Language    string   `json:"language,omitempty" yaml:"language,omitempty"`
	Topics      []string `json:"topics,omitempty" yaml:"topics,omitempty"`
	Stars       int      `json:"stars,omitempty" yaml:"stars,omitempty"`
}

func toOutput(r search.Result) searchOutput {
	out := searchOutput{
		Forge:    r.Forge,
		FullName: r.FullName,
		WebURL:   r.WebURL,
		Topics:   r.Topics,
	}
	if r.LocalPath != nil {
		out.LocalPath = *r.LocalPath
	}
	if r.Description != nil {
		out.Description = *r.Description
	}
	if r.Language != nil {
		out.Language = *r.Language
	}
	if r.StarsCount != nil {
		out.Stars = *r.StarsCount
	}
	return out
}

func outputJSON(cmd *cobra.Command, results []search.Result) error {
	out := make([]searchOutput, len(results))
	for i, r := range results {
		out[i] = toOutput(r)
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func outputYAML(cmd *cobra.Command, results []search.Result) error {
	out := make([]searchOutput, len(results))
	for i, r := range results {
		out[i] = toOutput(r)
	}

	enc := yaml.NewEncoder(cmd.OutOrStdout())
	return enc.Encode(out)
}

func outputMarkdown(cmd *cobra.Command, results []search.Result) error {
	for _, r := range results {
		fmt.Fprintf(cmd.OutOrStdout(), "## [%s](%s)\n\n", r.FullName, r.WebURL)
		if r.Description != nil && *r.Description != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n\n", *r.Description)
		}
		if r.Language != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "**Language:** %s", *r.Language)
		}
		if r.StarsCount != nil {
			fmt.Fprintf(cmd.OutOrStdout(), " | **Stars:** %d", *r.StarsCount)
		}
		fmt.Fprintln(cmd.OutOrStdout())
		if len(r.Topics) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "**Topics:** %s\n", strings.Join(r.Topics, ", "))
		}
		if r.LocalPath != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "**Local:** `%s`\n", *r.LocalPath)
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}
	return nil
}

func outputTable(cmd *cobra.Command, results []search.Result) error {
	fmt.Fprintf(cmd.OutOrStdout(), "Found %d results:\n\n", len(results))

	for i, r := range results {
		// Repo name with forge prefix
		fmt.Fprintf(cmd.OutOrStdout(), "%2d. %s/%s", i+1, r.Forge, r.FullName)

		// Language badge
		if r.Language != nil && *r.Language != "" {
			fmt.Fprintf(cmd.OutOrStdout(), " [%s]", *r.Language)
		}

		// Stars
		if r.StarsCount != nil {
			fmt.Fprintf(cmd.OutOrStdout(), " ⭐%d", *r.StarsCount)
		}

		fmt.Fprintln(cmd.OutOrStdout())

		// Description (truncated)
		if r.Description != nil && *r.Description != "" {
			desc := *r.Description
			if len(desc) > 80 {
				desc = desc[:77] + "..."
			}
			fmt.Fprintf(cmd.OutOrStdout(), "    %s\n", desc)
		}

		// Local path
		if r.LocalPath != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "    📁 %s\n", *r.LocalPath)
		}
	}

	return nil
}
