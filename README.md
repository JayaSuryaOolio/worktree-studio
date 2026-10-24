# worktree-studio

A small local tool for managing `git worktree`-based parallel development: a Go server backed by a SQLite registry of repos/worktrees (with a JSONL audit log of every mutating action) fronts a React dashboard for registering repos and creating/removing worktrees by name, laying the groundwork for later steps (tmux-backed terminals, dependency "spotlight" sync, and a Monaco-based editor) described in `PLAN.md`.

A repo can also point at another tool's own workspace root (its settings page's "External worktrees root" field) — worktree-studio then auto-discovers and registers any worktree that tool creates there on every page load, no manual attach needed. See `docs/architecture.md`'s "External worktree auto-discovery" section.

## Quick launch

`./install/install.sh` builds one binary (`~/.worktree-studio/bin/worktree-studio`, symlinked as `wts` in `~/.local/bin`; on macOS it includes the Wails window and a double-clickable `~/Applications/Worktree Studio.app`). Each piece runs as a detached background process:

```bash
wts start              # backend (API + UI); prints its URL and log path
wts web start          # standalone web server (proxies to the backend); errors if the backend isn't up
wts desktop start      # native window (macOS)
wts tui                # terminal UI, foreground
wts status             # backend / web / desktop: state, URL, pid, log folder
wts web stop | desktop stop | stop    # `stop` stops everything
```

Every piece finds the backend through `~/.worktree-studio/server.addr`, so a non-default port never matters. Pids are in `~/.worktree-studio/run/`, logs in `~/.worktree-studio/{server,web,desktop}.log`.

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

## Desktop app (Wails)

The same binary can also open in a native window instead of a browser tab — a second entry point over the identical server, so the web app keeps working unchanged. The window's webview has no browser clipboard-permission model, so copy/paste goes through Wails' Go-side clipboard and links open in the OS browser (`web/src/desktop.ts`); see `docs/terminal-clipboard.md`.

```bash
cd web && bun install && bun run build
cd ..
CGO_LDFLAGS="-framework UniformTypeIdentifiers" go build -tags desktop,production -o worktree-studio ./cmd/worktree-studio   # macOS
```

(`CGO_LDFLAGS` is only needed on macOS when building without the `wails` CLI.) The window attaches to a running backend (`wts start` first, then `wts desktop start`); it doesn't run a server of its own.

`install/install.sh` also installs a double-clickable `Worktree Studio.app` (Dock icon, no terminal): a thin launcher that runs `wts start` and then the window from the installed binary, with `/opt/homebrew/bin`, `/usr/local/bin` and `~/.local/bin` on `PATH` so tmux/git are found.

## CLI subcommands (need the binary on `PATH`)

`./worktree-studio` also doubles as a small CLI for one-off actions against an already-running server (`install-hooks`/`uninstall-hooks`, `open-file <path>`, `spotlight --start|--stop|--status [path]`, `orphans [--kill] [--min-age=<duration>]`, `create-worktree <name> [--branch <source-branch>] [--task <source>:<key>] [path]`, `set-task <source>:<key> | --new <title> | --clear [path]` — see the skill file for the full list). These only work if `worktree-studio` resolves as a command from wherever you run them (e.g. a tmux pane sitting in some worktree's directory, not this checkout) — a plain `go build -o worktree-studio ./cmd/worktree-studio` only produces a binary in the current directory, so a zsh `command not found` from anywhere else is expected until it's actually installed somewhere on `PATH`:

```bash
go build -o worktree-studio ./cmd/worktree-studio
cp worktree-studio ~/.local/bin/     # or wherever's already on your PATH
```
