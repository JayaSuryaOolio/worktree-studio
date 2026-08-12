package term

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// resolveSymlinks mirrors gitops/spotlight's own "tmux reports the
// symlink-resolved absolute path, not necessarily what t.TempDir() itself
// returned" reality on macOS (e.g. /var/folders/... vs /private/var/
// folders/... for the exact same directory) — same normalization
// internal/spotlight.resolveBestEffort already documents needing.
func resolveSymlinks(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", path, err)
	}
	return resolved
}

// sendKeys types a command into a live tmux session and waits for it to
// take effect — same "real command via tmux send-keys" approach
// CreateSession's own initialCommand support uses, not a simulated pty
// write.
func sendKeys(t *testing.T, tmuxSessionName, command string) {
	t.Helper()
	if out, err := exec.Command("tmux", "send-keys", "-t", tmuxSessionName, command, "Enter").CombinedOutput(); err != nil {
		t.Fatalf("tmux send-keys %q: %v (%s)", command, err, out)
	}
	// A freshly created session's shell needs a moment to actually start up
	// and begin processing input — confirmed empirically that 200-300ms
	// wasn't reliably enough for `cd` to take effect before the next
	// display-message query, while 1s consistently was.
	time.Sleep(1 * time.Second)
}

// TestCurrentPath verifies CurrentPath returns the tmux pane's own
// current-working-directory (`#{pane_current_path}`), tracking a real
// `cd` inside the session without needing to poll /proc or lsof —
// exercised against a real tmux session, not a fake, since the whole
// point is confirming tmux itself keeps this current.
func TestCurrentPath(t *testing.T) {
	requireTmux(t)
	st := newTestStore(t)
	m := &Manager{Store: st}

	worktreeDir := t.TempDir()
	ts, err := m.CreateSession("wt1", worktreeDir, "shell", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	t.Cleanup(func() { _ = m.CloseSession(ts) })

	got, err := CurrentPath(ts.TmuxSessionName)
	if err != nil {
		t.Fatalf("CurrentPath: %v", err)
	}
	if got != resolveSymlinks(t, worktreeDir) {
		t.Errorf("CurrentPath = %q, want %q (the session's own cwd at creation)", got, worktreeDir)
	}
}

// TestCurrentPathAfterCd verifies CurrentPath reflects a real `cd` typed
// into the session, not just its cwd at creation — the actual scenario
// the terminal-cwd-mismatch feature depends on (someone `cd`s out of
// their worktree, and the UI should notice on next open).
func TestCurrentPathAfterCd(t *testing.T) {
	requireTmux(t)
	st := newTestStore(t)
	m := &Manager{Store: st}

	ts, err := m.CreateSession("wt1", t.TempDir(), "shell", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	t.Cleanup(func() { _ = m.CloseSession(ts) })

	elsewhere := t.TempDir()
	sendKeys(t, ts.TmuxSessionName, "cd "+elsewhere)

	got, err := CurrentPath(ts.TmuxSessionName)
	if err != nil {
		t.Fatalf("CurrentPath: %v", err)
	}
	if got != resolveSymlinks(t, elsewhere) {
		t.Errorf("CurrentPath after cd = %q, want %q", got, elsewhere)
	}
}
