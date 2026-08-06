---
name: worktree-studio
description: How to run and use worktree-studio, a local Go+React tool for managing git worktrees across registered repos (repo registration, worktree create/remove, audit log). Use when asked to start worktree-studio, register a repo with it, or create/remove a worktree through it.
---

# worktree-studio

`worktree-studio` is a small local web tool for managing `git worktree`s across one or more repos: register a repo once, then create/remove worktrees through a dashboard instead of hand-running `git worktree add/remove`. It's meant to be driven by both humans and agents.

**Status as of this section (steps 1–3 of `PLAN.md`): repo registration, worktree create/remove, tmux-backed terminals, and spotlight all exist.** The following are explicitly **NOT built yet** — don't assume they exist or try to use them:
- No worktree status dashboard (dirty/ahead-behind badges) (planned: step 4)
- No Monaco file editor (planned: step 5)
- No git diff view or "send comment to agent" flow (planned: step 6 / TODO)

Also important: spotlight does **not** install dependencies into a worktree. A freshly created worktree has only what `git worktree add` gives it — if you need `node_modules` there directly (rather than running things from the mirrored root), you still run your own install/build step.

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

## Removing a worktree

Via the UI: click "Delete" next to a worktree row (there's a confirm prompt).

Via the API:

```bash
curl -X DELETE http://localhost:8787/api/repos/<repoId>/worktrees/<worktreeId>
```

This runs `git worktree remove --force <path>` and deletes the registry row. It does not touch the branch itself (only the worktree checkout).

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

Click "Open" on a worktree row to get to its detail page (`/repo/:repoId/worktree/:worktreeId`), which shows terminal tabs for that worktree. "+ New Terminal" starts a real shell rooted in that worktree's directory — run anything in it, including `claude` itself; there's no special agent framing, it's just a shell.

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

<!-- Each later build step (dashboard, Monaco, diff/comment-to-agent) appends its own section here per PLAN.md — this file is a living doc, not written once. -->
