package term

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/creack/pty"
)

// Attach starts `tmux attach-session -t <name>` under a pty and returns the
// pty's master end (for reading tmux's rendered output and writing
// keystrokes into it) plus the underlying *exec.Cmd so the caller can wait
// on / kill it when the client disconnects. Detaching this pty (closing it,
// or the client going away) does NOT kill the tmux session itself — that's
// the entire point: the shell inside tmux keeps running.
func Attach(tmuxSessionName string) (*os.File, *exec.Cmd, error) {
	cmd := exec.Command("tmux", "attach-session", "-t", tmuxSessionName)
	f, err := pty.Start(cmd)
	if err != nil {
		return nil, nil, fmt.Errorf("attach pty to tmux session %s: %w", tmuxSessionName, err)
	}
	return f, cmd, nil
}

// Resize applies new terminal dimensions to an attached pty.
func Resize(f *os.File, cols, rows uint16) error {
	return pty.Setsize(f, &pty.Winsize{Cols: cols, Rows: rows})
}

// CurrentTitle returns tmux's own composed title for the session's active
// pane (session:window:program - <inner title>, per the set-titles comment
// in CreateSession) — empty if tmux hasn't set one. tmux only emits an OSC
// title escape sequence to an attaching client on an actual change, not on
// attach itself, so a client that (re)connects to an already-running
// session (e.g. a page reload while `claude` is running) never sees the
// title sequence that would tell it what's running in the pane until the
// title happens to change again. handleTerminalWS uses this to synthesize
// that OSC sequence once, right after attaching, so the frontend's existing
// onTitleChange-driven tab detection (see web/src/terminalAppDetection.ts)
// gets the right answer immediately instead of showing a stale/generic
// label until something inside the pane happens to touch its title next.
func CurrentTitle(tmuxSessionName string) (string, error) {
	out, err := exec.Command("tmux", "display-message", "-p", "-t", tmuxSessionName, "#{pane_title}").Output()
	if err != nil {
		return "", fmt.Errorf("tmux display-message pane_title: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
