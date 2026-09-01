package term

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"worktree-studio/internal/store"
)

func requireTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not found on PATH")
	}
}

// newTestStore returns a store with a real repo+worktree row already
// inserted (id "wt1", matching every CreateSession call in this file) —
// terminal_sessions.worktree_id has a foreign key against worktrees(id),
// now actually enforced (see store's PRAGMA foreign_keys fix), so a
// terminal session can no longer reference a worktree id that doesn't
// exist. Before that fix this omission silently worked; now it correctly
// doesn't.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	if err := st.AddRepo(store.Repo{ID: "r1", Name: "test", Path: t.TempDir()}); err != nil {
		t.Fatalf("seed repo: %v", err)
	}
	if err := st.AddWorktree(store.Worktree{ID: "wt1", RepoID: "r1", Name: "wt1", Branch: "wt1", Path: t.TempDir()}); err != nil {
		t.Fatalf("seed worktree: %v", err)
	}
	return st
}

func TestCreateListCloseSession(t *testing.T) {
	requireTmux(t)
	st := newTestStore(t)
	m := &Manager{Store: st}

	ts, err := m.CreateSession("wt1", t.TempDir(), "shell", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if ts.TmuxSessionName == "" || ts.ID == "" {
		t.Fatalf("expected populated session, got %+v", ts)
	}

	live, err := ListLiveTmuxSessionNames()
	if err != nil {
		t.Fatalf("ListLiveTmuxSessionNames: %v", err)
	}
	if !live[ts.TmuxSessionName] {
		t.Fatalf("expected tmux session %s to be live, live=%v", ts.TmuxSessionName, live)
	}

	sessions, err := m.ListSessions("wt1")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != ts.ID {
		t.Fatalf("expected exactly the created session, got %+v", sessions)
	}

	if err := m.CloseSession(ts); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	live, err = ListLiveTmuxSessionNames()
	if err != nil {
		t.Fatalf("ListLiveTmuxSessionNames after close: %v", err)
	}
	if live[ts.TmuxSessionName] {
		t.Fatalf("expected tmux session %s to be gone after close", ts.TmuxSessionName)
	}

	sessions, err = m.ListSessions("wt1")
	if err != nil {
		t.Fatalf("ListSessions after close: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected no sessions after close, got %+v", sessions)
	}
}

func TestReconcileDropsDeadSessions(t *testing.T) {
	requireTmux(t)
	st := newTestStore(t)
	m := &Manager{Store: st}

	ts, err := m.CreateSession("wt1", t.TempDir(), "shell", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Kill the tmux session out from under the store, simulating tmux
	// dying independently of worktree-studio (e.g. `tmux kill-server`).
	if err := TmuxCmd("kill-session", "-t", ts.TmuxSessionName).Run(); err != nil {
		t.Fatalf("kill-session: %v", err)
	}

	dropped, err := Reconcile(st)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if dropped != 1 {
		t.Fatalf("expected 1 dropped session, got %d", dropped)
	}

	sessions, err := m.ListSessions("wt1")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected the dead session's row to be pruned, got %+v", sessions)
	}
}

func TestReconcileKeepsLiveSessions(t *testing.T) {
	requireTmux(t)
	st := newTestStore(t)
	m := &Manager{Store: st}

	ts, err := m.CreateSession("wt1", t.TempDir(), "shell", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	t.Cleanup(func() { _ = TmuxCmd("kill-session", "-t", ts.TmuxSessionName).Run() })

	dropped, err := Reconcile(st)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if dropped != 0 {
		t.Fatalf("expected 0 dropped sessions for a live one, got %d", dropped)
	}

	sessions, err := m.ListSessions("wt1")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected the live session's row to survive reconcile, got %+v", sessions)
	}
}

func TestAttachAndResize(t *testing.T) {
	requireTmux(t)
	st := newTestStore(t)
	m := &Manager{Store: st}

	ts, err := m.CreateSession("wt1", t.TempDir(), "shell", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	t.Cleanup(func() { _ = m.CloseSession(ts) })

	f, cmd, err := Attach(ts.TmuxSessionName)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	defer f.Close()

	if err := Resize(f, 100, 40); err != nil {
		t.Fatalf("Resize: %v", err)
	}

	// Give the attach a moment to actually connect, then send a keystroke
	// and confirm the tmux session (not just the pty) received it by
	// checking tmux's own pane contents.
	time.Sleep(200 * time.Millisecond)
	if _, err := f.Write([]byte("echo hello-from-test\n")); err != nil {
		t.Fatalf("write to pty: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	out, err := TmuxCmd("capture-pane", "-p", "-t", ts.TmuxSessionName).Output()
	if err != nil {
		t.Fatalf("capture-pane: %v", err)
	}
	if !strings.Contains(string(out), "hello-from-test") {
		t.Fatalf("expected pane output to contain the echoed text, got:\n%s", out)
	}

	_ = cmd.Process.Kill()
}

// TestCreateSessionSetsXtermKeys confirms CreateSession leaves tmux's global
// xterm-keys/extended-keys options "on" — the setting that lets a Ctrl/Alt
// modified arrow key survive tmux's own translation instead of being
// collapsed to a bare arrow before it reaches the pane's shell. See
// docs/terminal-keybindings.md. Checked against the real tmux server, same
// as every other global-option regression test in this file.
func TestCreateSessionSetsXtermKeys(t *testing.T) {
	requireTmux(t)
	st := newTestStore(t)
	m := &Manager{Store: st}

	ts, err := m.CreateSession("wt1", t.TempDir(), "shell", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	t.Cleanup(func() { _ = m.CloseSession(ts) })

	for _, opt := range []string{"xterm-keys", "extended-keys"} {
		out, err := TmuxCmd("show-options", "-g", opt).Output()
		if err != nil {
			t.Fatalf("tmux show-options -g %s: %v", opt, err)
		}
		got := strings.TrimSpace(string(out))
		if !strings.HasPrefix(got, opt+" on") {
			t.Fatalf("expected global %s to be \"on\", got %q", opt, got)
		}
	}
}

// TestMouseDragCopyRelaysOSC52 confirms a real mouse-drag selection in
// copy-mode both actually reaches the browser clipboard via OSC 52 and
// visibly says so — not just that tmux's own paste buffer gets set. This
// covers the exact gap from docs/terminal-clipboard.md's "Problem 7":
// "copy-selection-and-cancel" alone (previously bound directly to
// MouseDragEnd1Pane/Enter) fills tmux's paste buffer but never emits
// `\x1b]52;` on this tmux build, even with set-clipboard on — a
// `tmux list-keys` string check on the binding wouldn't have caught that,
// since the binding "worked" in the sense of registering and running; it
// just didn't relay to the client. Verified by feeding genuine SGR mouse
// press/motion/release bytes (the same byte shape a real xterm.js client
// sends) directly into the attached pty and reading raw bytes back off it,
// the same technique used to originally diagnose and fix this bug (see
// term.BindGlobalCopyModeKeys's comment).
func TestMouseDragCopyRelaysOSC52(t *testing.T) {
	requireTmux(t)
	st := newTestStore(t)
	m := &Manager{Store: st}

	ts, err := m.CreateSession("wt1", t.TempDir(), "shell", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	t.Cleanup(func() { _ = m.CloseSession(ts) })

	const marker = "OSC52_REGRESSION_TEST_MARKER"
	if err := TmuxCmd("send-keys", "-t", ts.TmuxSessionName, "echo "+marker, "Enter").Run(); err != nil {
		t.Fatalf("seed pane text: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	f, cmd, err := Attach(ts.TmuxSessionName)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	defer f.Close()
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	if err := Resize(f, 80, 24); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Genuine SGR mouse press/drag/release over the row containing marker:
	// button0 press at col5,row1, drag motion, then release — exactly the
	// byte shape a real browser/xterm.js sends on a click-drag.
	for _, seq := range []string{
		"\x1b[<0;5;1M",
		"\x1b[<32;15;1M",
		"\x1b[<32;30;1M",
		"\x1b[<0;30;1m",
	} {
		if _, err := f.Write([]byte(seq)); err != nil {
			t.Fatalf("write mouse sequence: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// The pty's *os.File doesn't support SetReadDeadline on this platform
	// ("file type does not support deadline"), so a blocking Read can't be
	// bounded directly. Read on a goroutine instead and bound the wait with
	// a select/timeout; the deferred f.Close() above unblocks the Read once
	// the test returns, whichever way it exits.
	type readResult struct {
		data []byte
		err  error
	}
	results := make(chan readResult, 64)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				results <- readResult{data: chunk}
			}
			if err != nil {
				results <- readResult{err: err}
				return
			}
		}
	}()

	// Both halves have to show up, for different reasons: the OSC 52 escape
	// is the copy actually reaching the browser (Problem 7/8), and the
	// "chars to clipboard" status message is the user-visible confirmation
	// that a copy happened at all (Problem 9 — without it a successful copy
	// is indistinguishable from a failed one, which caused two separate
	// misdiagnoses). Asserting both means neither can regress silently.
	var out []byte
	timeout := time.After(4 * time.Second)
	for {
		select {
		case r := <-results:
			if r.err != nil {
				t.Fatalf("read ended before both the OSC 52 escape and the copy confirmation appeared (err %v); got %d bytes: %q", r.err, len(out), out)
			}
			out = append(out, r.data...)
			if bytes.Contains(out, []byte("\x1b]52;")) && bytes.Contains(out, []byte("chars to clipboard")) {
				return // both the clipboard relay and its visible confirmation
			}
		case <-timeout:
			t.Fatalf("after a mouse-drag copy, wanted both an OSC 52 escape (found=%v) and a %q status message (found=%v) within 4s; got %d bytes: %q",
				bytes.Contains(out, []byte("\x1b]52;")),
				"chars to clipboard",
				bytes.Contains(out, []byte("chars to clipboard")),
				len(out), out)
		}
	}
}

func TestCreateSessionRunsInitialCommand(t *testing.T) {
	requireTmux(t)
	st := newTestStore(t)
	m := &Manager{Store: st}

	ts, err := m.CreateSession("wt1", t.TempDir(), "shell", "echo hello-from-initial-command")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	t.Cleanup(func() { _ = m.CloseSession(ts) })

	// tmux send-keys is asynchronous from the shell's perspective (it just
	// injects keystrokes) — give the shell inside a moment to actually
	// execute the echoed command before checking the pane.
	var out []byte
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		out, err = TmuxCmd("capture-pane", "-p", "-t", ts.TmuxSessionName).Output()
		if err != nil {
			t.Fatalf("capture-pane: %v", err)
		}
		if strings.Contains(string(out), "hello-from-initial-command") {
			return // success
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("expected the initial command's output in the pane within 3s, got:\n%s", out)
}
