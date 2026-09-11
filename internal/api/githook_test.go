package api

import (
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"worktree-studio/internal/store"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// waitForAuditEvent polls the worktree's audit log for an entry of the
// given event type matching match, since the real installed hook script
// POSTs asynchronously from git's own subprocess rather than something
// this test can await directly.
func waitForAuditEvent(t *testing.T, baseURL, repoID, worktreeID, event string, match func(map[string]any) bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp := doJSON(t, http.MethodGet, baseURL+"/api/repos/"+repoID+"/worktrees/"+worktreeID+"/audit-log", nil)
		var entries []map[string]any
		decodeInto(t, resp, &entries)
		for _, e := range entries {
			if e["event"] == event && match(e) {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for a %q audit entry matching the expected condition", event)
}

func TestGitHookInstallStatusUninstall(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/git-hook/", nil)
	var status map[string]bool
	decodeInto(t, resp, &status)
	if status["installed"] {
		t.Fatal("expected not installed before Install")
	}

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/git-hook/install", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("install: status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/git-hook/", nil)
	decodeInto(t, resp, &status)
	if !status["installed"] {
		t.Fatal("expected installed after Install")
	}

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/git-hook/uninstall", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("uninstall: status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/git-hook/", nil)
	decodeInto(t, resp, &status)
	if status["installed"] {
		t.Fatal("expected not installed after Uninstall")
	}
}

func TestGitHookStatusUnknownRepo(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := doJSON(t, http.MethodGet, ts.URL+"/api/repos/nope/git-hook/", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func postForm(t *testing.T, rawURL string, values url.Values) *http.Response {
	t.Helper()
	resp, err := http.PostForm(rawURL, values)
	if err != nil {
		t.Fatalf("POST %s: %v", rawURL, err)
	}
	return resp
}

func TestGitHookPostCheckoutLogsBranchChangeForMatchingWorktree(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	resp = postForm(t, ts.URL+"/api/git-hook/post-checkout", url.Values{
		"cwd":       {wt.Path},
		"branch":    {"other-branch"},
		"prev_head": {"aaa"},
		"new_head":  {"bbb"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST post-checkout: status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/audit-log", nil)
	var entries []map[string]any
	decodeInto(t, resp, &entries)

	var found map[string]any
	for _, e := range entries {
		if e["event"] == "worktree.branch_change" {
			found = e
		}
	}
	if found == nil {
		t.Fatalf("expected a worktree.branch_change entry, got %+v", entries)
	}
	if found["branch"] != "other-branch" {
		t.Errorf("branch = %v, want other-branch", found["branch"])
	}
}

func TestGitHookPostCheckoutIgnoresUnrelatedCwd(t *testing.T) {
	ts, _ := newTestServer(t)

	resp := postForm(t, ts.URL+"/api/git-hook/post-checkout", url.Values{
		"cwd":    {"/nowhere/tracked"},
		"branch": {"main"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (silent no-op)", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestGitHookPostCheckoutIgnoresMissingFields(t *testing.T) {
	ts, _ := newTestServer(t)

	resp := postForm(t, ts.URL+"/api/git-hook/post-checkout", url.Values{"cwd": {"/tmp/x"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (silent no-op)", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestGitHookInstallActuallyFiresOnRealCheckout(t *testing.T) {
	requireGit(t)
	ts, srv := newTestServer(t)
	// newTestServer doesn't know its own httptest URL ahead of time, so
	// SelfBaseURL (what the generated hook script POSTs back to) needs
	// setting here — main.go sets the real equivalent from the actual
	// listen address.
	srv.SelfBaseURL = ts.URL
	// This test, unlike the others in this file, runs a *real* `git
	// checkout` subprocess and lets the installed hook resolve its own cwd
	// via `git rev-parse --show-toplevel` — which resolves symlinks. On
	// macOS, $TMPDIR (what t.TempDir() builds on) sits under /var, itself a
	// symlink to /private/var, so WorktreeRoot's own path needs the same
	// resolution up front or the DB's stored worktree path never matches
	// what the hook script reports as cwd. See claudehook/install_test.go's
	// withFakeClaudeHome for the identical, previously-solved issue.
	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(srv.WorktreeRoot))
	if err != nil {
		t.Fatal(err)
	}
	srv.WorktreeRoot = filepath.Join(resolvedParent, filepath.Base(srv.WorktreeRoot))
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/git-hook/install", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("install: status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	runGit(t, wt.Path, "checkout", "-b", "toggled-branch")

	waitForAuditEvent(t, ts.URL, repo.ID, wt.ID, "worktree.branch_change", func(e map[string]any) bool {
		branch, _ := e["branch"].(string)
		return strings.Contains(branch, "toggled-branch")
	})
}
