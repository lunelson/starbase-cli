package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestClone(t *testing.T) {
	// Create a source repo to clone from
	sourceDir := t.TempDir()
	setupGitRepo(t, sourceDir, map[string]string{
		"README.md": "# Test\n",
		"main.go":   "package main\n",
	})

	// Clone it
	destDir := filepath.Join(t.TempDir(), "clone")
	err := Clone(context.Background(), sourceDir, destDir, DefaultCloneOptions())
	if err != nil {
		t.Fatalf("Clone() error = %v", err)
	}

	// Verify clone
	if !IsGitRepo(destDir) {
		t.Error("Clone did not create a git repo")
	}

	// Verify files exist
	if _, err := os.Stat(filepath.Join(destDir, "README.md")); os.IsNotExist(err) {
		t.Error("README.md not found in clone")
	}
}

func TestCloneOptions(t *testing.T) {
	sourceDir := t.TempDir()
	setupGitRepo(t, sourceDir, map[string]string{"test.txt": "hello"})

	tests := []struct {
		name string
		opts CloneOptions
	}{
		{"default", DefaultCloneOptions()},
		{"no depth", CloneOptions{Depth: 0, SingleBranch: true}},
		{"full clone", CloneOptions{Depth: 0, SingleBranch: false}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			destDir := filepath.Join(t.TempDir(), "clone")
			if err := Clone(context.Background(), sourceDir, destDir, tt.opts); err != nil {
				t.Errorf("Clone() error = %v", err)
			}
		})
	}
}

func TestPull(t *testing.T) {
	// Create source and clone
	sourceDir := t.TempDir()
	setupGitRepo(t, sourceDir, map[string]string{"test.txt": "initial"})

	destDir := filepath.Join(t.TempDir(), "clone")
	if err := Clone(context.Background(), sourceDir, destDir, DefaultCloneOptions()); err != nil {
		t.Fatal(err)
	}

	// Add a commit to source
	addCommit(t, sourceDir, "test.txt", "updated")

	// Pull
	if err := Pull(context.Background(), destDir, true); err != nil {
		t.Fatalf("Pull() error = %v", err)
	}

	// Verify content updated
	content, err := ReadFile(destDir, "test.txt")
	if err != nil {
		t.Fatal(err)
	}
	if content != "updated" {
		t.Errorf("Content = %q, want 'updated'", content)
	}
}

func TestIsGitRepo(t *testing.T) {
	// A git repo
	gitDir := t.TempDir()
	setupGitRepo(t, gitDir, map[string]string{"test.txt": "hello"})

	if !IsGitRepo(gitDir) {
		t.Error("IsGitRepo() = false for git repo")
	}

	// Not a git repo
	notGitDir := t.TempDir()
	if IsGitRepo(notGitDir) {
		t.Error("IsGitRepo() = true for non-git directory")
	}
}

func TestGetRemoteURL(t *testing.T) {
	sourceDir := t.TempDir()
	setupGitRepo(t, sourceDir, map[string]string{"test.txt": "hello"})

	destDir := filepath.Join(t.TempDir(), "clone")
	Clone(context.Background(), sourceDir, destDir, DefaultCloneOptions())

	url, err := GetRemoteURL(context.Background(), destDir)
	if err != nil {
		t.Fatalf("GetRemoteURL() error = %v", err)
	}

	if url != sourceDir {
		t.Errorf("URL = %q, want %q", url, sourceDir)
	}
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	content := "test content"
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte(content), 0644)

	got, err := ReadFile(dir, "test.txt")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got != content {
		t.Errorf("Content = %q, want %q", got, content)
	}

	// Non-existent file
	got, err = ReadFile(dir, "notfound.txt")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got != "" {
		t.Errorf("Content = %q, want empty", got)
	}
}

func TestFindFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Hello"), 0644)

	// Find with exact match
	content, path, err := FindFile(dir, []string{"README.md"})
	if err != nil {
		t.Fatalf("FindFile() error = %v", err)
	}
	if content != "# Hello" {
		t.Errorf("Content = %q, want '# Hello'", content)
	}
	if path != "README.md" {
		t.Errorf("Path = %q, want README.md", path)
	}

	// Find case-insensitive
	content, path, err = FindFile(dir, []string{"readme.md"})
	if err != nil {
		t.Fatal(err)
	}
	if content == "" {
		t.Error("Case-insensitive search failed")
	}

	// Not found
	content, path, err = FindFile(dir, []string{"notfound.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if content != "" || path != "" {
		t.Errorf("Expected empty for not found, got content=%q path=%q", content, path)
	}
}

// Helper functions

func setupGitRepo(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")

	for name, content := range files {
		path := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte(content), 0644)
	}

	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")
}

func addCommit(t *testing.T, dir, file, content string) {
	t.Helper()
	os.WriteFile(filepath.Join(dir, file), []byte(content), 0644)
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "update")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
