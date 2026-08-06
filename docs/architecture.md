# Architecture

worktree-studio is a Go HTTP server + embedded React SPA for managing `git worktree`s across one or more registered repos. This doc is updated at the end of every build step in `PLAN.md`; it currently reflects **steps 1–4**.

## Pieces that exist today (steps 1–4)

```
cmd/worktree-studio/main.go   entrypoint: chi router, embeds web/dist, graceful fallback page, reconciles tmux sessions at startup
web/embed.go                  go:embed all:dist (must live next to dist/, embed patterns are dir-relative)
internal/store/                SQLite registry: repos, worktrees, terminal_sessions
internal/gitops/                shells out to `git worktree add/remove/list --porcelain`, `git status`
internal/audit/                 JSONL audit logger, append-only
internal/term/                  tmux-backed terminal session manager: create/list/close, pty attach+resize, startup reconciliation
internal/spotlight/             thin os/exec wrapper around the external `spotlight` CLI (worktree->root mirroring)
internal/api/                   HTTP handlers (repos, worktrees, terminals, spotlight) + ws relay + adjective-noun name generator + slugify/id helpers
web/src/                        Vite + React + TS SPA: RepoPicker, Workspace (status polling), WorktreeList (+ Spotlight toggle, dirty/ahead-behind badges), NewWorktreeDialog, WorktreeDetail, Terminal (xterm.js)
```

### Request flow

1. Browser loads `/` → `RepoPicker.tsx` calls `GET /api/repos/`.
2. User adds a repo (`POST /api/repos/` with `{name, path}`). The handler validates the path is a real directory and a real git repo (`git rev-parse --is-inside-work-tree`) via `internal/gitops.IsGitRepo`, then persists it via `internal/store` and appends a `repo.add` line to the audit log.
3. Clicking a repo navigates to `/repo/:id` (`Workspace.tsx`), which loads worktrees via `GET /api/repos/:repoId/worktrees/`.
4. "+ New worktree" opens `NewWorktreeDialog.tsx`, which immediately calls `GET /api/repos/:repoId/worktrees/new-name-suggestion` to prefill an editable adjective-noun name (e.g. `amber-ridge`) from a small embedded Go wordlist (`internal/api/wordlist.go`) — no external dependency needed for this.
5. Submitting calls `POST /api/repos/:repoId/worktrees/` with `{name}`. The handler slugifies the (possibly user-edited) name, uses it as both branch name and directory name, and runs `git worktree add -b <branch> <path>` against the *registered* repo's path — creating the worktree at `~/.worktree-studio/worktrees/<repoId>/<slug>`, deliberately **outside** the source repo tree (avoids the repo's own tooling/.gitignore getting confused by a nested worktree). Persists a `worktrees` row and appends a `worktree.create` audit line.
6. Deleting calls `DELETE /api/repos/:repoId/worktrees/:worktreeId`, which runs `git worktree remove --force <path>` (or without `--force`, surfacing a `409` if the worktree is dirty — see the frontend's confirm-then-retry-with-force flow), deletes the DB row, and appends a `worktree.remove` audit line.
7. Clicking "Open" on a worktree navigates to `/repo/:repoId/worktree/:worktreeId` (`WorktreeDetail.tsx`), which lists that worktree's terminal sessions via `GET .../terminals/`.
8. "+ New Terminal" calls `POST .../terminals/`, which runs `tmux new-session -d -s wts-<id> -c <worktreePath>` (`internal/term.CreateSession`) and persists a `terminal_sessions` row. Closing a tab calls `DELETE .../terminals/:id`, which runs `tmux kill-session` and removes the row.
9. Each open terminal tab (`Terminal.tsx`) opens its own `WebSocket` to `/ws/terminals/:id`. The server (`handleTerminalWS` in `internal/api/terminals.go`) runs `tmux attach-session -t <name>` under a pty (`github.com/creack/pty`) and relays: binary ws messages are raw bytes fed straight into the pty (keystrokes in, tmux's rendered ANSI output out), text ws messages are JSON control frames — currently just `{"type":"resize","cols":N,"rows":N}`, sent whenever xterm.js's fit addon recomputes the terminal size. Closing the ws connection (switching tabs, closing the browser) kills the `tmux attach` client process but **not** the tmux session itself — the shell inside keeps running.
10. **Server-restart persistence**: at startup, `term.Reconcile` lists tmux's actual live sessions (`tmux list-sessions`) and drops any `terminal_sessions` DB rows whose tmux session no longer exists (e.g. tmux itself was killed) — it never kills a live session, only prunes stale rows. Because the tmux server process is independent of worktree-studio's Go process, killing and restarting the Go server does not affect running shells; reconnecting a terminal tab just re-runs `tmux attach-session` against the same still-alive session. See `docs/session-persistence.md` for the full rationale and a manual verification recipe.
11. A worktree row's "Spotlight" cell (`WorktreeList.tsx`) calls `GET .../spotlight/` to show whether that worktree is the one currently mirrored into its repo's root, another worktree is (via `internal/spotlight.StatusForRoot`), or spotlight is unavailable (CLI not installed). "Start" calls `POST .../spotlight/start`, which shells out to the external `spotlight` CLI (`internal/spotlight.Start`, cwd = the worktree path) — a `409` means the root has uncommitted changes and the CLI itself refused, surfaced verbatim rather than retried. "Stop"/the ● button calls `POST .../spotlight/stop`. See `docs/spotlight-sync.md` for what the underlying tool actually does (worktree → root mirroring, not the reverse — a design correction recorded there).
12. Each worktree row's "Status" cell (`GitStatusCell` in `WorktreeList.tsx`) calls `GET .../worktrees/:id/status`, which runs `internal/gitops.Status` (`git status --porcelain=2 --branch`) against that worktree's path and returns `{branch, dirty, has_upstream, ahead, behind}`. `Workspace.tsx` refreshes both this and spotlight status on a 5-second `setInterval` (REST polling, not a ws push — see the note below) so badges stay current without a manual page refresh. `ahead`/`behind` are only meaningful (and only rendered) when `has_upstream` is true — a freshly created worktree's branch typically has no upstream configured at all, in which case they're correctly not shown rather than misleadingly shown as `0`.

### Storage

- **SQLite** at `~/.worktree-studio/studio.db` via `modernc.org/sqlite` (pure Go, no cgo). Tables: `repos(id, name, path)`, `worktrees(id, repo_id, name, branch, path, created_at)`, and `terminal_sessions(id, worktree_id, tmux_session_name, tab_label)`.
- **Audit log** at `~/.worktree-studio/audit.log.jsonl`, one JSON object per line, written by `internal/audit.Logger.Log(event, fields)`. Every mutating handler in `internal/api` calls it (now including `terminal.create` / `terminal.close` / `spotlight.start` / `spotlight.stop`). See `docs/running-locally.md` for how to tail it.
- **Spotlight state** is NOT in worktree-studio's own store at all — the external `spotlight` CLI owns its own state file (which repo root is mirrored from which worktree, tracked by that tool, not queried here directly; `internal/spotlight.List`/`StatusForRoot` parse its `spotlight list` output instead). worktree-studio treats it as an external system to query, not data it's responsible for persisting.

### Frontend embedding

`cmd/worktree-studio/main.go` imports `worktree-studio/web`, a tiny package whose only job is holding `//go:embed all:dist` — go:embed patterns resolve relative to the *file* containing the directive, so the embed can't live in `cmd/worktree-studio` (a different directory from `web/dist`). `web/dist/.gitkeep` is checked into git so the directory (and thus the embed) always exists on a fresh checkout, even before `bun run build` has ever run — `go build ./...` works either way. At runtime, `mountFrontend` checks for a real `index.html` in the embedded FS; if it's missing (only the placeholder is there), it serves a small "run `bun run build`" HTML page instead of a blank/broken UI. Once a real build exists, static assets are served with an SPA fallback to `index.html` for client-side routes like `/repo/:id`.

### What's explicitly NOT built yet

Per `PLAN.md`'s build order, none of the following exist yet — don't assume they do:
- Monaco file editor (step 5)
- Git diff view + comment-to-agent (step 6 / TODO)
- Multiple terminal tabs opened via a single "+ New tab" click are NOT multiplexed over one shared websocket — each terminal tab has its own dedicated `/ws/terminals/:id` connection (a deliberate simplification vs. PLAN.md's originally-sketched single-multiplexed-channel design; see `docs/session-persistence.md`).
- Spotlight is a thin wrapper — it does NOT reimplement any sync logic, has no manifest/staging/lock scheme of its own (that was an earlier, wrong design; see `docs/spotlight-sync.md`'s "design correction" note), and there's no per-workspace/per-subdirectory scoping — it always mirrors an entire worktree into an entire root.
- The monitoring dashboard (step 4) is REST-polled on a 5s interval, not pushed over a websocket — same simplification pattern as terminals/spotlight above, recorded in `PLAN.md`. There's no server-side push/broadcast mechanism anywhere in this codebase yet.

See `docs/running-locally.md` for how to run this step's server + frontend, and `.claude/skills/worktree-studio/SKILL.md` for the agent-facing usage doc.
