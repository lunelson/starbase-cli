package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lunelson/starbase-cli/internal/config"
	"github.com/lunelson/starbase-cli/internal/database"
	"github.com/lunelson/starbase-cli/internal/search"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var exportCmd = &cobra.Command{
	Use:   "export [query]",
	Short: "Export repository information",
	Long: `Export repository information in various formats.

If a query is provided, exports only matching repos.
Without a query, exports all tracked repos.

Examples:
  starbase export --format=paths           # Local paths only
  starbase export "tui" --format=json      # Search and export as JSON
  starbase export --format=markdown        # All repos as markdown
  starbase export --include-readme         # Include README content`,
	RunE: runExport,
}

var (
	exportFormat        string
	exportIncludeReadme bool
	exportOutput        string
)

func init() {
	rootCmd.AddCommand(exportCmd)

	exportCmd.Flags().StringVarP(&exportFormat, "format", "f", "paths", "Output format: paths, json, yaml, markdown")
	exportCmd.Flags().BoolVar(&exportIncludeReadme, "include-readme", false, "Include README content")
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "Write to file instead of stdout")
}

func runExport(cmd *cobra.Command, args []string) error {
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

	var repos []exportRepo

	if len(args) > 0 {
		// Search mode
		query := strings.Join(args, " ")
		searcher := search.New(db.DB)
		results, err := searcher.Search(query, 1000)
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}

		for _, r := range results {
			repos = append(repos, toExportRepo(r, db, exportIncludeReadme))
		}
	} else {
		// All repos mode
		dbRepos, err := db.ListRepos("")
		if err != nil {
			return fmt.Errorf("listing repos: %w", err)
		}

		for _, r := range dbRepos {
			repos = append(repos, dbRepoToExport(r, db, exportIncludeReadme))
		}
	}

	if len(repos) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No repositories to export.")
		return nil
	}

	// Determine output
	var out = cmd.OutOrStdout()
	if exportOutput != "" {
		f, err := os.Create(exportOutput)
		if err != nil {
			return fmt.Errorf("creating output file: %w", err)
		}
		defer f.Close()
		out = f
	}

	switch exportFormat {
	case "paths":
		return exportPaths(out, repos)
	case "json":
		return exportJSON(out, repos)
	case "yaml":
		return exportYAML(out, repos)
	case "markdown":
		return exportMarkdown(out, repos)
	default:
		return fmt.Errorf("unknown format: %s", exportFormat)
	}
}

type exportRepo struct {
	Forge       string   `json:"forge" yaml:"forge"`
	FullName    string   `json:"full_name" yaml:"full_name"`
	WebURL      string   `json:"web_url" yaml:"web_url"`
	LocalPath   string   `json:"local_path,omitempty" yaml:"local_path,omitempty"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Language    string   `json:"language,omitempty" yaml:"language,omitempty"`
	Topics      []string `json:"topics,omitempty" yaml:"topics,omitempty"`
	Stars       int      `json:"stars,omitempty" yaml:"stars,omitempty"`
	Readme      string   `json:"readme,omitempty" yaml:"readme,omitempty"`
}

func toExportRepo(r search.Result, db *database.DB, includeReadme bool) exportRepo {
	e := exportRepo{
		Forge:    r.Forge,
		FullName: r.FullName,
		WebURL:   r.WebURL,
		Topics:   r.Topics,
	}

	if r.LocalPath != nil {
		e.LocalPath = *r.LocalPath
	}
	if r.Description != nil {
		e.Description = *r.Description
	}
	if r.Language != nil {
		e.Language = *r.Language
	}
	if r.StarsCount != nil {
		e.Stars = *r.StarsCount
	}

	if includeReadme && e.LocalPath != "" {
		doc, _ := db.GetDocument(r.RepoID, "readme", "README.md")
		if doc != nil && doc.Content != nil {
			e.Readme = *doc.Content
		}
	}

	return e
}

func dbRepoToExport(r *database.Repo, db *database.DB, includeReadme bool) exportRepo {
	e := exportRepo{
		Forge:    r.Forge,
		FullName: r.FullName,
		WebURL:   r.WebURL,
	}

	if r.LocalPath != nil {
		e.LocalPath = *r.LocalPath
	}

	meta, _ := db.GetMetadata(r.ID)
	if meta != nil {
		if meta.Description != nil {
			e.Description = *meta.Description
		}
		if meta.Language != nil {
			e.Language = *meta.Language
		}
		e.Topics = meta.Topics
		if meta.StarsCount != nil {
			e.Stars = *meta.StarsCount
		}
	}

	if includeReadme && e.LocalPath != "" {
		// Try to find any readme file
		for _, filename := range []string{"README.md", "readme.md", "README.txt"} {
			doc, _ := db.GetDocument(r.ID, "readme", filename)
			if doc != nil && doc.Content != nil {
				e.Readme = *doc.Content
				break
			}
		}
	}

	return e
}

func exportPaths(out interface{ Write([]byte) (int, error) }, repos []exportRepo) error {
	for _, r := range repos {
		if r.LocalPath != "" {
			fmt.Fprintln(out, r.LocalPath)
		}
	}
	return nil
}

func exportJSON(out interface{ Write([]byte) (int, error) }, repos []exportRepo) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(repos)
}

func exportYAML(out interface{ Write([]byte) (int, error) }, repos []exportRepo) error {
	enc := yaml.NewEncoder(out)
	return enc.Encode(repos)
}

func exportMarkdown(out interface{ Write([]byte) (int, error) }, repos []exportRepo) error {
	fmt.Fprintln(out, "# Starred Repositories")
	fmt.Fprintln(out)

	// Group by language
	byLang := make(map[string][]exportRepo)
	for _, r := range repos {
		lang := r.Language
		if lang == "" {
			lang = "Other"
		}
		byLang[lang] = append(byLang[lang], r)
	}

	for lang, langRepos := range byLang {
		fmt.Fprintf(out, "## %s\n\n", lang)

		for _, r := range langRepos {
			fmt.Fprintf(out, "### [%s](%s)\n\n", r.FullName, r.WebURL)

			if r.Description != "" {
				fmt.Fprintf(out, "%s\n\n", r.Description)
			}

			if len(r.Topics) > 0 {
				fmt.Fprintf(out, "**Topics:** %s\n\n", strings.Join(r.Topics, ", "))
			}

			if r.LocalPath != "" {
				// Make path relative for portability
				relPath := r.LocalPath
				if home, _ := os.UserHomeDir(); home != "" {
					if rel, err := filepath.Rel(home, r.LocalPath); err == nil {
						relPath = "~/" + rel
					}
				}
				fmt.Fprintf(out, "**Local:** `%s`\n\n", relPath)
			}

			if r.Readme != "" {
				fmt.Fprintln(out, "<details>")
				fmt.Fprintln(out, "<summary>README</summary>")
				fmt.Fprintln(out)
				fmt.Fprintln(out, r.Readme)
				fmt.Fprintln(out, "</details>")
				fmt.Fprintln(out)
			}

			fmt.Fprintln(out, "---")
			fmt.Fprintln(out)
		}
	}

	return nil
}
