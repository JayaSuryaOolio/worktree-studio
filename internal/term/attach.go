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
	cmd := TmuxCmd("attach-session", "-t", tmuxSessionName)
	// Force a known-capable TERM for this attaching client rather than
	// inheriting whatever the Go server process's own ambient TERM happens
	// to be (a login shell, a background/launchd context, an editor task
	// runner — all different, none guaranteed). tmux's own "xterm-keys"
	// translation (see term.go's CreateSession) needs a terminfo entry that
	// actually describes the modified-arrow-key sequences (`\x1b[1;5C` etc.)
	// xterm.js sends; a wrong or unset inherited TERM is exactly the kind of
	// thing that silently breaks that translation. See
	// docs/terminal-keybindings.md.
	cmd.Env = attachEnv()
	f, err := pty.Start(cmd)
	if err != nil {
		return nil, nil, fmt.Errorf("attach pty to tmux session %s: %w", tmuxSessionName, err)
	}
	return f, cmd, nil
}

// attachEnv returns the current environment with TERM overridden (not just
// appended — a duplicate TERM entry would leave which one "wins" up to
// whichever C library reads envp, which isn't guaranteed) to a value both
// tmux and its pane's own shell reliably support xterm-style modified-key
// sequences under.
func attachEnv() []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "TERM=") {
			env = append(env, kv)
		}
	}
	return append(env, "TERM=xterm-256color")
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
	out, err := TmuxCmd("display-message", "-p", "-t", tmuxSessionName, "#{pane_title}").Output()
	if err != nil {
		return "", fmt.Errorf("tmux display-message pane_title: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CurrentPath returns the tmux pane's own idea of its foreground process's
// current working directory (tmux's `#{pane_current_path}`, kept live by
// tmux itself as the shell cd's around — no polling of /proc or lsof
// needed). Used by handleGetTerminalCwd for a one-shot, on-open check of
// whether a terminal has drifted outside its worktree (e.g. someone `cd
// ..`d out of it) — deliberately not repeated on a timer: a single check
// right when the pane is opened is what the sidebar/tab UI actually needs
// this for, and this project already leans away from adding more polling
// than it has to (see RepoContext.tsx's StatusScheduler for the one place
// polling really does carry its weight — a live status many rows show at
// once, not a one-off directory check).
func CurrentPath(tmuxSessionName string) (string, error) {
	out, err := TmuxCmd("display-message", "-p", "-t", tmuxSessionName, "#{pane_current_path}").Output()
	if err != nil {
		return "", fmt.Errorf("tmux display-message pane_current_path: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
