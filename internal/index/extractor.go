package index

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/lunelson/starbase-cli/internal/database"
	"github.com/lunelson/starbase-cli/internal/git"
	"github.com/lunelson/starbase-cli/internal/search"
)

// ExtractorConfig configures document extraction
type ExtractorConfig struct {
	IndexReadme      bool
	HighSignalFiles  []string
	MaxFileSizeKB    int
}

// DefaultExtractorConfig returns default extraction settings
func DefaultExtractorConfig() ExtractorConfig {
	return ExtractorConfig{
		IndexReadme: true,
		HighSignalFiles: []string{
			"go.mod",
			"package.json",
			"Cargo.toml",
			"pyproject.toml",
			"Dockerfile",
			"docker-compose.yml",
		},
		MaxFileSizeKB: 100,
	}
}

// Extractor handles document extraction from cloned repos
type Extractor struct {
	db       *database.DB
	searcher *search.Searcher
	config   ExtractorConfig
}

// NewExtractor creates a new Extractor
func NewExtractor(db *database.DB, config ExtractorConfig) *Extractor {
	return &Extractor{
		db:       db,
		searcher: search.New(db.DB),
		config:   config,
	}
}

// ExtractAndIndex extracts documents from a repo and updates the search index
func (e *Extractor) ExtractAndIndex(ctx context.Context, repoID int64, localPath string) error {
	repo, err := e.db.GetRepo(repoID)
	if err != nil {
		return fmt.Errorf("getting repo: %w", err)
	}
	if repo == nil {
		return fmt.Errorf("repo not found: %d", repoID)
	}

	meta, _ := e.db.GetMetadata(repoID)

	var readmeContent string
	var manifestContent string

	// Extract README
	if e.config.IndexReadme {
		content, filename, err := git.FindFile(localPath, []string{
			"README.md",
			"README.txt",
			"README",
			"readme.md",
		})
		if err == nil && content != "" {
			// Check size limit
			if len(content) <= e.config.MaxFileSizeKB*1024 {
				readmeContent = content
				if err := e.saveDocument(repoID, "readme", filename, content); err != nil {
					return fmt.Errorf("saving readme: %w", err)
				}
			}
		}
	}

	// Extract high-signal files
	for _, filename := range e.config.HighSignalFiles {
		content, err := git.ReadFile(localPath, filename)
		if err != nil || content == "" {
			continue
		}

		// Check size limit
		if len(content) > e.config.MaxFileSizeKB*1024 {
			continue
		}

		if err := e.saveDocument(repoID, "manifest", filename, content); err != nil {
			return fmt.Errorf("saving %s: %w", filename, err)
		}

		// Concatenate manifest content for indexing
		manifestContent += "\n" + content
	}

	// Update search index
	var description, topics string
	if meta != nil {
		if meta.Description != nil {
			description = *meta.Description
		}
		if len(meta.Topics) > 0 {
			topics = joinTopics(meta.Topics)
		}
	}

	if err := e.searcher.IndexRepo(repoID, repo.Name, description, topics, readmeContent, manifestContent); err != nil {
		return fmt.Errorf("indexing repo: %w", err)
	}

	return nil
}

func (e *Extractor) saveDocument(repoID int64, docType, filename, content string) error {
	hash := hashContent(content)
	now := time.Now()

	doc := &database.RepoDocument{
		RepoID:      repoID,
		DocType:     docType,
		Filename:    filename,
		Content:     &content,
		ContentHash: &hash,
		ExtractedAt: &now,
	}

	return e.db.UpsertDocument(doc)
}

func hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:8]) // First 8 bytes is enough for change detection
}

func joinTopics(topics []string) string {
	result := ""
	for i, t := range topics {
		if i > 0 {
			result += " "
		}
		result += t
	}
	return result
}

// RebuildAll extracts and indexes all cloned repos
func (e *Extractor) RebuildAll(ctx context.Context) (int, error) {
	repos, err := e.db.ListRepos("")
	if err != nil {
		return 0, fmt.Errorf("listing repos: %w", err)
	}

	count := 0
	for _, repo := range repos {
		if repo.LocalPath == nil {
			continue
		}

		if err := e.ExtractAndIndex(ctx, repo.ID, *repo.LocalPath); err != nil {
			// Log error but continue
			continue
		}
		count++
	}

	return count, nil
}
