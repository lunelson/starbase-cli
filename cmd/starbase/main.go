package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	buildTime = "unknown"
	devBuild  = "true" // Set to "false" via ldflags for release builds
)

func main() {
	// Show dev build header for local development
	if devBuild == "true" {
		fmt.Fprintf(os.Stderr, "\033[2m[dev build: %s @ %s]\033[0m\n", version, buildTime)
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "starbase",
	Short: "A searchable local mirror of your starred repositories",
	Long: `starbase-cli maintains a searchable local mirror of GitHub/GitLab 
starred repositories, optimized for LLM-assisted development workflows.`,
	Version: version,
}

func init() {
	rootCmd.SetVersionTemplate(fmt.Sprintf("starbase version {{.Version}} (built %s)\n", buildTime))
}
