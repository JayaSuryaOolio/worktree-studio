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
	if err := s.migrateAddWorktreeSource(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate worktrees.source: %w", err)
	}
	if err := s.migrateAddTerminalSessionCreatedAt(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate terminal_sessions.created_at: %w", err)
	}
	if err := s.migrateAddRepoBaseBranch(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate repos.base_branch: %w", err)
	}
	if err := s.migrateAddWorktreeArchivedAt(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate worktrees.archived_at: %w", err)
	}
	if err := s.migrateAddWorktreeSourceBranch(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate worktrees.source_branch: %w", err)
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

// migrateAddWorktreeSource adds the worktrees.source column (distinguishing
// worktrees created through worktree-studio from ones attached/imported from
// an existing `git worktree`), same ALTER-if-missing approach as
// migrateAddWorktreeStatus above.
func (s *Store) migrateAddWorktreeSource() error {
	hasColumn, err := s.hasColumn("worktrees", "source")
	if err != nil {
		return err
	}
	if hasColumn {
		return nil
	}
	_, err = s.db.Exec(`ALTER TABLE worktrees ADD COLUMN source TEXT NOT NULL DEFAULT '` + WorktreeSourceCreated + `'`)
	return err
}

// migrateAddTerminalSessionCreatedAt adds the terminal_sessions.created_at
// column for databases created before it existed. Existing rows (whose
// actual creation time was never recorded) backfill to the migration time —
// the best available answer, not a real value, but no worse than leaving it
// NULL for a column that's meant to always have a display-worthy timestamp.
func (s *Store) migrateAddTerminalSessionCreatedAt() error {
	hasColumn, err := s.hasColumn("terminal_sessions", "created_at")
	if err != nil {
		return err
	}
	if hasColumn {
		return nil
	}
	_, err = s.db.Exec(`ALTER TABLE terminal_sessions ADD COLUMN created_at TEXT NOT NULL DEFAULT '` + time.Now().UTC().Format(time.RFC3339) + `'`)
	return err
}

// migrateAddRepoBaseBranch adds the repos.base_branch column: the branch new
// worktrees are created from (see gitops.AddWorktree's startPoint param).
// Empty string means "auto-detect" (origin/HEAD, else local main/master,
// else whatever the main checkout's HEAD happens to be) rather than a fixed
// branch — see internal/api's handleCreateWorktree.
func (s *Store) migrateAddRepoBaseBranch() error {
	hasColumn, err := s.hasColumn("repos", "base_branch")
	if err != nil {
		return err
	}
	if hasColumn {
		return nil
	}
	_, err = s.db.Exec(`ALTER TABLE repos ADD COLUMN base_branch TEXT NOT NULL DEFAULT ''`)
	return err
}

// migrateAddWorktreeArchivedAt adds the worktrees.archived_at column for
// databases created before it existed. Its real archive time is long
// lost for any row already sitting at status="archived", so rather than
// leave those permanently ineligible for the retention sweep (archived_at
// would stay "" forever — nothing else ever sets it for an
// already-archived row), this backfills the migration time itself: the
// same "best-available default, not a fabricated value" call
// migrateAddTerminalSessionCreatedAt makes for its own column, and it
// means these worktrees start their 60-day countdown today rather than
// being silently exempted from ever being swept.
func (s *Store) migrateAddWorktreeArchivedAt() error {
	hasColumn, err := s.hasColumn("worktrees", "archived_at")
	if err != nil {
		return err
	}
	if !hasColumn {
		if _, err := s.db.Exec(`ALTER TABLE worktrees ADD COLUMN archived_at TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	// Runs every startup, not just the one that adds the column above: an
	// already-archived row's archived_at only ever gets set by
	// SetWorktreeStatus on a *future* archive/unarchive call, so a row
	// that's stayed continuously archived since before this column
	// existed would otherwise sit at "" — permanently exempt from the
	// retention sweep — forever. Harmless to re-run: it only touches rows
	// still at "".
	_, err = s.db.Exec(
		`UPDATE worktrees SET archived_at = ? WHERE status = ? AND archived_at = ''`,
		time.Now().UTC().Format(time.RFC3339), WorktreeStatusArchived,
	)
	return err
}

// migrateAddWorktreeSourceBranch adds the worktrees.source_branch column
// for databases created before it existed — same ALTER-if-missing
// approach as the other single-column migrations above. Existing rows
// have no recorded source branch to backfill, so they're left at "" (the
// worktrees table for the settings page just shows "—" for those).
func (s *Store) migrateAddWorktreeSourceBranch() error {
	hasColumn, err := s.hasColumn("worktrees", "source_branch")
	if err != nil {
		return err
	}
	if hasColumn {
		return nil
	}
	_, err = s.db.Exec(`ALTER TABLE worktrees ADD COLUMN source_branch TEXT NOT NULL DEFAULT ''`)
	return err
}

// hasColumn reports whether table has a column named col.
func (s *Store) hasColumn(table, col string) (bool, error) {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == col {
			return true, nil
		}
	}
	return false, rows.Err()
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

// Worktree source values: "created" means worktree-studio itself ran `git
// worktree add` for it (the normal "+ New worktree" flow); "imported" means
// it already existed as a plain `git worktree` and was attached into the DB
// via the settings page's "attach" flow instead. "root" is the synthetic
// entry (see RootWorktreeID) standing in for the repo's own main checkout —
// no `git worktree add` is ever run for it, it just lets the repo's root
// folder be opened through the same worktree-detail UI (terminals, files,
// layout) as a real worktree.
const (
	WorktreeSourceCreated  = "created"
	WorktreeSourceImported = "imported"
	WorktreeSourceRoot     = "root"
)

// RootWorktreeID returns the synthetic worktree id used for a repo's own
// root checkout — see WorktreeSourceRoot.
func RootWorktreeID(repoID string) string {
	return "root-" + repoID
}

// Repo is a registered git repository.
type Repo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
	// BaseBranch is the branch new worktrees are created from. Empty means
	// "auto-detect" — see migrateAddRepoBaseBranch's doc comment.
	BaseBranch string `json:"base_branch"`
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
	Source    string `json:"source"`
	// ArchivedAt is when this worktree was last archived (RFC3339), or ""
	// if it's not currently archived. Set by SetWorktreeStatus and read by
	// ListArchivedWorktreesOlderThan to find worktrees due for the
	// retention sweep (see api.Server.SweepExpiredArchivedWorktrees).
	ArchivedAt string `json:"archived_at"`
	// SourceBranch is the branch this worktree's own Branch was created
	// from — e.g. "main" or "origin/main" if the new-worktree dialog's
	// branch dropdown was used to pick a specific (possibly
	// remote-tracking) start point, otherwise whatever repo.BaseBranch/
	// DetectDefaultBranch resolved to at creation time. "" for worktrees
	// where that's not meaningful (imported, or the synthetic root
	// worktree — see store.WorktreeSourceRoot).
	SourceBranch string `json:"source_branch"`
}

// AddRepo inserts a new repo row.
func (s *Store) AddRepo(r Repo) error {
	_, err := s.db.Exec(`INSERT INTO repos (id, name, path) VALUES (?, ?, ?)`, r.ID, r.Name, r.Path)
	return err
}

// ListRepos returns all registered repos.
func (s *Store) ListRepos() ([]Repo, error) {
	rows, err := s.db.Query(`SELECT id, name, path, base_branch FROM repos ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Repo
	for rows.Next() {
		var r Repo
		if err := rows.Scan(&r.ID, &r.Name, &r.Path, &r.BaseBranch); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetRepo fetches a single repo by id. Returns sql.ErrNoRows if not found.
func (s *Store) GetRepo(id string) (Repo, error) {
	var r Repo
	err := s.db.QueryRow(`SELECT id, name, path, base_branch FROM repos WHERE id = ?`, id).
		Scan(&r.ID, &r.Name, &r.Path, &r.BaseBranch)
	return r, err
}

// UpdateRepoBaseBranch sets the branch new worktrees are created from for
// this repo. Pass "" to revert to auto-detection.
func (s *Store) UpdateRepoBaseBranch(id, baseBranch string) error {
	_, err := s.db.Exec(`UPDATE repos SET base_branch = ? WHERE id = ?`, baseBranch, id)
	return err
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
	if w.Source == "" {
		w.Source = WorktreeSourceCreated
	}
	_, err := s.db.Exec(
		`INSERT INTO worktrees (id, repo_id, name, branch, path, created_at, status, source, source_branch) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.RepoID, w.Name, w.Branch, w.Path, w.CreatedAt, w.Status, w.Source, w.SourceBranch,
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
	query := `SELECT id, repo_id, name, branch, path, created_at, status, source, archived_at, source_branch FROM worktrees WHERE repo_id = ?`
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
		if err := rows.Scan(&w.ID, &w.RepoID, &w.Name, &w.Branch, &w.Path, &w.CreatedAt, &w.Status, &w.Source, &w.ArchivedAt, &w.SourceBranch); err != nil {
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

// FindWorktreeByPath returns the worktree whose path exactly equals the
// given path, or whose path is a parent directory of it (cwd inside a
// subdirectory of the worktree still counts). Used to resolve a claude
// hook's reported cwd back to a worktree — there's no other signal a hook
// payload carries that identifies "which worktree is this." Returns
// sql.ErrNoRows if no worktree matches (a completely normal, expected
// outcome: most claude sessions on this machine have nothing to do with
// worktree-studio).
func (s *Store) FindWorktreeByPath(cwd string) (Worktree, error) {
	rows, err := s.db.Query(`SELECT id, repo_id, name, branch, path, created_at, status, source, archived_at, source_branch FROM worktrees`)
	if err != nil {
		return Worktree{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var w Worktree
		if err := rows.Scan(&w.ID, &w.RepoID, &w.Name, &w.Branch, &w.Path, &w.CreatedAt, &w.Status, &w.Source, &w.ArchivedAt, &w.SourceBranch); err != nil {
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
		`SELECT id, repo_id, name, branch, path, created_at, status, source, archived_at, source_branch FROM worktrees WHERE id = ?`, id,
	).Scan(&w.ID, &w.RepoID, &w.Name, &w.Branch, &w.Path, &w.CreatedAt, &w.Status, &w.Source, &w.ArchivedAt, &w.SourceBranch)
	return w, err
}

// SetWorktreeStatus updates a worktree's status (active/archived/deleted).
// Used by archive/unarchive (no git changes at all — purely a visibility
// flag) and by the delete flow (status="deleted", after the git worktree
// and branch have actually been removed from disk). Also stamps/clears
// archived_at: set to now when transitioning to WorktreeStatusArchived,
// cleared otherwise — that's what ListArchivedWorktreesOlderThan reads to
// find worktrees due for the retention sweep, and what an unarchive (or a
// re-archive later) should reset rather than leave stale.
func (s *Store) SetWorktreeStatus(id, status string) error {
	archivedAt := ""
	if status == WorktreeStatusArchived {
		archivedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.db.Exec(`UPDATE worktrees SET status = ?, archived_at = ? WHERE id = ?`, status, archivedAt, id)
	return err
}

// SetWorktreeArchivedAt overwrites a worktree's archived_at directly,
// bypassing the normal "stamp to now on archive" behavior in
// SetWorktreeStatus. Not used by any handler — its one real caller today
// is the retention-sweep test, which needs an archived worktree that's
// provably past ArchivedWorktreeRetention without actually waiting 60
// days for it.
func (s *Store) SetWorktreeArchivedAt(id, archivedAt string) error {
	_, err := s.db.Exec(`UPDATE worktrees SET archived_at = ? WHERE id = ?`, archivedAt, id)
	return err
}

// ListArchivedWorktreesOlderThan returns every worktree, across every
// repo, that's been archived (status = WorktreeStatusArchived) since
// before cutoff (an RFC3339 timestamp) — the retention sweep's input (see
// api.Server.SweepExpiredArchivedWorktrees). Not scoped to one repo:
// unlike ListWorktrees, this is a maintenance query over the whole DB.
func (s *Store) ListArchivedWorktreesOlderThan(cutoff string) ([]Worktree, error) {
	rows, err := s.db.Query(
		`SELECT id, repo_id, name, branch, path, created_at, status, source, archived_at, source_branch FROM worktrees
		 WHERE status = ? AND archived_at != '' AND archived_at < ?`,
		WorktreeStatusArchived, cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Worktree
	for rows.Next() {
		var w Worktree
		if err := rows.Scan(&w.ID, &w.RepoID, &w.Name, &w.Branch, &w.Path, &w.CreatedAt, &w.Status, &w.Source, &w.ArchivedAt, &w.SourceBranch); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
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
	CreatedAt       string `json:"created_at"`
}

// AddTerminalSession inserts a new terminal session row, stamping CreatedAt
// if empty.
func (s *Store) AddTerminalSession(t TerminalSession) error {
	if t.CreatedAt == "" {
		t.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.db.Exec(
		`INSERT INTO terminal_sessions (id, worktree_id, tmux_session_name, tab_label, created_at) VALUES (?, ?, ?, ?, ?)`,
		t.ID, t.WorktreeID, t.TmuxSessionName, t.TabLabel, t.CreatedAt,
	)
	return err
}

// ListTerminalSessions returns all terminal sessions for a given worktree.
func (s *Store) ListTerminalSessions(worktreeID string) ([]TerminalSession, error) {
	rows, err := s.db.Query(
		`SELECT id, worktree_id, tmux_session_name, tab_label, created_at FROM terminal_sessions WHERE worktree_id = ?`,
		worktreeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TerminalSession
	for rows.Next() {
		var t TerminalSession
		if err := rows.Scan(&t.ID, &t.WorktreeID, &t.TmuxSessionName, &t.TabLabel, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListAllTerminalSessions returns every terminal session row, used at
// server startup to reconcile against tmux's actual live sessions.
func (s *Store) ListAllTerminalSessions() ([]TerminalSession, error) {
	rows, err := s.db.Query(`SELECT id, worktree_id, tmux_session_name, tab_label, created_at FROM terminal_sessions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TerminalSession
	for rows.Next() {
		var t TerminalSession
		if err := rows.Scan(&t.ID, &t.WorktreeID, &t.TmuxSessionName, &t.TabLabel, &t.CreatedAt); err != nil {
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
		`SELECT id, worktree_id, tmux_session_name, tab_label, created_at FROM terminal_sessions WHERE id = ?`, id,
	).Scan(&t.ID, &t.WorktreeID, &t.TmuxSessionName, &t.TabLabel, &t.CreatedAt)
	return t, err
}

// TerminalSessionWithWorktree is a TerminalSession joined with its parent
// worktree's branch/name — used by the "open shells" settings tab, a
// cross-worktree view where a bare worktree_id isn't useful to a person.
type TerminalSessionWithWorktree struct {
	TerminalSession
	WorktreeBranch string `json:"worktree_branch"`
	WorktreeName   string `json:"worktree_name"`
}

// ListTerminalSessionsForRepo returns every terminal session belonging to
// any worktree under repoID, newest first, joined with each worktree's
// branch/name for display.
func (s *Store) ListTerminalSessionsForRepo(repoID string) ([]TerminalSessionWithWorktree, error) {
	rows, err := s.db.Query(`
		SELECT t.id, t.worktree_id, t.tmux_session_name, t.tab_label, t.created_at, w.branch, w.name
		FROM terminal_sessions t JOIN worktrees w ON t.worktree_id = w.id
		WHERE w.repo_id = ?
		ORDER BY t.created_at DESC`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TerminalSessionWithWorktree
	for rows.Next() {
		var t TerminalSessionWithWorktree
		if err := rows.Scan(&t.ID, &t.WorktreeID, &t.TmuxSessionName, &t.TabLabel, &t.CreatedAt, &t.WorktreeBranch, &t.WorktreeName); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
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
