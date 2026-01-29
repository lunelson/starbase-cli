package forge

import (
	"context"
	"time"
)

// Forge represents a code hosting platform (GitHub, GitLab, etc.)
type Forge interface {
	// Name returns the forge identifier (e.g., "github", "gitlab")
	Name() string

	// ListStars returns starred repositories with pagination
	ListStars(ctx context.Context, opts ListOptions) (*ListResult, error)

	// GetRepo returns detailed information about a repository
	GetRepo(ctx context.Context, owner, name string) (*Repository, error)

	// GetReadme returns the README content for a repository
	GetReadme(ctx context.Context, owner, name string) (string, error)
}

// ListOptions configures pagination for listing stars
type ListOptions struct {
	Page    int
	PerPage int
	Since   *time.Time // Only return stars after this time
}

// ListResult contains paginated results
type ListResult struct {
	Repos      []StarredRepo
	NextPage   int  // 0 if no more pages
	TotalCount int  // -1 if unknown
}

// StarredRepo represents a starred repository from a forge
type StarredRepo struct {
	ForgeID     string
	Owner       string
	Name        string
	FullName    string
	CloneURL    string
	WebURL      string
	Description string
	Language    string
	Topics      []string
	StarsCount  int
	ForksCount  int
	IsArchived  bool
	IsPrivate   bool
	PushedAt    *time.Time
	StarredAt   *time.Time
}

// Repository contains detailed repository information
type Repository struct {
	StarredRepo
	DefaultBranch   string
	SizeKB          int
	LatestRelease   string
	LatestReleaseAt *time.Time
}
