---
name: worktree-studio
description: How to run and use worktree-studio, a local Go+React tool for managing git worktrees across registered repos (repo registration, worktree create/remove, audit log). Use when asked to start worktree-studio, register a repo with it, or create/remove a worktree through it.
---

# worktree-studio

`worktree-studio` is a small local web tool for managing `git worktree`s across one or more repos: register a repo once, then create/remove worktrees through a dashboard instead of hand-running `git worktree add/remove`. It's meant to be driven by both humans and agents.

**Status as of this section (step 1 of `PLAN.md`): only repo registration and worktree create/remove exist.** The following are explicitly **NOT built yet** — don't assume they exist or try to use them:
- No terminal / tmux sessions (planned: step 2)
- No spotlight sync of `node_modules`/`.env`/build caches into new worktrees (planned: step 3) — a freshly created worktree has **only** what `git worktree add` gives it; you still need to run your own install/build step in it for now
- No worktree status dashboard (dirty/ahead-behind badges) (planned: step 4)
- No Monaco file editor (planned: step 5)
- No git diff view or "send comment to agent" flow (planned: step 6 / TODO)

## Starting the server

```bash
cd ~/work/worktree-studio
go run ./cmd/worktree-studio          # dev, or:
go build -o worktree-studio ./cmd/worktree-studio && ./worktree-studio   # built binary
```

Listens on `http://localhost:8787` by default (`WORKTREE_STUDIO_ADDR` env var overrides). The frontend must be built at least once for the real UI to render (`cd web && npm install && npm run build`) — see `docs/running-locally.md`. Until then the server still starts fine and serves a placeholder page telling you to build the frontend.

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

<!-- Each later build step (terminal/tmux, spotlight sync, dashboard, Monaco, diff/comment-to-agent) appends its own section here per PLAN.md — this file is a living doc, not written once. -->
