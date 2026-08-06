# Architecture

worktree-studio is a Go HTTP server + embedded React SPA for managing `git worktree`s across one or more registered repos. This doc is updated at the end of every build step in `PLAN.md`; it currently reflects **step 1 only**.

## Pieces that exist today (step 1)

```
cmd/worktree-studio/main.go   entrypoint: chi router, embeds web/dist, graceful fallback page
web/embed.go                  go:embed all:dist (must live next to dist/, embed patterns are dir-relative)
internal/store/                SQLite registry: repos, worktrees, terminal_sessions (schema only)
internal/gitops/                shells out to `git worktree add/remove/list --porcelain`, `git status`
internal/audit/                 JSONL audit logger, append-only
internal/api/                   HTTP handlers + adjective-noun name generator + slugify/id helpers
web/src/                        Vite + React + TS SPA: RepoPicker, Workspace, WorktreeList, NewWorktreeDialog
```

### Request flow

1. Browser loads `/` → `RepoPicker.tsx` calls `GET /api/repos/`.
2. User adds a repo (`POST /api/repos/` with `{name, path}`). The handler validates the path is a real directory and a real git repo (`git rev-parse --is-inside-work-tree`) via `internal/gitops.IsGitRepo`, then persists it via `internal/store` and appends a `repo.add` line to the audit log.
3. Clicking a repo navigates to `/repo/:id` (`Workspace.tsx`), which loads worktrees via `GET /api/repos/:repoId/worktrees/`.
4. "+ New worktree" opens `NewWorktreeDialog.tsx`, which immediately calls `GET /api/repos/:repoId/worktrees/new-name-suggestion` to prefill an editable adjective-noun name (e.g. `amber-ridge`) from a small embedded Go wordlist (`internal/api/wordlist.go`) — no external dependency needed for this.
5. Submitting calls `POST /api/repos/:repoId/worktrees/` with `{name}`. The handler slugifies the (possibly user-edited) name, uses it as both branch name and directory name, and runs `git worktree add -b <branch> <path>` against the *registered* repo's path — creating the worktree at `~/.worktree-studio/worktrees/<repoId>/<slug>`, deliberately **outside** the source repo tree (avoids the repo's own tooling/.gitignore getting confused by a nested worktree). Persists a `worktrees` row and appends a `worktree.create` audit line.
6. Deleting calls `DELETE /api/repos/:repoId/worktrees/:worktreeId`, which runs `git worktree remove --force <path>`, deletes the DB row, and appends a `worktree.remove` audit line.

### Storage

- **SQLite** at `~/.worktree-studio/studio.db` via `modernc.org/sqlite` (pure Go, no cgo). Tables: `repos(id, name, path)`, `worktrees(id, repo_id, name, branch, path, created_at)`, and `terminal_sessions(id, worktree_id, tmux_session_name, tab_label)` — the last one is schema-only for now, populated starting with the tmux terminal step (step 2 of `PLAN.md`).
- **Audit log** at `~/.worktree-studio/audit.log.jsonl`, one JSON object per line, written by `internal/audit.Logger.Log(event, fields)`. Every mutating handler in `internal/api` calls it. See `docs/running-locally.md` for how to tail it.

### Frontend embedding

`cmd/worktree-studio/main.go` imports `worktree-studio/web`, a tiny package whose only job is holding `//go:embed all:dist` — go:embed patterns resolve relative to the *file* containing the directive, so the embed can't live in `cmd/worktree-studio` (a different directory from `web/dist`). `web/dist/.gitkeep` is checked into git so the directory (and thus the embed) always exists on a fresh checkout, even before `npm run build` has ever run — `go build ./...` works either way. At runtime, `mountFrontend` checks for a real `index.html` in the embedded FS; if it's missing (only the placeholder is there), it serves a small "run `npm run build`" HTML page instead of a blank/broken UI. Once a real build exists, static assets are served with an SPA fallback to `index.html` for client-side routes like `/repo/:id`.

### What's explicitly NOT built yet

Per `PLAN.md`'s build order, none of the following exist yet — don't assume they do:
- Terminal / tmux sessions (step 2)
- Spotlight sync of `node_modules`/`.env`/etc. into new worktrees (step 3)
- Worktree status dashboard (git dirty/ahead-behind badges) (step 4)
- Monaco file editor (step 5)
- Git diff view + comment-to-agent (step 6 / TODO)

See `docs/running-locally.md` for how to run this step's server + frontend, and `.claude/skills/worktree-studio/SKILL.md` for the agent-facing usage doc.
