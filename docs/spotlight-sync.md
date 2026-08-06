# Spotlight

## What it actually does (corrected from an earlier wrong sketch)

Spotlight does **not** copy dependencies from the main repo into a worktree. It mirrors a **worktree's source files into its repo's root checkout**, continuously, so:

- path-bound tools (Finder, macOS Spotlight search, anything hardcoded to a fixed directory) always see whichever worktree is currently "in focus"
- you run dev/build tooling **from the root path**, which already has `node_modules`/build output installed — no separate install per worktree needed

worktree-studio does not implement this sync logic itself. It wraps an already-existing, separately-installed CLI: [`github.com/JayaSuryaOolio/spotlight`](https://github.com/JayaSuryaOolio/spotlight) (a zsh script using `fswatch` + `rsync`, installed at `~/.local/bin/spotlight` on this machine). `internal/spotlight` is a thin `os/exec` wrapper around it — same "shell out to the proven tool" philosophy as `internal/gitops` and `internal/term`.

## How the underlying tool works

- `spotlight start` (run with `cwd` = the worktree): resolves the worktree's root repo via git, **refuses if the root has uncommitted changes** (never clobber unsaved root work), does one `rsync -a --delete --exclude='.git' --exclude-from=<flattened .gitignore>` sync, then keeps re-syncing on every `fswatch` event.
- Only **one worktree can mirror into a given root at a time** — starting a different worktree for the same root automatically stops the previous mirror first.
- `spotlight stop [root]`: kills the watcher, then `git checkout -- .` + `git clean -fd` to restore the root to a clean state.
- The corruption-proofing already lives in that tool, not reinvented here: flattening every `.gitignore` in the tree into one static `--exclude-from` file, rather than relying on rsync's per-directory `--filter=':- .gitignore'` — the latter does **not** reliably protect destination-only gitignored directories (e.g. a root's `node_modules` that doesn't exist in the worktree) from `--delete`, confirmed on both `openrsync` and upstream rsync 3.4.4.

## worktree-studio's integration surface

- `internal/spotlight.BinaryPath()` resolves the CLI via `exec.LookPath`, falling back to `~/.local/bin/spotlight` (the server process may not inherit an interactive shell's `PATH`).
- `Start(worktreePath) (root string, err error)`, `Stop(root string) error`, `List() ([]MirrorStatus, error)`, `StatusForRoot(root string) (*MirrorStatus, error)`.
- REST: `GET/POST /api/repos/:repoId/worktrees/:worktreeId/spotlight/{,start,stop}`. Status distinguishes three states: no mirror active for this repo at all, *this* worktree is the active mirror, or a *different* worktree of the same repo is currently mirrored (shown in the UI as "will replace active mirror" if you start anyway — matches the underlying tool's own auto-takeover behavior).
- A `409` from `start` means the root is dirty — surfaced as-is rather than retried/forced, since force-mirroring over uncommitted root work is exactly the failure mode the tool exists to prevent.
- Every successful start/stop is audit-logged (`spotlight.start` / `spotlight.stop`, with `repo_id`/`worktree_id`/`worktree` path/`root` fields). A *refused* start (409) is not audit-logged, since nothing actually happened.

## A real subtlety this surfaced: path resolution on macOS

`StatusForRoot`'s first implementation compared paths with a plain string `==`, and failed silently against real activity — a `t.TempDir()` path like `/var/folders/.../T/...` never equals what the CLI reports for the *same directory*, `/private/var/folders/.../T/...`, because `/var` is itself a symlink to `/private/var` on macOS (same story for `/tmp` → `/private/tmp`). Fixed by comparing `filepath.EvalSymlinks`'d paths on both sides, falling back to the raw string if resolution fails (e.g. the path doesn't exist). Any future code comparing a worktree-studio-known path against something spotlight reports needs the same treatment — see `resolveBestEffortEqual` in `internal/api/spotlight.go`.

## Manually verifying

```bash
# from a worktree-studio worktree's path:
curl -X POST http://localhost:8787/api/repos/<repoId>/worktrees/<worktreeId>/spotlight/start
# edit a file in the worktree, confirm it appears in the repo's root checkout
curl http://localhost:8787/api/repos/<repoId>/worktrees/<worktreeId>/spotlight/
curl -X POST http://localhost:8787/api/repos/<repoId>/worktrees/<worktreeId>/spotlight/stop
# confirm `git status` in the root is clean again
```

Or directly against the installed CLI (bypassing worktree-studio entirely, useful for debugging):

```bash
cd /path/to/a/worktree && spotlight start
spotlight list
spotlight stop
```

## Known limitations (inherited from the underlying tool, not specific to this integration)

- Mirrors the **entire** worktree checkout into the **entire** root — there's no per-subdirectory/per-workspace scoping. Fine for this repo's scale (tracked-file total is ~22MB, excluding `node_modules`), but worth knowing if a much larger monorepo's tracked files ever became the bottleneck.
- Requires `fswatch` installed separately from the `spotlight` CLI itself (`brew install fswatch` on macOS).
- If the root is dirty for reasons unrelated to spotlight (e.g. you were mid-edit there for something else), `start` will correctly refuse — there's no "stash it for me" option, by design.
