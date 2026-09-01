package term

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestAttachEnvOverridesTERM confirms attachEnv forces TERM=xterm-256color
// regardless of whatever TERM the calling process itself has (or lacks) —
// the fix for tmux's xterm-keys translation depending on a terminfo entry
// that actually describes modified-key sequences, rather than whichever
// TERM the Go server happened to inherit from however it was launched. See
// docs/terminal-keybindings.md. Pure function, no real tmux/pty needed.
func TestAttachEnvOverridesTERM(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("WORKTREE_STUDIO_ATTACH_ENV_TEST_MARKER", "kept")

	env := attachEnv()

	var sawTerm, sawMarker bool
	for _, kv := range env {
		if kv == "TERM=xterm-256color" {
			sawTerm = true
		}
		if strings.HasPrefix(kv, "TERM=") && kv != "TERM=xterm-256color" {
			t.Fatalf("expected the original TERM to be overridden, but found %q alongside it", kv)
		}
		if kv == "WORKTREE_STUDIO_ATTACH_ENV_TEST_MARKER=kept" {
			sawMarker = true
		}
	}
	if !sawTerm {
		t.Fatalf("expected TERM=xterm-256color in attachEnv(), got %v", env)
	}
	if !sawMarker {
		t.Fatalf("expected an unrelated env var to survive attachEnv(), got %v", env)
	}
}

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

// sendKeys types a command into a live tmux session — same "real command
// via tmux send-keys" approach CreateSession's own initialCommand support
// uses, not a simulated pty write. It does NOT wait for the command to take
// effect; pair it with a poll for whatever the command should produce (see
// waitForPanePath).
func sendKeys(t *testing.T, tmuxSessionName, command string) {
	t.Helper()
	if out, err := TmuxCmd("send-keys", "-t", tmuxSessionName, command, "Enter").CombinedOutput(); err != nil {
		t.Fatalf("tmux send-keys %q: %v (%s)", command, err, out)
	}
}

// waitForPanePath polls CurrentPath until it reports want, returning the
// last value seen if it never does.
//
// This replaced a fixed one-second sleep after sendKeys. A freshly created
// session's shell needs a moment to start up and process input, and any
// fixed wait is a bet on how loaded the machine is: one second was enough
// when internal/term ran alone but NOT when `go test ./...` ran it in
// parallel with internal/api's tmux-heavy tests, which is what made
// TestCurrentPathAfterCd flaky. Polling waits exactly as long as needed and
// no longer, so it's both more reliable and usually much faster.
func waitForPanePath(t *testing.T, tmuxSessionName, want string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var got string
	for {
		p, err := CurrentPath(tmuxSessionName)
		if err == nil {
			got = p
			if got == want {
				return got
			}
		}
		if time.Now().After(deadline) {
			return got
		}
		time.Sleep(50 * time.Millisecond)
	}
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

	want := resolveSymlinks(t, elsewhere)
	if got := waitForPanePath(t, ts.TmuxSessionName, want); got != want {
		t.Errorf("CurrentPath after cd = %q, want %q", got, want)
	}
}
