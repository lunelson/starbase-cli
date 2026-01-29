package github

import (
	"os"
	"testing"
)

func TestResolveTokenFromEnv(t *testing.T) {
	// Set env variable
	os.Setenv("STARBASE_GITHUB_TOKEN", "test-token-123")
	defer os.Unsetenv("STARBASE_GITHUB_TOKEN")

	result, err := ResolveToken()
	if err != nil {
		t.Fatalf("ResolveToken() error = %v", err)
	}

	if result.Token != "test-token-123" {
		t.Errorf("Token = %q, want test-token-123", result.Token)
	}
	if result.Source != TokenSourceEnv {
		t.Errorf("Source = %v, want %v", result.Source, TokenSourceEnv)
	}
}

func TestResolveTokenFromGitHubToken(t *testing.T) {
	// Ensure STARBASE_GITHUB_TOKEN is not set
	os.Unsetenv("STARBASE_GITHUB_TOKEN")
	os.Setenv("GITHUB_TOKEN", "fallback-token")
	defer os.Unsetenv("GITHUB_TOKEN")

	result, err := ResolveToken()
	if err != nil {
		t.Fatalf("ResolveToken() error = %v", err)
	}

	if result.Token != "fallback-token" {
		t.Errorf("Token = %q, want fallback-token", result.Token)
	}
	if result.Source != TokenSourceEnv {
		t.Errorf("Source = %v, want %v", result.Source, TokenSourceEnv)
	}
}

func TestResolveTokenEnvPriority(t *testing.T) {
	// STARBASE_GITHUB_TOKEN should take priority over GITHUB_TOKEN
	os.Setenv("STARBASE_GITHUB_TOKEN", "starbase-token")
	os.Setenv("GITHUB_TOKEN", "github-token")
	defer os.Unsetenv("STARBASE_GITHUB_TOKEN")
	defer os.Unsetenv("GITHUB_TOKEN")

	result, err := ResolveToken()
	if err != nil {
		t.Fatalf("ResolveToken() error = %v", err)
	}

	if result.Token != "starbase-token" {
		t.Errorf("Token = %q, want starbase-token", result.Token)
	}
}

func TestResolveTokenFromGHCLI(t *testing.T) {
	// Skip if gh is not available or not authenticated
	if !IsGHCLIAvailable() {
		t.Skip("gh CLI not available or not authenticated")
	}

	// Ensure env variables are not set
	os.Unsetenv("STARBASE_GITHUB_TOKEN")
	os.Unsetenv("GITHUB_TOKEN")

	result, err := ResolveToken()
	if err != nil {
		t.Fatalf("ResolveToken() error = %v", err)
	}

	if result.Token == "" {
		t.Error("Token is empty")
	}
	if result.Source != TokenSourceGHCLI {
		t.Errorf("Source = %v, want %v", result.Source, TokenSourceGHCLI)
	}
}

func TestResolveTokenNoTokenAvailable(t *testing.T) {
	// Save and clear all token sources
	origStarbase := os.Getenv("STARBASE_GITHUB_TOKEN")
	origGithub := os.Getenv("GITHUB_TOKEN")
	os.Unsetenv("STARBASE_GITHUB_TOKEN")
	os.Unsetenv("GITHUB_TOKEN")
	defer func() {
		if origStarbase != "" {
			os.Setenv("STARBASE_GITHUB_TOKEN", origStarbase)
		}
		if origGithub != "" {
			os.Setenv("GITHUB_TOKEN", origGithub)
		}
	}()

	// If gh CLI is available and authenticated, this test will pass unexpectedly
	if IsGHCLIAvailable() {
		t.Skip("gh CLI is authenticated, cannot test no-token scenario")
	}

	_, err := ResolveToken()
	if err == nil {
		t.Error("ResolveToken() should fail when no token available")
	}
}
