package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRepoCRUD(t *testing.T) {
	s := newTestStore(t)

	repos, err := s.ListRepos()
	if err != nil {
		t.Fatalf("ListRepos (empty): %v", err)
	}
	if len(repos) != 0 {
		t.Fatalf("ListRepos (empty) = %+v, want empty", repos)
	}

	exists, err := s.RepoPathExists("/tmp/does-not-exist")
	if err != nil {
		t.Fatalf("RepoPathExists: %v", err)
	}
	if exists {
		t.Fatalf("RepoPathExists on unregistered path = true, want false")
	}

	r := Repo{ID: "repo1", Name: "adelaide", Path: "/tmp/adelaide"}
	if err := s.AddRepo(r); err != nil {
		t.Fatalf("AddRepo: %v", err)
	}

	got, err := s.GetRepo("repo1")
	if err != nil {
		t.Fatalf("GetRepo: %v", err)
	}
	if got != r {
		t.Errorf("GetRepo = %+v, want %+v", got, r)
	}

	exists, err = s.RepoPathExists("/tmp/adelaide")
	if err != nil {
		t.Fatalf("RepoPathExists: %v", err)
	}
	if !exists {
		t.Errorf("RepoPathExists on registered path = false, want true")
	}

	if _, err := s.GetRepo("does-not-exist"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetRepo(missing) error = %v, want sql.ErrNoRows", err)
	}

	repos, err = s.ListRepos()
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	if len(repos) != 1 || repos[0] != r {
		t.Errorf("ListRepos = %+v, want [%+v]", repos, r)
	}
}

func TestRepoPathIsUnique(t *testing.T) {
	s := newTestStore(t)
	r1 := Repo{ID: "repo1", Name: "a", Path: "/tmp/same"}
	r2 := Repo{ID: "repo2", Name: "b", Path: "/tmp/same"}

	if err := s.AddRepo(r1); err != nil {
		t.Fatalf("AddRepo(r1): %v", err)
	}
	if err := s.AddRepo(r2); err == nil {
		t.Fatalf("AddRepo(r2) with duplicate path succeeded, want a UNIQUE constraint error")
	}
}

func TestWorktreeCRUD(t *testing.T) {
	s := newTestStore(t)
	repo := Repo{ID: "repo1", Name: "adelaide", Path: "/tmp/adelaide"}
	if err := s.AddRepo(repo); err != nil {
		t.Fatalf("AddRepo: %v", err)
	}

	wts, err := s.ListWorktrees(repo.ID)
	if err != nil {
		t.Fatalf("ListWorktrees (empty): %v", err)
	}
	if len(wts) != 0 {
		t.Fatalf("ListWorktrees (empty) = %+v, want empty", wts)
	}

	w := Worktree{
		ID:        "wt1",
		RepoID:    repo.ID,
		Name:      "my-feature",
		Branch:    "my-feature",
		Path:      "/tmp/wt1",
		CreatedAt: "2026-08-06T12:00:00Z",
		Status:    WorktreeStatusActive,
		Source:    WorktreeSourceCreated,
	}
	if err := s.AddWorktree(w); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	got, err := s.GetWorktree(w.ID)
	if err != nil {
		t.Fatalf("GetWorktree: %v", err)
	}
	if got != w {
		t.Errorf("GetWorktree = %+v, want %+v", got, w)
	}

	// AddWorktree should stamp CreatedAt itself when the caller leaves it
	// empty, rather than persisting an empty string (this was a real bug
	// fixed in the initial implementation — regression-guard it).
	w2 := Worktree{ID: "wt2", RepoID: repo.ID, Name: "n2", Branch: "n2", Path: "/tmp/wt2"}
	if err := s.AddWorktree(w2); err != nil {
		t.Fatalf("AddWorktree (no CreatedAt): %v", err)
	}
	got2, err := s.GetWorktree("wt2")
	if err != nil {
		t.Fatalf("GetWorktree(wt2): %v", err)
	}
	if got2.CreatedAt == "" {
		t.Errorf("GetWorktree(wt2).CreatedAt is empty, want a stamped timestamp")
	}

	wts, err = s.ListWorktrees(repo.ID)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if len(wts) != 2 {
		t.Fatalf("ListWorktrees = %+v, want 2 entries", wts)
	}

	if err := s.RemoveWorktree(w.ID); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	if _, err := s.GetWorktree(w.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetWorktree(removed) error = %v, want sql.ErrNoRows", err)
	}

	wts, err = s.ListWorktrees(repo.ID)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if len(wts) != 1 {
		t.Fatalf("ListWorktrees after remove = %+v, want 1 entry", wts)
	}
}

func TestWorktreeStatusDefaultsAndFiltering(t *testing.T) {
	s := newTestStore(t)
	repo := Repo{ID: "r1", Name: "test", Path: "/tmp/r1"}
	if err := s.AddRepo(repo); err != nil {
		t.Fatalf("AddRepo: %v", err)
	}

	wt := Worktree{ID: "wt1", RepoID: repo.ID, Name: "feature", Branch: "feature", Path: "/tmp/wt1"}
	if err := s.AddWorktree(wt); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	got, err := s.GetWorktree(wt.ID)
	if err != nil {
		t.Fatalf("GetWorktree: %v", err)
	}
	if got.Status != WorktreeStatusActive {
		t.Fatalf("newly created worktree Status = %q, want %q (should default without the caller setting it)", got.Status, WorktreeStatusActive)
	}

	if err := s.SetWorktreeStatus(wt.ID, WorktreeStatusArchived); err != nil {
		t.Fatalf("SetWorktreeStatus: %v", err)
	}
	got, err = s.GetWorktree(wt.ID)
	if err != nil {
		t.Fatalf("GetWorktree after archive: %v", err)
	}
	if got.Status != WorktreeStatusArchived {
		t.Fatalf("Status after SetWorktreeStatus = %q, want %q", got.Status, WorktreeStatusArchived)
	}

	activeOnly, err := s.ListWorktrees(repo.ID, WorktreeStatusActive)
	if err != nil {
		t.Fatalf("ListWorktrees(active): %v", err)
	}
	if len(activeOnly) != 0 {
		t.Fatalf("ListWorktrees(active) after archiving the only worktree = %+v, want empty", activeOnly)
	}

	all, err := s.ListWorktrees(repo.ID)
	if err != nil {
		t.Fatalf("ListWorktrees(no filter): %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("ListWorktrees(no filter) = %+v, want the archived worktree still included", all)
	}
}

// TestWorktreePinnedDefaultsAndOrdering covers both halves of pinning at
// the store layer: it defaults to false for a freshly created worktree
// (so every pre-existing row, and every ordinary new one, behaves exactly
// as before this feature existed), SetWorktreePinned flips it, and
// ListWorktrees' ORDER BY puts a pinned worktree ahead of an unpinned one
// even when the unpinned one is newer — the actual display-order rule,
// not just that the flag round-trips.
func TestWorktreePinnedDefaultsAndOrdering(t *testing.T) {
	s := newTestStore(t)
	repo := Repo{ID: "r1", Name: "test", Path: "/tmp/r1"}
	if err := s.AddRepo(repo); err != nil {
		t.Fatalf("AddRepo: %v", err)
	}

	older := Worktree{ID: "older", RepoID: repo.ID, Name: "older", Branch: "older", Path: "/tmp/older", CreatedAt: "2026-08-01T00:00:00Z"}
	if err := s.AddWorktree(older); err != nil {
		t.Fatalf("AddWorktree(older): %v", err)
	}
	newer := Worktree{ID: "newer", RepoID: repo.ID, Name: "newer", Branch: "newer", Path: "/tmp/newer", CreatedAt: "2026-08-02T00:00:00Z"}
	if err := s.AddWorktree(newer); err != nil {
		t.Fatalf("AddWorktree(newer): %v", err)
	}

	got, err := s.GetWorktree(older.ID)
	if err != nil {
		t.Fatalf("GetWorktree: %v", err)
	}
	if got.Pinned {
		t.Fatal("a freshly created worktree should default to Pinned=false")
	}

	// Plain newest-first would put newer ahead of older.
	list, err := s.ListWorktrees(repo.ID)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if list[0].ID != newer.ID {
		t.Fatalf("ListWorktrees (neither pinned) = %+v, want newer first (newest-first)", list)
	}

	if err := s.SetWorktreePinned(older.ID, true); err != nil {
		t.Fatalf("SetWorktreePinned: %v", err)
	}
	got, err = s.GetWorktree(older.ID)
	if err != nil {
		t.Fatalf("GetWorktree after pin: %v", err)
	}
	if !got.Pinned {
		t.Fatal("Pinned should be true after SetWorktreePinned(id, true)")
	}

	list, err = s.ListWorktrees(repo.ID)
	if err != nil {
		t.Fatalf("ListWorktrees after pin: %v", err)
	}
	if list[0].ID != older.ID || list[1].ID != newer.ID {
		t.Fatalf("ListWorktrees after pinning the older one = %+v, want [older, newer] despite older being created first", list)
	}

	if err := s.SetWorktreePinned(older.ID, false); err != nil {
		t.Fatalf("SetWorktreePinned(false): %v", err)
	}
	got, err = s.GetWorktree(older.ID)
	if err != nil {
		t.Fatalf("GetWorktree after unpin: %v", err)
	}
	if got.Pinned {
		t.Fatal("Pinned should be false after SetWorktreePinned(id, false)")
	}
}

func TestWorktreeLayoutSaveLoad(t *testing.T) {
	s := newTestStore(t)
	repo := Repo{ID: "r1", Name: "test", Path: "/tmp/r1"}
	if err := s.AddRepo(repo); err != nil {
		t.Fatalf("AddRepo: %v", err)
	}
	wt := Worktree{ID: "wt1", RepoID: repo.ID, Name: "feature", Branch: "feature", Path: "/tmp/wt1"}
	if err := s.AddWorktree(wt); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	if _, err := s.GetWorktreeLayout(wt.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetWorktreeLayout before any save: err = %v, want sql.ErrNoRows", err)
	}

	if err := s.SaveWorktreeLayout(wt.ID, `{"grid":"v1"}`); err != nil {
		t.Fatalf("SaveWorktreeLayout: %v", err)
	}
	got, err := s.GetWorktreeLayout(wt.ID)
	if err != nil {
		t.Fatalf("GetWorktreeLayout: %v", err)
	}
	if got != `{"grid":"v1"}` {
		t.Errorf("GetWorktreeLayout = %q, want %q", got, `{"grid":"v1"}`)
	}

	// Saving again for the same worktree must upsert, not insert a
	// second conflicting row (worktree_id is the primary key).
	if err := s.SaveWorktreeLayout(wt.ID, `{"grid":"v2"}`); err != nil {
		t.Fatalf("SaveWorktreeLayout (update): %v", err)
	}
	got, err = s.GetWorktreeLayout(wt.ID)
	if err != nil {
		t.Fatalf("GetWorktreeLayout after update: %v", err)
	}
	if got != `{"grid":"v2"}` {
		t.Errorf("GetWorktreeLayout after update = %q, want %q", got, `{"grid":"v2"}`)
	}
}

// TestForeignKeysPragmaActuallyEnforced is a regression test for a real,
// silent gap found while adding worktree_layouts: SQLite does NOT enforce
// foreign keys (including every ON DELETE CASCADE in this schema) unless
// `PRAGMA foreign_keys = ON` is set on the connection — off by default.
// Verified empirically (a throwaway script) that a parent-row delete left
// a child row behind with the pragma unset, before Open() was fixed to
// set it. This test proves the fix holds for the real schema, not just a
// minimal repro: deleting a worktree must actually cascade-delete its
// terminal_sessions and worktree_layouts rows via SQL alone, with no
// application code doing the cleanup.
func TestForeignKeysPragmaActuallyEnforced(t *testing.T) {
	s := newTestStore(t)
	repo := Repo{ID: "r1", Name: "test", Path: "/tmp/r1"}
	if err := s.AddRepo(repo); err != nil {
		t.Fatalf("AddRepo: %v", err)
	}
	wt := Worktree{ID: "wt1", RepoID: repo.ID, Name: "feature", Branch: "feature", Path: "/tmp/wt1"}
	if err := s.AddWorktree(wt); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	if err := s.AddTerminalSession(TerminalSession{ID: "t1", WorktreeID: wt.ID, TmuxSessionName: "wts-t1", TabLabel: "shell"}); err != nil {
		t.Fatalf("AddTerminalSession: %v", err)
	}
	if err := s.SaveWorktreeLayout(wt.ID, `{"grid":"v1"}`); err != nil {
		t.Fatalf("SaveWorktreeLayout: %v", err)
	}

	// Delete the worktree row directly via SQL (bypassing RemoveWorktree,
	// which doesn't itself touch these tables) to isolate exactly what
	// the database's own cascade behavior does, independent of any
	// application-level cleanup code like internal/api's own explicit
	// terminal-closing logic.
	if _, err := s.db.Exec(`DELETE FROM worktrees WHERE id = ?`, wt.ID); err != nil {
		t.Fatalf("delete worktree row: %v", err)
	}

	if _, err := s.GetTerminalSession("t1"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("terminal_sessions row survived the worktree delete: err = %v, want sql.ErrNoRows (cascade should have removed it)", err)
	}
	if _, err := s.GetWorktreeLayout(wt.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("worktree_layouts row survived the worktree delete: err = %v, want sql.ErrNoRows (cascade should have removed it)", err)
	}
}
