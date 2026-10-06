package ptyterm

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

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
	if _, err := s.(*screen).ptmx.Write([]byte("x")); !errors.Is(err, os.ErrClosed) {
		t.Fatal("pty still open after the session ended")
	}
}
