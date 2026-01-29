package github

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// TokenSource represents a method of obtaining the token
type TokenSource string

const (
	TokenSourceEnv    TokenSource = "env"
	TokenSourceGHCLI  TokenSource = "gh-cli"
	TokenSourceConfig TokenSource = "config"
)

// TokenResult contains the resolved token and its source
type TokenResult struct {
	Token  string
	Source TokenSource
}

// ResolveToken attempts to find a GitHub token from various sources
// Priority: 1. Environment variable, 2. gh CLI, 3. config file
func ResolveToken() (*TokenResult, error) {
	// Try environment variable first
	if token := os.Getenv("STARBASE_GITHUB_TOKEN"); token != "" {
		return &TokenResult{Token: token, Source: TokenSourceEnv}, nil
	}

	// Also check GITHUB_TOKEN as fallback
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return &TokenResult{Token: token, Source: TokenSourceEnv}, nil
	}

	// Try gh CLI
	if token, err := getTokenFromGHCLI(); err == nil && token != "" {
		return &TokenResult{Token: token, Source: TokenSourceGHCLI}, nil
	}

	return nil, fmt.Errorf("no GitHub token found: set STARBASE_GITHUB_TOKEN or run 'gh auth login'")
}

// getTokenFromGHCLI retrieves the token from the gh CLI
func getTokenFromGHCLI() (string, error) {
	cmd := exec.Command("gh", "auth", "token")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gh auth token failed: %w", err)
	}

	token := strings.TrimSpace(stdout.String())
	if token == "" {
		return "", fmt.Errorf("gh returned empty token")
	}

	return token, nil
}

// IsGHCLIAvailable checks if the gh CLI is installed and authenticated
func IsGHCLIAvailable() bool {
	cmd := exec.Command("gh", "auth", "status")
	return cmd.Run() == nil
}
