package gitops

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// requireGit skips the test if git isn't on PATH — these are integration
// tests that shell out to the real git binary, matching this package's own
// "shell out, don't reimplement" philosophy (see gitops.go's doc comment).
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}
}

// newTestRepo creates a throwaway git repo (with one commit, so it has a
// HEAD to branch from) under a t.TempDir, which is auto-cleaned.
func newTestRepo(t *testing.T) string {
	t.Helper()
	requireGit(t)

	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.name", "test")
	run("config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README.md")
	run("commit", "-q", "-m", "initial commit")
	return dir
}

func TestIsGitRepo(t *testing.T) {
	requireGit(t)
	repo := newTestRepo(t)
	if !IsGitRepo(repo) {
		t.Errorf("IsGitRepo(%q) = false, want true", repo)
	}

	notRepo := t.TempDir()
	if IsGitRepo(notRepo) {
		t.Errorf("IsGitRepo(%q) = true, want false", notRepo)
	}
}

func TestAddListRemoveWorktree(t *testing.T) {
	repo := newTestRepo(t)
	wtPath := filepath.Join(t.TempDir(), "my-feature")

	if err := AddWorktree(repo, wtPath, "my-feature", ""); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("worktree dir not created: %v", err)
	}

	entries, err := ListWorktrees(repo)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	var found bool
	for _, e := range entries {
		if e.Branch == "refs/heads/my-feature" {
			found = true
		}
	}
	if !found {
		t.Errorf("ListWorktrees(%q) = %+v, want an entry with branch refs/heads/my-feature", repo, entries)
	}

	if err := RemoveWorktree(repo, wtPath, false); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree dir still exists after remove: err=%v", err)
	}

	// RemoveWorktree does NOT delete the branch it was created with — this
	// is real git behavior, not a bug, but it's the exact gap that once
	// let a failed worktree-creation rollback leave a branch behind
	// blocking retries (see internal/api's regression test for the full
	// story). DeleteBranch is the other half of a full rollback.
	branchOut, err := exec.Command("git", "-C", repo, "branch", "--list", "my-feature").Output()
	if err != nil {
		t.Fatalf("git branch --list: %v", err)
	}
	if strings.TrimSpace(string(branchOut)) == "" {
		t.Fatal("expected the branch to still exist after RemoveWorktree alone")
	}

	if err := DeleteBranch(repo, "my-feature"); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	branchOut, err = exec.Command("git", "-C", repo, "branch", "--list", "my-feature").Output()
	if err != nil {
		t.Fatalf("git branch --list: %v", err)
	}
	if strings.TrimSpace(string(branchOut)) != "" {
		t.Errorf("expected the branch to be gone after DeleteBranch, got: %q", branchOut)
	}
}

func TestRemoveWorktreeDirtyWithoutForceFails(t *testing.T) {
	repo := newTestRepo(t)
	wtPath := filepath.Join(t.TempDir(), "dirty-feature")

	if err := AddWorktree(repo, wtPath, "dirty-feature", ""); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	// Make the worktree dirty: an untracked file is enough for git to
	// refuse a non-forced `worktree remove`.
	if err := os.WriteFile(filepath.Join(wtPath, "scratch.txt"), []byte("wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := RemoveWorktree(repo, wtPath, false)
	if err == nil {
		t.Fatal("RemoveWorktree(force=false) on a dirty worktree returned nil error, want ErrWorktreeDirty")
	}
	if !errors.Is(err, ErrWorktreeDirty) {
		t.Errorf("RemoveWorktree(force=false) error = %v, want errors.Is(err, ErrWorktreeDirty)", err)
	}
	if _, statErr := os.Stat(wtPath); statErr != nil {
		t.Errorf("worktree dir should still exist after a refused (non-forced) remove: %v", statErr)
	}

	// Now force it, and it should actually go away, changes and all.
	if err := RemoveWorktree(repo, wtPath, true); err != nil {
		t.Fatalf("RemoveWorktree(force=true): %v", err)
	}
	if _, statErr := os.Stat(wtPath); !os.IsNotExist(statErr) {
		t.Errorf("worktree dir still exists after forced remove: err=%v", statErr)
	}
}

func TestStatus(t *testing.T) {
	repo := newTestRepo(t)

	res, err := Status(repo)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if res.Dirty {
		t.Errorf("Status on a freshly committed repo: Dirty = true, want false")
	}

	if err := os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = Status(repo)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !res.Dirty {
		t.Errorf("Status with an untracked file: Dirty = false, want true")
	}
}

func TestStatusNoUpstream(t *testing.T) {
	repo := newTestRepo(t)

	res, err := Status(repo)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if res.HasUpstream {
		t.Errorf("freshly created repo's branch has no upstream configured: HasUpstream = true, want false")
	}
	if res.Ahead != 0 || res.Behind != 0 {
		t.Errorf("Ahead/Behind without an upstream = %d/%d, want 0/0", res.Ahead, res.Behind)
	}
}

func TestStatusAheadBehind(t *testing.T) {
	repo := newTestRepo(t)

	clone := t.TempDir()
	// Clone into an existing empty dir via `git clone . <dir>` — the
	// default sibling-directory form refuses when the target already
	// exists (as t.TempDir() ensures), so clone into it explicitly.
	cmd := exec.Command("git", "clone", "-q", repo, clone)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}
	run := func(dir string, args ...string) {
		c := exec.Command("git", append([]string{"-C", dir}, args...)...)
		c.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// The clone commits locally (ahead of origin/main by 1)...
	if err := os.WriteFile(filepath.Join(clone, "clone.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(clone, "add", ".")
	run(clone, "commit", "-q", "-m", "ahead commit")

	// ...while the original repo (the clone's "origin") also gains a commit
	// the clone doesn't have yet, so after fetching, the clone is both
	// ahead AND behind.
	if err := os.WriteFile(filepath.Join(repo, "origin.txt"), []byte("y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(repo, "add", ".")
	run(repo, "commit", "-q", "-m", "origin-side commit")
	run(clone, "fetch", "-q")

	res, err := Status(clone)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !res.HasUpstream {
		t.Fatal("expected the clone's branch to have an upstream configured")
	}
	if res.Ahead != 1 || res.Behind != 1 {
		t.Errorf("Ahead/Behind = %d/%d, want 1/1", res.Ahead, res.Behind)
	}
}
