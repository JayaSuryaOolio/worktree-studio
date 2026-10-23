# worktree-studio

A small local tool for managing `git worktree`-based parallel development: a Go server backed by a SQLite registry of repos/worktrees (with a JSONL audit log of every mutating action) fronts a React dashboard for registering repos and creating/removing worktrees by name, laying the groundwork for later steps (tmux-backed terminals, dependency "spotlight" sync, and a Monaco-based editor) described in `PLAN.md`.

A repo can also point at another tool's own workspace root (its settings page's "External worktrees root" field) — worktree-studio then auto-discovers and registers any worktree that tool creates there on every page load, no manual attach needed. See `docs/architecture.md`'s "External worktree auto-discovery" section.

## Quick launch

`./wts web` (browser), `./wts dev` (Vite hot reload), `./wts tui` (terminal UI), `./wts desktop` (Wails app, needs the `wails-app` branch merged), `./wts status`, `./wts stop`. It starts the server if needed, or reuses a running one, and wires up whatever port it landed on (the server records it in `~/.worktree-studio/server.addr`).

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
CGO_LDFLAGS="-framework UniformTypeIdentifiers" go build -tags desktop,production -o worktree-studio-desktop ./cmd/worktree-studio   # macOS
```

(`CGO_LDFLAGS` is only needed on macOS when building without the `wails` CLI.) It still listens on a loopback port like the web build.

For a double-clickable macOS app (Dock icon, no terminal), run `install/build-app.sh` — it builds the frontend, then assembles `build/Worktree Studio.app` (no `wails` CLI needed); copy it to `/Applications`. Launched from Finder it gets `/opt/homebrew/bin`, `/usr/local/bin` and `~/.local/bin` prepended to `PATH` so tmux/git are found.

## CLI subcommands (need the binary on `PATH`)

`./worktree-studio` also doubles as a small CLI for one-off actions against an already-running server (`install-hooks`/`uninstall-hooks`, `open-file <path>`, `spotlight --start|--stop|--status [path]`, `orphans [--kill] [--min-age=<duration>]`, `create-worktree <name> [--branch <source-branch>] [path]` — see the skill file for the full list). These only work if `worktree-studio` resolves as a command from wherever you run them (e.g. a tmux pane sitting in some worktree's directory, not this checkout) — a plain `go build -o worktree-studio ./cmd/worktree-studio` only produces a binary in the current directory, so a zsh `command not found` from anywhere else is expected until it's actually installed somewhere on `PATH`:

```bash
go build -o worktree-studio ./cmd/worktree-studio
cp worktree-studio ~/.local/bin/     # or wherever's already on your PATH
```
