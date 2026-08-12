package api

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

// TestWorktreeSummaryChangedFilesIsNeverNullInJSON is a regression test
// for a real crash: a Go nil []string marshals to JSON `null`, not `[]`,
// and the frontend (WorktreeHoverPopover.tsx) called
// `summary.changed_files.length` on it unguarded — since a clean worktree
// (the common case) is exactly when ChangedFiles returns an empty slice,
// this crashed the whole sidebar on every hover over a clean worktree.
// Asserting on the raw response body, not the decoded struct: decoding
// JSON `null` into a Go []string field produces the same nil value decoding
// `[]` would, so only inspecting the actual bytes on the wire catches this.
func TestWorktreeSummaryChangedFilesIsNeverNullInJSON(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "adelaide", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	// A freshly created worktree, untouched — the clean case.
	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "clean-one"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/summary", nil)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if strings.Contains(string(body), `"changed_files":null`) {
		t.Fatalf("response body contains \"changed_files\":null, want \"changed_files\":[] — got: %s", body)
	}
	if !strings.Contains(string(body), `"changed_files":[]`) {
		t.Fatalf("expected \"changed_files\":[] in the response for a clean worktree, got: %s", body)
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
