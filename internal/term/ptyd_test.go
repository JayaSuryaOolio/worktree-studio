package term

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"worktree-studio/internal/ptyd"
	"worktree-studio/internal/store"
)

// newPtydManager is a Manager whose ptyd client talks to an in-process
// daemon on a short temp socket (macOS caps socket paths at 104 bytes).
func newPtydManager(t *testing.T) *Manager {
	t.Helper()
	t.Setenv(BackendEnv, "")
	dir, err := os.MkdirTemp("", "ptyd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	c := &ptyd.Client{Socket: filepath.Join(dir, "s")}
	servePtyd(t, c, "")
	return &Manager{Store: newTestStore(t), Ptyd: c}
}

// servePtyd runs an in-process daemon for c, reporting version (or the real
// BuildID if "") and saving scrollback next to the socket.
func servePtyd(t *testing.T, c *ptyd.Client, version string) {
	t.Helper()
	ln, err := net.Listen("unix", c.Socket)
	if err != nil {
		t.Fatal(err)
	}
	srv := ptyd.NewServer()
	srv.Dir = ptyd.LogDir(c.Socket)
	if version != "" {
		srv.Version = version
	}
	go srv.Serve(ln)
	t.Cleanup(func() {
		sessions, _ := c.List()
		for _, s := range sessions {
			_ = c.Kill(s.ID)
		}
		ln.Close()
	})
}

func TestBackendSettingAndEnvOverride(t *testing.T) {
	m := newPtydManager(t)
	if b, _ := m.Backend(); b != BackendTmux {
		t.Fatalf("default backend = %q, want tmux", b)
	}
	if err := m.SetBackend("screen"); err == nil {
		t.Fatal("SetBackend accepted an unknown backend")
	}
	if err := m.SetBackend(BackendPtyd); err != nil {
		t.Fatal(err)
	}
	if b, env := m.Backend(); b != BackendPtyd || env {
		t.Fatalf("Backend() = %q, %v after setting ptyd", b, env)
	}
	t.Setenv(BackendEnv, BackendTmux)
	if b, env := m.Backend(); b != BackendTmux || !env {
		t.Fatalf("Backend() = %q, %v, want the env override", b, env)
	}
}

// With the toggle on ptyd, CreateSession lands in the daemon and every
// backend-aware Manager method follows the row's backend.
func TestPtydBackendSessionLifecycle(t *testing.T) {
	m := newPtydManager(t)
	if err := m.SetBackend(BackendPtyd); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", "/bin/sh")
	wtPath := t.TempDir()
	ts, err := m.CreateSession("wt1", wtPath, "shell", "echo from-''initial")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if ts.Backend != BackendPtyd || ts.TmuxSessionName != "" {
		t.Fatalf("row = %+v, want a ptyd row with no tmux name", ts)
	}
	if got, _ := m.Store.GetTerminalSession(ts.ID); got.Backend != BackendPtyd {
		t.Fatalf("stored backend = %q", got.Backend)
	}
	if !m.Alive(ts) {
		t.Fatal("session not alive in ptyd")
	}

	att, err := m.Attach(ts)
	if err != nil {
		t.Fatal(err)
	}
	out := append([]byte(nil), att.Replay...)
	deadline := time.Now().Add(10 * time.Second)
	buf := make([]byte, 4096)
	for !bytes.Contains(out, []byte("from-initial")) && time.Now().Before(deadline) {
		n, err := att.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil {
			break
		}
	}
	att.Close()
	if !bytes.Contains(out, []byte("from-initial")) {
		t.Fatalf("initial command output missing: %q", out)
	}

	cwd, err := m.CurrentPath(ts)
	want, _ := filepath.EvalSymlinks(wtPath)
	if got, _ := filepath.EvalSymlinks(cwd); err != nil || got != want {
		t.Fatalf("CurrentPath = %q, %v; want %q", cwd, err, want)
	}

	if n, _, err := m.Reconcile(); err != nil || n != 0 {
		t.Fatalf("Reconcile dropped %d (%v) with the session alive", n, err)
	}
	orphans, err := m.FindOrphans(0)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range orphans {
		if strings.HasSuffix(o.Name, ts.ID) {
			t.Fatalf("live ptyd session with a row reported as orphan: %+v", o)
		}
	}

	// Gone from the daemon behind our back (crash, exit): reconcile prunes.
	if err := m.Ptyd.Kill(ts.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50 && m.Alive(ts); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if n, _, err := m.Reconcile(); err != nil || n != 1 {
		t.Fatalf("Reconcile dropped %d (%v), want the dead ptyd row", n, err)
	}
}

// After the daemon goes away with the scrollback saved (restart, reboot),
// Reconcile brings the tab back under the same id, resuming its claude.
func TestReconcileRestoresSavedPtydSession(t *testing.T) {
	m := newPtydManager(t)
	if err := m.SetBackend(BackendPtyd); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", "/bin/sh")
	wt, _ := m.Store.GetWorktree("wt1")
	ts, err := m.CreateSession(wt.ID, wt.Path, "shell", "echo before-''restart")
	if err != nil {
		t.Fatal(err)
	}
	waitForOutput(t, m, ts, "before-restart")
	if err := m.Store.SetTerminalClaudeSession(ts.ID, "abc-123"); err != nil {
		t.Fatal(err)
	}
	if err := m.Ptyd.Shutdown(); err != nil {
		t.Fatal(err)
	}

	servePtyd(t, m.Ptyd, "")
	ts, _ = m.Store.GetTerminalSession(ts.ID)
	if dropped, restored, err := m.Reconcile(); err != nil || dropped != 0 || restored != 1 {
		t.Fatalf("Reconcile = %d dropped, %d restored, %v", dropped, restored, err)
	}
	out := waitForOutput(t, m, ts, "claude --resume abc-123")
	if !strings.Contains(out, "before-restart") || !strings.Contains(out, "restored after restart") {
		t.Fatalf("restored terminal output = %q", out)
	}
}

// waitForOutput attaches to ts until its replay plus live output contains want.
func waitForOutput(t *testing.T, m *Manager, ts store.TerminalSession, want string) string {
	t.Helper()
	att, err := m.Attach(ts)
	if err != nil {
		t.Fatal(err)
	}
	defer att.Close()
	out := append([]byte(nil), att.Replay...)
	deadline := time.Now().Add(10 * time.Second)
	buf := make([]byte, 4096)
	for !bytes.Contains(out, []byte(want)) && time.Now().Before(deadline) {
		n, err := att.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil {
			break
		}
	}
	if !bytes.Contains(out, []byte(want)) {
		t.Fatalf("output lacks %q: %q", want, out)
	}
	return string(out)
}
