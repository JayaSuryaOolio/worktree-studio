package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"worktree-studio/internal/store"
)

// TestWorktreeSummary verifies the combined git-status + changed-files
// response, and that a gh CLI failure (this throwaway test repo has no
// GitHub remote at all, so `gh pr view` can't succeed) degrades to PR:
// null rather than failing the whole request — the endpoint's whole
// reason for treating gh as best-effort.
func TestWorktreeSummary(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "adelaide", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	if err := os.WriteFile(filepath.Join(wt.Path, "scratch.txt"), []byte("wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/summary", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET worktree summary: status = %d, want 200", resp.StatusCode)
	}
	var body worktreeSummary
	decodeInto(t, resp, &body)

	if body.Branch != wt.Branch {
		t.Errorf("Branch = %q, want %q", body.Branch, wt.Branch)
	}
	if !body.Dirty {
		t.Error("Dirty = false, want true (an untracked file was added)")
	}
	if len(body.ChangedFiles) != 1 || body.ChangedFiles[0] != "scratch.txt" {
		t.Errorf("ChangedFiles = %v, want exactly [\"scratch.txt\"]", body.ChangedFiles)
	}
	if body.PR != nil {
		t.Errorf("PR = %+v, want nil (this test repo has no GitHub remote)", body.PR)
	}
}

func TestWorktreeSummaryNotFound(t *testing.T) {
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "adelaide", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/does-not-exist/summary", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET summary for missing worktree: status = %d, want 404", resp.StatusCode)
	}
}
