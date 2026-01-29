package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TempStarbase creates temporary config and data directories for testing
func TempStarbase(t *testing.T) (configDir, dataDir string, cleanup func()) {
	t.Helper()

	configDir, err := os.MkdirTemp("", "starbase-config-*")
	if err != nil {
		t.Fatal(err)
	}

	dataDir, err = os.MkdirTemp("", "starbase-data-*")
	if err != nil {
		os.RemoveAll(configDir)
		t.Fatal(err)
	}

	cleanup = func() {
		os.RemoveAll(configDir)
		os.RemoveAll(dataDir)
	}

	return configDir, dataDir, cleanup
}

// GitRepo creates a temporary git repository with the given files
func GitRepo(t *testing.T, files map[string]string) (path string, cleanup func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "starbase-git-*")
	if err != nil {
		t.Fatal(err)
	}

	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")

	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			os.RemoveAll(dir)
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			os.RemoveAll(dir)
			t.Fatal(err)
		}
	}

	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")

	cleanup = func() {
		os.RemoveAll(dir)
	}

	return dir, cleanup
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
