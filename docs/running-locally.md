# Running locally

## Prerequisites

- Go 1.21+ (module uses `modernc.org/sqlite`, a pure-Go driver — no cgo/sqlite3 headers needed).
- [Bun](https://bun.sh) (for the Vite/React frontend — used as the package manager/runner instead of npm; `bun install`/`bun run` in place of `npm install`/`npm run`).
- `git` on `PATH` (all worktree operations shell out to the real `git` binary).
- `tmux` on `PATH` (terminal tabs are tmux sessions under the hood — see `docs/session-persistence.md`; `brew install tmux` on macOS).

## Production-style: one binary

Build the frontend first, then the Go binary embeds it via `go:embed`:

```bash
cd web && bun install && bun run build   # writes web/dist/
cd ..
go build -o worktree-studio ./cmd/worktree-studio
./worktree-studio
```

Server listens on `:8787` by default (override with `WORKTREE_STUDIO_ADDR=:9000 ./worktree-studio`). Open `http://localhost:8787/`.

If you run `go build` **before** ever building the frontend, it still succeeds — `web/dist/` ships with a placeholder file so the `go:embed` directive always has something to embed, and the server serves a small "run `bun run build`" page instead of crashing. This lets a fresh checkout compile immediately; you only need the frontend built to get the real UI.

## Development: hot-reloading frontend

Run the API server and the Vite dev server side by side:

```bash
# terminal 1
go run ./cmd/worktree-studio

# terminal 2
cd web
bun install
bun run dev       # http://localhost:5173, proxies /api/* to :8787 (see web/vite.config.ts)
```

Open `http://localhost:5173/` — API calls to `/api/...` are proxied to the Go server.

## Data locations

- SQLite registry: `~/.worktree-studio/studio.db`
- Audit log (JSONL, append-only): `~/.worktree-studio/audit.log.jsonl`
- Created worktrees live under: `~/.worktree-studio/worktrees/<repoId>/<worktree-name>/`
- Terminal sessions are tmux sessions named `wts-<terminalId>` — not a file on disk, but visible via `tmux list-sessions`.

None of the above are created by the repo itself — the server creates `~/.worktree-studio/` and its subdirectories on first run if missing.

## Debugging

```bash
# tail the audit log while poking the UI
tail -f ~/.worktree-studio/audit.log.jsonl

# pretty-print with jq
cat ~/.worktree-studio/audit.log.jsonl | jq .

# confirm git's own view of worktrees for a given repo matches the UI
git -C /path/to/repo worktree list --porcelain
```

## Resetting local state

Local state is disposable (it's not migration data — it's a small registry):

```bash
rm -rf ~/.worktree-studio
```

This clears registered repos, the worktree registry, and the audit log. It does **not** remove any actual git worktrees already created on disk — remove those with `git worktree remove` (or delete them via the UI first).
