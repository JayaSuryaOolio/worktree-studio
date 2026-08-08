// Package term manages terminal tabs backed by real tmux sessions rather
// than a bare PTY owned by this process. tmux's own server process outlives
// worktree-studio's Go process, so a terminal tab survives a server
// restart: on restart we just attach to the same tmux session again
// instead of needing to serialize/restore any PTY state ourselves. See
// PLAN.md section 3 for the full rationale.
package term

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"worktree-studio/internal/audit"
	"worktree-studio/internal/store"
)

// Manager creates, lists, and closes tmux-backed terminal sessions, and
// keeps the SQLite store's terminal_sessions table in sync with tmux's
// actual live sessions.
type Manager struct {
	Store *store.Store
	Audit *audit.Logger
}

// tmuxNamePrefix namespaces worktree-studio's tmux sessions so Reconcile
// and ListLive never touch tmux sessions the user created themselves.
const tmuxNamePrefix = "wts-"

// CreateSession starts a new detached tmux session rooted at worktreePath,
// records it in the store, and audit-logs the creation. If initialCommand
// is non-empty, it's typed into the session and run immediately (via
// `tmux send-keys`) — e.g. auto-starting `claude` in a freshly created
// worktree's first terminal. Passed as a real argv element to tmux (not
// through a shell), so there's no shell-injection concern regardless of
// its content.
func (m *Manager) CreateSession(worktreeID, worktreePath, tabLabel, initialCommand string) (store.TerminalSession, error) {
	id := newSessionID()
	tmuxName := tmuxNamePrefix + id

	cmd := exec.Command("tmux", "new-session", "-d", "-s", tmuxName, "-c", worktreePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return store.TerminalSession{}, fmt.Errorf("tmux new-session: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	// set-clipboard is a tmux SERVER option (no per-session scope exists),
	// so this affects every tmux session on the machine, including ones
	// the user created themselves outside worktree-studio — deliberate,
	// not an oversight: it only enables OSC 52 relay (copy-mode selections
	// get forwarded up the pty chain as a clipboard-set escape sequence),
	// doesn't change any keybinding or mouse behavior, and is a commonly
	// recommended tmux setting on its own merits. See
	// docs/terminal-clipboard.md for why this exists at all. Best-effort:
	// a failure here doesn't affect the session's usability as a shell,
	// only whether tmux copy-mode's clipboard integration works.
	_ = exec.Command("tmux", "set-option", "-g", "set-clipboard", "on").Run()

	if initialCommand != "" {
		sendKeys := exec.Command("tmux", "send-keys", "-t", tmuxName, initialCommand, "Enter")
		if err := sendKeys.Run(); err != nil {
			// Not fatal to the whole operation — the session exists and is
			// usable, it just didn't get its auto-run command. Reflected in
			// the audit log below (initial_command field omitted) rather
			// than failing session creation over it.
			initialCommand = ""
		}
	}

	ts := store.TerminalSession{
		ID:              id,
		WorktreeID:      worktreeID,
		TmuxSessionName: tmuxName,
		TabLabel:        tabLabel,
	}
	if err := m.Store.AddTerminalSession(ts); err != nil {
		// Don't leak the tmux session we just created if we can't record it.
		_ = exec.Command("tmux", "kill-session", "-t", tmuxName).Run()
		return store.TerminalSession{}, fmt.Errorf("save terminal session: %w", err)
	}

	if m.Audit != nil {
		fields := map[string]any{
			"terminal_id": id, "worktree_id": worktreeID, "tmux_session_name": tmuxName, "tab_label": tabLabel,
		}
		if initialCommand != "" {
			fields["initial_command"] = initialCommand
		}
		_ = m.Audit.Log(audit.EventTerminalCreate, fields)
	}
	return ts, nil
}

// ListSessions returns the terminal sessions recorded for a worktree.
func (m *Manager) ListSessions(worktreeID string) ([]store.TerminalSession, error) {
	return m.Store.ListTerminalSessions(worktreeID)
}

// CloseSession kills the underlying tmux session and removes the store row.
// Killing an already-dead tmux session is not an error — the row is
// removed regardless, since the goal is "this tab is gone" either way.
func (m *Manager) CloseSession(ts store.TerminalSession) error {
	_ = exec.Command("tmux", "kill-session", "-t", ts.TmuxSessionName).Run()
	if err := m.Store.RemoveTerminalSession(ts.ID); err != nil {
		return fmt.Errorf("remove terminal session row: %w", err)
	}
	if m.Audit != nil {
		_ = m.Audit.Log(audit.EventTerminalClose, map[string]any{
			"terminal_id": ts.ID, "worktree_id": ts.WorktreeID, "tmux_session_name": ts.TmuxSessionName,
		})
	}
	return nil
}

// ListLiveTmuxSessionNames returns the set of tmux session names that are
// actually running right now. If the tmux server itself isn't running at
// all, tmux exits non-zero with "no server running on ..." — that's not a
// real error for our purposes, just an empty set.
func ListLiveTmuxSessionNames() (map[string]bool, error) {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		if isNoServerRunning(err) {
			return map[string]bool{}, nil
		}
		return nil, fmt.Errorf("tmux list-sessions: %w", err)
	}
	live := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			live[line] = true
		}
	}
	return live, nil
}

func isNoServerRunning(err error) bool {
	var exitErr *exec.ExitError
	if ee, ok := err.(*exec.ExitError); ok {
		exitErr = ee
	}
	return exitErr != nil && bytes.Contains(exitErr.Stderr, []byte("no server running"))
}

// Reconcile drops terminal_sessions rows whose backing tmux session no
// longer exists (e.g. tmux itself was killed, or the session ended on its
// own). It never kills a live tmux session — only prunes stale DB rows.
// Call this once at server startup, before any client can list sessions.
func Reconcile(st *store.Store) (dropped int, err error) {
	live, err := ListLiveTmuxSessionNames()
	if err != nil {
		return 0, err
	}
	all, err := st.ListAllTerminalSessions()
	if err != nil {
		return 0, fmt.Errorf("list terminal sessions: %w", err)
	}
	for _, ts := range all {
		if !live[ts.TmuxSessionName] {
			if err := st.RemoveTerminalSession(ts.ID); err != nil {
				return dropped, fmt.Errorf("remove stale terminal session %s: %w", ts.ID, err)
			}
			dropped++
		}
	}
	return dropped, nil
}
