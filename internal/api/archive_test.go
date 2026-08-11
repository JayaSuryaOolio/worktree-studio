package api

import (
	"net/http"
	"os"
	"testing"
	"time"

	"worktree-studio/internal/store"
)

func TestArchiveWorktreeHidesItFromListButKeepsItOnDisk(t *testing.T) {
	requireGit(t)
	ts, srv := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/archive", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST archive: status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Hidden from the normal list...
	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", nil)
	var listed []store.Worktree
	decodeInto(t, resp, &listed)
	if len(listed) != 0 {
		t.Fatalf("worktree list after archiving = %+v, want empty (archived worktrees are hidden)", listed)
	}

	// ...but the git worktree itself is untouched (archiving is not delete).
	got, err := srv.Store.GetWorktree(wt.ID)
	if err != nil {
		t.Fatalf("GetWorktree after archive: %v", err)
	}
	if got.Status != store.WorktreeStatusArchived {
		t.Fatalf("Status = %q, want %q", got.Status, store.WorktreeStatusArchived)
	}
	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatalf("archived worktree's directory should still exist on disk: %v", err)
	}

	// Unarchiving brings it back.
	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/unarchive", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST unarchive: status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", nil)
	decodeInto(t, resp, &listed)
	if len(listed) != 1 {
		t.Fatalf("worktree list after unarchiving = %+v, want the worktree back", listed)
	}

	// Both transitions are audit-logged.
	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/audit-log", nil)
	var entries []map[string]any
	decodeInto(t, resp, &entries)
	var sawArchive, sawUnarchive bool
	for _, e := range entries {
		switch e["event"] {
		case "worktree.archive":
			sawArchive = true
		case "worktree.unarchive":
			sawUnarchive = true
		}
	}
	if !sawArchive || !sawUnarchive {
		t.Fatalf("expected both worktree.archive and worktree.unarchive in the audit log, got %+v", entries)
	}
}

// TestArchiveStampsAndClearsArchivedAt verifies SetWorktreeStatus's
// archived_at bookkeeping: set on archive, cleared on unarchive — the
// signal SweepExpiredArchivedWorktrees relies on to find worktrees due for
// hard removal.
func TestArchiveStampsAndClearsArchivedAt(t *testing.T) {
	requireGit(t)
	ts, srv := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/archive", nil)
	resp.Body.Close()

	got, err := srv.Store.GetWorktree(wt.ID)
	if err != nil {
		t.Fatalf("GetWorktree after archive: %v", err)
	}
	if got.ArchivedAt == "" {
		t.Fatal("archived_at should be set after archiving")
	}
	if _, err := time.Parse(time.RFC3339, got.ArchivedAt); err != nil {
		t.Fatalf("archived_at %q is not a valid RFC3339 timestamp: %v", got.ArchivedAt, err)
	}

	// Also surfaced through the archived-worktrees listing.
	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/archived", nil)
	var archived []store.Worktree
	decodeInto(t, resp, &archived)
	if len(archived) != 1 || archived[0].ID != wt.ID {
		t.Fatalf("GET .../worktrees/archived = %+v, want just %q", archived, wt.ID)
	}

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/unarchive", nil)
	resp.Body.Close()

	got, err = srv.Store.GetWorktree(wt.ID)
	if err != nil {
		t.Fatalf("GetWorktree after unarchive: %v", err)
	}
	if got.ArchivedAt != "" {
		t.Errorf("archived_at should be cleared after unarchiving, got %q", got.ArchivedAt)
	}

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/archived", nil)
	decodeInto(t, resp, &archived)
	if len(archived) != 0 {
		t.Fatalf("GET .../worktrees/archived after unarchive = %+v, want empty", archived)
	}
}

// TestSweepExpiredArchivedWorktrees verifies the 60-day retention cleanup:
// a worktree archived long enough ago is hard-removed (git worktree gone
// from disk, DB row gone entirely — no soft-delete), while one archived
// more recently is left alone.
func TestSweepExpiredArchivedWorktrees(t *testing.T) {
	requireGit(t)
	ts, srv := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "old-one"})
	var oldWt store.Worktree
	decodeInto(t, resp, &oldWt)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "recent-one"})
	var recentWt store.Worktree
	decodeInto(t, resp, &recentWt)

	for _, id := range []string{oldWt.ID, recentWt.ID} {
		resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+id+"/archive", nil)
		resp.Body.Close()
	}

	// Backdate oldWt's archived_at past the retention window directly in
	// the DB — no need to wait 60 real days to exercise the sweep.
	longAgo := time.Now().UTC().Add(-ArchivedWorktreeRetention - time.Hour).Format(time.RFC3339)
	if err := srv.Store.SetWorktreeArchivedAt(oldWt.ID, longAgo); err != nil {
		t.Fatalf("backdate archived_at: %v", err)
	}

	srv.SweepExpiredArchivedWorktrees()

	if _, err := srv.Store.GetWorktree(oldWt.ID); err == nil {
		t.Error("expired archived worktree should be gone from the DB after the sweep")
	}
	if _, err := os.Stat(oldWt.Path); !os.IsNotExist(err) {
		t.Errorf("expired archived worktree's directory should be gone from disk, err=%v", err)
	}

	if _, err := srv.Store.GetWorktree(recentWt.ID); err != nil {
		t.Errorf("recently archived worktree should survive the sweep: %v", err)
	}
	if _, err := os.Stat(recentWt.Path); err != nil {
		t.Errorf("recently archived worktree's directory should still exist: %v", err)
	}
}

func TestArchiveWorktreeNotFound(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/does-not-exist/archive", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("POST archive for missing worktree: status = %d, want 404", resp.StatusCode)
	}
}
