package index

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lunelson/starbase-cli/internal/database"
	"github.com/lunelson/starbase-cli/internal/search"
)

func TestExtractAndIndex(t *testing.T) {
	// Setup database
	dbDir := t.TempDir()
	db, err := database.Open(filepath.Join(dbDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create a fake repo directory
	repoDir := t.TempDir()
	os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Test Repo\n\nThis is a test."), 0644)
	os.WriteFile(filepath.Join(repoDir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0644)

	// Insert repo
	localPath := repoDir
	repo := &database.Repo{
		Forge:     "github",
		ForgeID:   "123",
		Owner:     "test",
		Name:      "repo",
		FullName:  "test/repo",
		CloneURL:  "https://github.com/test/repo.git",
		WebURL:    "https://github.com/test/repo",
		LocalPath: &localPath,
		Status:    "active",
	}
	repoID, _ := db.InsertRepo(repo)

	desc := "A test repository"
	db.UpsertMetadata(&database.RepoMetadata{
		RepoID:      repoID,
		Description: &desc,
		Topics:      []string{"test", "example"},
	})

	// Extract and index
	extractor := NewExtractor(db, DefaultExtractorConfig())
	err = extractor.ExtractAndIndex(context.Background(), repoID, repoDir)
	if err != nil {
		t.Fatalf("ExtractAndIndex() error = %v", err)
	}

	// Verify documents saved
	doc, err := db.GetDocument(repoID, "readme", "README.md")
	if err != nil {
		t.Fatal(err)
	}
	if doc == nil {
		t.Error("README not saved")
	}
	if doc != nil && *doc.Content != "# Test Repo\n\nThis is a test." {
		t.Errorf("README content = %q", *doc.Content)
	}

	// Verify go.mod saved
	doc, _ = db.GetDocument(repoID, "manifest", "go.mod")
	if doc == nil {
		t.Error("go.mod not saved")
	}

	// Verify search works
	searcher := search.New(db.DB)
	results, err := searcher.Search("test", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("Search results = %d, want 1", len(results))
	}
}

func TestExtractorSkipsLargeFiles(t *testing.T) {
	dbDir := t.TempDir()
	db, _ := database.Open(filepath.Join(dbDir, "test.db"))
	defer db.Close()

	repoDir := t.TempDir()

	// Create a large README (over 100KB)
	largeContent := make([]byte, 150*1024)
	for i := range largeContent {
		largeContent[i] = 'x'
	}
	os.WriteFile(filepath.Join(repoDir, "README.md"), largeContent, 0644)

	localPath := repoDir
	repo := &database.Repo{
		Forge:     "github",
		ForgeID:   "123",
		Owner:     "test",
		Name:      "repo",
		FullName:  "test/repo",
		CloneURL:  "https://github.com/test/repo.git",
		WebURL:    "https://github.com/test/repo",
		LocalPath: &localPath,
		Status:    "active",
	}
	repoID, _ := db.InsertRepo(repo)

	extractor := NewExtractor(db, DefaultExtractorConfig())
	err := extractor.ExtractAndIndex(context.Background(), repoID, repoDir)
	if err != nil {
		t.Fatal(err)
	}

	// Large README should NOT be saved
	doc, _ := db.GetDocument(repoID, "readme", "README.md")
	if doc != nil {
		t.Error("Large README should not be saved")
	}
}

func TestHashContent(t *testing.T) {
	h1 := hashContent("hello world")
	h2 := hashContent("hello world")
	h3 := hashContent("different")

	if h1 != h2 {
		t.Error("Same content should produce same hash")
	}
	if h1 == h3 {
		t.Error("Different content should produce different hash")
	}
	if len(h1) != 16 { // 8 bytes = 16 hex chars
		t.Errorf("Hash length = %d, want 16", len(h1))
	}
}

func TestRebuildAll(t *testing.T) {
	dbDir := t.TempDir()
	db, _ := database.Open(filepath.Join(dbDir, "test.db"))
	defer db.Close()

	// Create two repo directories
	for i, name := range []string{"repo1", "repo2"} {
		repoDir := filepath.Join(t.TempDir(), name)
		os.MkdirAll(repoDir, 0755)
		os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# "+name), 0644)

		localPath := repoDir
		repo := &database.Repo{
			Forge:     "github",
			ForgeID:   string(rune('1' + i)),
			Owner:     "test",
			Name:      name,
			FullName:  "test/" + name,
			CloneURL:  "https://github.com/test/" + name + ".git",
			WebURL:    "https://github.com/test/" + name,
			LocalPath: &localPath,
			Status:    "active",
		}
		db.InsertRepo(repo)
	}

	extractor := NewExtractor(db, DefaultExtractorConfig())
	count, err := extractor.RebuildAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("RebuildAll count = %d, want 2", count)
	}
}
