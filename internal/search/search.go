package search

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// Result represents a search result
type Result struct {
	RepoID      int64
	Forge       string
	Owner       string
	Name        string
	FullName    string
	WebURL      string
	LocalPath   *string
	Description *string
	Language    *string
	Topics      []string
	StarsCount  *int
	Score       float64
}

// Searcher provides search functionality
type Searcher struct {
	db *sql.DB
}

// New creates a new Searcher
func New(db *sql.DB) *Searcher {
	return &Searcher{db: db}
}

// Search performs a BM25 search and returns ranked results
func (s *Searcher) Search(query string, limit int) ([]Result, error) {
	if limit <= 0 {
		limit = 20
	}

	// Sanitize query for FTS5
	ftsQuery := sanitizeFTSQuery(query)

	rows, err := s.db.Query(`
		SELECT 
			r.id, r.forge, r.owner, r.name, r.full_name, r.web_url, r.local_path,
			m.description, m.language, m.topics, m.stars_count,
			bm25(search_index) as score
		FROM search_index si
		JOIN repos r ON r.id = si.repo_id
		LEFT JOIN repo_metadata m ON m.repo_id = r.id
		WHERE search_index MATCH ?
		ORDER BY score
		LIMIT ?
	`, ftsQuery, limit)

	if err != nil {
		return nil, fmt.Errorf("search query failed: %w", err)
	}
	defer rows.Close()

	var results []Result
	for rows.Next() {
		var r Result
		var topicsJSON sql.NullString

		err := rows.Scan(
			&r.RepoID, &r.Forge, &r.Owner, &r.Name, &r.FullName, &r.WebURL, &r.LocalPath,
			&r.Description, &r.Language, &topicsJSON, &r.StarsCount,
			&r.Score,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning result: %w", err)
		}

		if topicsJSON.Valid {
			json.Unmarshal([]byte(topicsJSON.String), &r.Topics)
		}

		results = append(results, r)
	}

	return results, rows.Err()
}

// IndexRepo adds or updates a repo in the search index
func (s *Searcher) IndexRepo(repoID int64, name, description, topics, readmeContent, manifestContent string) error {
	// Delete existing entry
	_, err := s.db.Exec("DELETE FROM search_index WHERE repo_id = ?", repoID)
	if err != nil {
		return fmt.Errorf("deleting old index: %w", err)
	}

	// Insert new entry
	_, err = s.db.Exec(`
		INSERT INTO search_index (repo_id, name, description, topics, readme_content, manifest_content)
		VALUES (?, ?, ?, ?, ?, ?)
	`, repoID, name, description, topics, readmeContent, manifestContent)

	if err != nil {
		return fmt.Errorf("inserting index: %w", err)
	}

	return nil
}

// RemoveRepo removes a repo from the search index
func (s *Searcher) RemoveRepo(repoID int64) error {
	_, err := s.db.Exec("DELETE FROM search_index WHERE repo_id = ?", repoID)
	return err
}

// RebuildIndex rebuilds the entire search index from scratch
func (s *Searcher) RebuildIndex() (int, error) {
	// Clear existing index
	if _, err := s.db.Exec("DELETE FROM search_index"); err != nil {
		return 0, fmt.Errorf("clearing index: %w", err)
	}

	// Build index from repos and documents
	rows, err := s.db.Query(`
		SELECT 
			r.id, r.name, 
			COALESCE(m.description, ''),
			COALESCE(m.topics, '[]'),
			COALESCE((SELECT content FROM repo_documents WHERE repo_id = r.id AND doc_type = 'readme' LIMIT 1), ''),
			COALESCE((SELECT content FROM repo_documents WHERE repo_id = r.id AND doc_type = 'manifest' LIMIT 1), '')
		FROM repos r
		LEFT JOIN repo_metadata m ON m.repo_id = r.id
	`)
	if err != nil {
		return 0, fmt.Errorf("querying repos: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var repoID int64
		var name, description, topicsJSON, readme, manifest string

		if err := rows.Scan(&repoID, &name, &description, &topicsJSON, &readme, &manifest); err != nil {
			return count, fmt.Errorf("scanning repo: %w", err)
		}

		// Parse topics JSON to space-separated string
		var topics []string
		json.Unmarshal([]byte(topicsJSON), &topics)
		topicsStr := strings.Join(topics, " ")

		if err := s.IndexRepo(repoID, name, description, topicsStr, readme, manifest); err != nil {
			return count, fmt.Errorf("indexing repo %d: %w", repoID, err)
		}

		count++
	}

	return count, rows.Err()
}

// sanitizeFTSQuery escapes special FTS5 characters and prepares query
func sanitizeFTSQuery(query string) string {
	// Remove FTS5 operators that might cause issues
	// Keep alphanumeric, spaces, and basic punctuation
	var parts []string

	tokens := strings.Fields(query)
	for _, token := range tokens {
		// Handle special filter syntax
		if strings.HasPrefix(token, "lang:") ||
			strings.HasPrefix(token, "topic:") ||
			strings.HasPrefix(token, "has:") {
			// These are filter prefixes, skip them for now
			// (filters will be handled separately in a future phase)
			continue
		}

		// Escape special characters
		clean := strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') || r == '-' || r == '_' {
				return r
			}
			return -1
		}, token)

		if clean != "" {
			parts = append(parts, clean+"*") // Prefix search
		}
	}

	return strings.Join(parts, " ")
}
