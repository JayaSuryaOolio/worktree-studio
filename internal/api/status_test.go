package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"worktree-studio/internal/store"
)

func TestWorktreeStatusCleanThenDirty(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/status", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET status: status = %d, want 200", resp.StatusCode)
	}
	var status map[string]any
	decodeInto(t, resp, &status)
	if status["dirty"] != false {
		t.Fatalf("expected a freshly created worktree to be clean, got %+v", status)
	}
	if status["has_upstream"] != false {
		t.Fatalf("expected a freshly created worktree's branch to have no upstream, got %+v", status)
	}
	if status["branch"] != wt.Branch {
		t.Fatalf("status branch = %v, want %q", status["branch"], wt.Branch)
	}

	if err := os.WriteFile(filepath.Join(wt.Path, "scratch.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/status", nil)
	decodeInto(t, resp, &status)
	if status["dirty"] != true {
		t.Fatalf("expected dirty after adding an untracked file, got %+v", status)
	}
}

func TestWorktreeStatusNotFound(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/does-not-exist/status", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET status for missing worktree: status = %d, want 404", resp.StatusCode)
	}
}
