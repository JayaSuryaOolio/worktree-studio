package ptyterm

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"worktree-studio/internal/ptyd"
	"worktree-studio/tui/domain"
)

// Attaches to a throwaway tmux session (unique name, killed on cleanup) and
// checks output shows up in the rendered frame and typed bytes reach it.
func TestAttachRendersAndForwardsInput(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	const name = "wts-tui-ptyterm-test"
	exec.Command("tmux", "kill-session", "-t", name).Run()
	if out, err := exec.Command("tmux", "new-session", "-d", "-s", name, "-x", "60", "-y", "10", "cat").CombinedOutput(); err != nil {
		t.Fatalf("tmux new-session: %v %s", err, out)
	}
	t.Cleanup(func() { exec.Command("tmux", "kill-session", "-t", name).Run() })

	s, err := Attacher{}.Attach(domain.TerminalSession{TmuxName: name}, 60, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.Write([]byte("hello-pty\r")); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	for !strings.Contains(s.Render(), "hello-pty") {
		select {
		case <-s.Updates():
		case <-deadline:
			t.Fatalf("typed text never rendered:\n%s", s.Render())
		}
	}
}

// When the session ends, the screen releases its pty by itself: leaking one
// per exited shell used up the system's ptys ("fork failed: Device not
// configured" for every new tmux window).
func TestEndedSessionReleasesPty(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	const name = "wts-tui-ptyterm-exit-test"
	exec.Command("tmux", "kill-session", "-t", name).Run()
	if out, err := exec.Command("tmux", "new-session", "-d", "-s", name, "cat").CombinedOutput(); err != nil {
		t.Fatalf("tmux new-session: %v %s", err, out)
	}
	s, err := Attacher{}.Attach(domain.TerminalSession{TmuxName: name}, 60, 10)
	if err != nil {
		t.Fatal(err)
	}
	exec.Command("tmux", "kill-session", "-t", name).Run()
	deadline := time.After(5 * time.Second)
	for open := true; open; {
		select {
		case _, open = <-s.Updates():
		case <-deadline:
			t.Fatal("updates never closed")
		}
	}
	if _, err := s.(*screen).conn.Write([]byte("x")); !errors.Is(err, os.ErrClosed) {
		t.Fatal("pty still open after the session ended")
	}
}

// A ptyd session: the replay shows up, and the emulator's answer to a cursor
// query in the replay never reaches the session (cat echoing it as "^[[").
func TestPtydAttachReplaysWithoutReplies(t *testing.T) {
	dir, err := os.MkdirTemp("", "ptyd") // short: macOS caps socket paths
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	ln, err := net.Listen("unix", filepath.Join(dir, "s"))
	if err != nil {
		t.Fatal(err)
	}
	go ptyd.NewServer().Serve(ln)
	c := &ptyd.Client{Socket: ln.Addr().String()}
	t.Cleanup(func() { c.Kill("t"); ln.Close() })
	if err := c.Create("t", "", []string{"/bin/sh", "-c", "printf 'past-''output\\033[6n\\n'; exec cat"}, nil, 60, 10); err != nil {
		t.Fatal(err)
	}
	// Wait until the query is in the scrollback (nobody answers it live).
	for i := 0; ; i++ {
		replay, st, err := c.Attach("t")
		if err != nil {
			t.Fatal(err)
		}
		st.Close()
		if strings.Contains(string(replay), "past-output") {
			break
		}
		if i == 50 {
			t.Fatalf("output never buffered: %q", replay)
		}
		time.Sleep(100 * time.Millisecond)
	}

	s, err := Attacher{PtydSocket: c.Socket}.Attach(domain.TerminalSession{ID: "t", Backend: "ptyd"}, 60, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Write([]byte("typed\r")); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	for !strings.Contains(s.Render(), "typed") || !strings.Contains(s.Render(), "past-output") {
		select {
		case <-s.Updates():
		case <-deadline:
			t.Fatalf("replay or typed text never rendered:\n%s", s.Render())
		}
	}
	if strings.Contains(s.Render(), "^[") {
		t.Fatalf("replay's cursor-position reply reached the session:\n%s", s.Render())
	}
}
