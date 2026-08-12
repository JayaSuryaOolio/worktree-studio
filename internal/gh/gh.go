// Package gh wraps the `gh` CLI (GitHub's own, github.com/cli/cli) for
// looking up a branch's pull request — same "shell out to the real tool
// rather than reimplement its auth/API-plumbing" call this project already
// makes for git itself and for the standalone `spotlight` CLI (see
// internal/gitops and internal/spotlight). Deliberately thin: this package
// only ever asks for one branch's PR at a time, on demand — the caller
// (internal/api's worktree-summary endpoint) is what's responsible for not
// calling it too often; see that handler's own doc comment for the
// caching/rate-limit story.
package gh

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ErrNotFound is returned when the `gh` CLI can't be located on PATH.
var ErrNotFound = errors.New("gh CLI not found on PATH (see https://cli.github.com)")

// PRInfo is one pull request, as reported by `gh pr view --json`.
type PRInfo struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	State   string `json:"state"` // "OPEN", "CLOSED", or "MERGED"
	URL     string `json:"url"`
	IsDraft bool   `json:"isDraft"`
}

// PRForBranch looks up the pull request (if any) for branch in the repo
// rooted at repoPath, via `gh pr view <branch>` run with that directory as
// cwd (the same way `gh` resolves "which GitHub repo" for any of its
// commands — from the git remote configured in the current directory, not
// an explicit --repo flag, so this works unmodified for any repo `gh` is
// already authenticated against).
//
// Returns (nil, nil) — not an error — if the branch simply has no PR,
// which `gh pr view` reports as a non-zero exit with "no pull requests
// found" on stderr; that's the expected, common case for a worktree
// that's still local-only work, not a failure.
func PRForBranch(repoPath, branch string) (*PRInfo, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, ErrNotFound
	}

	cmd := exec.Command("gh", "pr", "view", branch, "--json", "number,title,state,url,isDraft")
	cmd.Dir = repoPath
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(strings.ToLower(stderr.String()), "no pull requests found") {
			return nil, nil
		}
		return nil, fmt.Errorf("gh pr view %s: %w (%s)", branch, err, strings.TrimSpace(stderr.String()))
	}

	var pr PRInfo
	if err := json.Unmarshal(stdout.Bytes(), &pr); err != nil {
		return nil, fmt.Errorf("parse gh pr view output: %w", err)
	}
	return &pr, nil
}
