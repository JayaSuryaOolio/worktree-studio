# Session persistence (tmux-backed terminals)

## Why tmux instead of a bare PTY

worktree-studio's terminal tabs don't run a shell directly under a PTY owned by the Go process. Instead, each tab is a real **tmux session**, and the Go server merely `tmux attach-session`s to it under a pty. This is the "best available working solution now, custom later if needed" call from `PLAN.md`: tmux already solves "a shell session survives its controlling process dying" correctly — scrollback, running jobs, signal handling, all of it — and reimplementing that from scratch in Go would just be redoing tmux, badly.

The practical payoff: **restarting the worktree-studio server does not kill anything running inside a terminal tab.** A long-running build, a `claude` session, a `tail -f` — none of it notices the Go process going away, because none of it was a child of that process. tmux's own server process is independent and keeps running.

## How it works

- `internal/term.CreateSession` runs `tmux new-session -d -s wts-<id> -c <worktreePath>` and records `{id, worktree_id, tmux_session_name, tab_label}` in the `terminal_sessions` SQLite table.
- Each browser tab's `Terminal.tsx` opens a websocket to `/ws/terminals/:id`. The server attaches via `tmux attach-session -t <name>` under a `creack/pty`-managed pty and relays bytes both directions (see `docs/architecture.md` for the exact wire protocol).
- Closing the websocket (switching tabs, closing the browser, network hiccup) kills the `tmux attach` *client* process, not the session — functionally identical to detaching a tmux client at a real terminal. The shell inside keeps running.
- At server startup (`cmd/worktree-studio/main.go`), `term.Reconcile` calls `tmux list-sessions` and drops any DB rows whose `tmux_session_name` isn't actually live anymore (e.g. someone ran `tmux kill-server`, or the machine rebooted and tmux itself didn't survive that). It never kills a session that IS live — reconciliation is purely about pruning stale bookkeeping, never destructive to a running shell.

## Manually verifying persistence

1. Create a worktree and a terminal tab for it (via the UI, or `POST /api/repos/:repoId/worktrees/:worktreeId/terminals/`).
2. Run something long-lived in it, e.g. `sleep 300` or start `claude`.
3. Kill the Go server process (`pkill -f cmd/worktree-studio` or Ctrl-C).
4. Confirm the tmux session is still there: `tmux list-sessions | grep wts-`.
5. Restart the Go server.
6. Reload the terminal tab in the browser (or reconnect the websocket) — the running command is still going, and scrollback from before the restart is still visible (tmux re-renders its current pane state to any newly-attached client).

## Known simplification vs. the original plan sketch

`PLAN.md`'s section 3 originally sketched a single websocket multiplexing every terminal tab's I/O by `sessionId`. The actual implementation gives each terminal tab its own dedicated `/ws/terminals/:id` connection instead — simpler to reason about, and there was no other consumer of a shared channel yet (worktree status pushes and file-change events, the other planned users of a multiplexed channel, don't exist yet either — steps 4 and 5). If a real need for a single shared connection shows up later (e.g. to cut down on connection count at scale), that's a contained change inside `Terminal.tsx` and `handleTerminalWS`, not a rearchitecture.

## Not handled yet (by design, deferred)

- **Orphan tmux sessions**: if you create sessions outside worktree-studio's `wts-` naming convention, or restore from a machine where the DB was wiped but tmux sessions survived, those live tmux sessions are simply invisible to worktree-studio — `Reconcile` only prunes rows for dead sessions, it doesn't adopt live-but-unknown ones. Not needed for v1.
- **tmux itself dying** (not just worktree-studio): if `tmux kill-server` runs or the machine reboots, the actual shells are gone too — `Reconcile` correctly notices and prunes those rows, but there's obviously no scrollback/process to recover at that point.
- **Multiple terminal tabs opened via "+ New tab"** (the browser-tab button) each get their own full page load and their own terminal websockets for whatever tabs exist in that worktree — this is intentional per PLAN.md's "a second browser tab is enough" multi-repo/multi-window design, not a bug.
