// Package store provides a SQLite-backed registry of repos, worktrees, and
// (for a later step) terminal sessions. Uses modernc.org/sqlite, a pure-Go
// SQLite driver, so the project needs no cgo toolchain.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// DefaultPath returns the default DB location: ~/.worktree-studio/studio.db
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".worktree-studio", "studio.db"), nil
}

// Store wraps a *sql.DB with worktree-studio's schema and query helpers.
type Store struct {
	db *sql.DB
}

// Open opens (creating if necessary) the SQLite database at path and
// ensures the schema exists.
func Open(path string) (*Store, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create db dir %s: %w", dir, err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db %s: %w", path, err)
	}
	// SQLite handles one writer at a time; keep it simple for this local tool.
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	return s, nil
}

// OpenDefault opens the store at DefaultPath().
func OpenDefault() (*Store, error) {
	p, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return Open(p)
}

// Close closes the underlying database.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS repos (
	id   TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	path TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS worktrees (
	id         TEXT PRIMARY KEY,
	repo_id    TEXT NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
	name       TEXT NOT NULL,
	branch     TEXT NOT NULL,
	path       TEXT NOT NULL UNIQUE,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_worktrees_repo_id ON worktrees(repo_id);

-- Schema only for now; populated starting with the tmux terminal step.
CREATE TABLE IF NOT EXISTS terminal_sessions (
	id                 TEXT PRIMARY KEY,
	worktree_id        TEXT NOT NULL REFERENCES worktrees(id) ON DELETE CASCADE,
	tmux_session_name  TEXT NOT NULL,
	tab_label          TEXT NOT NULL
);
`
	_, err := s.db.Exec(schema)
	return err
}

// Repo is a registered git repository.
type Repo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// Worktree is a git worktree created under a registered repo.
type Worktree struct {
	ID        string `json:"id"`
	RepoID    string `json:"repo_id"`
	Name      string `json:"name"`
	Branch    string `json:"branch"`
	Path      string `json:"path"`
	CreatedAt string `json:"created_at"`
}

// AddRepo inserts a new repo row.
func (s *Store) AddRepo(r Repo) error {
	_, err := s.db.Exec(`INSERT INTO repos (id, name, path) VALUES (?, ?, ?)`, r.ID, r.Name, r.Path)
	return err
}

// ListRepos returns all registered repos.
func (s *Store) ListRepos() ([]Repo, error) {
	rows, err := s.db.Query(`SELECT id, name, path FROM repos ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Repo
	for rows.Next() {
		var r Repo
		if err := rows.Scan(&r.ID, &r.Name, &r.Path); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetRepo fetches a single repo by id. Returns sql.ErrNoRows if not found.
func (s *Store) GetRepo(id string) (Repo, error) {
	var r Repo
	err := s.db.QueryRow(`SELECT id, name, path FROM repos WHERE id = ?`, id).Scan(&r.ID, &r.Name, &r.Path)
	return r, err
}

// RepoPathExists reports whether a repo with the given path is already registered.
func (s *Store) RepoPathExists(path string) (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM repos WHERE path = ?`, path).Scan(&count)
	return count > 0, err
}

// AddWorktree inserts a new worktree row, stamping CreatedAt if empty.
func (s *Store) AddWorktree(w Worktree) error {
	if w.CreatedAt == "" {
		w.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.db.Exec(
		`INSERT INTO worktrees (id, repo_id, name, branch, path, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		w.ID, w.RepoID, w.Name, w.Branch, w.Path, w.CreatedAt,
	)
	return err
}

// ListWorktrees returns all worktrees for a given repo, newest first.
func (s *Store) ListWorktrees(repoID string) ([]Worktree, error) {
	rows, err := s.db.Query(
		`SELECT id, repo_id, name, branch, path, created_at FROM worktrees WHERE repo_id = ? ORDER BY created_at DESC`,
		repoID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Worktree
	for rows.Next() {
		var w Worktree
		if err := rows.Scan(&w.ID, &w.RepoID, &w.Name, &w.Branch, &w.Path, &w.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// GetWorktree fetches a single worktree by id. Returns sql.ErrNoRows if not found.
func (s *Store) GetWorktree(id string) (Worktree, error) {
	var w Worktree
	err := s.db.QueryRow(
		`SELECT id, repo_id, name, branch, path, created_at FROM worktrees WHERE id = ?`, id,
	).Scan(&w.ID, &w.RepoID, &w.Name, &w.Branch, &w.Path, &w.CreatedAt)
	return w, err
}

// RemoveWorktree deletes a worktree row by id.
func (s *Store) RemoveWorktree(id string) error {
	_, err := s.db.Exec(`DELETE FROM worktrees WHERE id = ?`, id)
	return err
}
