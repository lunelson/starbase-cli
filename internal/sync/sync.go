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
	"github.com/lunelson/starbase-cli/internal/index"
)

// Options configures sync behavior
type Options struct {
	Full         bool       // Clone all stars, not just recent window
	MetadataOnly bool       // Skip git operations
	PullOnly     bool       // Only update existing clones
	Since        *time.Time // Override recency window
	DryRun       bool       // Show plan without executing
	MaxRepos     int        // Max repos to process
}

// Result contains sync statistics
type Result struct {
	Fetched     int
	Cloned      int
	Updated     int
	Skipped     int
	Errors      int
	ErrorMsgs   []string
	SkipReasons map[string][]string // reason -> list of repo names
}

// Syncer handles synchronization of starred repos
type Syncer struct {
	cfg       *config.Config
	paths     config.Paths
	db        *database.DB
	forges    map[string]forge.Forge
	manifest  *config.Manifest
	extractor *index.Extractor
	out       io.Writer
	progress  *Progress
}

// New creates a new Syncer
func New(cfg *config.Config, paths config.Paths, db *database.DB, forges map[string]forge.Forge, manifest *config.Manifest, out io.Writer) *Syncer {
	extractorCfg := index.ExtractorConfig{
		IndexReadme:     cfg.Index.Readme,
		HighSignalFiles: cfg.Index.HighSignalFiles,
		MaxFileSizeKB:   cfg.Index.MaxFileSizeKB,
	}
	if len(extractorCfg.HighSignalFiles) == 0 {
		extractorCfg = index.DefaultExtractorConfig()
	}

	return &Syncer{
		cfg:       cfg,
		paths:     paths,
		db:        db,
		forges:    forges,
		manifest:  manifest,
		extractor: index.NewExtractor(db, extractorCfg),
		out:       out,
		progress:  NewProgress(out),
	}
}

// Run executes the sync operation
func (s *Syncer) Run(ctx context.Context, opts Options) (*Result, error) {
	result := &Result{
		SkipReasons: make(map[string][]string),
	}

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

	// Collect all stars and prepare git jobs
	var allJobs []git.Job
	jobToStar := make(map[string]starInfo) // job ID -> star info

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

		// Process each star: save to DB and prepare git jobs
		for i, star := range stars {
			if opts.MaxRepos > 0 && i >= opts.MaxRepos {
				fmt.Fprintf(s.out, "Reached max repos limit (%d)\n", opts.MaxRepos)
				break
			}

			job, info, err := s.prepareStar(ctx, f, star, opts, result)
			if err != nil {
				result.Errors++
				result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("%s: %v", star.FullName, err))
				continue
			}

			if job != nil {
				allJobs = append(allJobs, *job)
				jobToStar[job.ID] = info
			}
		}
	}

	// Execute git operations in parallel (unless dry-run or metadata-only)
	if !opts.DryRun && !opts.MetadataOnly && len(allJobs) > 0 {
		concurrency := s.cfg.Sync.Concurrency
		if concurrency <= 0 {
			concurrency = 4
		}

		fmt.Fprintf(s.out, "\nExecuting %d git operations (%d workers)...\n", len(allJobs), concurrency)

		s.progress.StartGitOps(len(allJobs))
		resultsCh := git.RunStream(ctx, allJobs, concurrency)
		s.processGitResultsStream(ctx, resultsCh, jobToStar, result)
		s.progress.StopGitOps()
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
		s.progress.FetchPage(opts.Page, len(allStars))

		result, err := f.ListStars(ctx, opts)
		if err != nil {
			return nil, err
		}

		allStars = append(allStars, result.Repos...)
		s.progress.FetchPageDone(opts.Page, len(result.Repos), len(allStars))

		if result.NextPage == 0 {
			break
		}
		opts.Page = result.NextPage
	}

	s.progress.FetchComplete(len(allStars))
	return allStars, nil
}

// starInfo holds info needed to process git results
type starInfo struct {
	RepoID        int64
	FullName      string
	LocalPath     string
	IsUpdate      bool
	NeedsDBUpdate bool // true if repo exists on disk but DB lacks local_path
}

// prepareStar saves metadata to DB and returns a git job if needed
func (s *Syncer) prepareStar(ctx context.Context, f forge.Forge, star forge.StarredRepo, opts Options, result *Result) (*git.Job, starInfo, error) {
	forgeName := f.Name()

	if s.manifest != nil {
		tombstoneID := fmt.Sprintf("%s:%s", forge.HostFromForge(forgeName), star.FullName)
		if s.manifest.IsTombstoned(tombstoneID) {
			result.Skipped++
			result.SkipReasons["tombstoned"] = append(result.SkipReasons["tombstoned"], star.FullName)
			return nil, starInfo{}, nil
		}
	}

	// Check if repo exists in DB
	existing, err := s.db.GetRepoByForgeID(forgeName, star.ForgeID)
	if err != nil {
		return nil, starInfo{}, fmt.Errorf("checking repo: %w", err)
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
		return nil, starInfo{}, fmt.Errorf("saving repo: %w", err)
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
		return nil, starInfo{}, fmt.Errorf("saving metadata: %w", err)
	}

	// Skip git operations if metadata-only
	if opts.MetadataOnly {
		result.Skipped++
		result.SkipReasons["metadata_only"] = append(result.SkipReasons["metadata_only"], star.FullName)
		return nil, starInfo{}, nil
	}

	// Skip private repos if configured
	if star.IsPrivate && !s.cfg.Sync.ClonePrivate {
		result.Skipped++
		result.SkipReasons["private"] = append(result.SkipReasons["private"], star.FullName)
		return nil, starInfo{}, nil
	}

	// Skip archived repos if configured
	if star.IsArchived && !s.cfg.Sync.CloneArchived {
		result.Skipped++
		result.SkipReasons["archived"] = append(result.SkipReasons["archived"], star.FullName)
		return nil, starInfo{}, nil
	}

	// Determine local path using template
	pathTemplate := s.cfg.Clone.PathTemplate
	if pathTemplate == "" {
		pathTemplate = "{{.Forge}}/{{.Owner}}/{{.Name}}"
	}

	relativePath, err := config.ExpandPathTemplate(pathTemplate, config.PathTemplateData{
		Forge:    forgeName,
		Owner:    star.Owner,
		Name:     star.Name,
		FullName: star.FullName,
		Language: star.Language,
	})
	if err != nil {
		return nil, starInfo{}, fmt.Errorf("expanding path template: %w", err)
	}
	localPath := filepath.Join(s.paths.ClonesDir, relativePath)

	info := starInfo{
		RepoID:    repoID,
		FullName:  star.FullName,
		LocalPath: localPath,
	}

	if opts.DryRun {
		if existing != nil && existing.LocalPath != nil {
			fmt.Fprintf(s.out, "[dry-run] Would update: %s\n", star.FullName)
		} else {
			fmt.Fprintf(s.out, "[dry-run] Would clone: %s\n", star.FullName)
		}
		return nil, starInfo{}, nil
	}

	// Check if already cloned
	if git.IsGitRepo(localPath) {
		// Already cloned - pull to update
		info.IsUpdate = true
		// Check if DB needs to learn about this existing clone
		if existing == nil || existing.LocalPath == nil {
			info.NeedsDBUpdate = true
		}
		return &git.Job{
			ID:    star.FullName,
			Type:  git.JobPull,
			Path:  localPath,
			Reset: s.cfg.Sync.ResetOnConflict,
		}, info, nil
	}

	// Not cloned yet - clone if configured and not pull-only
	if opts.PullOnly {
		result.Skipped++
		result.SkipReasons["pull_only"] = append(result.SkipReasons["pull_only"], star.FullName)
		return nil, starInfo{}, nil
	}

	if !s.cfg.Sync.CloneMissing {
		result.Skipped++
		result.SkipReasons["clone_disabled"] = append(result.SkipReasons["clone_disabled"], star.FullName)
		return nil, starInfo{}, nil
	}

	return &git.Job{
		ID:   star.FullName,
		Type: git.JobClone,
		URL:  star.CloneURL,
		Path: localPath,
		Options: git.CloneOptions{
			Depth:          s.cfg.Clone.Depth,
			SingleBranch:   s.cfg.Clone.SingleBranch,
			SkipSubmodules: s.cfg.Clone.SkipSubmodules,
			SkipLFS:        s.cfg.Clone.SkipLFS,
		},
	}, info, nil
}

// processGitResultsStream handles completed git jobs as they arrive
func (s *Syncer) processGitResultsStream(ctx context.Context, results <-chan git.JobResult, jobToStar map[string]starInfo, result *Result) {
	for r := range results {
		info, ok := jobToStar[r.Job.ID]
		if !ok {
			continue
		}

		success := r.Err == nil
		s.progress.UpdateGitOp(info.FullName, success)

		if r.Err != nil {
			result.Errors++
			result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("%s: %v", info.FullName, r.Err))
			if !s.progress.IsTTY() {
				fmt.Fprintf(s.out, "  ✗ %s: %v\n", info.FullName, r.Err)
			}
			continue
		}

		if info.IsUpdate {
			result.Updated++
			// Update DB if repo exists on disk but DB didn't know
			if info.NeedsDBUpdate {
				if err := s.db.UpdateRepoLocalPath(info.RepoID, info.LocalPath, time.Now()); err != nil {
					if !s.progress.IsTTY() {
						fmt.Fprintf(s.out, "  Warning: failed to update local path for %s: %v\n", info.FullName, err)
					}
				}
			}
		} else {
			// Update database with local path for new clones
			if err := s.db.UpdateRepoLocalPath(info.RepoID, info.LocalPath, time.Now()); err != nil {
				if !s.progress.IsTTY() {
					fmt.Fprintf(s.out, "  Warning: failed to update local path for %s: %v\n", info.FullName, err)
				}
			}
			result.Cloned++
		}

		// Extract and index after clone/update
		if err := s.extractor.ExtractAndIndex(ctx, info.RepoID, info.LocalPath); err != nil {
			if !s.progress.IsTTY() {
				fmt.Fprintf(s.out, "  Warning: indexing failed for %s: %v\n", info.FullName, err)
			}
		}
	}
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
