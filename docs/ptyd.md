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
| `create` | id, cwd, argv, env, cols, rows, restore | start argv in a new PTY; `restore` seeds its buffer from the saved log |
| `list` | | `{"version":"<build>","sessions":[{id,pid,cwd,created,last_activity,clients}]}` |
| `write` | id, data (base64) | write to the PTY |
| `resize` | id, cols, rows | TIOCSWINSZ |
| `kill` | id, keep | SIGHUP the process group, SIGKILL after 2s; deletes the saved log unless `keep` (then saves it first) |
| `attach` | id | switch to framed streaming (below) |
| `shutdown` | | save every buffer, kill every session, exit |

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
`ptyd.sock.lock` makes concurrent starts safe.

**Saved scrollback.** Every 15s, and on `shutdown`/SIGTERM/SIGINT, the daemon
writes each changed buffer to `<socket dir>/ptyd/<id>.log`
(`~/.worktree-studio/ptyd/`, atomic temp+rename). The log is deleted when a
session is killed deliberately or its shell exits normally; a shell killed
by a signal (reboot, daemon crash) keeps it.

**Restore.** Startup reconcile never starts the daemon. A ptyd row whose
session is gone is restored if it has a saved log — a fresh `$SHELL -l` in
the worktree, same terminal id, buffer seeded with the old scrollback plus a
dim `— restored after restart —` line — and pruned otherwise. Opening a tab
whose session is gone but saved does the same. If the row has a claude
session id, `claude --resume <id>` is typed into the new shell: the server
records it every minute (and before archive/restart) from Claude Code's
session registry, matched to the terminal by process ancestry
(`terminal_sessions.claude_session_id`).

**Archive** suspends a worktree's ptyd terminals (`kill` with `keep`: shells
end, rows and logs stay); **unarchive** restores them all. Reconcile leaves
archived worktrees' rows alone. tmux terminals are untouched by archive.

**Idle reaper** (opt-in): Settings → Terminal → "Idle ptyd terminals", stored
as `ptyd_idle_reap_days` (0 = off). Hourly, it closes ptyd terminals with no
attached client and no output for N days whose shell is at its prompt (the
tty's foreground pgrp is the shell). One running something (claude, a dev
server) is never killed, only listed as busy. API:
`GET /api/idle-ptyd-sessions?days=N`, `POST /api/idle-ptyd-sessions/reap`.

**Upgrades.** The daemon keeps the binary it started from. Its build id
(sha256 of the executable) is in `list`; when it differs from the server's,
`GET /api/settings/terminal` reports `ptyd_outdated: true` and Settings →
Terminal shows **Restart ptyd** (`POST /api/settings/terminal/ptyd-restart`,
or `wts ptyd restart`). Restart = `shutdown` (everything saved), then each
ptyd tab is restored on a new daemon. Running programs stop (claude is
resumed); open web tabs reconnect on reload.

## How to toggle

The backend applies to **new** terminals only. Every `terminal_sessions` row
records its `backend` (`tmux` | `ptyd`, existing rows migrate as `tmux`) and
stays on it, so switching never strands an open tab.

- Settings UI: Settings → Terminal → backend.
- API: `GET /api/settings/terminal` →
  `{"backend":"ptyd","env_override":false,"idle_reap_days":0,"ptyd_running":true,"ptyd_outdated":false}`;
  `PUT` `{"backend":...}` and/or `{"idle_reap_days":N}` to change them (stored
  in the `settings` table, keys `terminal_backend`, `ptyd_idle_reap_days`).
- Env: `WORKTREE_STUDIO_TERMINAL_BACKEND=ptyd|tmux` pins it for that server
  run and wins over the stored setting (`env_override: true`).

Default is `ptyd` (since 2026-10-24) when nothing is stored or pinned; pick
`tmux` in Settings to go back. The setting is read on every create, so it
takes effect for the next new terminal without a restart.

### What each piece does per backend

| | tmux | ptyd |
|---|---|---|
| create | `tmux new-session` + global options | `ptyd create` of `$SHELL -l` (env: TERM=xterm-256color, COLORTERM=truecolor, no TMUX) |
| initial command | `send-keys ... Enter` | written to the PTY as `cmd\r` |
| close | `kill-session` | `kill` (SIGHUP, then SIGKILL) |
| web attach | `tmux attach` under a server-side pty | ptyd `attach`; replay framed by replay-start/done |
| title on reattach | synthesized from `#{pane_title}` | carried by the replay |
| current path | `#{pane_current_path}` | cwd of the tty's foreground pgrp (`ps -o tpgid` → `lsof`/`/proc`) |
| startup reconcile | rows whose tmux session is gone | rows absent from `ptyd list`: restored if a log is saved, else pruned |
| archive / unarchive | nothing | shells suspended (rows + logs kept) / restored |
| orphans | `wts-*` sessions with no row | daemon sessions with no row, listed as `ptyd:<id>`, same 7-day guard |
| `wts attach <id>` | falls back to `tmux attach -t wts-<id>` | direct to the daemon, Ctrl+\ detaches |

## Known limitations

- A shared session has one PTY size; the last client to resize wins (same as
  tmux `window-size latest`).
- Restore brings back scrollback and a fresh shell in the worktree root, not
  running programs, the shell's cwd or its history. Saves are every 15s, so a
  hard crash loses up to that much output. Only claude is resumed, and only
  if it was running at the last once-a-minute check.
- The TUI and `wts attach` don't restore a gone session themselves; server
  startup, opening the tab in the web UI, unarchive and restart do.
- Restarting a daemon from before build ids (no `shutdown` op) SIGTERMs it:
  its shells are lost without scrollback, tabs come back as fresh shells.
- Replay is raw bytes, not a screen snapshot: if the 4 MiB window has
  trimmed away a mode switch (alternate screen, bracketed paste, mouse
  tracking) or the last title, the reattached client won't know it. A
  `charmbracelet/x/vt` snapshot is the upgrade if this bites.
- Copying a mouse selection works in a plain shell but not inside `claude`:
  claude turns on mouse tracking, so the drag goes to claude instead of
  xterm.js's selection. Accepted for now (reported in manual testing).

## Criteria for making ptyd the default

Flipped on 2026-10-24 after daily use on ptyd since it merged; the
remaining items below are tracked in the ptyd lifecycle PR (persisted
buffers, reboot restore, idle reaping, restart-on-upgrade) rather than
blocking the default.

- Daily use on ptyd for a couple of weeks (web, desktop and TUI) with no
  lost sessions or stray input from replayed queries.
- Reattach of a long-running `claude` session renders correctly: if trimmed
  replay loses alternate-screen/mouse modes in practice, add the vt snapshot
  first (see Known limitations).
- ~~A story for daemon upgrades~~: outdated-build detection + Restart ptyd
  (saves and restores tabs). Re-exec with fd handoff would keep programs
  running, if that matters.
- Everything that still says tmux (attention hook, copy/paste docs, the
  Settings dependency check for tmux) re-checked
  against ptyd.

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
- [x] Docs (README, architecture, session-persistence, PROGRESS, PLAN,
      SKILL.md)
- [x] Default flipped to ptyd (tmux still selectable in Settings / env)
- [x] Lifecycle: saved scrollback + restore after reboot/daemon loss (claude
      resumed), archive suspends / unarchive restores, opt-in idle reaper,
      outdated-daemon detection + Restart ptyd (`wts ptyd restart`)

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
- A shell killed by SIGHUP/SIGKILL reports `ProcessState.Exited() == false`,
  which is what tells "the machine went away" (keep the log) from "the user
  typed exit" (delete it).
- `ps -o tpgid= -p <shell pid>` gives the tty's foreground pgrp on macOS and
  Linux: equal to the shell pid means it's at its prompt. Used for both the
  current path and the idle reaper.
- Claude Code's `~/.claude/sessions/<pid>.json` registry plus process
  ancestry is enough to map a ptyd terminal to its claude session; no hook
  change needed.
