package api

import (
	"net/http"
	"path/filepath"
	"testing"

	"worktree-studio/internal/store"
)

// TestCreateWorktreeCLIFromRepoRoot exercises the path-based
// POST /api/worktrees endpoint backing the `worktree-studio create-worktree
// <name> [path]` CLI subcommand (cmd/worktree-studio/createworktree.go),
// resolved from the repo's own root checkout — possible because handleAddRepo
// registers a synthetic root worktree for it (EnsureRootWorktree), which is
// exactly what resolveWorktreeByPathForCLI matches against.
func TestCreateWorktreeCLIFromRepoRoot(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/worktrees", map[string]string{"path": repoPath, "name": "amber-ridge"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create-worktree cli from repo root: status = %d, want 201", resp.StatusCode)
	}
	var wt store.Worktree
	decodeInto(t, resp, &wt)
	if wt.RepoID != repo.ID {
		t.Fatalf("created worktree repo_id = %q, want %q", wt.RepoID, repo.ID)
	}
	if wt.Branch != "amber-ridge" {
		t.Fatalf("created worktree branch = %q, want %q", wt.Branch, "amber-ridge")
	}
}

// TestCreateWorktreeCLIFromExistingWorktree covers the other realistic case:
// an agent already sitting inside one worktree of a repo asking for a
// sibling worktree of the *same* repo, identified by a path inside that
// existing worktree rather than the repo's root checkout.
func TestCreateWorktreeCLIFromExistingWorktree(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "first"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create first worktree: status = %d, want 201", resp.StatusCode)
	}
	var first store.Worktree
	decodeInto(t, resp, &first)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/worktrees", map[string]string{"path": filepath.Join(first.Path, "sub"), "name": "second"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create-worktree cli from existing worktree: status = %d, want 201", resp.StatusCode)
	}
	var second store.Worktree
	decodeInto(t, resp, &second)
	if second.RepoID != repo.ID {
		t.Fatalf("second worktree repo_id = %q, want %q", second.RepoID, repo.ID)
	}
	if second.Path == first.Path {
		t.Fatalf("second worktree got the same path as the first: %q", second.Path)
	}
}

// TestCreateWorktreeCLINameRequired covers the request-validation path: a
// resolvable repo but no name.
func TestCreateWorktreeCLINameRequired(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/worktrees", map[string]string{"path": repoPath})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("create-worktree cli with no name: status = %d, want 400", resp.StatusCode)
	}
}

// TestCreateWorktreeCLIPathOutsideAnyRepoIsANoOp mirrors
// TestSpotlightCLIPathOutsideAnyWorktreeIsANoOp's contract for the same
// reason: this subcommand can be run from anywhere on disk, and a path
// outside every tracked repo/worktree is a normal, expected outcome, not an
// error — reported as its own distinct status rather than a failure the CLI
// would report as broken.
func TestCreateWorktreeCLIPathOutsideAnyRepoIsANoOp(t *testing.T) {
	ts, _ := newTestServer(t)
	tmp := t.TempDir()

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/worktrees", map[string]string{"path": tmp, "name": "whatever"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create-worktree cli for untracked path: status = %d, want 200", resp.StatusCode)
	}
	var result map[string]string
	decodeInto(t, resp, &result)
	if result["status"] != "no matching worktree" {
		t.Fatalf("create-worktree cli for untracked path = %+v, want no matching worktree", result)
	}
}
