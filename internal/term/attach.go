package term

import (
	"fmt"
	"os"
	"os/exec"

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
