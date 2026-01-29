package database

import (
	"path/filepath"
	"testing"
	"time"
)

func TestOpenAndMigrate(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	// Verify tables exist
	tables := []string{"repos", "repo_metadata", "repo_documents", "repo_annotations", "embeddings", "search_index"}
	for _, table := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("Table %s does not exist: %v", table, err)
		}
	}
}

func TestReposCRUD(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().Truncate(time.Second)

	// Insert
	repo := &Repo{
		Forge:     "github",
		ForgeID:   "MDEwOlJlcG9zaXRvcnkxMjM=",
		Owner:     "owner",
		Name:      "repo",
		FullName:  "owner/repo",
		CloneURL:  "https://github.com/owner/repo.git",
		WebURL:    "https://github.com/owner/repo",
		StarredAt: &now,
		Status:    "active",
	}

	id, err := db.InsertRepo(repo)
	if err != nil {
		t.Fatalf("InsertRepo() error = %v", err)
	}
	if id == 0 {
		t.Error("InsertRepo() returned id = 0")
	}

	// Get by ID
	got, err := db.GetRepo(id)
	if err != nil {
		t.Fatalf("GetRepo() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetRepo() returned nil")
	}
	if got.FullName != "owner/repo" {
		t.Errorf("FullName = %q, want owner/repo", got.FullName)
	}
	if got.Status != "active" {
		t.Errorf("Status = %q, want active", got.Status)
	}

	// Get by ForgeID
	got, err = db.GetRepoByForgeID("github", "MDEwOlJlcG9zaXRvcnkxMjM=")
	if err != nil {
		t.Fatalf("GetRepoByForgeID() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetRepoByForgeID() returned nil")
	}

	// Get by FullName
	got, err = db.GetRepoByFullName("github", "owner/repo")
	if err != nil {
		t.Fatalf("GetRepoByFullName() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetRepoByFullName() returned nil")
	}

	// Update local path
	clonedAt := time.Now()
	if err := db.UpdateRepoLocalPath(id, "/path/to/clone", clonedAt); err != nil {
		t.Fatalf("UpdateRepoLocalPath() error = %v", err)
	}

	got, _ = db.GetRepo(id)
	if got.LocalPath == nil || *got.LocalPath != "/path/to/clone" {
		t.Errorf("LocalPath = %v, want /path/to/clone", got.LocalPath)
	}

	// Update status
	if err := db.UpdateRepoStatus(id, "archived"); err != nil {
		t.Fatalf("UpdateRepoStatus() error = %v", err)
	}

	got, _ = db.GetRepo(id)
	if got.Status != "archived" {
		t.Errorf("Status = %q, want archived", got.Status)
	}

	// List
	repos, err := db.ListRepos("")
	if err != nil {
		t.Fatalf("ListRepos() error = %v", err)
	}
	if len(repos) != 1 {
		t.Errorf("ListRepos() = %d, want 1", len(repos))
	}

	// List with filter
	repos, err = db.ListRepos("active")
	if err != nil {
		t.Fatalf("ListRepos(active) error = %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("ListRepos(active) = %d, want 0", len(repos))
	}

	repos, err = db.ListRepos("archived")
	if err != nil {
		t.Fatalf("ListRepos(archived) error = %v", err)
	}
	if len(repos) != 1 {
		t.Errorf("ListRepos(archived) = %d, want 1", len(repos))
	}

	// Count
	count, err := db.CountRepos()
	if err != nil {
		t.Fatalf("CountRepos() error = %v", err)
	}
	if count != 1 {
		t.Errorf("CountRepos() = %d, want 1", count)
	}

	// Delete
	if err := db.DeleteRepo(id); err != nil {
		t.Fatalf("DeleteRepo() error = %v", err)
	}

	got, _ = db.GetRepo(id)
	if got != nil {
		t.Error("GetRepo() returned repo after delete")
	}
}

func TestMetadataCRUD(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Insert repo first
	repo := &Repo{
		Forge:    "github",
		ForgeID:  "123",
		Owner:    "owner",
		Name:     "repo",
		FullName: "owner/repo",
		CloneURL: "https://github.com/owner/repo.git",
		WebURL:   "https://github.com/owner/repo",
		Status:   "active",
	}
	repoID, _ := db.InsertRepo(repo)

	description := "A test repo"
	language := "Go"
	stars := 100

	meta := &RepoMetadata{
		RepoID:      repoID,
		Description: &description,
		Language:    &language,
		Topics:      []string{"cli", "golang"},
		StarsCount:  &stars,
		IsArchived:  false,
	}

	if err := db.UpsertMetadata(meta); err != nil {
		t.Fatalf("UpsertMetadata() error = %v", err)
	}

	got, err := db.GetMetadata(repoID)
	if err != nil {
		t.Fatalf("GetMetadata() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetMetadata() returned nil")
	}
	if *got.Description != "A test repo" {
		t.Errorf("Description = %q, want A test repo", *got.Description)
	}
	if len(got.Topics) != 2 {
		t.Errorf("Topics = %d, want 2", len(got.Topics))
	}
}

func TestDocumentsCRUD(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Insert repo first
	repo := &Repo{
		Forge:    "github",
		ForgeID:  "123",
		Owner:    "owner",
		Name:     "repo",
		FullName: "owner/repo",
		CloneURL: "https://github.com/owner/repo.git",
		WebURL:   "https://github.com/owner/repo",
		Status:   "active",
	}
	repoID, _ := db.InsertRepo(repo)

	content := "# README\n\nThis is a test."
	hash := "abc123"
	now := time.Now()

	doc := &RepoDocument{
		RepoID:      repoID,
		DocType:     "readme",
		Filename:    "README.md",
		Content:     &content,
		ContentHash: &hash,
		ExtractedAt: &now,
	}

	if err := db.UpsertDocument(doc); err != nil {
		t.Fatalf("UpsertDocument() error = %v", err)
	}

	got, err := db.GetDocument(repoID, "readme", "README.md")
	if err != nil {
		t.Fatalf("GetDocument() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetDocument() returned nil")
	}
	if *got.Content != content {
		t.Errorf("Content = %q, want %q", *got.Content, content)
	}
}

func TestAnnotationsCRUD(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Insert repo first
	repo := &Repo{
		Forge:    "github",
		ForgeID:  "123",
		Owner:    "owner",
		Name:     "repo",
		FullName: "owner/repo",
		CloneURL: "https://github.com/owner/repo.git",
		WebURL:   "https://github.com/owner/repo",
		Status:   "active",
	}
	repoID, _ := db.InsertRepo(repo)

	notes := "Great library for TUI"
	ann := &RepoAnnotation{
		RepoID:      repoID,
		Collections: []string{"llm-refs", "studying"},
		Notes:       &notes,
		IsPinned:    true,
		LocalTags:   []string{"important"},
	}

	if err := db.UpsertAnnotation(ann); err != nil {
		t.Fatalf("UpsertAnnotation() error = %v", err)
	}

	got, err := db.GetAnnotation(repoID)
	if err != nil {
		t.Fatalf("GetAnnotation() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetAnnotation() returned nil")
	}
	if !got.IsPinned {
		t.Error("IsPinned = false, want true")
	}
	if len(got.Collections) != 2 {
		t.Errorf("Collections = %d, want 2", len(got.Collections))
	}
}

func TestUpsertRepoConflict(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	repo1 := &Repo{
		Forge:    "github",
		ForgeID:  "same-id",
		Owner:    "owner",
		Name:     "repo",
		FullName: "owner/repo",
		CloneURL: "https://github.com/owner/repo.git",
		WebURL:   "https://github.com/owner/repo",
		Status:   "active",
	}

	id1, err := db.InsertRepo(repo1)
	if err != nil {
		t.Fatalf("First InsertRepo() error = %v", err)
	}

	// Upsert with same forge_id but different name (simulating rename)
	repo2 := &Repo{
		Forge:    "github",
		ForgeID:  "same-id",
		Owner:    "newowner",
		Name:     "newrepo",
		FullName: "newowner/newrepo",
		CloneURL: "https://github.com/newowner/newrepo.git",
		WebURL:   "https://github.com/newowner/newrepo",
		Status:   "active",
	}

	id2, err := db.InsertRepo(repo2)
	if err != nil {
		t.Fatalf("Second InsertRepo() error = %v", err)
	}

	// Should return same ID
	if id2 != id1 {
		t.Errorf("Second insert returned different id: %d vs %d", id2, id1)
	}

	// Verify update happened
	got, _ := db.GetRepo(id1)
	if got.FullName != "newowner/newrepo" {
		t.Errorf("FullName = %q, want newowner/newrepo", got.FullName)
	}

	// Count should still be 1
	count, _ := db.CountRepos()
	if count != 1 {
		t.Errorf("CountRepos() = %d, want 1", count)
	}
}

func TestCascadeDelete(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Insert repo with related data
	repo := &Repo{
		Forge:    "github",
		ForgeID:  "123",
		Owner:    "owner",
		Name:     "repo",
		FullName: "owner/repo",
		CloneURL: "https://github.com/owner/repo.git",
		WebURL:   "https://github.com/owner/repo",
		Status:   "active",
	}
	repoID, _ := db.InsertRepo(repo)

	desc := "desc"
	db.UpsertMetadata(&RepoMetadata{RepoID: repoID, Description: &desc})
	content := "content"
	db.UpsertDocument(&RepoDocument{RepoID: repoID, DocType: "readme", Filename: "README.md", Content: &content})
	db.UpsertAnnotation(&RepoAnnotation{RepoID: repoID, IsPinned: true})

	// Delete repo
	db.DeleteRepo(repoID)

	// Verify cascade
	meta, _ := db.GetMetadata(repoID)
	if meta != nil {
		t.Error("Metadata not deleted on cascade")
	}

	doc, _ := db.GetDocument(repoID, "readme", "README.md")
	if doc != nil {
		t.Error("Document not deleted on cascade")
	}

	ann, _ := db.GetAnnotation(repoID)
	if ann != nil {
		t.Error("Annotation not deleted on cascade")
	}
}
