# ptyd — PTY daemon terminal backend

Living doc for the `ptyd` terminal backend. Updated with every commit on the
`ptyd` branch.

## Goal / motivation

Terminals are backed by tmux today (`wts-<id>` sessions) purely so shells
survive the API server restarting. tmux brings baggage the app has to fight:
its own mouse mode and copy-mode swallow xterm.js's native selection and
scrollback, clipboard needs OSC 52 workarounds (docs/terminal-clipboard.md),
and a server restart that loses the tmux socket loses everything anyway.

ptyd is the VS Code "ptyHost" split instead: a small long-lived daemon
(`wts ptyd`) owns every PTY and keeps a bounded output buffer per session.
The API server, `wts attach` and the TUI are thin clients over a unix socket.
Clients get raw PTY output, so xterm.js has real scrollback and native
selection with no tmux in the way.

## Design

Code: `internal/ptyd` (daemon + client), `internal/term/ptyd.go` (backend
seam).

### Socket protocol

Unix socket at `~/.worktree-studio/ptyd.sock` (mode 0600; override with
`WORKTREE_STUDIO_PTYD_SOCKET`). Each connection starts with one JSON request
line and one JSON response line (`{"error": "..."}` on failure):

| op | fields | effect |
|----|--------|--------|
| `create` | id, cwd, argv, env, cols, rows | start argv in a new PTY |
| `list` | | `{"sessions":[{id,pid,cwd,created,last_activity}]}` |
| `write` | id, data (base64) | write to the PTY |
| `resize` | id, cols, rows | TIOCSWINSZ |
| `kill` | id | SIGHUP the process group, SIGKILL after 2s |
| `attach` | id | switch to framed streaming (below) |

After an `attach` response the connection carries frames
`[type byte][uint32 BE length][payload]`:

- daemon → client: `o` output, `r` replay done (live from here), `x` process
  exited (payload = decimal exit code; then the daemon closes).
- client → daemon: `i` input, `s` resize (cols, rows as uint16 BE).

Closing the connection detaches; the session keeps running. A session is
removed from the daemon as soon as its process exits.

### Buffer and replay

Each session keeps the last `MaxScrollback` (4 MiB) bytes of output
(`scrollback.go`, append-and-trim). On attach the daemon snapshots the buffer
and subscribes the client under one lock, sends the snapshot as `o` frames,
then `r`, then live output — nothing lost or doubled in between. If older
output was trimmed, the replay starts at a clean boundary: after the first
newline, else at the first ESC, else at the first UTF-8 rune start
(`cleanStart`). A client more than 256 chunks behind is dropped rather than
stalling the shell; it can reattach.

### Dropping terminal replies during replay

Replaying output that contains queries (DSR/CPR `ESC[6n`, DA `ESC[c`, OSC
colour queries...) makes the client's emulator answer them, and those answers
would be typed into the shell. Each client drops them while replaying:

- **Web**: the server sends text frames `{"type":"replay-start"}` /
  `{"type":"replay-done"}` around the replay. Terminal.tsx drops `onData`
  that looks like a terminal reply until xterm.js has finished parsing the
  replay (`term.write("", cb)` after replay-done).
- **TUI** (`tui/infra/ptyterm`): after writing the replay into the vt
  emulator it writes a marker into the emulator's reply pipe and discards
  emulator replies until the marker comes back out — exact, since the pipe is
  ordered.
- **`wts attach`**: the real terminal's replies arrive on stdin; for a short
  window after the replay, stdin chunks matching `ptyd.IsTerminalReply` are
  dropped.

### Lifecycle

The API server auto-starts the daemon on the first ptyd `create` (spawns
`<own executable> ptyd` with `Setsid`, no inherited stdio, log at
`~/.worktree-studio/ptyd.log`), so it outlives the server. A `flock` on
`ptyd.sock.lock` makes concurrent starts safe. Startup reconcile never starts
the daemon: if it isn't running, ptyd terminal rows are pruned.

## How to toggle

The backend applies to **new** terminals only. Every `terminal_sessions` row
records its `backend` (`tmux` | `ptyd`, existing rows migrate as `tmux`) and
stays on it, so switching never strands an open tab.

- Settings UI: Settings → Terminal → backend.
- API: `GET /api/settings/terminal` → `{"backend":"tmux","env_override":false}`;
  `PUT` the same shape to change it (stored in the `settings` table, key
  `terminal_backend`).
- Env: `WORKTREE_STUDIO_TERMINAL_BACKEND=ptyd|tmux` pins it for that server
  run and wins over the stored setting (`env_override: true`).

Default is `tmux`.

### What each piece does per backend

| | tmux | ptyd |
|---|---|---|
| create | `tmux new-session` + global options | `ptyd create` of `$SHELL -l` (env: TERM=xterm-256color, COLORTERM=truecolor, no TMUX) |
| initial command | `send-keys ... Enter` | written to the PTY as `cmd\r` |
| close | `kill-session` | `kill` (SIGHUP, then SIGKILL) |
| web attach | `tmux attach` under a server-side pty | ptyd `attach`; replay framed by replay-start/done |
| title on reattach | synthesized from `#{pane_title}` | carried by the replay |
| current path | `#{pane_current_path}` | cwd of the tty's foreground pgrp (`ps -o tpgid` → `lsof`/`/proc`) |
| startup reconcile | rows whose tmux session is gone | rows absent from `ptyd list` (all, if the daemon is down) |
| orphans | `wts-*` sessions with no row | daemon sessions with no row, listed as `ptyd:<id>`, same 7-day guard |
| `wts attach <id>` | falls back to `tmux attach -t wts-<id>` | direct to the daemon, Ctrl+\ detaches |

## Known limitations

- A shared session has one PTY size; the last client to resize wins (same as
  tmux `window-size latest`).
- Buffers live in daemon memory: they don't survive a reboot or a ptyd crash.
  Claude sessions fall back to `claude --resume`.
- The daemon keeps running the binary it was started from; after upgrading
  worktree-studio, restart it (`pkill -f 'wts ptyd'` kills every ptyd shell).
- Replay is raw bytes, not a screen snapshot: if the 4 MiB window has
  trimmed away a mode switch (alternate screen, bracketed paste, mouse
  tracking) or the last title, the reattached client won't know it. A
  `charmbracelet/x/vt` snapshot is the upgrade if this bites.

## Criteria for making ptyd the default

TODO.

## Progress

- [x] Daemon core + socket protocol + client (`internal/ptyd`), unit +
      integration tests
- [x] Backend toggle (settings table + env), `terminal_sessions.backend`,
      term.Manager seam, backend-aware reconcile/orphans/cwd/ws, settings
      API, `wts ptyd`, `wts attach`
- [x] Web client: replay-frame handling + reply dropping, 10k-line
      scrollback, Settings → Terminal toggle
- [x] TUI (`tui/infra/ptyterm`): attaches to ptyd sessions directly, replay
      fed into the vt emulator, its replies dropped up to a marker
- [ ] Docs (README, architecture, PROGRESS, PLAN, SKILL.md)

## Learnings

- tmux not being installed used to make `ListLiveTmuxSessionNames` fail;
  under ptyd that's a normal setup, so `exec.ErrNotFound` now reads as "no
  sessions" (covers reconcile and orphans at once).
- macOS caps unix socket paths at 104 bytes; `t.TempDir()` paths exceed it,
  so tests use `os.MkdirTemp("", "ptyd")`.
- vt's reply pipe is an `io.Pipe`: a Read never spans two Writes, so the TUI's
  replay marker always comes out as one exact chunk — no parsing needed.
- `go test -race ./tui/infra/ptyterm` already fails on main: vt's
  `Emulator.Close` and `Read` race on an unguarded `closed` flag.
