# worktree-studio

A small local tool for managing `git worktree`-based parallel development: a Go server backed by a SQLite registry of repos/worktrees (with a JSONL audit log of every mutating action) fronts a React dashboard for registering repos and creating/removing worktrees by name, laying the groundwork for later steps (tmux-backed terminals, dependency "spotlight" sync, and a Monaco-based editor) described in `PLAN.md`.

## How to run

Production-style (single Go binary serving the built frontend):

```bash
cd web && bun install && bun run build   # builds web/dist, embedded into the binary
cd ..
go build -o worktree-studio ./cmd/worktree-studio
./worktree-studio            # serves on http://localhost:8787
```

Frontend dev mode (hot reload, proxies `/api` to the Go server):

```bash
go run ./cmd/worktree-studio      # terminal 1: API server on :8787
cd web && bun install && bun run dev  # terminal 2: Vite dev server on :5173
```

See `docs/running-locally.md` for more detail and `docs/architecture.md` for how the pieces fit together.

## CLI subcommands (need the binary on `PATH`)

`./worktree-studio` also doubles as a small CLI for one-off actions against an already-running server (`install-hooks`/`uninstall-hooks`, `open-file <path>`, `spotlight --start|--stop|--status [path]` — see the skill file for the full list). These only work if `worktree-studio` resolves as a command from wherever you run them (e.g. a tmux pane sitting in some worktree's directory, not this checkout) — a plain `go build -o worktree-studio ./cmd/worktree-studio` only produces a binary in the current directory, so a zsh `command not found` from anywhere else is expected until it's actually installed somewhere on `PATH`:

```bash
go build -o worktree-studio ./cmd/worktree-studio
cp worktree-studio ~/.local/bin/     # or wherever's already on your PATH
```
