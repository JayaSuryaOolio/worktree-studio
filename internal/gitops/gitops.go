// Package gitops wraps the `git worktree` and `git status` CLI commands via
// os/exec. Shelling out to the real git binary is simpler and more correct
// than any Go git reimplementation for this — see PLAN.md's stated
// philosophy of using the best available working tool rather than
// reinventing it.
package gitops

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ErrWorktreeDirty is returned by RemoveWorktree when force is false and git
// refuses to remove the worktree because it has uncommitted changes and/or
// untracked files. Callers can check for this with errors.Is and offer the
// user a way to retry with force.
var ErrWorktreeDirty = errors.New("worktree has uncommitted changes or untracked files")

// IsGitRepo reports whether path is the top level of a real git working
// tree (i.e. `git rev-parse --is-inside-work-tree` succeeds there).
func IsGitRepo(path string) bool {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--is-inside-work-tree")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// WorktreeEntry is one entry from `git worktree list --porcelain`.
type WorktreeEntry struct {
	Path   string
	Branch string
	Head   string
}

// ListWorktrees runs `git worktree list --porcelain` against repoPath and
// parses its output.
func ListWorktrees(repoPath string) ([]WorktreeEntry, error) {
	cmd := exec.Command("git", "-C", repoPath, "worktree", "list", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}

	var entries []WorktreeEntry
	var cur WorktreeEntry
	flush := func() {
		if cur.Path != "" {
			entries = append(entries, cur)
		}
		cur = WorktreeEntry{}
	}

	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			cur.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(line, "branch ")
		case line == "":
			flush()
		}
	}
	flush()

	return entries, nil
}

// AddWorktree runs `git worktree add -b <branch> <worktreePath> [startPoint]`
// from within repoPath, creating a new branch and worktree in one step. If
// startPoint is empty, git falls back to its own default: the commit
// repoPath's own checkout currently has as HEAD — which is whatever branch
// happened to be checked out there at the moment of creation, not
// necessarily any particular "base" branch. Callers that want new worktrees
// to consistently branch off e.g. main/master regardless of what repoPath's
// own working copy is on should resolve and pass a real startPoint instead
// of relying on that default — see DetectDefaultBranch and
// handleCreateWorktree's use of repo.BaseBranch.
func AddWorktree(repoPath, worktreePath, branch, startPoint string) error {
	args := []string{"-C", repoPath, "worktree", "add", "-b", branch, worktreePath}
	if startPoint != "" {
		args = append(args, startPoint)
	}
	cmd := exec.Command("git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git worktree add %s (branch %s, start point %q): %w: %s", worktreePath, branch, startPoint, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// DetectDefaultBranch makes a best-effort guess at repoPath's "main" branch,
// for use as AddWorktree's startPoint when a repo has no explicit
// Repo.BaseBranch setting. Tries, in order: the remote `origin`'s own
// advertised default branch (`git symbolic-ref refs/remotes/origin/HEAD` —
// what `origin/HEAD -> main` in `git branch -r` reflects, and the same
// signal `gh`/GitHub's own "default branch" concept is built on), then a
// local `main` branch, then a local `master` branch. Returns "" if none of
// those resolve, in which case the caller should fall back to git's own
// implicit-HEAD-of-repoPath behavior (pass "" as AddWorktree's startPoint).
func DetectDefaultBranch(repoPath string) string {
	if out, err := exec.Command("git", "-C", repoPath, "symbolic-ref", "--short", "refs/remotes/origin/HEAD").Output(); err == nil {
		if ref := strings.TrimSpace(string(out)); ref != "" {
			return strings.TrimPrefix(ref, "origin/")
		}
	}
	for _, candidate := range []string{"main", "master"} {
		if exec.Command("git", "-C", repoPath, "show-ref", "--verify", "--quiet", "refs/heads/"+candidate).Run() == nil {
			return candidate
		}
	}
	return ""
}

// ListBranches returns every local and remote-tracking branch name for
// repoPath (e.g. "main", "origin/main", "origin/feature-x"), via `git
// for-each-ref` — the new-worktree dialog's "branch to create from"
// dropdown, so a person can deliberately pick a remote-tracking ref
// (fresher than the local branch of the same name if nobody's fetched
// recently) instead of only ever seeing local branches. Excludes
// "origin/HEAD" and similar symbolic-ref pointers — not a real branch,
// just an alias DetectDefaultBranch already resolves separately.
func ListBranches(repoPath string) ([]string, error) {
	// Filtering has to happen on the *full* refname, not the shortened
	// one: git's own %(refname:short) collapses a remote's symbolic HEAD
	// pointer ("refs/remotes/origin/HEAD") down to just "origin" — not
	// "origin/HEAD" — so a suffix check against the short name alone
	// never matches it and it leaks into the list as a fake "origin"
	// branch. Found by hand testing against a real repo, not a hypothetical.
	cmd := exec.Command("git", "-C", repoPath, "for-each-ref", "--format=%(refname)\t%(refname:short)", "refs/heads", "refs/remotes")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git for-each-ref: %w", err)
	}

	var branches []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		full, short, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok || full == "" || strings.HasSuffix(full, "/HEAD") {
			continue
		}
		branches = append(branches, short)
	}
	return branches, nil
}

// DeleteBranch runs `git branch -D <branch>` from within repoPath. Note
// that RemoveWorktree does NOT delete the branch a worktree was created
// with — `git worktree remove` only removes the checkout, leaving the
// branch itself intact — so fully undoing an AddWorktree call requires
// both RemoveWorktree and DeleteBranch.
func DeleteBranch(repoPath, branch string) error {
	cmd := exec.Command("git", "-C", repoPath, "branch", "-D", branch)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git branch -D %s: %w: %s", branch, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// RemoveWorktree runs `git worktree remove <worktreePath>` from within
// repoPath. Without force, git itself refuses to remove a worktree that has
// uncommitted changes or untracked files, and that refusal is surfaced as
// ErrWorktreeDirty (wrapped, so errors.Is works) rather than a generic
// error, so callers can offer the user an explicit "remove anyway" retry
// with force=true instead of silently discarding their work. With force,
// --force is passed and git removes the worktree unconditionally.
func RemoveWorktree(repoPath, worktreePath string, force bool) error {
	args := []string{"-C", repoPath, "worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, worktreePath)

	cmd := exec.Command("git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if !force && isDirtyWorktreeError(msg) {
			return fmt.Errorf("%w: %s", ErrWorktreeDirty, msg)
		}
		return fmt.Errorf("git worktree remove %s: %w: %s", worktreePath, err, msg)
	}
	return nil
}

// ChangedFiles returns every modified/added/deleted/untracked file path in
// worktreePath, via `git status --porcelain` (v1 format — "XY <path>", or
// "XY <path> -> <newpath>" for a rename, always a 2-character status code
// then a space regardless of status kind, which is simpler to parse
// reliably than porcelain=2's per-kind field layout). Used for the
// sidebar's hover-summary popover's changed-file list — a separate git
// invocation from Status below (which sticks to porcelain=2 for its
// richer branch/ahead-behind info) since the two calls serve different,
// independently-cacheable purposes.
func ChangedFiles(worktreePath string) ([]string, error) {
	cmd := exec.Command("git", "-C", worktreePath, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status --porcelain: %w", err)
	}

	var files []string
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if len(line) < 4 {
			continue
		}
		path := line[3:]
		if arrow := strings.Index(path, " -> "); arrow != -1 {
			path = path[arrow+4:]
		}
		files = append(files, path)
	}
	return files, nil
}

// isDirtyWorktreeError reports whether git's stderr from a failed (non-
// forced) `worktree remove` indicates the refusal was due to the worktree
// being dirty (as opposed to some other failure, e.g. the path not being a
// worktree at all). Matches git's actual message wording ("... is dirty,
// use --force to delete it" / "... contains modified or untracked files,
// use --force to delete it").
func isDirtyWorktreeError(stderr string) bool {
	lower := strings.ToLower(stderr)
	return strings.Contains(lower, "is dirty") ||
		strings.Contains(lower, "contains modified or untracked files") ||
		strings.Contains(lower, "use --force")
}

// StatusResult is the parsed output of `git status --porcelain=2 --branch`.
type StatusResult struct {
	Branch      string
	Dirty       bool
	HasUpstream bool // false if the branch has no configured upstream (e.g. a freshly created worktree's branch that's never been pushed) — Ahead/Behind are meaningless without one
	Ahead       int
	Behind      int
	Raw         string
}

// Status runs `git status --porcelain=2 --branch` against worktreePath.
func Status(worktreePath string) (StatusResult, error) {
	cmd := exec.Command("git", "-C", worktreePath, "status", "--porcelain=2", "--branch")
	out, err := cmd.Output()
	if err != nil {
		return StatusResult{}, fmt.Errorf("git status: %w", err)
	}

	res := StatusResult{Raw: string(out)}
	for _, line := range strings.Split(res.Raw, "\n") {
		switch {
		case strings.HasPrefix(line, "# branch.head "):
			res.Branch = strings.TrimPrefix(line, "# branch.head ")
		case strings.HasPrefix(line, "# branch.ab "):
			// e.g. "# branch.ab +2 -1" — ahead 2, behind 1. Only present
			// when the branch has a configured upstream at all.
			res.HasUpstream = true
			fields := strings.Fields(strings.TrimPrefix(line, "# branch.ab "))
			if len(fields) == 2 {
				res.Ahead, _ = strconv.Atoi(strings.TrimPrefix(fields[0], "+"))
				res.Behind, _ = strconv.Atoi(strings.TrimPrefix(fields[1], "-"))
			}
		case line != "" && !strings.HasPrefix(line, "#"):
			res.Dirty = true
		}
	}
	return res, nil
}
