package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifestEmpty(t *testing.T) {
	dir := t.TempDir()

	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}

	if m.Version != 1 {
		t.Errorf("Version = %d, want 1", m.Version)
	}
	if len(m.Repos) != 0 {
		t.Errorf("Repos = %d, want 0", len(m.Repos))
	}
}

func TestLoadManifestFromFile(t *testing.T) {
	dir := t.TempDir()

	content := `
version: 1
collections:
  - name: llm-references
    description: Repos useful for LLM context
    color: "#4CAF50"
repos:
  - forge: github
    owner: charmbracelet
    name: bubbletea
    forge_id: "MDEwOlJlcG9zaXRvcnkyNDUyMTQ1MDA="
    collections: [llm-references]
    notes: TUI framework
    pinned: true
    tags: [golang, tui]
  - forge: github
    owner: spf13
    name: cobra
    collections: []
`
	if err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}

	if m.Version != 1 {
		t.Errorf("Version = %d, want 1", m.Version)
	}
	if len(m.Collections) != 1 {
		t.Errorf("Collections = %d, want 1", len(m.Collections))
	}
	if len(m.Repos) != 2 {
		t.Errorf("Repos = %d, want 2", len(m.Repos))
	}

	// Check first repo
	repo := m.Repos[0]
	if repo.Forge != "github" {
		t.Errorf("Repo[0].Forge = %q, want github", repo.Forge)
	}
	if repo.FullName() != "charmbracelet/bubbletea" {
		t.Errorf("Repo[0].FullName() = %q, want charmbracelet/bubbletea", repo.FullName())
	}
	if !repo.Pinned {
		t.Error("Repo[0].Pinned = false, want true")
	}
	if len(repo.Tags) != 2 {
		t.Errorf("Repo[0].Tags = %d, want 2", len(repo.Tags))
	}
}

func TestSaveManifest(t *testing.T) {
	dir := t.TempDir()

	m := &Manifest{
		Version: 1,
		Collections: []Collection{
			{Name: "test", Description: "Test collection"},
		},
		Repos: []ManifestRepo{
			{
				Forge:   "github",
				Owner:   "test",
				Name:    "repo",
				ForgeID: "12345",
				Pinned:  true,
			},
		},
	}

	if err := SaveManifest(dir, m); err != nil {
		t.Fatalf("SaveManifest() error = %v", err)
	}

	// Reload and verify
	loaded, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}

	if len(loaded.Repos) != 1 {
		t.Fatalf("Repos = %d, want 1", len(loaded.Repos))
	}
	if loaded.Repos[0].ForgeID != "12345" {
		t.Errorf("Repos[0].ForgeID = %q, want 12345", loaded.Repos[0].ForgeID)
	}
}

func TestManifestGetRepo(t *testing.T) {
	m := &Manifest{
		Repos: []ManifestRepo{
			{Forge: "github", Owner: "owner1", Name: "repo1"},
			{Forge: "github", Owner: "owner2", Name: "repo2"},
			{Forge: "gitlab", Owner: "owner1", Name: "repo1"},
		},
	}

	tests := []struct {
		forge, owner, name string
		wantFound          bool
	}{
		{"github", "owner1", "repo1", true},
		{"github", "owner2", "repo2", true},
		{"gitlab", "owner1", "repo1", true},
		{"github", "owner1", "notfound", false},
		{"bitbucket", "owner1", "repo1", false},
	}

	for _, tt := range tests {
		t.Run(tt.forge+"/"+tt.owner+"/"+tt.name, func(t *testing.T) {
			got := m.GetRepo(tt.forge, tt.owner, tt.name)
			if (got != nil) != tt.wantFound {
				t.Errorf("GetRepo() found = %v, want %v", got != nil, tt.wantFound)
			}
		})
	}
}

func TestManifestGetRepoByForgeID(t *testing.T) {
	m := &Manifest{
		Repos: []ManifestRepo{
			{Forge: "github", Owner: "owner1", Name: "repo1", ForgeID: "id1"},
			{Forge: "github", Owner: "owner2", Name: "repo2", ForgeID: "id2"},
		},
	}

	if got := m.GetRepoByForgeID("github", "id1"); got == nil {
		t.Error("GetRepoByForgeID(github, id1) = nil, want repo")
	}
	if got := m.GetRepoByForgeID("github", "notfound"); got != nil {
		t.Error("GetRepoByForgeID(github, notfound) = repo, want nil")
	}
}

func TestManifestAddRemoveRepo(t *testing.T) {
	m := &Manifest{}

	m.AddRepo(ManifestRepo{Forge: "github", Owner: "test", Name: "repo"})
	if len(m.Repos) != 1 {
		t.Errorf("Repos = %d after add, want 1", len(m.Repos))
	}

	removed := m.RemoveRepo("github", "test", "repo")
	if !removed {
		t.Error("RemoveRepo() = false, want true")
	}
	if len(m.Repos) != 0 {
		t.Errorf("Repos = %d after remove, want 0", len(m.Repos))
	}

	removed = m.RemoveRepo("github", "test", "notfound")
	if removed {
		t.Error("RemoveRepo(notfound) = true, want false")
	}
}

func TestManifestCollections(t *testing.T) {
	m := &Manifest{
		Collections: []Collection{
			{Name: "coll1", Description: "Collection 1"},
		},
		Repos: []ManifestRepo{
			{Forge: "github", Owner: "a", Name: "repo1", Collections: []string{"coll1"}},
			{Forge: "github", Owner: "b", Name: "repo2", Collections: []string{"coll1", "coll2"}},
			{Forge: "github", Owner: "c", Name: "repo3", Collections: []string{}},
		},
	}

	if got := m.GetCollection("coll1"); got == nil {
		t.Error("GetCollection(coll1) = nil, want collection")
	}
	if got := m.GetCollection("notfound"); got != nil {
		t.Error("GetCollection(notfound) = collection, want nil")
	}

	repos := m.ReposInCollection("coll1")
	if len(repos) != 2 {
		t.Errorf("ReposInCollection(coll1) = %d, want 2", len(repos))
	}

	repos = m.ReposInCollection("coll2")
	if len(repos) != 1 {
		t.Errorf("ReposInCollection(coll2) = %d, want 1", len(repos))
	}
}
