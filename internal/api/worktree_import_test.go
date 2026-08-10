package api

import (
	"net/http"
	"os/exec"
	"path/filepath"
	"testing"

	"worktree-studio/internal/store"
)

// addManualWorktree simulates a worktree created outside worktree-studio
// entirely (e.g. by hand, or by another tool) — a real `git worktree add`
// against repoPath, with no worktree-studio API call involved at all.
func addManualWorktree(t *testing.T, repoPath, dirName, branch string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), dirName)
	cmd := exec.Command("git", "-C", repoPath, "worktree", "add", "-b", branch, path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}
	return path
}

func TestImportWorktreeRegistersAManuallyCreatedOne(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	manualPath := addManualWorktree(t, repoPath, "manual-feature", "manual-feature")

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/import", map[string]string{
		"path": manualPath,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("import worktree: status = %d, want 201", resp.StatusCode)
	}
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	if wt.Branch != "manual-feature" {
		t.Errorf("Branch = %q, want %q", wt.Branch, "manual-feature")
	}
	if wt.Name != "ext_manual-feature" {
		t.Errorf("default Name = %q, want %q (ext_ prefixed)", wt.Name, "ext_manual-feature")
	}
	// Compared via resolveBestEffortEqual, not a raw string equality: the
	// stored path comes from git's own (symlink-canonicalized) `worktree
	// list` output, which can legitimately differ in string form from
	// manualPath (e.g. macOS's /var vs /private/var) while still being the
	// same directory — see resolveBestEffortEqual's doc comment.
	if !resolveBestEffortEqual(wt.Path, manualPath) {
		t.Errorf("Path = %q, want (a path equivalent to) %q", wt.Path, manualPath)
	}

	// Confirms it now shows up in the normal worktree list, same as one
	// created through the usual create flow.
	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", nil)
	var list []store.Worktree
	decodeInto(t, resp, &list)
	found := false
	for _, w := range list {
		if w.ID == wt.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("imported worktree %q not present in list: %+v", wt.ID, list)
	}
}

func TestImportWorktreeHonorsCustomName(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	manualPath := addManualWorktree(t, repoPath, "manual-feature", "manual-feature")

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/import", map[string]string{
		"path": manualPath,
		"name": "my-custom-name",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("import worktree: status = %d, want 201", resp.StatusCode)
	}
	var wt store.Worktree
	decodeInto(t, resp, &wt)
	if wt.Name != "my-custom-name" {
		t.Errorf("Name = %q, want %q", wt.Name, "my-custom-name")
	}
}

func TestImportWorktreeRejectsPathNotAGitWorktreeOfThisRepo(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/import", map[string]string{
		"path": t.TempDir(), // a real dir, but not a worktree of repoPath at all
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("import unrelated path: status = %d, want 400", resp.StatusCode)
	}
}

func TestImportWorktreeRejectsRelativePath(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/import", map[string]string{
		"path": "relative/path",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("import relative path: status = %d, want 400", resp.StatusCode)
	}
}

func TestImportWorktreeRejectsAlreadyRegisteredPath(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	manualPath := addManualWorktree(t, repoPath, "manual-feature", "manual-feature")

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/import", map[string]string{"path": manualPath})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("first import: status = %d, want 201", resp.StatusCode)
	}

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/import", map[string]string{"path": manualPath})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("re-import same path: status = %d, want 409", resp.StatusCode)
	}
}

func TestImportWorktreeRejectsDetachedHead(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	detachedPath := filepath.Join(t.TempDir(), "detached")
	cmd := exec.Command("git", "-C", repoPath, "worktree", "add", "--detach", detachedPath, "HEAD")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add --detach: %v\n%s", err, out)
	}

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/import", map[string]string{
		"path": detachedPath,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("import detached-HEAD worktree: status = %d, want 400", resp.StatusCode)
	}
}
