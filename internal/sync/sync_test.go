package sync

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lunelson/starbase-cli/internal/config"
	"github.com/lunelson/starbase-cli/internal/database"
	"github.com/lunelson/starbase-cli/internal/forge"
)

// mockForge implements forge.Forge for testing
type mockForge struct {
	name  string
	stars []forge.StarredRepo
}

func (m *mockForge) Name() string { return m.name }

func (m *mockForge) ListStars(ctx context.Context, opts forge.ListOptions) (*forge.ListResult, error) {
	return &forge.ListResult{
		Repos:      m.stars,
		NextPage:   0,
		TotalCount: len(m.stars),
	}, nil
}

func (m *mockForge) GetRepo(ctx context.Context, owner, name string) (*forge.Repository, error) {
	return nil, nil
}

func (m *mockForge) GetReadme(ctx context.Context, owner, name string) (string, error) {
	return "", nil
}

func TestSyncMetadataOnly(t *testing.T) {
	// Setup
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cfg := &config.Config{
		Sync: config.SyncConfig{
			DefaultWindow: "30d",
		},
	}
	paths := config.Paths{
		DataDir:   dir,
		ClonesDir: filepath.Join(dir, "clones"),
	}

	now := time.Now()
	mock := &mockForge{
		name: "github",
		stars: []forge.StarredRepo{
			{
				ForgeID:     "id1",
				Owner:       "owner",
				Name:        "repo1",
				FullName:    "owner/repo1",
				CloneURL:    "https://github.com/owner/repo1.git",
				WebURL:      "https://github.com/owner/repo1",
				Description: "Test repo",
				Language:    "Go",
				StarredAt:   &now,
			},
		},
	}

	forges := map[string]forge.Forge{"github": mock}
	manifest := &config.Manifest{}

	var buf bytes.Buffer
	syncer := New(cfg, paths, db, forges, manifest, &buf)

	// Run sync
	result, err := syncer.Run(context.Background(), Options{
		MetadataOnly: true,
		Full:         true,
	})

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Fetched != 1 {
		t.Errorf("Fetched = %d, want 1", result.Fetched)
	}
	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 (metadata-only skips cloning)", result.Skipped)
	}

	// Verify repo saved to DB
	repos, err := db.ListRepos("")
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Fatalf("Repos in DB = %d, want 1", len(repos))
	}
	if repos[0].FullName != "owner/repo1" {
		t.Errorf("FullName = %q, want owner/repo1", repos[0].FullName)
	}

	// Verify metadata saved
	meta, err := db.GetMetadata(repos[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if meta == nil {
		t.Fatal("Metadata not saved")
	}
	if *meta.Description != "Test repo" {
		t.Errorf("Description = %q, want Test repo", *meta.Description)
	}
}

func TestSyncDryRun(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cfg := &config.Config{
		Sync: config.SyncConfig{
			DefaultWindow: "30d",
			CloneMissing:  true,
		},
	}
	paths := config.Paths{
		DataDir:   dir,
		ClonesDir: filepath.Join(dir, "clones"),
	}

	now := time.Now()
	mock := &mockForge{
		name: "github",
		stars: []forge.StarredRepo{
			{
				ForgeID:   "id1",
				Owner:     "owner",
				Name:      "repo1",
				FullName:  "owner/repo1",
				CloneURL:  "https://github.com/owner/repo1.git",
				StarredAt: &now,
			},
		},
	}

	forges := map[string]forge.Forge{"github": mock}

	var buf bytes.Buffer
	syncer := New(cfg, paths, db, forges, nil, &buf)

	result, err := syncer.Run(context.Background(), Options{
		DryRun: true,
		Full:   true,
	})

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Dry run shouldn't clone anything
	if result.Cloned != 0 {
		t.Errorf("Cloned = %d, want 0 for dry run", result.Cloned)
	}

	// Output should mention dry-run
	if !bytes.Contains(buf.Bytes(), []byte("[dry-run]")) {
		t.Error("Output should mention dry-run")
	}
}

func TestSyncSkipsPrivate(t *testing.T) {
	dir := t.TempDir()
	db, _ := database.Open(filepath.Join(dir, "test.db"))
	defer db.Close()

	cfg := &config.Config{
		Sync: config.SyncConfig{
			DefaultWindow: "30d",
			ClonePrivate:  false, // Skip private repos
		},
	}
	paths := config.Paths{
		ClonesDir: filepath.Join(dir, "clones"),
	}

	now := time.Now()
	mock := &mockForge{
		name: "github",
		stars: []forge.StarredRepo{
			{
				ForgeID:   "id1",
				Owner:     "owner",
				Name:      "private-repo",
				FullName:  "owner/private-repo",
				IsPrivate: true,
				StarredAt: &now,
			},
		},
	}

	forges := map[string]forge.Forge{"github": mock}

	var buf bytes.Buffer
	syncer := New(cfg, paths, db, forges, nil, &buf)

	result, err := syncer.Run(context.Background(), Options{Full: true})
	if err != nil {
		t.Fatal(err)
	}

	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 (private repo)", result.Skipped)
	}
	if result.Cloned != 0 {
		t.Errorf("Cloned = %d, want 0", result.Cloned)
	}
}

func TestSyncSkipsArchived(t *testing.T) {
	dir := t.TempDir()
	db, _ := database.Open(filepath.Join(dir, "test.db"))
	defer db.Close()

	cfg := &config.Config{
		Sync: config.SyncConfig{
			DefaultWindow: "30d",
			CloneArchived: false, // Skip archived repos
		},
	}
	paths := config.Paths{
		ClonesDir: filepath.Join(dir, "clones"),
	}

	now := time.Now()
	mock := &mockForge{
		name: "github",
		stars: []forge.StarredRepo{
			{
				ForgeID:    "id1",
				Owner:      "owner",
				Name:       "archived-repo",
				FullName:   "owner/archived-repo",
				IsArchived: true,
				StarredAt:  &now,
			},
		},
	}

	forges := map[string]forge.Forge{"github": mock}

	var buf bytes.Buffer
	syncer := New(cfg, paths, db, forges, nil, &buf)

	result, err := syncer.Run(context.Background(), Options{Full: true})
	if err != nil {
		t.Fatal(err)
	}

	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 (archived repo)", result.Skipped)
	}
}

func TestSyncMaxRepos(t *testing.T) {
	dir := t.TempDir()
	db, _ := database.Open(filepath.Join(dir, "test.db"))
	defer db.Close()

	cfg := &config.Config{
		Sync: config.SyncConfig{
			DefaultWindow: "30d",
		},
	}
	paths := config.Paths{
		ClonesDir: filepath.Join(dir, "clones"),
	}

	now := time.Now()
	mock := &mockForge{
		name: "github",
		stars: []forge.StarredRepo{
			{ForgeID: "1", Owner: "o", Name: "r1", FullName: "o/r1", StarredAt: &now},
			{ForgeID: "2", Owner: "o", Name: "r2", FullName: "o/r2", StarredAt: &now},
			{ForgeID: "3", Owner: "o", Name: "r3", FullName: "o/r3", StarredAt: &now},
		},
	}

	forges := map[string]forge.Forge{"github": mock}

	var buf bytes.Buffer
	syncer := New(cfg, paths, db, forges, nil, &buf)

	result, err := syncer.Run(context.Background(), Options{
		Full:         true,
		MetadataOnly: true,
		MaxRepos:     2,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should only process 2 repos due to MaxRepos limit
	if result.Fetched != 3 {
		t.Errorf("Fetched = %d, want 3", result.Fetched)
	}

	repos, _ := db.ListRepos("")
	if len(repos) != 2 {
		t.Errorf("Repos processed = %d, want 2", len(repos))
	}
}
