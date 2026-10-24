package term

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"worktree-studio/internal/audit"
	"worktree-studio/internal/ptyd"
	"worktree-studio/internal/store"
)

// Terminal backends. Each terminal_sessions row records the one it was
// created with and stays on it; the setting only picks the backend for new
// terminals. See docs/ptyd.md.
const (
	BackendTmux = "tmux"
	BackendPtyd = "ptyd"

	// BackendSettingKey is the settings-table key for the backend new
	// terminals use.
	BackendSettingKey = "terminal_backend"
	// BackendEnv overrides the setting (for a whole server run).
	BackendEnv = "WORKTREE_STUDIO_TERMINAL_BACKEND"
)

func validBackend(b string) bool { return b == BackendTmux || b == BackendPtyd }

// Backend returns the backend new terminals are created with: BackendEnv if
// set to a valid value (fromEnv true), else the stored setting, else tmux.
func (m *Manager) Backend() (backend string, fromEnv bool) {
	if v := os.Getenv(BackendEnv); validBackend(v) {
		return v, true
	}
	if v, err := m.Store.GetSetting(BackendSettingKey); err == nil && validBackend(v) {
		return v, false
	}
	return BackendTmux, false
}

// SetBackend stores the backend setting for new terminals.
func (m *Manager) SetBackend(b string) error {
	if !validBackend(b) {
		return fmt.Errorf("unknown terminal backend %q", b)
	}
	return m.Store.SetSetting(BackendSettingKey, b)
}

func (m *Manager) ptydClient() (*ptyd.Client, error) {
	if m.Ptyd == nil {
		return nil, fmt.Errorf("ptyd backend not configured")
	}
	return m.Ptyd, nil
}

// createPtydSession is CreateSession for the ptyd backend: a login shell in
// a daemon-owned PTY, with the session id doubling as the ptyd id.
func (m *Manager) createPtydSession(worktreeID, worktreePath, tabLabel, initialCommand string) (store.TerminalSession, error) {
	c, err := m.ptydClient()
	if err != nil {
		return store.TerminalSession{}, err
	}
	id := newSessionID()
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	if err := c.Create(id, worktreePath, []string{shell, "-l"}, ptydEnv(), 0, 0); err != nil {
		return store.TerminalSession{}, err
	}
	if initialCommand != "" {
		// Buffered by the pty until the shell reads it, same as tmux send-keys.
		if err := c.Write(id, []byte(initialCommand+"\r")); err != nil {
			initialCommand = ""
		}
	}
	ts := store.TerminalSession{ID: id, WorktreeID: worktreeID, TabLabel: tabLabel, Backend: BackendPtyd}
	if err := m.Store.AddTerminalSession(ts); err != nil {
		_ = c.Kill(id)
		return store.TerminalSession{}, fmt.Errorf("save terminal session: %w", err)
	}
	if m.Audit != nil {
		fields := map[string]any{"terminal_id": id, "worktree_id": worktreeID, "backend": BackendPtyd, "tab_label": tabLabel}
		if initialCommand != "" {
			fields["initial_command"] = initialCommand
		}
		_ = m.Audit.Log(audit.EventTerminalCreate, fields)
	}
	return ts, nil
}

// ptydEnv is the environment for a ptyd shell: ours with TERM fixed (see
// attachEnv), tmux's markers dropped (the server may itself run inside
// tmux), and truecolor advertised since xterm.js renders it.
func ptydEnv() []string {
	var env []string
	for _, kv := range attachEnv() {
		if !strings.HasPrefix(kv, "TMUX=") && !strings.HasPrefix(kv, "TMUX_PANE=") && !strings.HasPrefix(kv, "COLORTERM=") {
			env = append(env, kv)
		}
	}
	return append(env, "COLORTERM=truecolor")
}

// Alive reports whether a terminal's backing session still exists.
func (m *Manager) Alive(ts store.TerminalSession) bool {
	if ts.Backend != BackendPtyd {
		return HasSession(ts.TmuxSessionName)
	}
	c, err := m.ptydClient()
	if err != nil {
		return false
	}
	_, err = c.Get(ts.ID)
	return err == nil
}

// Attachment is one client's connection to a terminal: read output, write
// input, resize, Close to detach (the session keeps running).
type Attachment struct {
	io.ReadWriteCloser
	// Replay is the ptyd scrollback to show before live output; nil for tmux,
	// which repaints the screen itself on attach.
	Replay []byte
	resize func(cols, rows uint16) error
	stream *ptyd.Stream
}

func (a *Attachment) Resize(cols, rows uint16) error { return a.resize(cols, rows) }

// ExitCode is the process's exit status once Read has returned io.EOF, or -1
// if unknown (always for tmux).
func (a *Attachment) ExitCode() int {
	if a.stream == nil {
		return -1
	}
	return a.stream.ExitCode
}

type tmuxAttachment struct {
	*os.File
	cmd *exec.Cmd
}

func (t tmuxAttachment) Close() error {
	err := t.File.Close()
	_ = t.cmd.Process.Kill() // the `tmux attach` client, NOT the tmux session
	return err
}

// Attach connects to a terminal on whichever backend it lives on.
func (m *Manager) Attach(ts store.TerminalSession) (*Attachment, error) {
	if ts.Backend != BackendPtyd {
		f, cmd, err := Attach(ts.TmuxSessionName)
		if err != nil {
			return nil, err
		}
		return &Attachment{ReadWriteCloser: tmuxAttachment{f, cmd}, resize: func(c, r uint16) error { return Resize(f, c, r) }}, nil
	}
	c, err := m.ptydClient()
	if err != nil {
		return nil, err
	}
	replay, st, err := c.Attach(ts.ID)
	if err != nil {
		return nil, err
	}
	return &Attachment{ReadWriteCloser: st, Replay: replay, resize: st.Resize, stream: st}, nil
}

// CurrentPath is the cwd of the terminal's foreground process.
func (m *Manager) CurrentPath(ts store.TerminalSession) (string, error) {
	if ts.Backend != BackendPtyd {
		return CurrentPath(ts.TmuxSessionName)
	}
	c, err := m.ptydClient()
	if err != nil {
		return "", err
	}
	s, err := c.Get(ts.ID)
	if err != nil {
		return "", err
	}
	pid := s.Pid
	// The tty's foreground process group (e.g. a `claude` started from the
	// shell), like tmux's pane_current_path; the shell itself if that fails.
	if out, err := exec.Command("ps", "-o", "tpgid=", "-p", strconv.Itoa(pid)).Output(); err == nil {
		if fg, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil && fg > 0 {
			if cwd, err := processCwd(fg); err == nil {
				return cwd, nil
			}
		}
	}
	return processCwd(pid)
}

func processCwd(pid int) (string, error) {
	if runtime.GOOS == "linux" {
		return os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid))
	}
	out, err := exec.Command("lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn").Output()
	if err != nil {
		return "", fmt.Errorf("lsof cwd of %d: %w", pid, err)
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if line := sc.Text(); strings.HasPrefix(line, "n") {
			return line[1:], nil
		}
	}
	return "", fmt.Errorf("no cwd for pid %d", pid)
}

// Reconcile prunes terminal rows whose backing session is gone, on both
// backends. Never starts the ptyd daemon: if it isn't running, every ptyd
// row is stale (its shells died with it).
func (m *Manager) Reconcile() (dropped int, err error) {
	if c, cerr := m.ptydClient(); cerr == nil {
		live := map[string]bool{}
		sessions, lerr := c.List()
		if lerr != nil {
			return 0, lerr
		}
		for _, s := range sessions {
			live[s.ID] = true
		}
		all, lerr := m.Store.ListAllTerminalSessions()
		if lerr != nil {
			return 0, lerr
		}
		for _, ts := range all {
			if ts.Backend == BackendPtyd && !live[ts.ID] {
				if err := m.Store.RemoveTerminalSession(ts.ID); err != nil {
					return dropped, err
				}
				dropped++
			}
		}
	}
	n, err := Reconcile(m.Store)
	return dropped + n, err
}

// ptydOrphanPrefix marks ptyd sessions in the orphan listing, whose names
// are otherwise tmux session names.
const ptydOrphanPrefix = "ptyd:"

// FindOrphans is FindOrphanTmuxSessions plus live ptyd sessions with no
// terminal row (named "ptyd:<id>"), under the same activity safeguard.
func (m *Manager) FindOrphans(minAge time.Duration) ([]OrphanTmuxSession, error) {
	orphans, err := FindOrphanTmuxSessions(m.Store, minAge)
	if err != nil {
		return nil, err
	}
	c, cerr := m.ptydClient()
	if cerr != nil {
		return orphans, nil
	}
	sessions, err := c.List()
	if err != nil {
		return nil, err
	}
	all, err := m.Store.ListAllTerminalSessions()
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, ts := range all {
		known[ts.ID] = true
	}
	cutoff := time.Now().Add(-minAge)
	for _, s := range sessions {
		if !known[s.ID] {
			orphans = append(orphans, OrphanTmuxSession{Name: ptydOrphanPrefix + s.ID, LastActivity: s.LastActivity, Protected: s.LastActivity.After(cutoff)})
		}
	}
	return orphans, nil
}

// KillOrphans kills every unprotected orphan from FindOrphans; see
// KillOrphanTmuxSessions for why there's no override.
func (m *Manager) KillOrphans(minAge time.Duration) (killed, protected []string, err error) {
	orphans, err := m.FindOrphans(minAge)
	if err != nil {
		return nil, nil, err
	}
	for _, o := range orphans {
		if o.Protected {
			protected = append(protected, o.Name)
			continue
		}
		var kerr error
		if id, ok := strings.CutPrefix(o.Name, ptydOrphanPrefix); ok {
			kerr = m.Ptyd.Kill(id)
		} else {
			kerr = TmuxCmd("kill-session", "-t", o.Name).Run()
		}
		if kerr == nil {
			killed = append(killed, o.Name)
		}
	}
	return killed, protected, nil
}
