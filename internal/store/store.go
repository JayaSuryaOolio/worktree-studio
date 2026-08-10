// Package store provides a SQLite-backed registry of repos, worktrees, and
// (for a later step) terminal sessions. Uses modernc.org/sqlite, a pure-Go
// SQLite driver, so the project needs no cgo toolchain.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	// SQLite does NOT enforce foreign keys (including every ON DELETE
	// CASCADE below) unless this pragma is set on the connection — off by
	// default for backwards-compatibility reasons dating back decades.
	// Found the hard way: verified empirically that a parent-row delete
	// left a child row behind despite its ON DELETE CASCADE declaration,
	// with this pragma unset. Since SetMaxOpenConns(1) above means there's
	// exactly one persistent connection for this Store's whole lifetime,
	// setting it once here is enough — it doesn't need to be reapplied per
	// query.
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign_keys pragma: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	if err := s.migrateAddWorktreeStatus(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate worktrees.status: %w", err)
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
	created_at TEXT NOT NULL,
	status     TEXT NOT NULL DEFAULT 'active'
);
CREATE INDEX IF NOT EXISTS idx_worktrees_repo_id ON worktrees(repo_id);

-- Schema only for now; populated starting with the tmux terminal step.
CREATE TABLE IF NOT EXISTS terminal_sessions (
	id                 TEXT PRIMARY KEY,
	worktree_id        TEXT NOT NULL REFERENCES worktrees(id) ON DELETE CASCADE,
	tmux_session_name  TEXT NOT NULL,
	tab_label          TEXT NOT NULL
);

-- One saved dockview layout per worktree (its terminal panel arrangement:
-- which panels, tiled how, which tabs). Upserted wholesale on every save,
-- never partially updated, so a single TEXT blob is the right shape here —
-- this isn't relational data, it's an opaque JSON document dockview itself
-- produces and consumes via toJSON()/fromJSON().
CREATE TABLE IF NOT EXISTS worktree_layouts (
	worktree_id  TEXT PRIMARY KEY REFERENCES worktrees(id) ON DELETE CASCADE,
	layout_json  TEXT NOT NULL,
	updated_at   TEXT NOT NULL
);
`
	_, err := s.db.Exec(schema)
	return err
}

// migrateAddWorktreeStatus adds the worktrees.status column for databases
// created before it existed. There's no migration framework in this
// project (see the plain CREATE TABLE IF NOT EXISTS schema above) — for a
// single added column, checking PRAGMA table_info and conditionally
// ALTER-ing is simpler than introducing one.
func (s *Store) migrateAddWorktreeStatus() error {
	rows, err := s.db.Query(`PRAGMA table_info(worktrees)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	hasStatus := false
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if name == "status" {
			hasStatus = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if hasStatus {
		return nil
	}

	_, err = s.db.Exec(`ALTER TABLE worktrees ADD COLUMN status TEXT NOT NULL DEFAULT '` + WorktreeStatusActive + `'`)
	return err
}

// Worktree status values. "active" and "archived" are both live today
// (see the archive/unarchive endpoints). "deleted" is reserved for a
// future bulk-management settings modal (filter by repo/status, then
// actually remove) — not wired into today's single-item delete endpoint,
// which still hard-removes the row via RemoveWorktree (see its doc
// comment for why).
const (
	WorktreeStatusActive   = "active"
	WorktreeStatusArchived = "archived"
	WorktreeStatusDeleted  = "deleted"
)

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
	Status    string `json:"status"`
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

// AddWorktree inserts a new worktree row, stamping CreatedAt and defaulting
// Status to "active" if empty.
func (s *Store) AddWorktree(w Worktree) error {
	if w.CreatedAt == "" {
		w.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if w.Status == "" {
		w.Status = WorktreeStatusActive
	}
	_, err := s.db.Exec(
		`INSERT INTO worktrees (id, repo_id, name, branch, path, created_at, status) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.RepoID, w.Name, w.Branch, w.Path, w.CreatedAt, w.Status,
	)
	return err
}

// WorktreePathExists reports whether a worktree with the given path is
// already registered — mirrors RepoPathExists, used the same way (a
// pre-insert check that turns a UNIQUE constraint violation into a
// friendly 409 instead of a raw SQL error reaching the client).
func (s *Store) WorktreePathExists(path string) (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM worktrees WHERE path = ?`, path).Scan(&count)
	return count > 0, err
}

// ListWorktrees returns worktrees for a given repo, newest first. statuses
// filters which status values to include; passing none returns every
// status (used by ListAllWorktreesForRepo, e.g. a future settings-modal
// datagrid) — the normal UI list calls this with WorktreeStatusActive only.
func (s *Store) ListWorktrees(repoID string, statuses ...string) ([]Worktree, error) {
	query := `SELECT id, repo_id, name, branch, path, created_at, status FROM worktrees WHERE repo_id = ?`
	args := []any{repoID}
	if len(statuses) > 0 {
		query += ` AND status IN (` + placeholders(len(statuses)) + `)`
		for _, st := range statuses {
			args = append(args, st)
		}
	}
	query += ` ORDER BY created_at DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Worktree
	for rows.Next() {
		var w Worktree
		if err := rows.Scan(&w.ID, &w.RepoID, &w.Name, &w.Branch, &w.Path, &w.CreatedAt, &w.Status); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func placeholders(n int) string {
	s := "?"
	for i := 1; i < n; i++ {
		s += ",?"
	}
	return s
}

// WorktreeWithRepo is a Worktree joined with its parent repo's display
// name — used by views that span every repo at once (the settings
// modal's "Worktrees" tab), where showing just a bare repo_id isn't useful.
type WorktreeWithRepo struct {
	Worktree
	RepoName string `json:"repo_name"`
}

// ListAllWorktreesWithRepo returns every worktree across every registered
// repo, any status, newest first, joined with its repo's name. Distinct
// from ListWorktrees (scoped to one repo, defaults to filtering status)
// deliberately: this is for a global cross-repo view, not the normal
// per-repo list, so it makes no assumption about which statuses matter to
// the caller.
func (s *Store) ListAllWorktreesWithRepo() ([]WorktreeWithRepo, error) {
	rows, err := s.db.Query(`
		SELECT w.id, w.repo_id, w.name, w.branch, w.path, w.created_at, w.status, r.name
		FROM worktrees w JOIN repos r ON w.repo_id = r.id
		ORDER BY w.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []WorktreeWithRepo
	for rows.Next() {
		var w WorktreeWithRepo
		if err := rows.Scan(&w.ID, &w.RepoID, &w.Name, &w.Branch, &w.Path, &w.CreatedAt, &w.Status, &w.RepoName); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// FindWorktreeByPath returns the worktree whose path exactly equals the
// given path, or whose path is a parent directory of it (cwd inside a
// subdirectory of the worktree still counts). Used to resolve a claude
// hook's reported cwd back to a worktree — there's no other signal a hook
// payload carries that identifies "which worktree is this." Returns
// sql.ErrNoRows if no worktree matches (a completely normal, expected
// outcome: most claude sessions on this machine have nothing to do with
// worktree-studio).
func (s *Store) FindWorktreeByPath(cwd string) (Worktree, error) {
	rows, err := s.db.Query(`SELECT id, repo_id, name, branch, path, created_at, status FROM worktrees`)
	if err != nil {
		return Worktree{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var w Worktree
		if err := rows.Scan(&w.ID, &w.RepoID, &w.Name, &w.Branch, &w.Path, &w.CreatedAt, &w.Status); err != nil {
			return Worktree{}, err
		}
		if cwd == w.Path || strings.HasPrefix(cwd, w.Path+"/") {
			return w, nil
		}
	}
	if err := rows.Err(); err != nil {
		return Worktree{}, err
	}
	return Worktree{}, sql.ErrNoRows
}

// GetWorktree fetches a single worktree by id. Returns sql.ErrNoRows if not found.
func (s *Store) GetWorktree(id string) (Worktree, error) {
	var w Worktree
	err := s.db.QueryRow(
		`SELECT id, repo_id, name, branch, path, created_at, status FROM worktrees WHERE id = ?`, id,
	).Scan(&w.ID, &w.RepoID, &w.Name, &w.Branch, &w.Path, &w.CreatedAt, &w.Status)
	return w, err
}

// SetWorktreeStatus updates a worktree's status (active/archived/deleted).
// Used by archive/unarchive (no git changes at all — purely a visibility
// flag) and by the delete flow (status="deleted", after the git worktree
// and branch have actually been removed from disk).
func (s *Store) SetWorktreeStatus(id, status string) error {
	_, err := s.db.Exec(`UPDATE worktrees SET status = ? WHERE id = ?`, status, id)
	return err
}

// RemoveWorktree deletes a worktree row by id outright. Still what the
// real (hard) delete flow calls today — WorktreeStatusDeleted is not wired
// up to any handler yet. It's reserved for a future settings-modal bulk
// delete (filter by repo/status, then actually remove), which is expected
// to soft-mark rows "deleted" first and purge them in a separate step; that
// design isn't built yet, and wiring "deleted" into today's single-item
// delete endpoint would collide with the `path TEXT UNIQUE` constraint the
// instant someone recreates a worktree with the same name (the old
// soft-deleted row would still be occupying that path). Left unresolved
// until the actual bulk-delete feature exists to make an informed call
// (e.g. mutating path on soft-delete, or dropping the UNIQUE constraint).
func (s *Store) RemoveWorktree(id string) error {
	_, err := s.db.Exec(`DELETE FROM worktrees WHERE id = ?`, id)
	return err
}

// TerminalSession is a terminal tab backed by a tmux session, scoped to a
// worktree. The tmux session itself outlives the worktree-studio server
// process, which is what makes a server restart safe to reattach after.
type TerminalSession struct {
	ID              string `json:"id"`
	WorktreeID      string `json:"worktree_id"`
	TmuxSessionName string `json:"tmux_session_name"`
	TabLabel        string `json:"tab_label"`
}

// AddTerminalSession inserts a new terminal session row.
func (s *Store) AddTerminalSession(t TerminalSession) error {
	_, err := s.db.Exec(
		`INSERT INTO terminal_sessions (id, worktree_id, tmux_session_name, tab_label) VALUES (?, ?, ?, ?)`,
		t.ID, t.WorktreeID, t.TmuxSessionName, t.TabLabel,
	)
	return err
}

// ListTerminalSessions returns all terminal sessions for a given worktree.
func (s *Store) ListTerminalSessions(worktreeID string) ([]TerminalSession, error) {
	rows, err := s.db.Query(
		`SELECT id, worktree_id, tmux_session_name, tab_label FROM terminal_sessions WHERE worktree_id = ?`,
		worktreeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TerminalSession
	for rows.Next() {
		var t TerminalSession
		if err := rows.Scan(&t.ID, &t.WorktreeID, &t.TmuxSessionName, &t.TabLabel); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListAllTerminalSessions returns every terminal session row, used at
// server startup to reconcile against tmux's actual live sessions.
func (s *Store) ListAllTerminalSessions() ([]TerminalSession, error) {
	rows, err := s.db.Query(`SELECT id, worktree_id, tmux_session_name, tab_label FROM terminal_sessions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TerminalSession
	for rows.Next() {
		var t TerminalSession
		if err := rows.Scan(&t.ID, &t.WorktreeID, &t.TmuxSessionName, &t.TabLabel); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetTerminalSession fetches a single terminal session by id. Returns
// sql.ErrNoRows if not found.
func (s *Store) GetTerminalSession(id string) (TerminalSession, error) {
	var t TerminalSession
	err := s.db.QueryRow(
		`SELECT id, worktree_id, tmux_session_name, tab_label FROM terminal_sessions WHERE id = ?`, id,
	).Scan(&t.ID, &t.WorktreeID, &t.TmuxSessionName, &t.TabLabel)
	return t, err
}

// RemoveTerminalSession deletes a terminal session row by id.
func (s *Store) RemoveTerminalSession(id string) error {
	_, err := s.db.Exec(`DELETE FROM terminal_sessions WHERE id = ?`, id)
	return err
}

// GetWorktreeLayout returns the saved dockview layout JSON for a worktree.
// Returns sql.ErrNoRows if nothing has been saved yet (e.g. first-ever
// open of that worktree's terminal view).
func (s *Store) GetWorktreeLayout(worktreeID string) (string, error) {
	var layoutJSON string
	err := s.db.QueryRow(
		`SELECT layout_json FROM worktree_layouts WHERE worktree_id = ?`, worktreeID,
	).Scan(&layoutJSON)
	return layoutJSON, err
}

// SaveWorktreeLayout upserts the saved dockview layout JSON for a
// worktree, stamping updated_at to now.
func (s *Store) SaveWorktreeLayout(worktreeID, layoutJSON string) error {
	_, err := s.db.Exec(
		`INSERT INTO worktree_layouts (worktree_id, layout_json, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(worktree_id) DO UPDATE SET layout_json = excluded.layout_json, updated_at = excluded.updated_at`,
		worktreeID, layoutJSON, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}
