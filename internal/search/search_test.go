package search

import (
	"path/filepath"
	"testing"

	"github.com/lunelson/starbase-cli/internal/database"
)

func TestSearchIndex(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Insert test repo
	repo := &database.Repo{
		Forge:    "github",
		ForgeID:  "123",
		Owner:    "charmbracelet",
		Name:     "bubbletea",
		FullName: "charmbracelet/bubbletea",
		CloneURL: "https://github.com/charmbracelet/bubbletea.git",
		WebURL:   "https://github.com/charmbracelet/bubbletea",
		Status:   "active",
	}
	repoID, _ := db.InsertRepo(repo)

	desc := "A powerful TUI framework for Go"
	db.UpsertMetadata(&database.RepoMetadata{
		RepoID:      repoID,
		Description: &desc,
		Topics:      []string{"tui", "terminal", "go"},
	})

	// Index repo
	searcher := New(db.DB)
	err = searcher.IndexRepo(repoID, "bubbletea", "A powerful TUI framework for Go", "tui terminal go", "", "")
	if err != nil {
		t.Fatalf("IndexRepo() error = %v", err)
	}

	// Search
	results, err := searcher.Search("tui framework", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Results = %d, want 1", len(results))
	}

	if results[0].FullName != "charmbracelet/bubbletea" {
		t.Errorf("FullName = %q, want charmbracelet/bubbletea", results[0].FullName)
	}
}

func TestSearchMultipleRepos(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Insert multiple repos
	repos := []struct {
		name        string
		description string
		topics      string
	}{
		{"bubbletea", "A powerful TUI framework", "tui terminal"},
		{"lipgloss", "Style definitions for nice terminal layouts", "tui styling"},
		{"cobra", "A Commander for modern Go CLI", "cli commands"},
	}

	searcher := New(db.DB)

	for i, r := range repos {
		repo := &database.Repo{
			Forge:    "github",
			ForgeID:  string(rune('1' + i)),
			Owner:    "test",
			Name:     r.name,
			FullName: "test/" + r.name,
			CloneURL: "https://github.com/test/" + r.name + ".git",
			WebURL:   "https://github.com/test/" + r.name,
			Status:   "active",
		}
		repoID, _ := db.InsertRepo(repo)
		searcher.IndexRepo(repoID, r.name, r.description, r.topics, "", "")
	}

	// Search for TUI should return 2 results
	results, err := searcher.Search("tui", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Errorf("TUI results = %d, want 2", len(results))
	}

	// Search for CLI should return 1 result
	results, err = searcher.Search("cli", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("CLI results = %d, want 1", len(results))
	}
}

func TestSearchLimit(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	searcher := New(db.DB)

	// Insert 10 repos
	for i := 0; i < 10; i++ {
		repo := &database.Repo{
			Forge:    "github",
			ForgeID:  string(rune('a' + i)),
			Owner:    "test",
			Name:     "repo" + string(rune('0'+i)),
			FullName: "test/repo" + string(rune('0'+i)),
			CloneURL: "https://github.com/test/repo.git",
			WebURL:   "https://github.com/test/repo",
			Status:   "active",
		}
		repoID, _ := db.InsertRepo(repo)
		searcher.IndexRepo(repoID, "repo", "test repository", "test", "", "")
	}

	// Limit results
	results, err := searcher.Search("repo", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 5 {
		t.Errorf("Results = %d, want 5", len(results))
	}
}

func TestRebuildIndex(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Insert repo with metadata
	repo := &database.Repo{
		Forge:    "github",
		ForgeID:  "123",
		Owner:    "test",
		Name:     "myrepo",
		FullName: "test/myrepo",
		CloneURL: "https://github.com/test/myrepo.git",
		WebURL:   "https://github.com/test/myrepo",
		Status:   "active",
	}
	repoID, _ := db.InsertRepo(repo)

	desc := "My test repository"
	db.UpsertMetadata(&database.RepoMetadata{
		RepoID:      repoID,
		Description: &desc,
		Topics:      []string{"testing", "example"},
	})

	// Rebuild index
	searcher := New(db.DB)
	count, err := searcher.RebuildIndex()
	if err != nil {
		t.Fatalf("RebuildIndex() error = %v", err)
	}
	if count != 1 {
		t.Errorf("Indexed count = %d, want 1", count)
	}

	// Search should work now
	results, err := searcher.Search("test", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("Results = %d, want 1", len(results))
	}
}

func TestSanitizeFTSQuery(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple*"},
		{"multi word", "multi* word*"},
		{"go-sqlite", "go-sqlite*"},
		{"special@chars!", "specialchars*"},
		{"lang:go query", "query*"}, // Filter removed
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeFTSQuery(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizeFTSQuery(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestRemoveRepo(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	repo := &database.Repo{
		Forge:    "github",
		ForgeID:  "123",
		Owner:    "test",
		Name:     "repo",
		FullName: "test/repo",
		CloneURL: "https://github.com/test/repo.git",
		WebURL:   "https://github.com/test/repo",
		Status:   "active",
	}
	repoID, _ := db.InsertRepo(repo)

	searcher := New(db.DB)
	searcher.IndexRepo(repoID, "repo", "test", "test", "", "")

	// Verify indexed
	results, _ := searcher.Search("repo", 10)
	if len(results) != 1 {
		t.Fatal("Repo not indexed")
	}

	// Remove from index
	err = searcher.RemoveRepo(repoID)
	if err != nil {
		t.Fatalf("RemoveRepo() error = %v", err)
	}

	// Should not find anymore
	results, _ = searcher.Search("repo", 10)
	if len(results) != 0 {
		t.Error("Repo still in index after remove")
	}
}
