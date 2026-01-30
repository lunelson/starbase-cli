package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/lunelson/starbase-cli/internal/forge"
)

const (
	defaultBaseURL = "https://api.github.com"
	userAgent      = "starbase-cli/1.0"
)

// Client implements the forge.Forge interface for GitHub
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// ClientOption configures the GitHub client
type ClientOption func(*Client)

// WithBaseURL sets a custom base URL (for testing)
func WithBaseURL(url string) ClientOption {
	return func(c *Client) {
		c.baseURL = url
	}
}

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(client *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = client
	}
}

// NewClient creates a new GitHub API client
func NewClient(token string, opts ...ClientOption) *Client {
	c := &Client{
		baseURL: defaultBaseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Name returns the forge identifier
func (c *Client) Name() string {
	return "github"
}

// ListStars returns starred repositories with pagination
func (c *Client) ListStars(ctx context.Context, opts forge.ListOptions) (*forge.ListResult, error) {
	page := opts.Page
	if page == 0 {
		page = 1
	}
	perPage := opts.PerPage
	if perPage == 0 {
		perPage = 30
	}

	url := fmt.Sprintf("%s/user/starred?page=%d&per_page=%d", c.baseURL, page, perPage)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	c.setHeaders(req)
	// Request starred_at timestamp
	req.Header.Set("Accept", "application/vnd.github.star+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	var starred []starredRepoResponse
	if err := json.NewDecoder(resp.Body).Decode(&starred); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	result := &forge.ListResult{
		Repos:      make([]forge.StarredRepo, 0, len(starred)),
		TotalCount: -1,
	}

	for _, s := range starred {
		repo := convertRepo(s.Repo)
		if s.StarredAt != "" {
			if t, err := time.Parse(time.RFC3339, s.StarredAt); err == nil {
				repo.StarredAt = &t
			}
		}

		// Filter by since if provided
		if opts.Since != nil && repo.StarredAt != nil {
			if repo.StarredAt.Before(*opts.Since) {
				continue
			}
		}

		result.Repos = append(result.Repos, repo)
	}

	// Parse Link header for pagination
	result.NextPage = parseNextPage(resp.Header.Get("Link"))

	return result, nil
}

// GetRepo returns detailed information about a repository
func (c *Client) GetRepo(ctx context.Context, owner, name string) (*forge.Repository, error) {
	url := fmt.Sprintf("%s/repos/%s/%s", c.baseURL, owner, name)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	var repo repoResponse
	if err := json.NewDecoder(resp.Body).Decode(&repo); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	starredRepo := convertRepo(repo)
	return &forge.Repository{
		StarredRepo:   starredRepo,
		DefaultBranch: repo.DefaultBranch,
		SizeKB:        repo.Size,
	}, nil
}

// GetReadme returns the README content for a repository
func (c *Client) GetReadme(ctx context.Context, owner, name string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/readme", c.baseURL, owner, name)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	var readme readmeResponse
	if err := json.NewDecoder(resp.Body).Decode(&readme); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	// Decode base64 content
	if readme.Encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(readme.Content, "\n", ""))
		if err != nil {
			return "", fmt.Errorf("decoding base64: %w", err)
		}
		return string(decoded), nil
	}

	return readme.Content, nil
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
}

// API response types

type starredRepoResponse struct {
	StarredAt string       `json:"starred_at"`
	Repo      repoResponse `json:"repo"`
}

type repoResponse struct {
	ID              int64    `json:"id"`
	NodeID          string   `json:"node_id"`
	Name            string   `json:"name"`
	FullName        string   `json:"full_name"`
	Owner           owner    `json:"owner"`
	Description     string   `json:"description"`
	Language        string   `json:"language"`
	Topics          []string `json:"topics"`
	CloneURL        string   `json:"clone_url"`
	HTMLURL         string   `json:"html_url"`
	StargazersCount int      `json:"stargazers_count"`
	ForksCount      int      `json:"forks_count"`
	Archived        bool     `json:"archived"`
	Private         bool     `json:"private"`
	DefaultBranch   string   `json:"default_branch"`
	Size            int      `json:"size"`
	PushedAt        string   `json:"pushed_at"`
}

type owner struct {
	Login string `json:"login"`
}

type readmeResponse struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

func convertRepo(r repoResponse) forge.StarredRepo {
	repo := forge.StarredRepo{
		ForgeID:     r.NodeID,
		Owner:       r.Owner.Login,
		Name:        r.Name,
		FullName:    r.FullName,
		CloneURL:    r.CloneURL,
		WebURL:      r.HTMLURL,
		Description: r.Description,
		Language:    r.Language,
		Topics:      r.Topics,
		StarsCount:  r.StargazersCount,
		ForksCount:  r.ForksCount,
		IsArchived:  r.Archived,
		IsPrivate:   r.Private,
	}

	if r.PushedAt != "" {
		if t, err := time.Parse(time.RFC3339, r.PushedAt); err == nil {
			repo.PushedAt = &t
		}
	}

	return repo
}

// parseNextPage extracts the next page number from the Link header
func parseNextPage(link string) int {
	if link == "" {
		return 0
	}

	// Parse Link header: <url>; rel="next", <url>; rel="last"
	re := regexp.MustCompile(`<[^>]*[?&]page=(\d+)[^>]*>;\s*rel="next"`)
	matches := re.FindStringSubmatch(link)
	if len(matches) < 2 {
		return 0
	}

	page, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0
	}

	return page
}
