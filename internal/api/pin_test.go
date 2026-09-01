package api

import (
	"net/http"
	"testing"

	"worktree-studio/internal/store"
)

// TestPinWorktreeBlocksArchive is the core lifecycle-rule test: pinning a
// worktree, then trying to archive it, must be refused (409) rather than
// silently archiving it anyway — "will never be archived" is the whole
// point of pinning. Unpinning removes the block.
func TestPinWorktreeBlocksArchive(t *testing.T) {
	requireGit(t)
	ts, srv := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/pin", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST pin: status = %d, want 200", resp.StatusCode)
	}
	var body map[string]bool
	decodeInto(t, resp, &body)
	if !body["pinned"] {
		t.Fatalf("pin response = %+v, want pinned=true", body)
	}

	got, err := srv.Store.GetWorktree(wt.ID)
	if err != nil {
		t.Fatalf("GetWorktree after pin: %v", err)
	}
	if !got.Pinned {
		t.Fatal("Pinned should be true after pinning")
	}

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/archive", nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("POST archive on a pinned worktree: status = %d, want 409", resp.StatusCode)
	}

	got, err = srv.Store.GetWorktree(wt.ID)
	if err != nil {
		t.Fatalf("GetWorktree after refused archive: %v", err)
	}
	if got.Status != store.WorktreeStatusActive {
		t.Fatalf("Status after refused archive = %q, want %q (unchanged)", got.Status, store.WorktreeStatusActive)
	}

	// Unpin lifts the block.
	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/unpin", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST unpin: status = %d, want 200", resp.StatusCode)
	}
	decodeInto(t, resp, &body)
	if body["pinned"] {
		t.Fatalf("unpin response = %+v, want pinned=false", body)
	}

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/archive", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST archive after unpin: status = %d, want 200", resp.StatusCode)
	}
}

// TestPinWorktreeAuditLogged confirms both transitions are recorded, same
// expectation as archive/unarchive.
func TestPinWorktreeAuditLogged(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/pin", nil)
	resp.Body.Close()
	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/unpin", nil)
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/audit-log", nil)
	var entries []map[string]any
	decodeInto(t, resp, &entries)
	var sawPin, sawUnpin bool
	for _, e := range entries {
		switch e["event"] {
		case "worktree.pin":
			sawPin = true
		case "worktree.unpin":
			sawUnpin = true
		}
	}
	if !sawPin || !sawUnpin {
		t.Fatalf("expected both worktree.pin and worktree.unpin in the audit log, got %+v", entries)
	}
}

// TestListWorktreesPinnedFirst verifies the display-order rule directly
// against the list endpoint: a pinned worktree created BEFORE an unpinned
// one (so plain newest-first would put it second) must still come first.
func TestListWorktreesPinnedFirst(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "older"})
	var older store.Worktree
	decodeInto(t, resp, &older)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "newer"})
	var newer store.Worktree
	decodeInto(t, resp, &newer)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+older.ID+"/pin", nil)
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", nil)
	var listed []store.Worktree
	decodeInto(t, resp, &listed)
	if len(listed) != 2 || listed[0].ID != older.ID || listed[1].ID != newer.ID {
		t.Fatalf("list order = %+v, want pinned %q first despite being older, then %q", listed, older.ID, newer.ID)
	}
	if !listed[0].Pinned {
		t.Fatalf("listed[0].Pinned = false, want true")
	}
}

func TestPinWorktreeNotFound(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/does-not-exist/pin", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("POST pin for missing worktree: status = %d, want 404", resp.StatusCode)
	}
}
