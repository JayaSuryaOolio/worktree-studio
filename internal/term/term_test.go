package term

import (
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

	ts, err := m.CreateSession("wt1", t.TempDir(), "shell")
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

	ts, err := m.CreateSession("wt1", t.TempDir(), "shell")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Kill the tmux session out from under the store, simulating tmux
	// dying independently of worktree-studio (e.g. `tmux kill-server`).
	if err := exec.Command("tmux", "kill-session", "-t", ts.TmuxSessionName).Run(); err != nil {
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

	ts, err := m.CreateSession("wt1", t.TempDir(), "shell")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	t.Cleanup(func() { _ = exec.Command("tmux", "kill-session", "-t", ts.TmuxSessionName).Run() })

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

	ts, err := m.CreateSession("wt1", t.TempDir(), "shell")
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

	out, err := exec.Command("tmux", "capture-pane", "-p", "-t", ts.TmuxSessionName).Output()
	if err != nil {
		t.Fatalf("capture-pane: %v", err)
	}
	if !strings.Contains(string(out), "hello-from-test") {
		t.Fatalf("expected pane output to contain the echoed text, got:\n%s", out)
	}

	_ = cmd.Process.Kill()
}
