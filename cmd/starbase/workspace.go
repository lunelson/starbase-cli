package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lunelson/starbase-cli/internal/config"
	"github.com/lunelson/starbase-cli/internal/database"
	"github.com/lunelson/starbase-cli/internal/search"
	"github.com/spf13/cobra"
)

var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Manage workspaces",
	Long:  `Create and manage workspaces containing selected repositories.`,
}

var workspaceCreateCmd = &cobra.Command{
	Use:   "create <name> [query]",
	Short: "Create a workspace with selected repos",
	Long: `Create a workspace directory containing symlinks or copies of selected repos.

If a query is provided, only matching repos are included.
Without a query, all cloned repos are included.

Examples:
  starbase workspace create my-tui-tools "tui"
  starbase workspace create go-projects "lang:go" --output=~/workspaces
  starbase workspace create all-repos --copy`,
	Args: cobra.MinimumNArgs(1),
	RunE: runWorkspaceCreate,
}

var (
	workspaceOutput  string
	workspaceSymlink bool
	workspaceCopy    bool
)

func init() {
	rootCmd.AddCommand(workspaceCmd)
	workspaceCmd.AddCommand(workspaceCreateCmd)

	workspaceCreateCmd.Flags().StringVarP(&workspaceOutput, "output", "o", ".", "Output directory for the workspace")
	workspaceCreateCmd.Flags().BoolVar(&workspaceSymlink, "symlink", true, "Create symlinks to repos (default)")
	workspaceCreateCmd.Flags().BoolVar(&workspaceCopy, "copy", false, "Copy repos instead of symlinking")
}

type workspaceRepo struct {
	FullName    string
	WebURL      string
	LocalPath   string
	Description string
	Language    string
	Topics      []string
}

func runWorkspaceCreate(cmd *cobra.Command, args []string) error {
	name := args[0]
	var query string
	if len(args) > 1 {
		query = strings.Join(args[1:], " ")
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

	var repos []workspaceRepo

	if query != "" {
		searcher := search.New(db.DB)
		results, err := searcher.Search(query, 1000)
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}

		for _, r := range results {
			if r.LocalPath == nil || *r.LocalPath == "" {
				continue
			}
			wr := workspaceRepo{
				FullName:  r.FullName,
				WebURL:    r.WebURL,
				LocalPath: *r.LocalPath,
				Topics:    r.Topics,
			}
			if r.Description != nil {
				wr.Description = *r.Description
			}
			if r.Language != nil {
				wr.Language = *r.Language
			}
			repos = append(repos, wr)
		}
	} else {
		dbRepos, err := db.ListRepos("")
		if err != nil {
			return fmt.Errorf("listing repos: %w", err)
		}

		for _, r := range dbRepos {
			if r.LocalPath == nil || *r.LocalPath == "" {
				continue
			}

			wr := workspaceRepo{
				FullName:  r.FullName,
				WebURL:    r.WebURL,
				LocalPath: *r.LocalPath,
			}

			meta, _ := db.GetMetadata(r.ID)
			if meta != nil {
				if meta.Description != nil {
					wr.Description = *meta.Description
				}
				if meta.Language != nil {
					wr.Language = *meta.Language
				}
				wr.Topics = meta.Topics
			}

			repos = append(repos, wr)
		}
	}

	if len(repos) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No cloned repositories found.")
		return nil
	}

	outputDir, err := filepath.Abs(workspaceOutput)
	if err != nil {
		return fmt.Errorf("resolving output path: %w", err)
	}

	workspaceDir := filepath.Join(outputDir, name)

	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		return fmt.Errorf("creating workspace directory: %w", err)
	}

	useCopy := workspaceCopy

	fmt.Fprintf(cmd.OutOrStdout(), "Creating workspace '%s' with %d repos...\n", name, len(repos))

	for _, repo := range repos {
		dirName := sanitizeRepoName(repo.FullName)
		targetPath := filepath.Join(workspaceDir, dirName)

		if _, err := os.Lstat(targetPath); err == nil {
			fmt.Fprintf(cmd.OutOrStdout(), "  Skipping %s (already exists)\n", dirName)
			continue
		}

		if useCopy {
			if err := copyDir(repo.LocalPath, targetPath); err != nil {
				fmt.Fprintf(cmd.OutOrStderr(), "  Warning: failed to copy %s: %v\n", repo.FullName, err)
				continue
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  Copied %s\n", repo.FullName)
		} else {
			if err := os.Symlink(repo.LocalPath, targetPath); err != nil {
				fmt.Fprintf(cmd.OutOrStderr(), "  Warning: failed to symlink %s: %v\n", repo.FullName, err)
				continue
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  Linked %s\n", repo.FullName)
		}
	}

	if err := generateContextMD(workspaceDir, name, repos); err != nil {
		return fmt.Errorf("generating CONTEXT.md: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nWorkspace created at: %s\n", workspaceDir)
	fmt.Fprintf(cmd.OutOrStdout(), "CONTEXT.md generated with %d repositories.\n", len(repos))

	return nil
}

func sanitizeRepoName(fullName string) string {
	return strings.ReplaceAll(fullName, "/", "-")
}

func generateContextMD(workspaceDir, name string, repos []workspaceRepo) error {
	contextPath := filepath.Join(workspaceDir, "CONTEXT.md")
	f, err := os.Create(contextPath)
	if err != nil {
		return err
	}
	defer f.Close()

	timestamp := time.Now().Format("2006-01-02 15:04:05")

	fmt.Fprintf(f, "# Workspace: %s\n\n", name)
	fmt.Fprintf(f, "Created: %s\n\n", timestamp)

	fmt.Fprintf(f, "## Repositories (%d)\n\n", len(repos))
	fmt.Fprintln(f, "| Repository | Language | Description |")
	fmt.Fprintln(f, "|------------|----------|-------------|")

	langCounts := make(map[string]int)
	topicCounts := make(map[string]int)

	for _, repo := range repos {
		dirName := sanitizeRepoName(repo.FullName)
		desc := repo.Description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		desc = strings.ReplaceAll(desc, "|", "\\|")

		lang := repo.Language
		if lang == "" {
			lang = "-"
		} else {
			langCounts[lang]++
		}

		fmt.Fprintf(f, "| [%s](./%s) | %s | %s |\n", repo.FullName, dirName, lang, desc)

		for _, topic := range repo.Topics {
			topicCounts[topic]++
		}
	}

	fmt.Fprintln(f)
	fmt.Fprintln(f, "## Topics")
	fmt.Fprintln(f)

	if len(topicCounts) > 0 {
		type topicCount struct {
			topic string
			count int
		}
		var sortedTopics []topicCount
		for t, c := range topicCounts {
			sortedTopics = append(sortedTopics, topicCount{t, c})
		}
		sort.Slice(sortedTopics, func(i, j int) bool {
			return sortedTopics[i].count > sortedTopics[j].count
		})

		var topicStrs []string
		for _, tc := range sortedTopics {
			if tc.count > 1 {
				topicStrs = append(topicStrs, fmt.Sprintf("%s (%d)", tc.topic, tc.count))
			} else {
				topicStrs = append(topicStrs, tc.topic)
			}
		}
		fmt.Fprintln(f, strings.Join(topicStrs, ", "))
	} else {
		fmt.Fprintln(f, "No topics available.")
	}

	fmt.Fprintln(f)
	fmt.Fprintln(f, "## Quick Reference")
	fmt.Fprintln(f)
	fmt.Fprintf(f, "- Total repos: %d\n", len(repos))

	if len(langCounts) > 0 {
		type langCount struct {
			lang  string
			count int
		}
		var sortedLangs []langCount
		for l, c := range langCounts {
			sortedLangs = append(sortedLangs, langCount{l, c})
		}
		sort.Slice(sortedLangs, func(i, j int) bool {
			return sortedLangs[i].count > sortedLangs[j].count
		})

		var langStrs []string
		for _, lc := range sortedLangs {
			langStrs = append(langStrs, fmt.Sprintf("%s (%d)", lc.lang, lc.count))
		}
		fmt.Fprintf(f, "- Languages: %s\n", strings.Join(langStrs, ", "))
	}

	return nil
}

func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
