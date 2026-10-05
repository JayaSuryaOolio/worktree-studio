# worktree-studio

A small local tool for managing `git worktree`-based parallel development: a Go server backed by a SQLite registry of repos/worktrees (with a JSONL audit log of every mutating action) fronts a React dashboard for registering repos and creating/removing worktrees by name, laying the groundwork for later steps (tmux-backed terminals, dependency "spotlight" sync, and a Monaco-based editor) described in `PLAN.md`.

A repo can also point at another tool's own workspace root (its settings page's "External worktrees root" field) — worktree-studio then auto-discovers and registers any worktree that tool creates there on every page load, no manual attach needed. See `docs/architecture.md`'s "External worktree auto-discovery" section.

## How to run

Production-style (single Go binary serving the built frontend):

```bash
cd web && bun install && bun run build   # builds web/dist, embedded into the binary
cd ..
go build -o worktree-studio ./cmd/worktree-studio
./worktree-studio            # serves on http://localhost:8787 (next free port if taken — see the startup log)
```

Frontend dev mode (hot reload, proxies `/api` to the Go server):

```bash
go run ./cmd/worktree-studio      # terminal 1: API server on :8787
cd web && bun install && bun run dev  # terminal 2: Vite dev server on :5173
```

See `docs/running-locally.md` for more detail and `docs/architecture.md` for how the pieces fit together.

## CLI subcommands (need the binary on `PATH`)

`./worktree-studio` also doubles as a small CLI for one-off actions against an already-running server (`install-hooks`/`uninstall-hooks`, `open-file <path>`, `spotlight --start|--stop|--status [path]`, `orphans [--kill] [--min-age=<duration>]`, `create-worktree <name> [--branch <source-branch>] [path]` — see the skill file for the full list). These only work if `worktree-studio` resolves as a command from wherever you run them (e.g. a tmux pane sitting in some worktree's directory, not this checkout) — a plain `go build -o worktree-studio ./cmd/worktree-studio` only produces a binary in the current directory, so a zsh `command not found` from anywhere else is expected until it's actually installed somewhere on `PATH`:

```bash
go build -o worktree-studio ./cmd/worktree-studio
cp worktree-studio ~/.local/bin/     # or wherever's already on your PATH
```

## `wtx` — the worktree exec harness

`scripts/wtx` wraps `create-worktree` into one command that creates (or reuses) a worktree for a task and then launches `claude` with that worktree as its cwd, so the repo's own `CLAUDE.md` and `.claude/` load without anyone having to remember to `cd` first:

```bash
wtx <repo> <name> [options] [-- <claude args>...]
```

`<name>` becomes both the branch name and the worktree directory name — one task, one worktree, one branch. Worktrees are always branched from a remote-tracking `origin/<base>` rather than a local branch, so two tasks started at the same time start from the same commit. When the server isn't running (or the repo isn't registered, or the branch already exists), it falls back to plain `git worktree add` and says so loudly. `install/install.sh` installs it alongside the binary.

See `docs/wtx.md` for the full contract: base-ref resolution, every fallback trigger, the safety assertions, and the exit codes.
