package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

// Open opens or creates a database at the given path
func Open(dbPath string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("creating database directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	d := &DB{db}
	if err := d.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating database: %w", err)
	}

	return d, nil
}

func (d *DB) migrate() error {
	_, err := d.Exec(schema)
	return err
}

const schema = `
-- Core identity (stable across renames)
CREATE TABLE IF NOT EXISTS repos (
    id INTEGER PRIMARY KEY,
    forge TEXT NOT NULL,
    forge_id TEXT NOT NULL,
    owner TEXT NOT NULL,
    name TEXT NOT NULL,
    full_name TEXT NOT NULL,
    clone_url TEXT NOT NULL,
    web_url TEXT NOT NULL,
    local_path TEXT,
    starred_at TEXT,
    cloned_at TEXT,
    synced_at TEXT,
    status TEXT DEFAULT 'active',
    UNIQUE(forge, forge_id)
);

CREATE INDEX IF NOT EXISTS idx_repos_full_name ON repos(forge, full_name);
CREATE INDEX IF NOT EXISTS idx_repos_status ON repos(status);

-- Mutable metadata (refreshed on sync)
CREATE TABLE IF NOT EXISTS repo_metadata (
    repo_id INTEGER PRIMARY KEY REFERENCES repos(id) ON DELETE CASCADE,
    description TEXT,
    language TEXT,
    topics TEXT,
    stars_count INTEGER,
    forks_count INTEGER,
    default_branch TEXT,
    is_archived INTEGER DEFAULT 0,
    is_private INTEGER DEFAULT 0,
    pushed_at TEXT,
    updated_at TEXT,
    latest_release TEXT,
    latest_release_at TEXT,
    size_kb INTEGER
);

-- Indexed documents (README + high-signal files)
CREATE TABLE IF NOT EXISTS repo_documents (
    id INTEGER PRIMARY KEY,
    repo_id INTEGER REFERENCES repos(id) ON DELETE CASCADE,
    doc_type TEXT NOT NULL,
    filename TEXT NOT NULL,
    content TEXT,
    content_hash TEXT,
    extracted_at TEXT,
    UNIQUE(repo_id, doc_type, filename)
);

CREATE INDEX IF NOT EXISTS idx_repo_documents_type ON repo_documents(repo_id, doc_type);

-- Local annotations (from manifest, cached here)
CREATE TABLE IF NOT EXISTS repo_annotations (
    repo_id INTEGER PRIMARY KEY REFERENCES repos(id) ON DELETE CASCADE,
    collections TEXT,
    notes TEXT,
    is_pinned INTEGER DEFAULT 0,
    local_tags TEXT
);

-- Optional: vector embeddings
CREATE TABLE IF NOT EXISTS embeddings (
    id INTEGER PRIMARY KEY,
    repo_id INTEGER REFERENCES repos(id) ON DELETE CASCADE,
    doc_type TEXT NOT NULL,
    chunk_index INTEGER,
    embedding BLOB,
    model TEXT,
    created_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_embeddings_repo ON embeddings(repo_id, doc_type);

-- FTS5 virtual table for BM25 search
CREATE VIRTUAL TABLE IF NOT EXISTS search_index USING fts5(
    repo_id,
    name,
    description,
    topics,
    readme_content,
    manifest_content,
    content='',
    tokenize='porter'
);
`
