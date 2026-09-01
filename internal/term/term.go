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

	cmd := TmuxCmd("new-session", "-d", "-s", tmuxName, "-c", worktreePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return store.TerminalSession{}, fmt.Errorf("tmux new-session: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	// set-clipboard is a tmux SERVER option (no per-session scope exists),
	// so it affects every tmux session on the machine, including ones the
	// user created themselves outside worktree-studio — deliberate, not an
	// oversight. It makes tmux's own copy-mode (Ctrl+b [ , select, Enter)
	// emit an OSC 52 clipboard-set escape sequence, which
	// @xterm/addon-clipboard in Terminal.tsx catches and writes to the real
	// browser clipboard. Best-effort — a failure here doesn't affect the
	// session's usability as a shell, only whether tmux's own copy-mode
	// relays to the browser clipboard.
	_ = TmuxCmd("set-option", "-g", "set-clipboard", "on").Run()
	// Deliberately NOT "allow-passthrough on" here, despite trying it (see
	// docs/terminal-clipboard.md's "Problem 5"): it would have let
	// `claude`'s own DCS-passthrough-wrapped OSC 52 clipboard write reach
	// the browser, but allow-passthrough is all-or-nothing in tmux — it
	// also lets through any OTHER program's DCS-wrapped sequences, in any
	// pane, not just `claude`'s. Per direct user report this revived a
	// mouse-reporting feature in an unrelated plain shell (very likely
	// from a shell prompt/plugin that also uses passthrough-wrapped
	// DECSET sequences for tmux compatibility), turning ordinary
	// mouse-wheel scroll into cursor-key bytes that bash/zsh's readline
	// interpreted as history navigation instead. Reverted before this was
	// even confirmed fully fixing the `claude` case it targeted — breaking
	// basic scrolling for every shell is a materially worse regression
	// than the clipboard gap it was meant to close. `claude`'s own OSC 52
	// copy remains a known, open, deliberately unfixed problem; DON'T
	// re-enable allow-passthrough globally to chase it — any real fix
	// would need to be scoped to a single pane/program, which plain tmux
	// options can't do.
	//
	// "mouse on" IS deliberately set here (see
	// docs/terminal-clipboard.md's "Problem 4" and "Problem 6"), even
	// though an earlier revert briefly turned it off. Turning it off traded
	// one regression for a worse one: tmux's own client-attach protocol
	// always sends the outer terminal an enter-alternate-screen sequence
	// on every fresh attach (its own full-repaint mechanism, unconditional,
	// unrelated to whatever's actually running in the pane), so from
	// xterm.js's point of view a tmux-attached session is *always*
	// "showing the alternate buffer, no scrollback" — permanently, for
	// every plain shell too. xterm.js's wheel handler treats that as "this
	// must be a fullscreen app like vim that wants arrow keys, not a
	// scrollback view" and converts wheel scroll into literal cursor-key
	// bytes, UNLESS tmux's own mouse-tracking protocol is active, in which
	// case xterm.js instead sends the wheel event as an SGR mouse report
	// for the pane to handle itself — which is exactly what tmux's default
	// root-table binding needs to route wheel-up into copy-mode and scroll
	// tmux's own (real) scrollback. So "mouse on" isn't optional here: it's
	// the only way wheel-scroll works at all in a tmux-attached xterm.js
	// session, plain shell or not. The cost — xterm.js's own native
	// click-drag text selection normally being suppressed whenever mouse
	// tracking is active — is the known, accepted tradeoff from Problem 4,
	// worked around by relying on xterm.js's own built-in Shift/Option
	// force-selection convention rather than disabling mouse tracking.
	// CorrectGlobalMouseAndPassthroughSettings (below) brings mouse back to
	// on and allow-passthrough back to off for an installation that
	// already ran the briefly-reverted, mouse-off version of this feature.
	_ = TmuxCmd("set-option", "-g", "mouse", "on").Run()

	// tmux does NOT forward a pane's OSC 0/2 title-set escape sequence to
	// the outer terminal by default — `set-titles` is off out of the box,
	// so a program running inside tmux (e.g. `claude`, which sets its
	// title to "<status glyph> Claude Code") never reaches xterm.js's
	// onTitleChange at all; the sequence is swallowed by tmux itself.
	// Verified empirically: piped raw bytes through a real tmux session
	// with set-titles off and confirmed zero OSC title sequences reached
	// the attaching pty; with it on, tmux emits its own composed title
	// (session:window:program - "<inner title>") on every title change,
	// which still contains the inner program's title as a substring — see
	// web/src/terminalAppDetection.ts, which matches on that substring
	// rather than an exact string for exactly this reason.
	_ = TmuxCmd("set-option", "-g", "set-titles", "on").Run()

	// tmux only translates a client's raw Ctrl/Alt-modified arrow-key bytes
	// (e.g. xterm.js's own default `\x1b[1;5C` for Ctrl+Right) into the
	// equivalent sequence it forwards on to the pane's actual shell when
	// "xterm-keys" is on — without it, tmux can collapse a modified arrow
	// key down to a bare one before the shell ever sees it, silently
	// dropping the modifier that readline/zle word-navigation bindings
	// (e.g. `bindkey '^[[1;5C' forward-word`) key off of. Default-on since
	// tmux 2.4, but set explicitly here rather than trusted, same posture
	// as every other tmux option in this function. "extended-keys" (tmux
	// 3.2+) improves fidelity for combinations xterm-keys alone doesn't
	// fully disambiguate; harmless to enable even for programs that never
	// ask for it. See docs/terminal-keybindings.md.
	_ = TmuxCmd("set-option", "-g", "xterm-keys", "on").Run()
	_ = TmuxCmd("set-option", "-g", "extended-keys", "on").Run()

	// Server-global key tables (see the function's own comment for the full
	// mechanism) — set here so a freshly created session is correct even if
	// the server process itself was never restarted since these changed.
	BindGlobalCopyModeKeys()

	if initialCommand != "" {
		sendKeys := TmuxCmd("send-keys", "-t", tmuxName, initialCommand, "Enter")
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
		_ = TmuxCmd("kill-session", "-t", tmuxName).Run()
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
	_ = TmuxCmd("kill-session", "-t", ts.TmuxSessionName).Run()
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
	out, err := TmuxCmd("list-sessions", "-F", "#{session_name}").Output()
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

// CorrectGlobalMouseAndPassthroughSettings brings tmux's global "mouse" and
// "allow-passthrough" server options back to the settings CreateSession
// itself now sets ("mouse on", "allow-passthrough off"), undoing whichever
// of two earlier, regressed states an existing installation might still be
// in (see CreateSession's own comment and docs/terminal-clipboard.md's
// "Problem 4"/"Problem 5"/"Problem 6"):
//   - a stale "mouse off" breaks wheel-scroll in every tmux session, plain
//     shells included — see Problem 6 — because tmux's own client-attach
//     protocol makes xterm.js permanently believe it has no scrollback,
//     and only an active mouse-tracking protocol stops xterm.js converting
//     wheel scroll into literal cursor-key bytes.
//   - a stale "allow-passthrough on" revives mouse-reporting features in
//     unrelated shells (anything else using tmux's DCS-passthrough
//     mechanism, not just the one program — `claude` — it was meant to
//     help), which surfaced as ordinary mouse-wheel scroll turning into
//     shell history navigation in a plain shell that never ran that
//     program at all.
//
// Both are tmux server-wide options, so they take effect for every existing
// session immediately, not just new ones — but the fix would otherwise only
// actually land the next time CreateSession happened to run, and an
// installation that already hit either regression shouldn't have to open a
// brand new terminal just to get a working one. Call this once at server
// startup (see cmd/worktree-studio/main.go), same idiom as Reconcile.
// Best-effort, same as every other tmux set-option call in this package:
// failure (e.g. no tmux server running yet, so there's nothing to correct
// anyway) doesn't block startup.
func CorrectGlobalMouseAndPassthroughSettings() {
	_ = TmuxCmd("set-option", "-g", "mouse", "on").Run()
	_ = TmuxCmd("set-option", "-g", "allow-passthrough", "off").Run()
}

// BindGlobalCopyModeKeys rebinds the copy-mode keys that a mouse-drag
// release and Enter-in-copy-mode trigger, so a copy actually reaches the
// browser's clipboard (and visibly says so). Called both from CreateSession
// and once at server startup — tmux key tables are server-global, so an
// installation whose tmux server predates this shouldn't have to open a
// brand new terminal to get a working copy; a restart is enough. Same
// call-once-at-startup posture and best-effort error handling as
// CorrectGlobalMouseAndPassthroughSettings.
//
// tmux's own DEFAULT bindings for both keys are "copy-pipe-and-cancel", not
// "copy-selection-and-cancel" — confirmed via `tmux list-keys`. Both copy
// into tmux's own paste buffer, but copy-pipe hands the text to an external
// shell command (`copy-command`, empty by default here), a no-op as far as
// the browser is concerned. Verified empirically: with the stock bindings,
// dragging to select sets tmux's paste buffer (`tmux show-buffer` proves
// it) but no OSC 52 frame ever reaches the browser. Rebinding both the
// emacs and vi copy-mode tables (mode-keys can be either) fixes it without
// touching `copy-command` or anyone's own tmux.conf.
//
// Plain "copy-selection-and-cancel" alone turned out not to be enough
// either, on this tmux build (3.7b) — see docs/terminal-clipboard.md's
// "Problem 7": it fills the paste buffer but does not itself emit the OSC
// 52 escape, even with set-clipboard on. The missing piece, confirmed with
// an isolated pty harness feeding raw SGR mouse bytes into a throwaway
// session: `set-buffer -w` (the command that DOES emit OSC 52) requires an
// explicit `data` argument — it has no mode that just "relays whatever's
// already in the buffer" — so it has to be re-run with the buffer's own
// current content.
//
// bind-key's own inline multi-command syntax (`cmd1 \; cmd2`,
// `{ cmd1 ; cmd2 }`) was tried first and rejected: passed as separate argv
// tokens tmux doesn't group them into one bound command at all (`;` there
// ends the whole bind-key invocation and starts a second, immediate,
// bind-TIME command instead of a second run-TIME one — `tmux list-keys`
// showed only the first command ever got bound); passed as a single string
// it hits tmux's own "syntax error" on this build. run-shell sidesteps that
// entirely by handing the sequence to a real shell. `#{client_tty}` is a
// tmux format specifier, expanded by tmux itself before the string reaches
// the shell — not a shell variable.
//
// The final display-message exists purely to fix a real UX trap, not for
// the copy itself. "copy-selection-and-cancel" exits copy-mode the instant
// it copies, which clears the visual highlight — so a SUCCESSFUL drag-copy
// looks exactly like a failed one ("the selection just disappears when I
// let go"). That cost real debugging time twice: the copy was working and
// the vanishing highlight was mistaken for the copy failing. A brief
// status-line confirmation makes the success visible. "-l" prints the
// message literally so tmux doesn't try to interpret anything in it as a
// format specifier; "${#B}" is ordinary POSIX shell parameter expansion
// (string length), which survives tmux's own format expansion untouched
// because it contains no "#{" sequence — verified against a real tmux
// server, not assumed. "-d 1500" sets this one message's duration
// explicitly rather than relying on (or mutating) the global display-time
// server option, whose 750ms default is short enough to miss and which is
// exactly the kind of thing a user may have deliberately tuned themselves.
func BindGlobalCopyModeKeys() {
	copySelectionAndRelay := "tmux send-keys -X copy-selection-and-cancel; " +
		"B=$(tmux show-buffer); " +
		"tmux set-buffer -w -t '#{client_tty}' \"$B\"; " +
		"tmux display-message -l -d 1500 -c '#{client_tty}' \"Copied ${#B} chars to clipboard\""
	for _, table := range []string{"copy-mode", "copy-mode-vi"} {
		for _, key := range []string{"MouseDragEnd1Pane", "Enter"} {
			_ = TmuxCmd("bind-key", "-T", table, key, "run-shell", copySelectionAndRelay).Run()
		}
	}
}

// CorrectGlobalKeyEncodingSettings brings tmux's global "xterm-keys" and
// "extended-keys" server options up to what CreateSession itself now sets,
// for an installation whose tmux server has been running since before this
// existed (see CreateSession's own comment and docs/terminal-keybindings.md).
// Both are tmux server-wide options, so this takes effect for every existing
// session immediately, not just new ones — same idiom and same
// call-once-at-startup posture as CorrectGlobalMouseAndPassthroughSettings.
func CorrectGlobalKeyEncodingSettings() {
	_ = TmuxCmd("set-option", "-g", "xterm-keys", "on").Run()
	_ = TmuxCmd("set-option", "-g", "extended-keys", "on").Run()
}
