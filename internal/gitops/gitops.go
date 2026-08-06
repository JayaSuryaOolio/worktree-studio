// Package gitops wraps the `git worktree` and `git status` CLI commands via
// os/exec. Shelling out to the real git binary is simpler and more correct
// than any Go git reimplementation for this — see PLAN.md's stated
// philosophy of using the best available working tool rather than
// reinventing it.
package gitops

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

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

// AddWorktree runs `git worktree add -b <branch> <worktreePath>` from
// within repoPath, creating a new branch and worktree in one step.
func AddWorktree(repoPath, worktreePath, branch string) error {
	cmd := exec.Command("git", "-C", repoPath, "worktree", "add", "-b", branch, worktreePath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git worktree add %s (branch %s): %w: %s", worktreePath, branch, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// RemoveWorktree runs `git worktree remove --force <worktreePath>` from
// within repoPath.
func RemoveWorktree(repoPath, worktreePath string) error {
	cmd := exec.Command("git", "-C", repoPath, "worktree", "remove", "--force", worktreePath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git worktree remove %s: %w: %s", worktreePath, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// StatusResult is the parsed output of `git status --porcelain=2 --branch`.
type StatusResult struct {
	Branch string
	Dirty  bool
	Raw    string
}

// Status runs `git status --porcelain=2 --branch` against worktreePath.
// Not yet wired into any handler (that lands in the monitoring-dashboard
// step), but provided now per PLAN.md's step-1 scope for internal/gitops.
func Status(worktreePath string) (StatusResult, error) {
	cmd := exec.Command("git", "-C", worktreePath, "status", "--porcelain=2", "--branch")
	out, err := cmd.Output()
	if err != nil {
		return StatusResult{}, fmt.Errorf("git status: %w", err)
	}

	res := StatusResult{Raw: string(out)}
	for _, line := range strings.Split(res.Raw, "\n") {
		if strings.HasPrefix(line, "# branch.head ") {
			res.Branch = strings.TrimPrefix(line, "# branch.head ")
		} else if line != "" && !strings.HasPrefix(line, "#") {
			res.Dirty = true
		}
	}
	return res, nil
}
