package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CloneOptions configures git clone behavior
type CloneOptions struct {
	Depth          int
	SingleBranch   bool
	SkipSubmodules bool
	SkipLFS        bool
}

// DefaultCloneOptions returns sensible defaults for shallow cloning
func DefaultCloneOptions() CloneOptions {
	return CloneOptions{
		Depth:          1,
		SingleBranch:   true,
		SkipSubmodules: true,
		SkipLFS:        true,
	}
}

// Clone clones a repository to the specified path
func Clone(ctx context.Context, url, destPath string, opts CloneOptions) error {
	args := []string{"clone"}

	if opts.Depth > 0 {
		args = append(args, "--depth", fmt.Sprintf("%d", opts.Depth))
	}
	if opts.SingleBranch {
		args = append(args, "--single-branch")
	}
	if opts.SkipSubmodules {
		args = append(args, "--no-recurse-submodules")
	}

	args = append(args, url, destPath)

	cmd := exec.CommandContext(ctx, "git", args...)

	// Skip LFS by setting environment variables and config
	if opts.SkipLFS {
		cmd.Env = append(os.Environ(),
			"GIT_LFS_SKIP_SMUDGE=1",
			"GIT_LFS_SKIP_PUSH=1",
		)
		// Add config to skip LFS filter during checkout
		args = append([]string{"-c", "filter.lfs.smudge=", "-c", "filter.lfs.process=", "-c", "filter.lfs.required=false"}, args...)
		cmd.Args = append([]string{"git"}, args...)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %w: %s", err, stderr.String())
	}

	return nil
}

// Pull updates a repository
func Pull(ctx context.Context, repoPath string, resetOnConflict bool) error {
	// First try a simple pull
	cmd := exec.CommandContext(ctx, "git", "pull", "--ff-only")
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(), "GIT_LFS_SKIP_SMUDGE=1")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if resetOnConflict {
			// Fall back to fetch + reset
			if err := Fetch(ctx, repoPath); err != nil {
				return err
			}
			return Reset(ctx, repoPath)
		}
		return fmt.Errorf("git pull failed: %w: %s", err, stderr.String())
	}

	return nil
}

// Fetch fetches from remote
func Fetch(ctx context.Context, repoPath string) error {
	cmd := exec.CommandContext(ctx, "git", "fetch", "--depth=1")
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(), "GIT_LFS_SKIP_SMUDGE=1")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git fetch failed: %w: %s", err, stderr.String())
	}

	return nil
}

// Reset hard resets to origin/HEAD
func Reset(ctx context.Context, repoPath string) error {
	// Get the default remote branch
	branch, err := getDefaultBranch(ctx, repoPath)
	if err != nil {
		branch = "origin/HEAD"
	}

	cmd := exec.CommandContext(ctx, "git", "reset", "--hard", branch)
	cmd.Dir = repoPath

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git reset failed: %w: %s", err, stderr.String())
	}

	return nil
}

// IsGitRepo checks if a path is a git repository
func IsGitRepo(path string) bool {
	gitDir := filepath.Join(path, ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// GetRemoteURL returns the origin remote URL
func GetRemoteURL(ctx context.Context, repoPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "origin")
	cmd.Dir = repoPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git remote get-url failed: %w: %s", err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

// ReadFile reads a file from the repository
func ReadFile(repoPath, filePath string) (string, error) {
	fullPath := filepath.Join(repoPath, filePath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// FindFile finds a file by trying multiple paths (case-insensitive)
func FindFile(repoPath string, filenames []string) (content string, foundPath string, err error) {
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return "", "", err
	}

	for _, filename := range filenames {
		for _, entry := range entries {
			if strings.EqualFold(entry.Name(), filename) {
				content, err := ReadFile(repoPath, entry.Name())
				if err != nil {
					return "", "", err
				}
				return content, entry.Name(), nil
			}
		}
	}

	return "", "", nil
}

// getDefaultBranch returns the default branch for the remote
func getDefaultBranch(ctx context.Context, repoPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "symbolic-ref", "refs/remotes/origin/HEAD")
	cmd.Dir = repoPath

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", err
	}

	// Output: refs/remotes/origin/main
	ref := strings.TrimSpace(stdout.String())
	ref = strings.TrimPrefix(ref, "refs/remotes/")
	return ref, nil
}
