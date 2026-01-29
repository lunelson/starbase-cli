package main

import (
	"fmt"
	"os"

	"github.com/lunelson/starbase-cli/internal/config"
	"github.com/lunelson/starbase-cli/internal/database"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize starbase directories and database",
	Long: `Initialize starbase by creating config and data directories,
default configuration files, and the SQLite database.`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	configDir := config.DefaultConfigDir()
	cfg, err := config.Load(configDir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	paths := config.ResolvePaths(cfg)

	// Create directories
	dirs := []string{
		paths.ConfigDir,
		paths.DataDir,
		paths.ClonesDir,
		paths.CacheDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Created %s\n", dir)
	}

	// Create default config if it doesn't exist
	configPath := paths.ConfigDir + "/config.yaml"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		defaultConfig := `# starbase configuration
version: 1

# Clone settings
clone:
  depth: 1
  single_branch: true
  skip_submodules: true
  skip_lfs: true

# Sync policies
sync:
  default_window: 30d
  clone_missing: true
  clone_private: false
  clone_archived: false
  max_repos_per_sync: 100

# Search settings
search:
  engine: bm25
  default_limit: 20

# Forges
forges:
  github:
    enabled: true
  gitlab:
    enabled: false
`
		if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
			return fmt.Errorf("writing config: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Created %s\n", configPath)
	}

	// Create empty manifest if it doesn't exist
	manifestPath := paths.ConfigDir + "/manifest.yaml"
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		manifest := &config.Manifest{Version: 1}
		if err := config.SaveManifest(paths.ConfigDir, manifest); err != nil {
			return fmt.Errorf("writing manifest: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Created %s\n", manifestPath)
	}

	// Initialize database
	db, err := database.Open(paths.DBPath)
	if err != nil {
		return fmt.Errorf("initializing database: %w", err)
	}
	db.Close()
	fmt.Fprintf(cmd.OutOrStdout(), "Initialized database %s\n", paths.DBPath)

	fmt.Fprintln(cmd.OutOrStdout(), "\nStarbase initialized successfully!")
	fmt.Fprintf(cmd.OutOrStdout(), "Config: %s\n", paths.ConfigDir)
	fmt.Fprintf(cmd.OutOrStdout(), "Data:   %s\n", paths.DataDir)

	return nil
}
