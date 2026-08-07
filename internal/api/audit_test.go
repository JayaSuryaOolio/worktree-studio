package api

import (
	"net/http"
	"testing"

	"worktree-studio/internal/store"
)

func TestWorktreeAuditLogFiltersByWorktreeAndOrdersNewestFirst(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "one"})
	var wt1 store.Worktree
	decodeInto(t, resp, &wt1)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "two"})
	var wt2 store.Worktree
	decodeInto(t, resp, &wt2)

	// Creating each worktree already logged one worktree.create event for
	// it (repo.add also landed a line, but with no worktree_id — must be
	// excluded). Delete wt1 to add a second, later event for the same
	// worktree, proving the filter keeps both of its own events but none
	// of wt2's.
	resp = doJSON(t, http.MethodDelete, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt1.ID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE worktree: status = %d, want 200", resp.StatusCode)
	}

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt1.ID+"/audit-log", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET audit-log: status = %d, want 200", resp.StatusCode)
	}
	var entries []map[string]any
	decodeInto(t, resp, &entries)

	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2 (create + remove for wt1 only), got %+v", len(entries), entries)
	}
	if entries[0]["event"] != "worktree.remove" {
		t.Errorf("entries[0][event] = %v, want worktree.remove (newest first)", entries[0]["event"])
	}
	if entries[1]["event"] != "worktree.create" {
		t.Errorf("entries[1][event] = %v, want worktree.create", entries[1]["event"])
	}
	for _, e := range entries {
		if e["worktree_id"] != wt1.ID {
			t.Errorf("entry %+v has worktree_id %v, want %q (leaked another worktree's event)", e, e["worktree_id"], wt1.ID)
		}
	}
}

func TestWorktreeAuditLogUnknownWorktreeReturnsEmptyNotNotFound(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	// A worktree id with no matching log entries (never existed, or existed
	// and was deleted) is not a 404 — the endpoint doesn't require a live
	// worktree row, only a real repo, so history survives deletion.
	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/does-not-exist/audit-log", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET audit-log for unknown worktree: status = %d, want 200", resp.StatusCode)
	}
	var entries []map[string]any
	decodeInto(t, resp, &entries)
	if len(entries) != 0 {
		t.Fatalf("expected no entries for an unknown worktree id, got %+v", entries)
	}
}

func TestWorktreeAuditLogRepoNotFound(t *testing.T) {
	ts, _ := newTestServer(t)

	resp := doJSON(t, http.MethodGet, ts.URL+"/api/repos/does-not-exist/worktrees/does-not-exist/audit-log", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET audit-log for missing repo: status = %d, want 404", resp.StatusCode)
	}
}
