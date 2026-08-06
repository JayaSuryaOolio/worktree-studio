# Progress log

Running log of work on worktree-studio across sessions. Newest entry at the top. Read this at the start of every new session before touching code — it says what's done, what's verified, and what's next per `PLAN.md`'s build order.

---

## 2026-08-06 — Step 1: project init + skeleton + worktree CRUD + audit log + skill stub

**Completed this session:**

- Repo scaffolding: `.gitignore` (Go binaries, `web/node_modules/`, `web/dist/*` except a checked-in `.gitkeep` placeholder, `*.db`, `.DS_Store`), `README.md`.
- `go mod init worktree-studio`. Dependencies added: `github.com/go-chi/chi/v5` (routing), `modernc.org/sqlite` (pure-Go SQLite, no cgo).
- `internal/audit`: JSONL audit logger (`Logger.Log(event, fields)`), append-only at `~/.worktree-studio/audit.log.jsonl`.
- `internal/store`: SQLite registry at `~/.worktree-studio/studio.db`. Tables: `repos`, `worktrees`, `terminal_sessions` (schema only, unused until step 2).
- `internal/gitops`: wraps `git worktree add/remove/list --porcelain` and `git status --porcelain=2 --branch` via `os/exec`. (`Status` is written but not yet wired into any handler — that's step 4's job.)
- `internal/api`: chi handlers for `POST/GET /api/repos/`, `GET/POST /api/repos/:repoId/worktrees/`, `GET .../new-name-suggestion`, `DELETE .../:worktreeId`. Adjective-noun name generator (`internal/api/wordlist.go`, small embedded wordlist, no external dep). Every mutating handler (`repo.add`, `worktree.create`, `worktree.remove`) calls `internal/audit`.
- `cmd/worktree-studio/main.go`: entrypoint wiring store + audit + api + chi router. Embeds the built frontend via `worktree-studio/web` (a small package holding the `//go:embed all:dist` directive — had to live next to `web/dist/` since embed patterns are directory-relative to the source file, not the module root; a direct `//go:embed web/dist` in `cmd/worktree-studio/main.go` fails to compile). Graceful fallback: if `web/dist` has no real `index.html` yet, serves a friendly "run `npm run build`" placeholder page instead of crashing, so `go build ./...` and running the binary both work on a fresh checkout before the frontend is ever built.
- Frontend (`web/`): Vite + React + TypeScript, hand-scaffolded (`package.json`, `tsconfig.json`, `vite.config.ts` with `/api` dev proxy to `:8787`). `RepoPicker.tsx` at `/` (add repo by path, list repos, link to `/repo/:id`). `Workspace.tsx` at `/repo/:id` composing `WorktreeList.tsx` (branch, path, created_at, delete button) and `NewWorktreeDialog.tsx` (name input prefilled via the name-suggestion endpoint, editable). Minimal CSS, no component library.
- `docs/architecture.md`, `docs/running-locally.md` written for step 1's scope.
- `.claude/skills/worktree-studio/SKILL.md` stubbed with what exists so far, explicitly listing terminal/tmux, spotlight sync, dashboard, Monaco, and diff/comment-to-agent as **not built yet**.

**Verified working (real commands run, not just claimed):**

- `go build ./...` — success, zero errors. `go vet ./...` — clean, no issues.
- `cd web && npm install && npm run build` — succeeded, produced `web/dist/{index.html,assets/*}`.
- Rebuilt the Go binary with the real frontend embedded; confirmed it serves the real `index.html` at `/` and falls back to `index.html` for SPA routes like `/repo/:id` (200, not 404); confirmed an unknown API route still 404s.
- Ran the server against a real repo (`/Users/jayasurya/conductor/workspaces/pos/adelaide`) end-to-end via curl:
  - `POST /api/repos/` → registered the repo, got back an id.
  - `GET /api/repos/:id/worktrees/` → empty list.
  - `GET /api/repos/:id/worktrees/new-name-suggestion` → e.g. `{"name":"amber-ridge"}`.
  - `POST /api/repos/:id/worktrees/` with that name → created worktree, response included a real `created_at` timestamp.
  - `git -C .../adelaide worktree list` → confirmed the new worktree (`amber-ridge`, branch `amber-ridge`) actually exists on disk, outside the source repo tree at `~/.worktree-studio/worktrees/<repoId>/amber-ridge`.
  - `DELETE /api/repos/:id/worktrees/:wtId` → removed it; re-ran `git worktree list` and confirmed it was gone.
  - `cat ~/.worktree-studio/audit.log.jsonl` → confirmed matching `repo.add`, `worktree.create`, and `worktree.remove` JSONL lines with correct `repo_id`/`worktree_id`/`name`/`branch`/`path` fields.
- Bug found and fixed during this verification: the `POST` worktree-create response initially returned an empty `created_at` (the store stamped the timestamp on its own local copy, not the caller's struct) — fixed by stamping `CreatedAt` in the handler before calling `store.AddWorktree`, then re-verified the fix with a second full create/delete/audit cycle.
- Test server process was killed after verification; no server left running.

**Not built (explicitly out of scope for this step, per instructions):** terminal/tmux, spotlight sync, Monaco editor, git-diff/comment-to-agent. `internal/gitops.Status` exists but isn't wired to any endpoint yet.

**Next up:** Step 2 — terminal via tmux (`internal/term`, tmux session-per-tab, `creack/pty` attach, ws relay, xterm.js client, "+ New tab" button). Verify by running real commands in a tab, then killing and restarting the Go server and confirming the tab reconnects to the same tmux session with scrollback/running process intact. Write `docs/session-persistence.md` and append a tmux section to the skill file.

**Fixes applied after an independent verification pass (same day, 2026-08-06):**

- `web/vite.config.ts` had `emptyOutDir: true`, which deleted the checked-in `web/dist/.gitkeep` on every `npm run build`, dirtying `git status` every time the documented build command ran. Fixed by setting `emptyOutDir: false` (the `.gitignore` `web/dist/*` + `!web/dist/.gitkeep` rule already keeps built assets untracked, so Vite doesn't need to clean the dir itself). Re-verified: ran `npm run build` twice in a row from the current checkout — `.gitkeep` survives both times, `git status` stays clean.
- `internal/gitops.RemoveWorktree` always passed `--force`, silently discarding uncommitted changes/untracked files on every delete, and the frontend's confirm dialog didn't warn about that. Fixed: `RemoveWorktree` now takes a `force bool`; without force, a dirty worktree's refusal from git is surfaced as a distinguishable `ErrWorktreeDirty` (wrapped, `errors.Is`-able) instead of a generic error. `handleDeleteWorktree` reads a `?force=true` query param and returns `409 Conflict` (not 500) when git refuses due to dirtiness, only actually removing on a true force. Frontend: the delete confirm now says uncommitted changes will be lost, and a `409`/`ConflictError` response triggers a second, more explicit confirm ("permanently discard those changes") before retrying with `force=true`.
- `internal/api.handleAddRepo` accepted relative paths (which would silently resolve against the server process's CWD). Fixed: rejects any `path` that isn't `filepath.IsAbs` with a `400`.
- Added the project's first automated tests (previously zero): `internal/api/{util,api}_test.go`, `internal/gitops/gitops_test.go`, `internal/store/store_test.go`, `internal/audit/audit_test.go` — 18 tests total, covering slugify/newID, the full repo-add → worktree create/list/delete HTTP flow (including the new force-delete-on-dirty 409 path and the relative-path rejection), real `git worktree add/remove/list --porcelain` and `git status` against throwaway git repos, SQLite store CRUD (including the `CreatedAt`-stamping regression this step's PROGRESS entry above already called out), and audit-log JSONL correctness. `go test ./...` passes (18/18) alongside `go build ./...`, `go vet ./...`, and `gofmt -l .` (all clean).
- Left `cmd/worktree-studio/main.go`'s `:8787` (all-interfaces) bind address unchanged: PLAN.md explicitly defers "any auth/remote-access hardening (bind to `localhost` only)" out of v1 scope, so this is intentional, not an oversight — flagging it here so it isn't mistaken for a forgotten fix in a future session.
- Re-ran the full curl+git+audit-log verification end-to-end against the real `adelaide` repo after all fixes: repo already registered (persists across restarts, as expected) → relative-path `POST /api/repos/` correctly `400`s → name-suggestion → create (`201`, real `created_at`) → dirtied the new worktree → `DELETE` without force correctly `409`s and leaves it in `git worktree list` and the API's list → `DELETE ?force=true` correctly removes it from git, the API list, and disk → a second create/delete cycle without dirtying it confirms plain deletes still work with no force needed → audit log gained exactly the expected 4 new lines (two create/remove pairs), all valid JSON with `ts`+`event`. Server killed afterward; confirmed dead (connection refused, port free, no stray process).
