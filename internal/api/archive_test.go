package api

import (
	"net/http"
	"os"
	"testing"

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
