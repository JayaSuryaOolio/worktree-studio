package api

import (
	"net/http"
	"os/exec"
	"path/filepath"
	"testing"

	"worktree-studio/internal/store"
)

// addWorktreeUnder simulates another tool (e.g. Conductor) creating a git
// worktree for repoPath under its own workspace root — a real `git worktree
// add` at a path inside root, no worktree-studio API call involved.
func addWorktreeUnder(t *testing.T, repoPath, root, dirName, branch string) string {
	t.Helper()
	path := filepath.Join(root, dirName)
	cmd := exec.Command("git", "-C", repoPath, "worktree", "add", "-b", branch, path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}
	return path
}

func TestUpdateRepoSettingsSetsBothFieldsTogether(t *testing.T) {
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	root := t.TempDir()
	resp = doJSON(t, http.MethodPut, ts.URL+"/api/repos/"+repo.ID+"/settings", map[string]string{
		"base_branch":             "develop",
		"external_worktrees_root": root,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update settings: status = %d, want 200", resp.StatusCode)
	}
	var updated store.Repo
	decodeInto(t, resp, &updated)
	if updated.BaseBranch != "develop" {
		t.Errorf("BaseBranch = %q, want %q", updated.BaseBranch, "develop")
	}
	if updated.ExternalWorktreesRoot != root {
		t.Errorf("ExternalWorktreesRoot = %q, want %q", updated.ExternalWorktreesRoot, root)
	}

	// A second save that only means to change one field must still send the
	// other's current value — the request struct has no way to distinguish
	// "not provided" from "clear this" (see handleUpdateRepoSettings's doc
	// comment) — confirming here that a save carrying both current values
	// really does leave both intact, not just the one under test.
	resp = doJSON(t, http.MethodPut, ts.URL+"/api/repos/"+repo.ID+"/settings", map[string]string{
		"base_branch":             "develop",
		"external_worktrees_root": "",
	})
	decodeInto(t, resp, &updated)
	if updated.BaseBranch != "develop" {
		t.Errorf("BaseBranch after clearing external root = %q, want unchanged %q", updated.BaseBranch, "develop")
	}
	if updated.ExternalWorktreesRoot != "" {
		t.Errorf("ExternalWorktreesRoot = %q, want cleared", updated.ExternalWorktreesRoot)
	}
}

func TestDiscoverExternalWorktreesIsNoopWithoutConfiguredRoot(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	addWorktreeUnder(t, repoPath, t.TempDir(), "amber", "amber")

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/discover", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("discover: status = %d, want 200", resp.StatusCode)
	}
	var onboarded []store.Worktree
	decodeInto(t, resp, &onboarded)
	if len(onboarded) != 0 {
		t.Errorf("onboarded = %+v, want none (no external_worktrees_root configured)", onboarded)
	}
}

func TestDiscoverExternalWorktreesOnboardsOnlyOnesUnderConfiguredRoot(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	otherToolRoot := t.TempDir()
	underRoot := addWorktreeUnder(t, repoPath, otherToolRoot, "amber-ridge", "amber-ridge")
	elsewhere := addWorktreeUnder(t, repoPath, t.TempDir(), "unrelated", "unrelated")

	resp = doJSON(t, http.MethodPut, ts.URL+"/api/repos/"+repo.ID+"/settings", map[string]string{
		"base_branch":             "",
		"external_worktrees_root": otherToolRoot,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("configure external root: status = %d, want 200", resp.StatusCode)
	}

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/discover", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("discover: status = %d, want 200", resp.StatusCode)
	}
	var onboarded []store.Worktree
	decodeInto(t, resp, &onboarded)
	if len(onboarded) != 1 {
		t.Fatalf("onboarded = %+v, want exactly 1", onboarded)
	}
	wt := onboarded[0]
	if wt.Branch != "amber-ridge" {
		t.Errorf("Branch = %q, want %q", wt.Branch, "amber-ridge")
	}
	if wt.Source != store.WorktreeSourceImported {
		t.Errorf("Source = %q, want %q", wt.Source, store.WorktreeSourceImported)
	}
	if !resolveBestEffortEqual(wt.Path, underRoot) {
		t.Errorf("Path = %q, want (equivalent to) %q", wt.Path, underRoot)
	}

	// The one outside the configured root must still show up as a manual
	// "attach" candidate rather than having been silently onboarded too.
	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/external", nil)
	var external []externalWorktreeEntry
	decodeInto(t, resp, &external)
	foundElsewhere := false
	for _, e := range external {
		if resolveBestEffortEqual(e.Path, elsewhere) {
			foundElsewhere = true
		}
	}
	if !foundElsewhere {
		t.Errorf("worktree outside the configured root should remain a manual-attach candidate, got %+v", external)
	}

	// Re-running discover is a no-op the second time: the same worktree is
	// already registered, so it isn't duplicated or returned again.
	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/discover", nil)
	decodeInto(t, resp, &onboarded)
	if len(onboarded) != 0 {
		t.Errorf("second discover run onboarded = %+v, want none (already registered)", onboarded)
	}
}

func TestDiscoverExternalWorktreesSkipsDetachedHead(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	otherToolRoot := t.TempDir()
	detachedPath := filepath.Join(otherToolRoot, "detached")
	cmd := exec.Command("git", "-C", repoPath, "worktree", "add", "--detach", detachedPath, "HEAD")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add --detach: %v\n%s", err, out)
	}

	resp = doJSON(t, http.MethodPut, ts.URL+"/api/repos/"+repo.ID+"/settings", map[string]string{
		"base_branch":             "",
		"external_worktrees_root": otherToolRoot,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("configure external root: status = %d, want 200", resp.StatusCode)
	}

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/discover", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("discover: status = %d, want 200", resp.StatusCode)
	}
	var onboarded []store.Worktree
	decodeInto(t, resp, &onboarded)
	if len(onboarded) != 0 {
		t.Errorf("onboarded = %+v, want none (detached HEAD skipped)", onboarded)
	}
}
