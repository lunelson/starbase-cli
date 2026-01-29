package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lunelson/starbase-cli/internal/forge"
)

func TestClientImplementsForge(t *testing.T) {
	var _ forge.Forge = (*Client)(nil)
}

func TestListStars(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/starred" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}

		// Check headers
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
			t.Errorf("Authorization = %q, want Bearer test-token", auth)
		}

		// Return mock response
		w.Header().Set("Link", `<https://api.github.com/user/starred?page=2>; rel="next"`)
		json.NewEncoder(w).Encode([]starredRepoResponse{
			{
				StarredAt: "2024-01-15T10:00:00Z",
				Repo: repoResponse{
					NodeID:   "MDEwOlJlcG9zaXRvcnkxMjM=",
					Name:     "testrepo",
					FullName: "owner/testrepo",
					Owner:    owner{Login: "owner"},
					CloneURL: "https://github.com/owner/testrepo.git",
					HTMLURL:  "https://github.com/owner/testrepo",
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient("test-token", WithBaseURL(server.URL))
	result, err := client.ListStars(context.Background(), forge.ListOptions{})

	if err != nil {
		t.Fatalf("ListStars() error = %v", err)
	}

	if len(result.Repos) != 1 {
		t.Fatalf("Repos = %d, want 1", len(result.Repos))
	}

	repo := result.Repos[0]
	if repo.FullName != "owner/testrepo" {
		t.Errorf("FullName = %q, want owner/testrepo", repo.FullName)
	}
	if repo.ForgeID != "MDEwOlJlcG9zaXRvcnkxMjM=" {
		t.Errorf("ForgeID = %q, want MDEwOlJlcG9zaXRvcnkxMjM=", repo.ForgeID)
	}
	if result.NextPage != 2 {
		t.Errorf("NextPage = %d, want 2", result.NextPage)
	}
}

func TestListStarsWithSince(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]starredRepoResponse{
			{
				StarredAt: "2024-01-15T10:00:00Z",
				Repo: repoResponse{
					NodeID:   "1",
					FullName: "owner/new-repo",
					Owner:    owner{Login: "owner"},
				},
			},
			{
				StarredAt: "2024-01-01T10:00:00Z",
				Repo: repoResponse{
					NodeID:   "2",
					FullName: "owner/old-repo",
					Owner:    owner{Login: "owner"},
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient("test-token", WithBaseURL(server.URL))
	since := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	result, err := client.ListStars(context.Background(), forge.ListOptions{Since: &since})

	if err != nil {
		t.Fatalf("ListStars() error = %v", err)
	}

	// Should only include repo starred after since
	if len(result.Repos) != 1 {
		t.Fatalf("Repos = %d, want 1", len(result.Repos))
	}
	if result.Repos[0].FullName != "owner/new-repo" {
		t.Errorf("Repo = %q, want owner/new-repo", result.Repos[0].FullName)
	}
}

func TestListStarsPagination(t *testing.T) {
	page := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		if page == 1 {
			w.Header().Set("Link", `<https://api.github.com/user/starred?page=2>; rel="next"`)
		}
		json.NewEncoder(w).Encode([]starredRepoResponse{
			{
				StarredAt: "2024-01-15T10:00:00Z",
				Repo: repoResponse{
					NodeID:   "id-" + r.URL.Query().Get("page"),
					FullName: "owner/repo-" + r.URL.Query().Get("page"),
					Owner:    owner{Login: "owner"},
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient("test-token", WithBaseURL(server.URL))

	// Page 1
	result, err := client.ListStars(context.Background(), forge.ListOptions{Page: 1})
	if err != nil {
		t.Fatalf("Page 1 error = %v", err)
	}
	if result.NextPage != 2 {
		t.Errorf("Page 1 NextPage = %d, want 2", result.NextPage)
	}

	// Page 2 (last page)
	result, err = client.ListStars(context.Background(), forge.ListOptions{Page: 2})
	if err != nil {
		t.Fatalf("Page 2 error = %v", err)
	}
	if result.NextPage != 0 {
		t.Errorf("Page 2 NextPage = %d, want 0", result.NextPage)
	}
}

func TestGetRepo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}

		json.NewEncoder(w).Encode(repoResponse{
			NodeID:        "MDEwOlJlcG9zaXRvcnkxMjM=",
			Name:          "repo",
			FullName:      "owner/repo",
			Owner:         owner{Login: "owner"},
			Description:   "A test repo",
			Language:      "Go",
			Topics:        []string{"cli", "golang"},
			DefaultBranch: "main",
			Size:          1024,
		})
	}))
	defer server.Close()

	client := NewClient("test-token", WithBaseURL(server.URL))
	repo, err := client.GetRepo(context.Background(), "owner", "repo")

	if err != nil {
		t.Fatalf("GetRepo() error = %v", err)
	}
	if repo == nil {
		t.Fatal("GetRepo() returned nil")
	}
	if repo.Description != "A test repo" {
		t.Errorf("Description = %q, want A test repo", repo.Description)
	}
	if repo.DefaultBranch != "main" {
		t.Errorf("DefaultBranch = %q, want main", repo.DefaultBranch)
	}
	if len(repo.Topics) != 2 {
		t.Errorf("Topics = %d, want 2", len(repo.Topics))
	}
}

func TestGetRepoNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient("test-token", WithBaseURL(server.URL))
	repo, err := client.GetRepo(context.Background(), "owner", "nonexistent")

	if err != nil {
		t.Fatalf("GetRepo() error = %v", err)
	}
	if repo != nil {
		t.Error("GetRepo() should return nil for not found")
	}
}

func TestGetReadme(t *testing.T) {
	content := "# Test README\n\nThis is a test."
	encoded := base64.StdEncoding.EncodeToString([]byte(content))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/readme" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}

		json.NewEncoder(w).Encode(readmeResponse{
			Content:  encoded,
			Encoding: "base64",
		})
	}))
	defer server.Close()

	client := NewClient("test-token", WithBaseURL(server.URL))
	readme, err := client.GetReadme(context.Background(), "owner", "repo")

	if err != nil {
		t.Fatalf("GetReadme() error = %v", err)
	}
	if readme != content {
		t.Errorf("README = %q, want %q", readme, content)
	}
}

func TestGetReadmeNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient("test-token", WithBaseURL(server.URL))
	readme, err := client.GetReadme(context.Background(), "owner", "repo")

	if err != nil {
		t.Fatalf("GetReadme() error = %v", err)
	}
	if readme != "" {
		t.Errorf("README = %q, want empty", readme)
	}
}

func TestParseNextPage(t *testing.T) {
	tests := []struct {
		link     string
		expected int
	}{
		{`<https://api.github.com/user/starred?page=2>; rel="next"`, 2},
		{`<https://api.github.com/user/starred?page=5>; rel="next", <https://api.github.com/user/starred?page=10>; rel="last"`, 5},
		{`<https://api.github.com/user/starred?page=1>; rel="prev"`, 0},
		{``, 0},
	}

	for _, tt := range tests {
		t.Run(tt.link, func(t *testing.T) {
			got := parseNextPage(tt.link)
			if got != tt.expected {
				t.Errorf("parseNextPage(%q) = %d, want %d", tt.link, got, tt.expected)
			}
		})
	}
}

func TestAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message": "Bad credentials"}`))
	}))
	defer server.Close()

	client := NewClient("bad-token", WithBaseURL(server.URL))
	_, err := client.ListStars(context.Background(), forge.ListOptions{})

	if err == nil {
		t.Error("Expected error for unauthorized request")
	}
}
