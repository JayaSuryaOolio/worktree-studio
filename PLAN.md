# Worktree Studio — simplified Conductor.build clone

## Context

Working across multiple git worktrees for parallel agent-driven tasks currently means re-running `yarn install`/build in every new worktree, juggling raw terminals for `git worktree` plumbing, and no single view of what's dirty/running where. Conductor.build solves this but is a closed product. The goal is a small local tool, scoped to this workflow, that gives us: (1) a "spotlight" sync so a new worktree inherits the heavy setup (deps/build cache) from the main checkout instead of rebuilding it, (2) a dashboard over all active worktrees, (3) a real terminal (not just an exec-and-capture box) for running agents/commands per worktree, and (4) Monaco-based editing without leaving the tool. It should work across more than one repo — a second browser tab pointed at a different repo is enough, no need for a unified multi-repo UI.

Explored the `pos-3`/adelaide repo first (as the initial worktree-heavy use case) and found no existing worktree-manager code or node-pty/xterm/monaco/ws usage to reuse there. That repo is *not* where this tool lives, though — this is a standalone new project, independent of any single monorepo it happens to manage. It gets its own home and its own git history at `~/work/worktree-studio/`, created fresh as part of step 1 below (not nested inside pos-3 or any other managed repo).

## Decisions locked in from clarification

- **Shape**: local web app — **Go server** + browser UI (not Electron/VS Code extension, not Node).
- **Philosophy**: use the best available *working* solution for each piece now (shell out to existing battle-tested tools rather than reimplementing them), and only swap in custom code later where a real need shows up. Concretely this means: git CLI for worktree ops, **tmux** for session persistence, `git ls-files`/`fsnotify` for file watching — not bespoke reimplementations.
- **Agent backend**: start dumbest-possible — a terminal tab just runs whatever CLI the user types (`claude`, `git`, anything) through a real PTY/tmux session. No Agent SDK integration yet; that's a clean future upgrade once the terminal layer exists.
- **Sessions must survive a server restart**: solved by backing every terminal tab with a real **tmux session** rather than a bare PTY owned by the server process. tmux's own server process outlives the Go server; on restart the Go server just re-attaches. This is the "best available working solution" — bespoke session-persistence (serializing PTY state) would be reinventing tmux badly.
- **Multi-repo**: server holds a small registry of repos; every route is namespaced by `repoId`. A "+ New tab" button in the UI does `window.open` on the current workspace URL (or a repo picker URL) — that's the whole multi-repo/multi-tab story, no shared client state needed.
- **New worktree UX**: creation form prefills a random human-friendly name (adjective-noun style, like Docker container names) into an editable text input; user can accept or rename before the worktree/branch is created.
- **Docs**: maintain a `docs/` folder inside the tool package as a living wiki, kept up to date as each build step lands (architecture, how to run it, how spotlight/tmux persistence work, troubleshooting) — treated as a deliverable of every step below, not a one-time doc.
- **Scope for v1**: single repo works end-to-end (worktrees + spotlight + one terminal + Monaco); multi-repo comes free from the namespacing above, so it's not deferred, just not specially built.

## Package layout

Brand-new standalone project — its own directory, own git repo, own Go module. Created at `~/work/worktree-studio/` (new folder under `~/work/`, `git init`, `go mod init`), fully independent of any repo it will later manage:

```
~/work/worktree-studio/
  go.mod, go.sum               # module deps: gorilla/websocket, go-chi/chi, fsnotify/fsnotify,
                                #   modernc.org/sqlite (pure-Go, no cgo), creack/pty (fallback path only)
  cmd/worktree-studio/main.go  # entrypoint: http server + ws upgrade, embeds web/dist via go:embed
  internal/
    store/                     # sqlite-backed registry: repos, worktrees, terminal-session metadata
    gitops/                    # wraps `git worktree list/add/remove --porcelain`, `git status --porcelain=2 --branch`
    spotlight/                 # sync logic (see below)
    term/                      # tmux-backed session manager (see below)
    files/                     # file tree / read / write for Monaco, fsnotify watcher
    api/                       # HTTP handlers: repos, worktrees, files
    ws/                        # single ws endpoint multiplexing: terminal i/o, worktree status pushes, file-change events
  web/
    index.html, vite.config.ts
    src/
      App.tsx                  # routes: "/" repo picker, "/repo/:id" workspace
      RepoPicker.tsx
      WorktreeList.tsx         # dashboard: branch, dirty/clean, ahead/behind, spotlight status, re-sync button
      NewWorktreeDialog.tsx    # name input prefilled with a random adjective-noun name, editable
      Terminal.tsx             # xterm.js + fit addon, talks to /ws terminal channel; "+ New tab" button = window.open
      Editor.tsx               # monaco-editor, file tree + open/save
      api.ts, ws.ts            # thin client wrappers
  docs/
    architecture.md            # how the pieces fit together (updated every build step)
    running-locally.md
    spotlight-sync.md          # how sync + corruption-proofing works
    session-persistence.md     # how the tmux-backed model works, troubleshooting
```

No `specs/` convention applies here (that's a pos-3-specific convention) — `docs/` inside this new project is the living source of truth, kept current per step in the build order below.

## Core mechanics

### 1. Repo registry & worktree ops (`internal/store`, `internal/gitops`)
- Registry lives in a single SQLite file at `~/.worktree-studio/studio.db` (via `modernc.org/sqlite`, no cgo needed — "best available working solution" for embedded persistent storage in Go): tables for `repos(id, name, path)`, `worktrees(id, repo_id, name, branch, path, created_at)`, `terminal_sessions(id, worktree_id, tmux_session_name, tab_label)`. Using a real DB (not a JSON blob) is what makes session persistence across restarts trivial and safe under concurrent writes.
- Worktrees are created under `~/.worktree-studio/worktrees/<repoId>/<worktree-name>` — deliberately **outside** the main repo tree (avoids the repo's own `.gitignore`/tooling getting confused by nested worktrees).
- New-worktree flow: server generates a random adjective-noun name (small embedded wordlist, no external dep needed) and returns it to prefill the create dialog's text input; user can edit before submitting. The (possibly edited) name is used as both the branch name and worktree directory name (slugified).
- `git worktree add/remove/list --porcelain` and `git status --porcelain=2 --branch` are invoked via `os/exec` — shelling out to the real `git` binary is simpler and more correct than any Go git library for this. Status polling runs on an interval (e.g. every few seconds) and results are pushed to clients over the ws channel, not computed per-keystroke.

### 2. Spotlight sync (`internal/spotlight`) — corruption-proof, one-directional
Reuses the constraint already learned the hard way (see memory: rsync `--delete` + gitignore pitfall — dir-merge filters don't protect destination-only gitignored dirs, must use `--exclude-from` instead).

- **Direction**: always main-repo → worktree, never the reverse (a worktree's local edits must never leak back).
- **What's synced**: allow-listed heavy/derived paths only — `node_modules` (root + per-workspace, if Yarn nodeLinker is `node-modules`) or `.yarn/cache` + `.pnp.cjs` (if `nodeLinker: pnp` — check `.yarnrc.yml` at build time and branch on it), plus `.turbo/` cache and any `.env*` files. Tracked source files are never touched — the worktree's checkout already has those correctly via normal git worktree behavior.
- **Corruption-proofing**:
  1. Sync into a fresh temp staging dir under the worktree's parent (`<worktree>.spotlight-tmp-<ts>`), not directly into the live target.
  2. On success, atomically swap via `fs.rename` (same filesystem, so it's a single inode-pointer update, not a copy) — a crash mid-sync leaves the old state intact, never a half-written one.
  3. Per-worktree lock file (`.spotlight.lock`) held for the duration of a sync to prevent two syncs racing.
  4. Manifest file (`.spotlight-manifest.json`) records source commit + `yarn.lock` hash + synced-dir list + timestamp. Re-sync is skipped automatically unless `yarn.lock` changed since the manifest was written; a manual "Re-sync" button in the UI forces it regardless.
  5. `rsync -a --checksum --exclude-from=<allowlist-derived-excludes>` rather than a bare `--delete` + dir-merge filter, per the known pitfall. `rsync` invoked via `os/exec`, same "shell out to the proven tool" philosophy as git.

### 3. Terminal, backed by tmux for persistence (`internal/term`, `web/src/Terminal.tsx`)
- Each terminal tab = one **tmux session** (`tmux new-session -d -s wts-<worktreeId>-<tabId> -c <worktreePath>`), not a bare PTY the Go process owns directly. The Go server attaches to it via `creack/pty` running `tmux attach -t <name>` and streams that over the ws channel.
- Why tmux instead of hand-rolled persistence: tmux already solves "shell session survives the controlling process dying" correctly (scrollback, running jobs, signals) — reimplementing that in Go would be redoing tmux badly. This is exactly the "best available working solution now, custom later if truly needed" call from the brief.
- **Server-restart flow**: `terminal_sessions` rows in SQLite map tab → tmux session name. On Go server startup, it lists live tmux sessions (`tmux list-sessions`), reconciles against the DB (drops rows for sessions that no longer exist, e.g. if tmux itself was also killed), and the UI reconnects to whatever's still running — no session state needs to be serialized/restored by hand.
- ws protocol multiplexes by `sessionId`: client sends `{type: 'input', sessionId, data}` / `{type: 'resize', sessionId, cols, rows}`; server streams `{type: 'output', sessionId, data}`.
- xterm.js + `@xterm/addon-fit` on the client for a real terminal feel (matches the "Warp-like" ask) — no special agent framing, it's just a shell; running `claude` in it is a normal command.
- "+ New tab" button in the terminal panel does `window.open(currentWorkspaceUrl)` — satisfies the "button to open a new tab" ask with zero new backend surface.

### 4. Monaco editor (`internal/files`, `internal/api`, `web/src/Editor.tsx`)
- File tree = `git ls-files` (tracked) unioned with untracked-but-not-ignored files (`git ls-files --others --exclude-standard`), scoped to the worktree.
- Read/write via simple REST (`GET/PUT /api/repos/:repoId/worktrees/:wtId/files?path=...`) — no live-collab/CRDT needed for a single-user tool.
- `fsnotify` (the standard Go filesystem-watch library) watches the worktree; external changes push a ws event so the open buffer can prompt "file changed on disk, reload?".

### 4b. Git diff view in Monaco + comment-to-agent (TODO, not yet scoped into a build step)
- Add a simple git diff view alongside the editor: per-file diff using Monaco's built-in diff editor (`monaco-editor`'s `createDiffEditor`, comparing `git show HEAD:<path>` against the worktree's on-disk content) or a full working-tree diff list (`git diff --name-status` for the file list, diff editor per selected file). Reuses `internal/gitops` (already shells out to git) — just needs a `git show`/`git diff` wrapper, no new dependency.
- Let the user add inline comments on diff lines (stored per worktree, e.g. a `review_comments(id, worktree_id, file, line, side, body, created_at)` table in the existing SQLite store).
- "Send to agent" action: comments get formatted (file:line + quoted diff context + comment body) and piped into the worktree's terminal/tmux session (once step 2's terminal exists) as agent input, so the running `claude` session addresses them — e.g. `tmux send-keys -t <session> "<formatted prompt>" Enter`, reusing `internal/term` once it exists.
- Depends on both step 4 (Monaco/file APIs) and step 2 (terminal/tmux) being done first; slot in as a follow-up step after both land, before calling v1 complete.

### 5. Multi-repo via tabs
- Every REST/WS route is `/api/repos/:repoId/...`; the frontend root `/` is a repo picker (add/select from registry), `/repo/:id` is the full workspace. Two browser tabs on two different `repoId`s just work — no cross-tab state needed.

### 6. Audit log (`internal/audit`)
- Append-only JSONL file at `~/.worktree-studio/audit.log.jsonl` — one line per event, e.g. `{"ts":"2026-08-06T12:00:00Z","event":"worktree.create","repo_id":"...","worktree_id":"...","name":"...","branch":"..."}`. Plain `os.OpenFile` with `O_APPEND`, no DB table needed — JSONL is the right tool here (append-only, human-greppable, no migrations).
- Foundational piece, wired in from step 1 onward: every mutating action across every later step (repo add, worktree create/remove, spotlight sync start/success/failure, terminal session create, file write, diff comment sent-to-agent) logs one line through the same small `audit.Log(event string, fields map[string]any)` helper. Not a separate build step — it's a cross-cutting concern each step's handlers call into as they're built.
- Deliberately basic for v1: no rotation, no querying UI — just a file you can `tail -f` or `jq` through. Rotation/query tooling is a "custom later if needed" upgrade per the project's stated philosophy.

### 7. `worktree-studio` dev skill
- A Claude Code skill (SKILL.md) that teaches devs/agents how to use this tool efficiently — not app code, but part of the deliverable since the tool is meant to be driven by agents as much as humans.
- Lives at `~/work/worktree-studio/.claude/skills/worktree-studio/SKILL.md` (project-scoped skill, ships with the repo) covering: starting the server, registering a repo, creating/removing worktrees (and what the name-prefill is for), where spotlight sync stands, how to open a terminal against a worktree and why it's tmux-backed (so killing/restarting the server is safe), how to use the editor and diff/comment-to-agent flow, and where the audit log lives for debugging ("something happened, check `~/.worktree-studio/audit.log.jsonl`").
- Like `docs/`, this is a living document — grows a section per build step rather than being written once at the end. Stub it out in step 1 with what exists so far (server/repo/worktree basics); each later step appends its own section.

## Build order

Every step below ends with a matching update to `docs/` (architecture.md gets the new piece added, plus a dedicated doc for anything non-trivial) — docs are not a final cleanup pass, they're part of "done" for each step.

1. **Project init + skeleton + worktree CRUD + audit log foundation + skill stub**: create `~/work/worktree-studio/`, `git init`, `go mod init`, initial commit with `.gitignore`/README. Then Go module structure, SQLite store, gitops wrappers, `internal/audit` JSONL logger wired into every mutating handler from the start, chi router + API, Vite+React shell embedded via `go:embed`, repo picker, worktree list with create/remove (including the random-name-prefill dialog). Verify by registering the pos-3/adelaide repo as a managed repo and creating/removing a real worktree of it through the UI, confirming `git worktree list` matches AND confirming matching lines land in `~/.worktree-studio/audit.log.jsonl`. Write `docs/architecture.md`, `docs/running-locally.md`, and stub `.claude/skills/worktree-studio/SKILL.md` covering what exists so far.
2. **Terminal via tmux**: tmux session-per-tab, `creack/pty` attach, ws relay, xterm.js client, "+ New tab" button. Verify by running `git status`/`yarn --version`/an actual `claude` session inside it, then **kill and restart the Go server** and confirm the tab reconnects to the same tmux session with scrollback/running process intact. Write `docs/session-persistence.md`.
3. **Spotlight sync**: manifest + staging + atomic swap + lock, triggered on worktree creation, manual re-sync button. Verify: create a worktree, confirm `node_modules`/`.env` appear without running `yarn install`, edit `yarn.lock` in main repo, confirm re-sync picks it up; kill the process mid-sync (SIGKILL) and confirm the worktree is left in its last-good state, not half-copied. Write `docs/spotlight-sync.md`.
4. **Worktree monitoring dashboard**: git-status polling pushed over ws, dirty/ahead-behind/spotlight-status badges in `WorktreeList.tsx`.
5. **Monaco editor**: file tree, open/save, external-change watch via fsnotify.
6. **TODO — Git diff view in Monaco + comment-to-agent**: diff editor view per file (Monaco diff editor against `git show HEAD:<path>`), inline review comments stored in SQLite, "send to agent" action that pipes formatted comments into the worktree's tmux session as input. Depends on steps 2 and 5 being done. Verify: make a local edit in a worktree, open the diff view, add a comment on a changed line, send it, confirm the text arrives in that worktree's terminal session, and confirm a `diff.comment_sent` line lands in the audit log.

Every step above (2 through 6) also: (a) adds audit-log calls for its new mutating actions, and (b) appends a section to `.claude/skills/worktree-studio/SKILL.md` documenting the new capability — both are part of that step's definition of done, not a separate pass at the end.

Explicitly deferred (not v1): Agent SDK integration (terminal-as-CLI is enough to start), multiple terminal tabs per worktree beyond the "open in new browser tab" model (trivial extension of step 2's session model later), any auth/remote-access hardening (bind to `localhost` only), audit log rotation/query UI.

## Execution approach

- Drive implementation through subagents (Agent tool), one step (or sub-piece of a step) per subagent dispatch, rather than one long inline session.
- Model allocation: Sonnet 5 as the default implementation model; drop to Haiku 4.5 for small/mechanical sub-pieces (boilerplate CRUD handlers, doc-file updates) where full Sonnet reasoning isn't needed, to keep token spend efficient. Fable is reserved for planning/design-judgment work only, never for writing code.
- Skills: invoke only skills clearly relevant to the concrete step in progress (e.g. `run` when it's time to actually launch and click through the app) — don't reach for research/brainstorming/domain-modeling skills on a plan this concrete.
- Proceeding autonomously step-by-step per the build order above once this plan is approved; will surface blockers (e.g. missing `tmux`) rather than pausing for confirmation on routine sub-decisions already covered by this plan.

## Dependencies to verify are available before starting

`tmux` must be installed on the machine running the server (check with `tmux -V`) — it's the load-bearing "best available working solution" for persistence; if it's missing, install it first rather than working around its absence.

## Status

`~/work/worktree-studio/` exists with `git init` done, but nothing committed yet — only this `PLAN.md` is present. Two subagent dispatch attempts for step 1 failed for tooling reasons (one stayed in read-only plan mode, one lacked Bash permission) — no project code has been written yet. Next action: implement step 1 (now including the audit log foundation and skill stub) directly in this session, working from this directory.

## Open question to resolve during step 3

Confirm this repo's `.yarnrc.yml` `nodeLinker` setting (`node-modules` vs `pnp`) before finalizing the exact spotlight allow-list — determines whether we sync `node_modules` directories or `.yarn/cache` + `.pnp.cjs`.
