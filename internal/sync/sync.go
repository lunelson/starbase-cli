package sync

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/lunelson/starbase-cli/internal/config"
	"github.com/lunelson/starbase-cli/internal/database"
	"github.com/lunelson/starbase-cli/internal/forge"
	"github.com/lunelson/starbase-cli/internal/git"
)

// Options configures sync behavior
type Options struct {
	Full         bool          // Clone all stars, not just recent window
	MetadataOnly bool          // Skip git operations
	PullOnly     bool          // Only update existing clones
	Since        *time.Time    // Override recency window
	DryRun       bool          // Show plan without executing
	MaxRepos     int           // Max repos to process
}

// Result contains sync statistics
type Result struct {
	Fetched   int
	Cloned    int
	Updated   int
	Skipped   int
	Errors    int
	ErrorMsgs []string
}

// Syncer handles synchronization of starred repos
type Syncer struct {
	cfg      *config.Config
	paths    config.Paths
	db       *database.DB
	forges   map[string]forge.Forge
	manifest *config.Manifest
	out      io.Writer
}

// New creates a new Syncer
func New(cfg *config.Config, paths config.Paths, db *database.DB, forges map[string]forge.Forge, manifest *config.Manifest, out io.Writer) *Syncer {
	return &Syncer{
		cfg:      cfg,
		paths:    paths,
		db:       db,
		forges:   forges,
		manifest: manifest,
		out:      out,
	}
}

// Run executes the sync operation
func (s *Syncer) Run(ctx context.Context, opts Options) (*Result, error) {
	result := &Result{}

	// Determine time window
	var since *time.Time
	if opts.Since != nil {
		since = opts.Since
	} else if !opts.Full {
		window, err := s.cfg.Sync.DefaultWindowDuration()
		if err != nil {
			return nil, fmt.Errorf("parsing window duration: %w", err)
		}
		t := time.Now().Add(-window)
		since = &t
	}

	// Fetch stars from all enabled forges
	for forgeName, f := range s.forges {
		fmt.Fprintf(s.out, "Fetching stars from %s...\n", forgeName)

		stars, err := s.fetchAllStars(ctx, f, since)
		if err != nil {
			result.Errors++
			result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("%s: %v", forgeName, err))
			continue
		}
		result.Fetched += len(stars)

		fmt.Fprintf(s.out, "Found %d starred repos\n", len(stars))

		// Process each star
		for i, star := range stars {
			if opts.MaxRepos > 0 && i >= opts.MaxRepos {
				fmt.Fprintf(s.out, "Reached max repos limit (%d)\n", opts.MaxRepos)
				break
			}

			err := s.processStar(ctx, f, star, opts, result)
			if err != nil {
				result.Errors++
				result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("%s: %v", star.FullName, err))
			}
		}
	}

	return result, nil
}

func (s *Syncer) fetchAllStars(ctx context.Context, f forge.Forge, since *time.Time) ([]forge.StarredRepo, error) {
	var allStars []forge.StarredRepo

	opts := forge.ListOptions{
		Page:    1,
		PerPage: 100,
		Since:   since,
	}

	for {
		result, err := f.ListStars(ctx, opts)
		if err != nil {
			return nil, err
		}

		allStars = append(allStars, result.Repos...)

		if result.NextPage == 0 {
			break
		}
		opts.Page = result.NextPage
	}

	return allStars, nil
}

func (s *Syncer) processStar(ctx context.Context, f forge.Forge, star forge.StarredRepo, opts Options, result *Result) error {
	forgeName := f.Name()

	// Check if repo exists in DB
	existing, err := s.db.GetRepoByForgeID(forgeName, star.ForgeID)
	if err != nil {
		return fmt.Errorf("checking repo: %w", err)
	}

	// Upsert repo to database
	now := time.Now()
	repo := &database.Repo{
		Forge:     forgeName,
		ForgeID:   star.ForgeID,
		Owner:     star.Owner,
		Name:      star.Name,
		FullName:  star.FullName,
		CloneURL:  star.CloneURL,
		WebURL:    star.WebURL,
		StarredAt: star.StarredAt,
		SyncedAt:  &now,
		Status:    "active",
	}

	if existing != nil {
		repo.LocalPath = existing.LocalPath
		repo.ClonedAt = existing.ClonedAt
	}

	repoID, err := s.db.InsertRepo(repo)
	if err != nil {
		return fmt.Errorf("saving repo: %w", err)
	}

	// Upsert metadata
	meta := &database.RepoMetadata{
		RepoID:      repoID,
		Description: strPtr(star.Description),
		Language:    strPtr(star.Language),
		Topics:      star.Topics,
		StarsCount:  intPtr(star.StarsCount),
		ForksCount:  intPtr(star.ForksCount),
		IsArchived:  star.IsArchived,
		IsPrivate:   star.IsPrivate,
		PushedAt:    star.PushedAt,
	}

	if err := s.db.UpsertMetadata(meta); err != nil {
		return fmt.Errorf("saving metadata: %w", err)
	}

	// Skip git operations if metadata-only
	if opts.MetadataOnly {
		result.Skipped++
		return nil
	}

	// Skip private repos if configured
	if star.IsPrivate && !s.cfg.Sync.ClonePrivate {
		result.Skipped++
		return nil
	}

	// Skip archived repos if configured
	if star.IsArchived && !s.cfg.Sync.CloneArchived {
		result.Skipped++
		return nil
	}

	// Determine local path
	localPath := filepath.Join(s.paths.ClonesDir, forgeName, star.Owner, star.Name)

	if opts.DryRun {
		if existing != nil && existing.LocalPath != nil {
			fmt.Fprintf(s.out, "[dry-run] Would update: %s\n", star.FullName)
		} else {
			fmt.Fprintf(s.out, "[dry-run] Would clone: %s\n", star.FullName)
		}
		return nil
	}

	// Check if already cloned
	if git.IsGitRepo(localPath) {
		if !opts.PullOnly && s.cfg.Sync.CloneMissing {
			// Update existing clone
			fmt.Fprintf(s.out, "Updating %s...\n", star.FullName)
			if err := git.Pull(ctx, localPath, s.cfg.Sync.ResetOnConflict); err != nil {
				return fmt.Errorf("pulling: %w", err)
			}
			result.Updated++
		}
	} else if !opts.PullOnly {
		// Clone new repo
		fmt.Fprintf(s.out, "Cloning %s...\n", star.FullName)

		cloneOpts := git.CloneOptions{
			Depth:          s.cfg.Clone.Depth,
			SingleBranch:   s.cfg.Clone.SingleBranch,
			SkipSubmodules: s.cfg.Clone.SkipSubmodules,
			SkipLFS:        s.cfg.Clone.SkipLFS,
		}

		if err := git.Clone(ctx, star.CloneURL, localPath, cloneOpts); err != nil {
			return fmt.Errorf("cloning: %w", err)
		}

		// Update database with local path
		if err := s.db.UpdateRepoLocalPath(repoID, localPath, time.Now()); err != nil {
			return fmt.Errorf("updating local path: %w", err)
		}

		result.Cloned++
	} else {
		result.Skipped++
	}

	return nil
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func intPtr(i int) *int {
	return &i
}
