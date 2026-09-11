// Package githook installs a real `git` post-checkout hook so
// worktree-studio can record every branch a worktree gets checked out onto
// over its lifetime ("which branches did this worktree toggle through"),
// not just the one it was created with.
//
// A `git worktree`'s hooks are NOT per-worktree: every worktree of a repo
// shares the same hooks directory as the main checkout (git resolves it via
// `--git-common-dir`, same physical .git/hooks/ regardless of which
// worktree you're standing in) — see `git help worktree`'s "DETAILS"
// section. So this installs once per registered repo, and the installed
// script itself resolves which worktree fired it from $PWD at hook-run
// time (post-checkout runs with cwd set to the worktree's own root), the
// same way the git status polling in internal/gitops already resolves
// "current branch" per worktree without needing any per-worktree state.
//
// This was chosen over polling git status for branch changes (which this
// project already does every 5s for the dirty/ahead-behind dashboard —
// see internal/gitops.Status) because that poll only runs while a
// worktree's page happens to be open in a browser tab, and doesn't already
// keep a "previous branch seen" value to diff against — turning it into a
// checkout tracker would still miss anything checked out while no tab was
// open, exactly the gap an audit trail exists to close. A real hook fires
// on every checkout regardless of whether the UI is open, at the actual
// moment it happens, for the same reason internal/claudehook's SessionStart
// hook replaced the launch-time-only claude session tracking.
//
// Mirrors internal/claudehook/install.go's safety posture where it can:
// only ever installed via an explicit user action, idempotent, and backs
// up whatever it's about to change before writing. It differs in mechanism
// because the artifact being edited is different — a single executable
// script file, not a JSON config with a hooks.<event> array to merge into
// — so instead of merging a marked *entry* into a list, this merges a
// marked *block* into the script's text, bracketed by beginMarker/
// endMarker, so any hook a repo already had (or gets later, by hand)
// survives untouched.
package githook

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	hookFileName = "post-checkout"

	beginMarker = "# --- worktree-studio:post-checkout begin (internal/githook) ---"
	endMarker   = "# --- worktree-studio:post-checkout end ---"
)

// CommonHooksDir resolves repoPath's shared hooks directory — the same one
// every worktree of this repo uses — via `git rev-parse --git-common-dir`.
// That command's output is relative to repoPath when repoPath's own .git is
// a plain directory (the common case), so a relative result is joined back
// onto repoPath rather than the process's own cwd.
func CommonHooksDir(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--git-common-dir")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --git-common-dir: %w", err)
	}
	gitDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repoPath, gitDir)
	}
	return filepath.Join(gitDir, "hooks"), nil
}

func hookPath(repoPath string) (string, error) {
	dir, err := CommonHooksDir(repoPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, hookFileName), nil
}

// block is the marked section installed into post-checkout. `$3` is git's
// own "was this a branch checkout" flag (1 for `git checkout <branch>`/
// `git switch`, 0 for a plain file checkout like `git checkout -- path`) —
// skipping it there means this never fires for the common file-restore
// case. `--data-urlencode` (not hand-rolled JSON) sidesteps needing `jq` or
// any other dependency just to escape a branch/path name safely; the
// server parses it as an ordinary form POST (see
// internal/api.handleGitHookPostCheckout). Fire-and-forget, same posture
// as internal/claudehook's scripts: a short timeout, failures discarded,
// and it never affects the real `git checkout`'s own exit code (there's
// nothing after it in the file for git itself to fail on).
func block(serverBaseURL string) string {
	return fmt.Sprintf(`%s
if [ "$3" = "1" ]; then
  wts_cwd="$(git rev-parse --show-toplevel 2>/dev/null)"
  wts_branch="$(git symbolic-ref --quiet --short HEAD 2>/dev/null)"
  if [ -z "$wts_branch" ]; then
    wts_branch="$(git rev-parse --short HEAD 2>/dev/null)"
  fi
  if [ -n "$wts_cwd" ] && [ -n "$wts_branch" ]; then
    curl -s -m 2 -X POST \
      --data-urlencode "cwd=$wts_cwd" \
      --data-urlencode "branch=$wts_branch" \
      --data-urlencode "prev_head=$1" \
      --data-urlencode "new_head=$2" \
      '%s/api/git-hook/post-checkout' >/dev/null 2>&1 || true
  fi
fi
%s
`, beginMarker, serverBaseURL, endMarker)
}

// IsInstalled reports whether repoPath's shared post-checkout hook already
// contains our marked block.
func IsInstalled(repoPath string) (bool, error) {
	path, err := hookPath(repoPath)
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	return strings.Contains(string(data), beginMarker), nil
}

// Install ensures repoPath's shared post-checkout hook contains our marked
// block, appending it to whatever's already there (backed up first if the
// file pre-exists) rather than overwriting — a repo may already have a
// post-checkout hook doing something unrelated, and post-checkout is a
// single script git runs top-to-bottom, so appending preserves it. Refreshes
// the block's content unconditionally (e.g. if the server's address
// changed), same as claudehook.InstallHookByID's own "cheap to redo, and
// re-installing is the natural fix-it action" reasoning.
func Install(repoPath, serverBaseURL string) error {
	path, err := hookPath(repoPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create hooks dir: %w", err)
	}

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}

	before, _, after, found := splitOnMarkers(string(existing))
	if !found && len(existing) > 0 {
		if err := backup(existing); err != nil {
			return err
		}
	}

	var buf bytes.Buffer
	if strings.TrimSpace(before) == "" {
		buf.WriteString("#!/bin/sh\n")
	} else {
		buf.WriteString(before)
		if !strings.HasSuffix(before, "\n") {
			buf.WriteString("\n")
		}
	}
	buf.WriteString(block(serverBaseURL))
	buf.WriteString(after)

	return os.WriteFile(path, buf.Bytes(), 0o755)
}

// Uninstall removes our marked block from repoPath's shared post-checkout
// hook, leaving any other content (pre-existing or hand-added since)
// exactly as it was. If our block was the hook's entire content, the file
// is removed entirely rather than left behind as a shebang-only no-op —
// unlike claudehook's own scripts (harmless to leave under
// ~/.worktree-studio/hooks), this lives inside the user's actual repo, so
// tidying up fully once we're the only thing there is the more considerate
// default. A repo with no installed hook at all is a silent no-op, not an
// error.
func Uninstall(repoPath string) error {
	path, err := hookPath(repoPath)
	if err != nil {
		return err
	}
	existing, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}

	before, _, after, found := splitOnMarkers(string(existing))
	if !found {
		return nil
	}

	remaining := strings.TrimSpace(before + after)
	if remaining == "" || remaining == "#!/bin/sh" {
		return os.Remove(path)
	}

	var buf bytes.Buffer
	buf.WriteString(before)
	buf.WriteString(after)
	return os.WriteFile(path, buf.Bytes(), 0o755)
}

// splitOnMarkers finds our block (including its markers) in content and
// returns the text before it, the block itself, and the text after it.
// found is false if content has no (or only a partial) marker pair, in
// which case before is the entirety of content and mid/after are empty.
func splitOnMarkers(content string) (before, mid, after string, found bool) {
	start := strings.Index(content, beginMarker)
	if start == -1 {
		return content, "", "", false
	}
	end := strings.Index(content, endMarker)
	if end == -1 || end < start {
		return content, "", "", false
	}
	end += len(endMarker)
	// Consume a single trailing newline after the end marker, if present,
	// so re-appending before+after doesn't accumulate blank lines across
	// repeated install/uninstall cycles.
	if end < len(content) && content[end] == '\n' {
		end++
	}
	return content[:start], content[start:end], content[end:], true
}

func backup(data []byte) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	backupDir := filepath.Join(home, ".worktree-studio", "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}
	backupPath := filepath.Join(backupDir, fmt.Sprintf("post-checkout-%d", time.Now().UnixNano()))
	return os.WriteFile(backupPath, data, 0o600)
}
