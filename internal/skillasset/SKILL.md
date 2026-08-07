---
name: worktree-studio
description: How to run and use worktree-studio, a local Go+React tool for managing git worktrees across registered repos (repo registration, worktree create/remove, audit log). Use when asked to start worktree-studio, register a repo with it, or create/remove a worktree through it.
---

# worktree-studio

`worktree-studio` is a small local web tool for managing `git worktree`s across one or more repos: register a repo once, then create/remove worktrees through a dashboard instead of hand-running `git worktree add/remove`. It's meant to be driven by both humans and agents.

**Status as of this section (steps 1–4 of `PLAN.md`): repo registration, worktree create/remove, tmux-backed terminals, spotlight, and the dirty/ahead-behind status dashboard all exist.** The following are explicitly **NOT built yet** — don't assume they exist or try to use them:
- No Monaco file editor (planned: step 5)
- No git diff view or "send comment to agent" flow (planned: step 6 / TODO)

Also important: spotlight does **not** install dependencies into a worktree. A freshly created worktree has only what `git worktree add` gives it — if you need `node_modules` there directly (rather than running things from the mirrored root), you still run your own install/build step.

## Installing worktree-studio

There's no installer script today — setup is a few manual commands, and this section exists so an agent can do first-time setup from the skill alone rather than having to separately discover `docs/running-locally.md`.

**Prerequisites** (check before starting; installing them is out of scope for this skill — surface which are missing and stop rather than guessing at install commands for the user's OS):
- Go 1.21+ (`go version`) — the module uses `modernc.org/sqlite`, a pure-Go driver, so no cgo/sqlite3 headers are needed.
- [Bun](https://bun.sh) (`bun --version`) — used as the frontend package manager/runner instead of npm.
- `git` on `PATH` (`git --version`) — every worktree operation shells out to the real binary.
- `tmux` on `PATH` (`tmux -V`) — terminal tabs are real tmux sessions; see "Using terminals" below for why. Without it, terminal creation fails outright (not a soft-degrade like spotlight below).
- Optional: the standalone `spotlight` CLI (`command -v spotlight`, `github.com/JayaSuryaOolio/spotlight`) plus its own `fswatch` dependency. worktree-studio runs fine without it — spotlight's endpoints just report `{"available": false}` / `503` until it's installed. See "Using Spotlight" below.

**First-time setup:**

```bash
cd ~/work/worktree-studio         # or wherever this project's checkout lives
cd web && bun install && bun run build && cd ..   # builds web/dist/, embedded into the Go binary
go build -o worktree-studio ./cmd/worktree-studio
./worktree-studio                 # foreground, so you can see logs and Ctrl-C it — see "Debugging: server logs" below
```

`go build ./...` alone (before the frontend is ever built) still succeeds — `web/dist/` ships a placeholder so the `go:embed` directive always has something, and the server serves a "run `bun run build`" page instead of crashing. You only need the frontend build for the real UI.

Verify it worked: `curl http://localhost:8787/api/repos/` should return `[]` (or your existing registered repos, if `~/.worktree-studio/studio.db` already has data from a prior run — this state persists across restarts by design).

There's currently no dependency-status report on install/startup (e.g. "tmux: ✓, spotlight: not found, this skill: ✓ installed") — that's a recorded `PLAN.md` TODO (a possible future `worktree-studio doctor` check), not built yet. For now, check each prerequisite above by hand if something isn't working as expected.

## Starting the server

```bash
cd ~/work/worktree-studio
go run ./cmd/worktree-studio          # dev, or:
go build -o worktree-studio ./cmd/worktree-studio && ./worktree-studio   # built binary
```

Listens on `http://localhost:8787` by default (`WORKTREE_STUDIO_ADDR` env var overrides). The frontend must be built at least once for the real UI to render (`cd web && bun install && bun run build`) — see `docs/running-locally.md`. Until then the server still starts fine and serves a placeholder page telling you to build the frontend.

## Registering a repo

Via the UI: go to `/`, enter an absolute path to an existing git repo, optionally a display name, submit.

Via the API directly (e.g. from a script or agent):

```bash
curl -X POST http://localhost:8787/api/repos/ \
  -H "Content-Type: application/json" \
  -d '{"name": "adelaide", "path": "/Users/you/conductor/workspaces/pos/adelaide"}'
```

The path must be a real directory and a real git working tree (checked via `git rev-parse --is-inside-work-tree`); a repo already registered at that exact path is rejected with 409. The response includes the repo's `id`, which every subsequent worktree call needs.

## Creating a worktree

The create dialog prefills a random **adjective-noun** name (e.g. `amber-ridge`, Docker-container-name style) from `GET /api/repos/:repoId/worktrees/new-name-suggestion` — this is just a convenience default; edit the text field before submitting if you want a different name. Whatever name ends up submitted is slugified and used as **both** the git branch name and the worktree's directory name.

```bash
# get a suggested name (optional — you can pick your own)
curl http://localhost:8787/api/repos/<repoId>/worktrees/new-name-suggestion

# create
curl -X POST http://localhost:8787/api/repos/<repoId>/worktrees/ \
  -H "Content-Type: application/json" \
  -d '{"name": "amber-ridge"}'
```

This runs `git worktree add -b amber-ridge <path>` against the registered repo, placing the new worktree at `~/.worktree-studio/worktrees/<repoId>/amber-ridge` — **outside** the source repo's own directory tree (deliberate: keeps the main repo's `.gitignore`/tooling from getting confused by a nested worktree). List worktrees for a repo with `GET /api/repos/:repoId/worktrees/`.

## Archiving a worktree (not "delete" anymore)

Via the UI: each worktree row's kebab menu ("⋮") has **"Archive"** — this replaced "Delete" as the everyday action. Archiving is a pure visibility flag: it hides the worktree from the normal list, but does **not** touch git at all — the worktree checkout, its branch, and anything recorded against it (a claude session, see below) all stay exactly as they were on disk. There's a confirm prompt explaining this before it happens.

Via the API:

```bash
curl -X POST http://localhost:8787/api/repos/<repoId>/worktrees/<worktreeId>/archive
curl -X POST http://localhost:8787/api/repos/<repoId>/worktrees/<worktreeId>/unarchive   # reverse it
```

There's currently no UI to browse archived worktrees — that's planned as part of a future settings-modal datagrid (bulk-manage worktrees across repos, filtered by repo/status), not built yet. To find one again in the meantime, query the API directly:

```bash
curl http://localhost:8787/api/repos/<repoId>/worktrees/     # active only by default
```

(The store layer supports filtering `ListWorktrees` by any set of statuses, but the REST endpoint doesn't expose a status query param yet — another piece the settings modal will need.)

Real, destructive deletion (`git worktree remove --force` + removing the registry row) still exists at the API layer, it's just not reachable from the kebab menu anymore:

```bash
curl -X DELETE http://localhost:8787/api/repos/<repoId>/worktrees/<worktreeId>
```

This does not touch the branch itself (only the worktree checkout) and, unlike archive, is not reversible.

## Debugging: server logs

The server logs to stdout/stderr (structured, via `log/slog`) — always run it in a foreground terminal you can see, not backgrounded/detached, so you have the logs and a way to Ctrl-C it. If a request fails ("failed to create git worktree", "failed to save worktree record", etc.), the corresponding `s.Log.Error(...)` line with the real underlying error is right there in that terminal — check it before guessing. If you suspect an orphaned/forgotten instance is running (e.g. an error you can't explain, or the UI behaving as if state exists that you don't recognize): `lsof -iTCP -sTCP:LISTEN | grep 8787` finds it by port; `ps -p <pid> -o command,lstart` shows what it actually is and when it started before you kill it.

## Debugging: the audit log

Every mutating action (repo add, worktree create, worktree remove, and every mutating action added in later steps) writes one JSON line to:

```
~/.worktree-studio/audit.log.jsonl
```

If something happened that you're trying to explain ("did that worktree actually get created?", "when was this repo registered?"), check there first:

```bash
tail -f ~/.worktree-studio/audit.log.jsonl      # watch live
cat ~/.worktree-studio/audit.log.jsonl | jq .    # pretty-print everything
```

Example lines:

```json
{"ts":"2026-08-06T18:36:03.004484Z","event":"worktree.create","repo_id":"97cc943ecf3a0613","worktree_id":"5bb4a682c09f0a28","name":"amber-ridge","branch":"amber-ridge","path":"/Users/you/.worktree-studio/worktrees/97cc943ecf3a0613/amber-ridge"}
```

Other useful local state:
- SQLite registry: `~/.worktree-studio/studio.db`
- Created worktrees: `~/.worktree-studio/worktrees/<repoId>/<name>/`

`rm -rf ~/.worktree-studio` resets all registry/audit state (does not remove worktrees already created on disk — use `git worktree remove` or the UI for those first).

## Using terminals

Click "Open" on a worktree row to get to its detail page (`/repo/:repoId/worktree/:worktreeId`). Terminals are arranged via **dockview**, not a plain tab strip — "+ New Terminal ▾" opens a dropdown with three placement actions:

- **New tab** — adds it as a tab within the currently active group; selecting that tab shows it full-size, other tabs in the same group hidden (classic single-visible-at-a-time behavior).
- **Split right** / **Split down** — adds it as a new tile shown *simultaneously* alongside the existing one. Drag the boundary between tiles to resize them (dockview's native split-view behavior).

Each starts a real shell rooted in that worktree's directory — run anything in it, including `claude` itself; there's no special agent framing, it's just a shell. There is deliberately **no OS-level popout** into a separate browser window — that was considered and explicitly ruled out, not deferred.

**The arrangement itself persists** — reload the page, or kill and restart the whole worktree-studio server, and the same split/tab layout comes back (verified both ways against a real server). It's saved server-side (`GET/PUT /api/repos/<repoId>/worktrees/<worktreeId>/layout`, debounced ~500ms after any drag/resize/tab-switch), not in browser storage — it'll follow you to a different browser or machine too.

Via the API:

```bash
# create a terminal
curl -X POST http://localhost:8787/api/repos/<repoId>/worktrees/<worktreeId>/terminals/ \
  -H "Content-Type: application/json" -d '{"tab_label": "main"}'

# list terminals for a worktree
curl http://localhost:8787/api/repos/<repoId>/worktrees/<worktreeId>/terminals/

# close one (kills the shell inside it)
curl -X DELETE http://localhost:8787/api/repos/<repoId>/worktrees/<worktreeId>/terminals/<terminalId>
```

Each terminal is actually a **tmux session** (named `wts-<terminalId>`) that the server attaches to — not a bare process the server owns directly. The load-bearing consequence: **restarting worktree-studio's server does not kill anything running in a terminal**. A long build, a `claude` session, whatever — it keeps running, and reattaching (reload the tab, or just leave it open through a restart) picks the session back up with scrollback intact. See `docs/session-persistence.md` for the full explanation and a manual restart-survival test you can run yourself.

If you need to poke at a terminal's underlying tmux session directly (e.g. to debug something the UI isn't showing correctly), it's a completely normal tmux session:

```bash
tmux list-sessions | grep wts-
tmux attach -t wts-<terminalId>       # attach from your own real terminal, alongside worktree-studio's ws client
tmux capture-pane -p -t wts-<terminalId>   # dump the current pane contents without attaching
```

The "⧉ New tab" button on a worktree's detail page just opens the same page in a new browser tab (`window.open`) — it's the mechanism for the multi-repo story too: open a different repo's workspace in another tab, no special multi-repo UI needed.

## Using Spotlight

Spotlight mirrors a worktree's source files into its repo's **root checkout** — continuously, while active — so you can build/run from the root path (which already has `node_modules`/build output installed) and have it always reflect whichever worktree is "in focus." It does **not** copy dependencies into the worktree itself; see the caveat at the top of this file.

In the UI: the worktree list has a "Spotlight" column per row. "Start" begins mirroring that worktree into its repo's root; once active, the row shows "● in focus — Stop." If a *different* worktree of the same repo is already the active mirror, that row's button reads "Start (will replace active mirror)" — starting one automatically stops the other, matching the underlying tool's own one-at-a-time design.

Via the API:

```bash
curl http://localhost:8787/api/repos/<repoId>/worktrees/<worktreeId>/spotlight/          # status
curl -X POST http://localhost:8787/api/repos/<repoId>/worktrees/<worktreeId>/spotlight/start
curl -X POST http://localhost:8787/api/repos/<repoId>/worktrees/<worktreeId>/spotlight/stop
```

`start` returns `409` if the repo's root checkout has uncommitted changes — that's the underlying `spotlight` CLI itself refusing, on purpose, so it never clobbers real work sitting in the root. Commit or stash there first, then retry. This is real, unforced behavior: it will happen for genuinely dirty repos, not just in tests.

Requires the standalone `spotlight` CLI to actually be installed (`github.com/JayaSuryaOolio/spotlight`, plus its own `fswatch` dependency) — if it's missing, the status endpoint reports `{"available": false}` rather than erroring, and start/stop return `503`. See `docs/spotlight-sync.md` for the full design (including a correction: this used to be planned backwards before the real tool was found) and for debugging directly against the CLI (`spotlight list`, etc.) if something looks off.

## Monitoring dashboard (dirty / ahead-behind)

Every worktree row has a "Status" column showing whether it's dirty (uncommitted changes/untracked files) and, if its branch has a configured upstream and actually diverges from it, an ahead/behind indicator (e.g. `↑1↓1`). This refreshes automatically every 5 seconds (plain REST polling — there's no websocket push for this) — no manual refresh needed, but also don't expect sub-5-second freshness.

Via the API directly:

```bash
curl http://localhost:8787/api/repos/<repoId>/worktrees/<worktreeId>/status
# {"branch":"amber-ridge","dirty":false,"has_upstream":false,"ahead":0,"behind":0}
```

`has_upstream` is `false` for most freshly created worktrees — `git worktree add -b <branch>` doesn't set up an upstream by itself, so `ahead`/`behind` being `0` in that case doesn't mean "in sync," it means "not tracking anything to compare against." The UI only shows the ahead/behind badge when `has_upstream` is true and there's an actual difference — check `has_upstream` yourself if scripting against this, don't just look at whether ahead/behind are zero.

## UI overhaul: sidebar, command palette, kebab actions

The UI is now built around a persistent left sidebar (not the flat pages described earlier in this file) — every registered repo is listed with its worktrees nested directly underneath, auto-loading whenever the selected repo changes (including a fresh browser tab opened straight at a worktree URL). All per-worktree operational actions (start/stop spotlight, delete) live behind each row's "⋮" kebab menu, not separate always-visible buttons.

**Command palette**: press `Cmd+K` (or `Ctrl+K`) from anywhere in the app to open a fuzzy-searchable palette — jump to any repo or worktree, or trigger "+ Add repo" / "+ New worktree in `<repo>`" without leaving the page you're on. This app is deliberately an SPA in the literal sense: no page-transition navigations for new capability, just modals, dropdowns, and this palette.

Adding a repo or a worktree is a modal now, not a page — reachable via the sidebar's "+" buttons or the command palette, not a separate route.

**Visual design**: the app has one theme, "Command Deck" (dark, mission-control-styled — see `docs/design.md` for the full token system and rationale). Worktree rows in the sidebar are styled as flight strips: status figures and branch names render in monospace like telemetry, page titles use a display face. Row color is deliberately minimal now — every row is neutral gray by default, and only the currently-selected worktree gets a green left-edge accent; there's no separate dirty/clean color-coding on the row itself (per direct feedback that red/green/amber all at once on every row read as noise, not signal — dirty state is still visible via the ahead/behind ticks). There's no theme switcher — that's a recorded `PLAN.md` TODO, not built.

**Deleting a worktree also closes its terminal sessions now** (real tmux kill, not just a DB row disappearing via cascade) — found and fixed while building the dockview arrangement above; before this fix, a deleted worktree's tmux sessions leaked forever with no trace in the DB pointing back to them.

**Creating a worktree auto-starts a `claude` terminal with a known, resumable session id.** Both the sidebar "+"/command-palette flow and the worktree-list "+ New worktree" button create the worktree, then immediately create one terminal in it running `claude --session-id <uuid> -n <name>` as the first command (via `tmux send-keys`, a real argv element — no shell interpolation involved; the uuid is generated client-side via `crypto.randomUUID()` *before* claude ever starts, specifically so it's known and loggable). This also logs a `claude.session.create` audit event with that session id and title — see "Per-worktree audit log" below for why that survives independent of the terminal itself. If the terminal-creation step fails (e.g. tmux unavailable), the worktree itself is still created and usable; you just don't get the auto-started terminal, and the error is logged to the browser console rather than blocking worktree creation. To get the same behavior from the API directly, pass `initial_command` plus the two claude fields when creating a terminal:

```bash
curl -X POST http://localhost:8787/api/repos/<repoId>/worktrees/<worktreeId>/terminals/ \
  -H "Content-Type: application/json" -d '{
    "tab_label": "claude",
    "initial_command": "claude --session-id 11111111-1111-1111-1111-111111111111 -n feature",
    "claude_session_id": "11111111-1111-1111-1111-111111111111",
    "claude_session_title": "feature"
  }'
```

## Per-worktree audit log

Every worktree's kebab menu ("⋮") has a **"View worktree log"** item, and `WorktreeDetail.tsx` (the terminal view) has a "🕐 Log" toolbar button — both open the same dialog: every audit-log event recorded for that specific worktree (worktree create/remove/archive/unarchive, terminal open/close, spotlight start/stop, claude session started), newest first, with a friendly label/icon and a one-line summary (branch name, terminal tab label, claude session id). Click "raw" on any entry to see the full JSON line as actually written to `~/.worktree-studio/audit.log.jsonl`.

**Resuming a claude session later**: find its `claude.session.create` entry in this log (or grep the JSONL file directly for `claude_session_id`), then in a terminal inside that worktree run `claude --resume <the-id>`. There's no one-click "Resume" button yet — this is a manually-actionable record, not an automated flow (see the TODO in `PLAN.md`). This only works if the worktree itself hasn't actually been deleted (archiving is fine — see "Archiving a worktree" above — but real delete removes the git checkout `claude --resume` would need).

This view survives the worktree itself being deleted — it's driven by the worktree id, not a live DB row, so it's a real record of "what happened to this piece of work" independent of whether the worktree/terminal sessions behind it are still alive. Useful as a lightweight checkpoint trail: e.g. confirm exactly when a worktree was created, or when its last terminal was closed, without digging through the raw JSONL by hand.

Via the API directly:

```bash
curl http://localhost:8787/api/repos/<repoId>/worktrees/<worktreeId>/audit-log
# [{"ts":"...","event":"worktree.create","repo_id":"...","worktree_id":"...","name":"...","branch":"..."}, ...]
```

Not yet built (ideas, not commitments): a way to add a free-text checkpoint note manually (e.g. "sent PR link to reviewer"), and event types for repo-hosting-platform actions (branch pushed, PR opened/merged) once any such integration exists — right now every event this log can show is one this tool itself already causes.

<!-- Each later build step (Monaco, diff/comment-to-agent) appends its own section here per PLAN.md — this file is a living doc, not written once. -->
