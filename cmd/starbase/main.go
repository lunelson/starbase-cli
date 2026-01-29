package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
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
	rootCmd.SetVersionTemplate("starbase version {{.Version}}\n")
}
