package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type Repo struct {
	ID        int64
	Forge     string
	ForgeID   string
	Owner     string
	Name      string
	FullName  string
	CloneURL  string
	WebURL    string
	LocalPath *string
	StarredAt *time.Time
	ClonedAt  *time.Time
	SyncedAt  *time.Time
	Status    string
}

type RepoMetadata struct {
	RepoID          int64
	Description     *string
	Language        *string
	Topics          []string
	StarsCount      *int
	ForksCount      *int
	DefaultBranch   *string
	IsArchived      bool
	IsPrivate       bool
	PushedAt        *time.Time
	UpdatedAt       *time.Time
	LatestRelease   *string
	LatestReleaseAt *time.Time
	SizeKB          *int
}

type RepoDocument struct {
	ID          int64
	RepoID      int64
	DocType     string
	Filename    string
	Content     *string
	ContentHash *string
	ExtractedAt *time.Time
}

type RepoAnnotation struct {
	RepoID      int64
	Collections []string
	Notes       *string
	IsPinned    bool
	LocalTags   []string
}

// InsertRepo inserts a new repo and returns its ID
func (d *DB) InsertRepo(r *Repo) (int64, error) {
	result, err := d.Exec(`
		INSERT INTO repos (forge, forge_id, owner, name, full_name, clone_url, web_url, local_path, starred_at, cloned_at, synced_at, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(forge, forge_id) DO UPDATE SET
			owner = excluded.owner,
			name = excluded.name,
			full_name = excluded.full_name,
			clone_url = excluded.clone_url,
			web_url = excluded.web_url,
			synced_at = excluded.synced_at
	`, r.Forge, r.ForgeID, r.Owner, r.Name, r.FullName, r.CloneURL, r.WebURL,
		r.LocalPath, formatTime(r.StarredAt), formatTime(r.ClonedAt), formatTime(r.SyncedAt), r.Status)
	if err != nil {
		return 0, fmt.Errorf("inserting repo: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil || id == 0 {
		// If conflict occurred (update not insert), fetch the existing ID
		row := d.QueryRow("SELECT id FROM repos WHERE forge = ? AND forge_id = ?", r.Forge, r.ForgeID)
		if err := row.Scan(&id); err != nil {
			return 0, fmt.Errorf("getting repo id: %w", err)
		}
	}

	return id, nil
}

// GetRepo retrieves a repo by ID
func (d *DB) GetRepo(id int64) (*Repo, error) {
	row := d.QueryRow(`
		SELECT id, forge, forge_id, owner, name, full_name, clone_url, web_url, local_path, starred_at, cloned_at, synced_at, status
		FROM repos WHERE id = ?
	`, id)

	return scanRepo(row)
}

// GetRepoByForgeID retrieves a repo by forge and forge ID
func (d *DB) GetRepoByForgeID(forge, forgeID string) (*Repo, error) {
	row := d.QueryRow(`
		SELECT id, forge, forge_id, owner, name, full_name, clone_url, web_url, local_path, starred_at, cloned_at, synced_at, status
		FROM repos WHERE forge = ? AND forge_id = ?
	`, forge, forgeID)

	return scanRepo(row)
}

// GetRepoByFullName retrieves a repo by forge and full name
func (d *DB) GetRepoByFullName(forge, fullName string) (*Repo, error) {
	row := d.QueryRow(`
		SELECT id, forge, forge_id, owner, name, full_name, clone_url, web_url, local_path, starred_at, cloned_at, synced_at, status
		FROM repos WHERE forge = ? AND full_name = ?
	`, forge, fullName)

	return scanRepo(row)
}

// ListRepos returns all repos with optional status filter
func (d *DB) ListRepos(status string) ([]*Repo, error) {
	var rows *sql.Rows
	var err error

	if status != "" {
		rows, err = d.Query(`
			SELECT id, forge, forge_id, owner, name, full_name, clone_url, web_url, local_path, starred_at, cloned_at, synced_at, status
			FROM repos WHERE status = ?
			ORDER BY starred_at DESC
		`, status)
	} else {
		rows, err = d.Query(`
			SELECT id, forge, forge_id, owner, name, full_name, clone_url, web_url, local_path, starred_at, cloned_at, synced_at, status
			FROM repos
			ORDER BY starred_at DESC
		`)
	}

	if err != nil {
		return nil, fmt.Errorf("listing repos: %w", err)
	}
	defer rows.Close()

	var repos []*Repo
	for rows.Next() {
		r, err := scanRepoRows(rows)
		if err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}

	return repos, rows.Err()
}

// UpdateRepoLocalPath updates the local path for a repo
func (d *DB) UpdateRepoLocalPath(id int64, localPath string, clonedAt time.Time) error {
	_, err := d.Exec("UPDATE repos SET local_path = ?, cloned_at = ? WHERE id = ?",
		localPath, formatTime(&clonedAt), id)
	return err
}

// UpdateRepoStatus updates the status for a repo
func (d *DB) UpdateRepoStatus(id int64, status string) error {
	_, err := d.Exec("UPDATE repos SET status = ? WHERE id = ?", status, id)
	return err
}

// DeleteRepo removes a repo and all related data
func (d *DB) DeleteRepo(id int64) error {
	_, err := d.Exec("DELETE FROM repos WHERE id = ?", id)
	return err
}

// UpsertMetadata inserts or updates repo metadata
func (d *DB) UpsertMetadata(m *RepoMetadata) error {
	topicsJSON, _ := json.Marshal(m.Topics)

	_, err := d.Exec(`
		INSERT INTO repo_metadata (repo_id, description, language, topics, stars_count, forks_count, default_branch, is_archived, is_private, pushed_at, updated_at, latest_release, latest_release_at, size_kb)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repo_id) DO UPDATE SET
			description = excluded.description,
			language = excluded.language,
			topics = excluded.topics,
			stars_count = excluded.stars_count,
			forks_count = excluded.forks_count,
			default_branch = excluded.default_branch,
			is_archived = excluded.is_archived,
			is_private = excluded.is_private,
			pushed_at = excluded.pushed_at,
			updated_at = excluded.updated_at,
			latest_release = excluded.latest_release,
			latest_release_at = excluded.latest_release_at,
			size_kb = excluded.size_kb
	`, m.RepoID, m.Description, m.Language, string(topicsJSON), m.StarsCount, m.ForksCount,
		m.DefaultBranch, m.IsArchived, m.IsPrivate, formatTime(m.PushedAt), formatTime(m.UpdatedAt),
		m.LatestRelease, formatTime(m.LatestReleaseAt), m.SizeKB)

	return err
}

// GetMetadata retrieves metadata for a repo
func (d *DB) GetMetadata(repoID int64) (*RepoMetadata, error) {
	row := d.QueryRow(`
		SELECT repo_id, description, language, topics, stars_count, forks_count, default_branch, is_archived, is_private, pushed_at, updated_at, latest_release, latest_release_at, size_kb
		FROM repo_metadata WHERE repo_id = ?
	`, repoID)

	var m RepoMetadata
	var topicsJSON sql.NullString
	var pushedAt, updatedAt, latestReleaseAt sql.NullString

	err := row.Scan(&m.RepoID, &m.Description, &m.Language, &topicsJSON, &m.StarsCount, &m.ForksCount,
		&m.DefaultBranch, &m.IsArchived, &m.IsPrivate, &pushedAt, &updatedAt, &m.LatestRelease, &latestReleaseAt, &m.SizeKB)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if topicsJSON.Valid {
		json.Unmarshal([]byte(topicsJSON.String), &m.Topics)
	}
	m.PushedAt = parseTime(pushedAt)
	m.UpdatedAt = parseTime(updatedAt)
	m.LatestReleaseAt = parseTime(latestReleaseAt)

	return &m, nil
}

// UpsertDocument inserts or updates a document
func (d *DB) UpsertDocument(doc *RepoDocument) error {
	_, err := d.Exec(`
		INSERT INTO repo_documents (repo_id, doc_type, filename, content, content_hash, extracted_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(repo_id, doc_type, filename) DO UPDATE SET
			content = excluded.content,
			content_hash = excluded.content_hash,
			extracted_at = excluded.extracted_at
	`, doc.RepoID, doc.DocType, doc.Filename, doc.Content, doc.ContentHash, formatTime(doc.ExtractedAt))

	return err
}

// GetDocument retrieves a document by repo, type, and filename
func (d *DB) GetDocument(repoID int64, docType, filename string) (*RepoDocument, error) {
	row := d.QueryRow(`
		SELECT id, repo_id, doc_type, filename, content, content_hash, extracted_at
		FROM repo_documents WHERE repo_id = ? AND doc_type = ? AND filename = ?
	`, repoID, docType, filename)

	var doc RepoDocument
	var extractedAt sql.NullString

	err := row.Scan(&doc.ID, &doc.RepoID, &doc.DocType, &doc.Filename, &doc.Content, &doc.ContentHash, &extractedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	doc.ExtractedAt = parseTime(extractedAt)
	return &doc, nil
}

// UpsertAnnotation inserts or updates annotations
func (d *DB) UpsertAnnotation(a *RepoAnnotation) error {
	collectionsJSON, _ := json.Marshal(a.Collections)
	tagsJSON, _ := json.Marshal(a.LocalTags)

	_, err := d.Exec(`
		INSERT INTO repo_annotations (repo_id, collections, notes, is_pinned, local_tags)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(repo_id) DO UPDATE SET
			collections = excluded.collections,
			notes = excluded.notes,
			is_pinned = excluded.is_pinned,
			local_tags = excluded.local_tags
	`, a.RepoID, string(collectionsJSON), a.Notes, a.IsPinned, string(tagsJSON))

	return err
}

// GetAnnotation retrieves annotations for a repo
func (d *DB) GetAnnotation(repoID int64) (*RepoAnnotation, error) {
	row := d.QueryRow(`
		SELECT repo_id, collections, notes, is_pinned, local_tags
		FROM repo_annotations WHERE repo_id = ?
	`, repoID)

	var a RepoAnnotation
	var collectionsJSON, tagsJSON sql.NullString

	err := row.Scan(&a.RepoID, &collectionsJSON, &a.Notes, &a.IsPinned, &tagsJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if collectionsJSON.Valid {
		json.Unmarshal([]byte(collectionsJSON.String), &a.Collections)
	}
	if tagsJSON.Valid {
		json.Unmarshal([]byte(tagsJSON.String), &a.LocalTags)
	}

	return &a, nil
}

// CountRepos returns the total number of repos
func (d *DB) CountRepos() (int, error) {
	var count int
	err := d.QueryRow("SELECT COUNT(*) FROM repos").Scan(&count)
	return count, err
}

// Helper functions

func scanRepo(row *sql.Row) (*Repo, error) {
	var r Repo
	var starredAt, clonedAt, syncedAt sql.NullString

	err := row.Scan(&r.ID, &r.Forge, &r.ForgeID, &r.Owner, &r.Name, &r.FullName, &r.CloneURL, &r.WebURL,
		&r.LocalPath, &starredAt, &clonedAt, &syncedAt, &r.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	r.StarredAt = parseTime(starredAt)
	r.ClonedAt = parseTime(clonedAt)
	r.SyncedAt = parseTime(syncedAt)

	return &r, nil
}

func scanRepoRows(rows *sql.Rows) (*Repo, error) {
	var r Repo
	var starredAt, clonedAt, syncedAt sql.NullString

	err := rows.Scan(&r.ID, &r.Forge, &r.ForgeID, &r.Owner, &r.Name, &r.FullName, &r.CloneURL, &r.WebURL,
		&r.LocalPath, &starredAt, &clonedAt, &syncedAt, &r.Status)
	if err != nil {
		return nil, err
	}

	r.StarredAt = parseTime(starredAt)
	r.ClonedAt = parseTime(clonedAt)
	r.SyncedAt = parseTime(syncedAt)

	return &r, nil
}

func formatTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.RFC3339)
	return &s
}

func parseTime(s sql.NullString) *time.Time {
	if !s.Valid {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s.String)
	if err != nil {
		return nil
	}
	return &t
}
