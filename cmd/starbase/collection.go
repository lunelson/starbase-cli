package main

import (
	"fmt"
	"strings"

	"github.com/lunelson/starbase-cli/internal/config"
	"github.com/lunelson/starbase-cli/internal/database"
	"github.com/lunelson/starbase-cli/internal/search"
	"github.com/spf13/cobra"
)

var collectionCmd = &cobra.Command{
	Use:     "collection",
	Aliases: []string{"coll"},
	Short:   "Manage collections",
	Long:    `Commands for managing local repository collections.`,
}

var collectionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all collections",
	RunE:  runCollectionList,
}

var collectionCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new collection",
	Args:  cobra.ExactArgs(1),
	RunE:  runCollectionCreate,
}

var collectionDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a collection",
	Args:  cobra.ExactArgs(1),
	RunE:  runCollectionDelete,
}

var collectionAddCmd = &cobra.Command{
	Use:   "add <collection> <query>",
	Short: "Add repos matching query to a collection",
	Args:  cobra.MinimumNArgs(2),
	RunE:  runCollectionAdd,
}

var collectionRemoveCmd = &cobra.Command{
	Use:   "remove <collection> <query>",
	Short: "Remove repos matching query from a collection",
	Args:  cobra.MinimumNArgs(2),
	RunE:  runCollectionRemove,
}

var (
	collectionDesc  string
	collectionColor string
)

func init() {
	rootCmd.AddCommand(collectionCmd)
	collectionCmd.AddCommand(collectionListCmd)
	collectionCmd.AddCommand(collectionCreateCmd)
	collectionCmd.AddCommand(collectionDeleteCmd)
	collectionCmd.AddCommand(collectionAddCmd)
	collectionCmd.AddCommand(collectionRemoveCmd)

	collectionCreateCmd.Flags().StringVar(&collectionDesc, "description", "", "Collection description")
	collectionCreateCmd.Flags().StringVar(&collectionColor, "color", "", "Collection color (hex)")
}

func runCollectionList(cmd *cobra.Command, args []string) error {
	configDir := config.DefaultConfigDir()
	manifest, err := config.LoadManifest(configDir)
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	if len(manifest.Collections) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No collections defined.")
		fmt.Fprintln(cmd.OutOrStdout(), "Create one with: starbase collection create <name>")
		return nil
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Collections:")
	for _, c := range manifest.Collections {
		fmt.Fprintf(cmd.OutOrStdout(), "  • %s", c.Name)
		if c.Description != "" {
			fmt.Fprintf(cmd.OutOrStdout(), " - %s", c.Description)
		}
		count := len(manifest.ReposInCollection(c.Name))
		fmt.Fprintf(cmd.OutOrStdout(), " (%d repos)\n", count)
	}

	return nil
}

func runCollectionCreate(cmd *cobra.Command, args []string) error {
	name := args[0]
	configDir := config.DefaultConfigDir()

	manifest, err := config.LoadManifest(configDir)
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	// Check if exists
	if manifest.GetCollection(name) != nil {
		return fmt.Errorf("collection %q already exists", name)
	}

	manifest.AddCollection(config.Collection{
		Name:        name,
		Description: collectionDesc,
		Color:       collectionColor,
	})

	if err := config.SaveManifest(configDir, manifest); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Created collection: %s\n", name)
	return nil
}

func runCollectionDelete(cmd *cobra.Command, args []string) error {
	name := args[0]
	configDir := config.DefaultConfigDir()

	manifest, err := config.LoadManifest(configDir)
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	// Find and remove
	found := false
	for i, c := range manifest.Collections {
		if c.Name == name {
			manifest.Collections = append(manifest.Collections[:i], manifest.Collections[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("collection %q not found", name)
	}

	// Remove collection from all repos
	for i := range manifest.Repos {
		var newColls []string
		for _, c := range manifest.Repos[i].Collections {
			if c != name {
				newColls = append(newColls, c)
			}
		}
		manifest.Repos[i].Collections = newColls
	}

	if err := config.SaveManifest(configDir, manifest); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Deleted collection: %s\n", name)
	return nil
}

func runCollectionAdd(cmd *cobra.Command, args []string) error {
	collName := args[0]
	query := strings.Join(args[1:], " ")

	configDir := config.DefaultConfigDir()
	cfg, err := config.Load(configDir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	manifest, err := config.LoadManifest(configDir)
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	// Ensure collection exists
	if manifest.GetCollection(collName) == nil {
		return fmt.Errorf("collection %q not found", collName)
	}

	paths := config.ResolvePaths(cfg)
	db, err := database.Open(paths.DBPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	// Search for repos
	searcher := search.New(db.DB)
	results, err := searcher.Search(query, 100)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No repos matched the query.")
		return nil
	}

	// Add each repo to collection
	added := 0
	for _, r := range results {
		repo := manifest.GetRepo(r.Forge, r.Owner, r.Name)
		if repo == nil {
			// Add to manifest
			manifest.AddRepo(config.ManifestRepo{
				Forge:       r.Forge,
				Owner:       r.Owner,
				Name:        r.Name,
				Collections: []string{collName},
			})
			added++
		} else {
			// Update existing
			hasCollection := false
			for _, c := range repo.Collections {
				if c == collName {
					hasCollection = true
					break
				}
			}
			if !hasCollection {
				repo.Collections = append(repo.Collections, collName)
				added++
			}
		}
	}

	if err := config.SaveManifest(configDir, manifest); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Added %d repos to collection %q\n", added, collName)
	return nil
}

func runCollectionRemove(cmd *cobra.Command, args []string) error {
	collName := args[0]
	query := strings.Join(args[1:], " ")

	configDir := config.DefaultConfigDir()
	cfg, err := config.Load(configDir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	manifest, err := config.LoadManifest(configDir)
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	paths := config.ResolvePaths(cfg)
	db, err := database.Open(paths.DBPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	// Search for repos
	searcher := search.New(db.DB)
	results, err := searcher.Search(query, 100)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No repos matched the query.")
		return nil
	}

	removed := 0
	for _, r := range results {
		repo := manifest.GetRepo(r.Forge, r.Owner, r.Name)
		if repo != nil {
			var newColls []string
			for _, c := range repo.Collections {
				if c != collName {
					newColls = append(newColls, c)
				} else {
					removed++
				}
			}
			repo.Collections = newColls
		}
	}

	if err := config.SaveManifest(configDir, manifest); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Removed %d repos from collection %q\n", removed, collName)
	return nil
}
