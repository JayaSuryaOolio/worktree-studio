# Progress log

Running log of work on worktree-studio across sessions. Newest entry at the top. Read this at the start of every new session before touching code — it says what's done, what's verified, and what's next per `PLAN.md`'s build order.

---

## 2026-08-07 — Step 7.5: terminal layout persistence (UI overhaul complete)

Last sub-step of the step 7 UI overhaul — the whole thing (7.1–7.5: sidebar, command palette, Command Deck visuals, dockview arrangement, this persistence) is now done. User asked to review at the end of this step.

**Completed:**

- `worktree_layouts(worktree_id, layout_json, updated_at)` table + `GetWorktreeLayout`/`SaveWorktreeLayout` store methods, `GET/PUT /api/repos/:repoId/worktrees/:worktreeId/layout` (this project's first `PUT` route). The layout is treated as an opaque JSON blob — the server never inspects its shape, just stores/returns dockview's own `toJSON()`/`fromJSON()` output verbatim.
- `WorktreeDetail.tsx`: on mount, fetches the saved layout (a `404` means none yet, not an error) and applies it via `dockviewApi.fromJSON()` once both dockview is ready and the fetch has resolved (order between those two isn't guaranteed — handled via a state variable for the api reference, not a ref, since refs don't retrigger effects); `seedPanels` then fills in any terminal that exists server-side but wasn't part of the saved layout. Every `onDidLayoutChange` debounces (500ms) a `PUT` of the current `toJSON()`.
- **Known simplification, recorded not hidden**: a saved layout referencing a terminal id that no longer exists server-side is left to fail gracefully at the websocket layer (`Terminal.tsx`'s own connection-error message) rather than this code hand-pruning dockview's serialized grid structure to remove the dangling reference. A real but rare edge case (only reachable if a terminal was deleted through some path that didn't get a chance to save first) with a non-destructive fallback.

**Two more real bugs found and fixed while doing this (not planned scope, found by hand):**

1. **SQLite foreign keys were never enforced anywhere in this schema.** `PRAGMA foreign_keys = ON` was never set on the connection — off by default. Verified empirically with a throwaway script (insert parent+child, delete parent, child row survives) before fixing. Every `ON DELETE CASCADE` declared since step 1 (`worktrees`→`repos`, `terminal_sessions`→`worktrees`) had been silently decorative the whole time. Fixed in `store.Open`. This broke `internal/term`'s existing tests, which created terminal sessions referencing a worktree id that was never actually inserted — silently worked before, correctly rejected now; fixed by seeding a real parent row in that test's helper. New regression test proves cascade now genuinely fires for the real schema.
2. (Carried from 7.4's fix, now doubly-confirmed correct): the explicit terminal-closing loop in `handleDeleteWorktree` was never relying on cascade to clean up the DB rows anyway (it calls `CloseSession` explicitly) — so bug #1 didn't silently break that fix, but it does mean the docs' earlier claim that "the DB row disappears via cascade regardless" was factually wrong at the time it was written. Corrected in `docs/architecture.md`.

**Verified:**

- `go build`/`go vet`/`gofmt`/`go test` all clean — **40/40** (5 new: 2 store tests for `worktree_layouts` + the FK-cascade regression, 3 API tests for the layout endpoint). `bun run build`/`bun run test` clean — **15/15** (2 new: layout-fetched-on-mount, and a real debounced-save test that exercises dockview's actual `toJSON()` output, not a fixture — asserts it's shaped like a real serialized layout rather than pinning exact contents, which would be brittle against dockview's own internal schema).
- **The actual persistence claim, verified for real against a live server** (this project's standing bar, per the tmux-restart precedent from step 2): registered a repo/worktree, `PUT` a realistic layout, confirmed `GET` returns it byte-identical immediately, **killed and fully restarted the Go server process**, confirmed `GET` still returns the exact same bytes. Also confirmed the served JS bundle contains the `/layout` route string.
- Test resources cleaned up; confirmed again (as in 7.4) that only the user's own real tmux sessions remain, not test artifacts.
- Still cannot verify actual drag/resize *feel* or exact visual rendering without a browser — the tests and real-server checks prove the mechanics are wired correctly, not that using it feels right. Worth a real look.

**Not built (still, unchanged from before this overhaul):** Monaco editor (step 5), git-diff/comment-to-agent (step 6/TODO). **Deferred as explicit TODOs:** theme switching. **Never, not deferred:** OS-level terminal popout.

---

## 2026-08-07 — Step 7.4: dockview terminal arrangement

**Completed:**

- `dockview` + `dockview-react` added (MIT, zero runtime deps). `WorktreeDetail.tsx` replaced its manually-toggled tab strip with a `DockviewReact` host — one panel per terminal session, panel `id` == `TerminalSession.id`. `Terminal.tsx` itself needed **zero changes** — a panel is just a React component in the tree.
- "+ New Terminal ▾" dropdown with three actions mapped directly to dockview's `addPanel({ position: { referencePanel, direction } })`: **New tab** (`within` — classic full-size tab switching), **Split right** (`right`), **Split down** (`below` — both real, simultaneously-visible, resizable tiles). No drag-to-float, no OS-level popout — the latter was considered and ruled out entirely, not deferred.
- Closing a panel wires dockview's `onDidRemovePanel` to the existing `DELETE .../terminals/:id`.
- Styled `.command-deck-dockview` by overriding dockview's `--dv-*` variables with Command Deck's own tokens, starting from the built-in "abyss" theme.
- `docs/architecture.md` got a comprehensive refresh covering the whole overhaul so far (sidebar/context, command palette, dockview) in one pass rather than three separate micro-edits, since the old flat-page description was fully stale by this point.

**A real bug found and fixed while testing this (not part of the planned scope, but directly related):** deleting a worktree never closed its terminal sessions. The `terminal_sessions` DB row disappears via `ON DELETE CASCADE` when the worktree row goes, but nothing else ever killed the actual tmux process behind it — leaking it forever with zero trace back to it in the database. Found by hand: created a test worktree+terminal, deleted the worktree through the API, then noticed via `tmux list-sessions` that the session was still alive. `handleDeleteWorktree` now closes every terminal session for a worktree (real `tmux kill-session` via `internal/term.Manager.CloseSession`) before removing it. Regression test (`TestDeleteWorktreeClosesItsTerminalSessions`) creates a real terminal, confirms its tmux session is live, deletes the worktree, confirms the tmux session is actually gone — not just the DB row. This also surfaced that `internal/api`'s test helper never set the `Server.Term` field at all (no terminal-endpoint Go tests existed before this one) — fixed as part of adding the regression test.

**Verified:**

- `go build`/`go vet`/`gofmt`/`go test` all clean — **35/35** (one new regression test). `bun run build`/`bun run test` clean — **13/13** (three new `WorktreeDetail.test.tsx` cases, mocking `./Terminal` and `./api` so the test is about panel/dropdown orchestration, not xterm/websocket internals).
- Real server + real bundle check: registered a repo, created a worktree and a terminal through the actual running API, confirmed the tmux session existed, confirmed the served JS bundle contains `dockview-theme-abyss`/`command-deck-dockview`.
- **While doing that verification, found two tmux sessions I did NOT create** — `wts-fa6adebbc09de5d9` and `wts-fcc1e1d9369a775b`, both rooted in `~/.worktree-studio/worktrees/.../urban-otter`, one running an actual Claude Code process. These belong to the user's own real, active work (matches their earlier screenshot) — inspected before touching anything, recognized they weren't mine, left them alone, and only cleaned up my own `/tmp`-rooted test session. Exactly the class of mistake the earlier "orphaned server" incident was about, caught this time by checking first.
- Cannot verify actual visual appearance, drag behavior, or resize-by-dragging without a browser (this session has none) — the frontend test suite proves the dropdown-driven placement logic is wired correctly (real DOM assertions), not that it looks or feels right. Still worth a real look before calling this step fully done.

**Next:** Step 7.5 — terminal layout persistence (`worktree_layouts` table + `GET/PUT .../layout`, frontend load/save with debounce). Last step of the UI overhaul; user asked to review at the end of this one.

---

## 2026-08-07 — Step 7.3: Command Deck visual design pass

**Completed:**

- `web/src/style.css` rewritten around a real token system (CSS custom properties in `:root`): named color tokens (`--bg`, `--bg-elevated`, `--border`, `--text`, `--text-dim`, `--amber`, `--cyan`, `--green`, `--red`, plus `-dim` variants for focus rings/selected backgrounds) and a 3-tier type system (`--font-display` Space Grotesk for page/section titles, `--font-ui` Inter for chrome, `--font-mono` the existing terminal stack extended to every data-shaped element — branch names, paths, status figures, table cells, sidebar rows, command palette items).
- Sidebar worktree rows restyled as the signature element: **flight strips** (ATC paper strips for tracking simultaneous in-progress flights) — a colored left-edge tab (red=dirty, green=clean, amber=currently-open) instead of a decorative dot, ahead/behind rendered as small tick marks. `Sidebar.tsx` sets an explicit `data-dirty="true"|"false"` attribute per row (read by CSS) rather than having CSS infer status from a child dot's class — deliberately explicit, one grep away if the mapping ever needs to change.
- `docs/design.md` (new): the token table, the "why this direction" self-critique against generic near-black+one-neon-accent defaults, and where the flight-strip status color actually comes from in code.
- `web/index.html` loads Space Grotesk/Inter from the Google Fonts CDN with real system-font fallbacks listed first in every font-stack — offline means slightly-less-on-brand headings, not broken text.
- Restyled every existing surface against the new tokens: dialogs, buttons, inputs, badges, the kebab menu, the command palette (previously all hardcoded grays/canvas-keywords).

**Verified:**

- `bun run build` / `bun run test` clean (10/10, unaffected by a pure-CSS pass as expected). Backend untouched (`go build`/`go test` clean).
- Confirmed against a real running server + real served bundle (not just local build output): the CSS asset actually contains `--amber`, `--bg-elevated`, etc., and `Space Grotesk` both in the CSS and as the actual Google Fonts link tag in the served `index.html`.
- Still can't verify actual visual appearance without a browser (this session has none) — the honest limit stated in `docs/running-locally.md`'s testing section applies here too. A real look is still worth doing before calling this pass fully done.

**Next:** Step 7.4 — dockview terminal arrangement (New tab / Split right / Split down dropdown, resizable splits).

---

## 2026-08-07 — Step 7.2: command palette (Cmd+K)

**Completed:**

- `cmdk` added (headless command-menu primitive — no default styling, same "wrap the proven primitive" call already made for dockview/tmux/spotlight/git).
- `CommandPalette.tsx`: global `Cmd/Ctrl+K` listener, fuzzy-searches repos + their worktrees, actions for "+ Add repo" and "+ New worktree in `<repo>`". Scoped deliberately narrow — jump/create only, not arbitrary command execution.
- Refactored `Layout.tsx`/`Sidebar.tsx`: add-repo and new-worktree modal state moved up from `Sidebar.tsx` into `Layout.tsx`'s `LayoutShell`, since both the sidebar's own "+" buttons and the new command palette need to open the *same* dialog instance rather than each owning a duplicate copy.
- Structural-only CSS for the palette (real Command Deck styling is next).

**Verified:**

- `bun run build` clean, backend untouched (`go build`/`go test` clean — noting one cosmetic non-issue: `go list ./...`/`go test ./...` now also picks up a bundled Go port shipped inside the `flatted` npm package's own `node_modules` transitively pulled in by `cmdk` — builds fine, adds to the reported package count, gitignored, doesn't affect correctness, not worth engineering around).
- Real component tests (`CommandPalette.test.tsx`, mocking `api.ts`): closed by default, opens on `Cmd+K`/`Ctrl+K`, selecting a worktree navigates to it and closes the palette, selecting "+ New worktree in X" calls the right callback with the right repo id and closes. Needed two jsdom polyfills in `vitest.setup.ts` (`ResizeObserver`, `Element.prototype.scrollIntoView`) since cmdk uses both internally and jsdom implements neither — documented inline, not silently patched.
- 10/10 frontend tests passing, all previously-passing tests unaffected by the `Sidebar`/`Layout` refactor.

**Next:** Step 7.3 — Command Deck visual design pass.

---

## 2026-08-07 — Real-usage bug report: worktree creation failing, orphaned state

User reported the actual app (not a test) failing to create a worktree named `oc-5678-2`: first attempt showed "failed to save worktree record," retrying showed "failed to create git worktree: ... a branch named 'oc-5678-2' already exists." Also flagged: a server I'd started during earlier verification steps was still running in the background with no visible logs or way to stop it.

**Root cause, in order:**
1. During step 2/3 verification, I ran a test server with `HOME=/tmp/wts-home` backgrounded via my own tooling, then later ran `rm -rf /tmp/wts-home` as part of cleanup — **without checking that server was still running**. That deleted its SQLite data directory out from under a live process.
2. The user then used that same (stray, forgotten-about) server for real work. `git worktree add` succeeded (filesystem operation, unaffected), but the subsequent DB write failed (the data directory it needed no longer existed) — surfaced as "failed to save worktree record."
3. **The real bug**: `handleCreateWorktree` had no rollback for this case. The successfully-created git worktree + branch were left behind with no DB record. Every retry with the same name then failed at the git layer ("branch already exists") — a dead end with no recovery path short of manual git surgery.
4. Separately, `git worktree remove` (used for cleanup elsewhere in the code) does **not** delete the branch it was created with — a real git behavior, not a bug, but it meant a naive rollback attempt would have only half-worked.

**Fixed:**
- `internal/gitops.DeleteBranch` (new): `git branch -D`, documented as the other half of undoing `AddWorktree`.
- `handleCreateWorktree` now rolls back both the worktree checkout AND the branch if `store.AddWorktree` fails, so a transient store failure (this specific cause, or disk-full/DB-locked in general) no longer leaves poisoned state blocking retries.
- Regression test (`TestCreateWorktreeRollsBackGitOnStoreFailure`) forces the store write to fail deterministically via a real `UNIQUE(path)` collision (not a mock) and asserts both the worktree directory and the branch are actually gone afterward — this is the test that caught the "remove doesn't delete the branch" gap; it failed on the first rollback attempt (worktree dir gone, branch still there) before `DeleteBranch` was added.
- New `TestDeleteBranch`-equivalent coverage added inline in `TestAddListRemoveWorktree` (`internal/gitops`), proving the branch really does survive `RemoveWorktree` alone.
- Cleaned up the actual damage: killed the stray server (PID had `PPID 1`, reparented/orphaned — started `2026-08-07 00:50:44`, five-plus hours before this was caught), and removed the two orphaned `oc-5678`/`oc-5678-2` branches and worktrees from the **real** `adelaide` repo (confirmed first: zero commits ahead of `main`, no uncommitted changes — pure test artifacts, nothing lost).

**Process fix — how to avoid repeating this:**
- `docs/running-locally.md` and the skill file now say explicitly: **run the server in your own foreground terminal**, not backgrounded/detached, specifically so you always have its logs and a plain Ctrl-C to stop it. Also documented how to find/kill an orphaned instance by port if one ever turns up anyway (`lsof -iTCP -sTCP:LISTEN | grep 8787`).
- For me, specifically: before any `rm -rf` on a directory a background process might still be using, check `lsof`/`ps` for live users of that path first — I did this reflexively for git-tracked repo state (per the standing safety instructions) but not for my own throwaway test scaffolding, which is exactly what bit here.

**Verified:** `go build`/`go vet`/`gofmt`/`go test` all clean (34/34, up from 33 — one new regression test, one extended existing test). Did not re-verify through a live server this time since the fix is narrowly scoped and already has a real regression test proving the exact failure mode; the next real session using the app is itself the remaining verification.

---

## 2026-08-07 — Step 4: worktree monitoring dashboard (dirty / ahead-behind)

**Design simplification, recorded here and in `PLAN.md`:** this step's original wording called for git-status "pushed over ws." Implemented as a plain 5-second `setInterval` REST poll instead (same pattern already used for spotlight status in step 3) — there's still no other consumer of a shared push channel anywhere in this codebase, so building one now would be speculative. Not a step that was skipped, just built the simpler way that already matches this project's established pattern.

**Completed this session:**

- `internal/gitops.StatusResult` extended with `HasUpstream bool`, `Ahead int`, `Behind int`, parsed from git's `# branch.ab +N -M` line (only present when the branch has a configured upstream at all — a freshly created worktree's branch typically doesn't, since `git worktree add -b <branch>` doesn't set one up).
- `GET /api/repos/:repoId/worktrees/:worktreeId/status` (`internal/api/status.go`): returns `{branch, dirty, has_upstream, ahead, behind}`.
- Frontend: `Workspace.tsx` polls both worktree-status and spotlight-status every 5s (`STATUS_POLL_INTERVAL_MS`), without disturbing the worktree list itself. `WorktreeList.tsx` gets a "Status" column — a dirty/clean badge, plus an ahead/behind indicator shown only when `has_upstream` is true and there's an actual difference (not just whenever ahead/behind happen to be present).
- Docs: `docs/architecture.md` updated (steps 1–4 now), `.claude/skills/worktree-studio/SKILL.md` gained a "Monitoring dashboard" section explicitly warning that `has_upstream: false` means "not comparable," not "in sync." `PLAN.md` records the REST-vs-ws simplification inline.

**Verified working (real commands run, not just claimed):**

- `go build ./...`, `go vet ./...`, `gofmt -l .` clean. `go test ./...` — 33/33 passing (2 new ahead/behind parsing tests in `internal/gitops`, using a real clone that diverges from its origin in *both* directions simultaneously to prove ahead and behind aren't just aliases of each other; 2 new tests in `internal/api` for the status endpoint's clean→dirty transition and a 404 on a missing worktree).
- Ran the full flow against a **real running server + real built frontend bundle**: created a fresh throwaway repo and worktree through the actual API, confirmed clean/no-upstream status, dirtied it and confirmed the badge would flip, then set up a genuine upstream-tracking relationship (`git branch --set-upstream-to`) and diverged both sides for real (one commit in the worktree, one commit on the "main" side) — confirmed the status endpoint reported exactly `ahead: 1, behind: 1`, not a guess or a mock.
- Confirmed the served JS bundle actually contains the new dashboard code (`git-status-badges`, `badge-dirty`, `has_upstream` strings present).
- All test repos/worktrees cleaned up, audit log showed the expected `repo.add`/`worktree.create`/`worktree.remove` lines, server process killed, no stray processes left.

**Not built (explicitly out of scope for this step):** Monaco editor, git-diff/comment-to-agent.

**Next up:** Step 5 — Monaco editor (file tree, open/save, external-change watch via `fsnotify`).

---

## 2026-08-07 — Frontend tooling switched to bun; Step 3: spotlight (design corrected mid-step)

**Tooling change:** switched `web/` from npm to [bun](https://bun.sh) — `bun install`/`bun run` in place of `npm install`/`npm run` everywhere (README, docs, skill, `main.go`'s placeholder page). `web/package-lock.json` replaced with `web/bun.lock`. No app code changes, same Vite/React/TS scripts, just run through a different tool.

**Important mid-step correction — read this before touching spotlight code:** `PLAN.md`'s original step 3 sketch (written before checking for prior art) had spotlight copying `node_modules`/`.env`/`.turbo` *from* the root repo *into* each worktree, with a manifest/staging/atomic-swap scheme. **That was backwards and has been replaced.** The user pointed at an already-built, already-installed tool (`github.com/JayaSuryaOolio/spotlight`, at `~/.local/bin/spotlight`) that does the opposite: it mirrors a **worktree's source files into the repo's root checkout**, continuously (fswatch+rsync), so you build/run from the root — which already has deps installed — while it reflects whichever worktree is in focus. `PLAN.md` section 2 now documents the corrected design; the old sketch is preserved there as a labeled "design correction" note, not deleted, so future-you can see what changed and why.

**Completed this session:**

- `internal/spotlight` (new package): thin `os/exec` wrapper — `Start`/`Stop`/`List`/`StatusForRoot` — around the installed `spotlight` CLI. Does **not** reimplement any sync/mirroring logic; the corruption-proofing (flattened-`.gitignore`-into-`--exclude-from`, dirty-root refusal, clean teardown via `git checkout`+`git clean`) all already lives in that external tool.
- `internal/api/spotlight.go`: `GET/POST /api/repos/:repoId/worktrees/:worktreeId/spotlight/{,start,stop}`. Status distinguishes "no mirror," "this worktree is the active mirror," and "a different worktree of this repo is the active mirror" (via a symlink-tolerant path comparison). A dirty root surfaces as `409` verbatim from the CLI's own refusal, not retried or forced. Both start/stop are audit-logged (`spotlight.start`/`spotlight.stop`); a refused start is not (nothing happened).
- Frontend: a "Spotlight" column in `WorktreeList.tsx` — Start/Stop per row, "will replace active mirror" wording when a sibling worktree is already active, an inline error message on the 409 dirty-root case.
- Docs: new `docs/spotlight-sync.md` (what the tool does, the integration surface, the macOS path-symlink bug found and fixed, manual verification steps, known limitations). `docs/architecture.md` and `docs/running-locally.md` updated (spotlight CLI + fswatch added as an optional prerequisite). `.claude/skills/worktree-studio/SKILL.md` gained a "Using Spotlight" section.

**Verified working (real commands run, not just claimed):**

- `go build ./...`, `go vet ./...`, `gofmt -l .` clean. `go test ./...` — 29/29 passing (5 new tests in `internal/spotlight` against real throwaway git repos: initial mirror, live-edit propagation, clean teardown, dirty-root refusal, switching-worktree-stops-previous; 2 new tests in `internal/api` exercising the same flow through real HTTP).
- **Real bug found and fixed mid-verification**: `StatusForRoot` did a raw string compare of paths, which silently failed against genuinely active mirrors whenever `/tmp` or `/var` are themselves symlinks into `/private` (true on macOS) — `t.TempDir()` paths and the CLI's own resolved paths for the *same directory* never string-equal. Fixed with `filepath.EvalSymlinks` on both sides before comparing; also needed a short startup delay in the live-edit test since `fswatch`'s watcher isn't instantly ready the moment `spotlight start` returns.
- Ran the full flow against the **real, running server** with the **real, built frontend bundle** (not just Go tests): registered a synthetic throwaway repo, created a worktree through the API, started spotlight, confirmed a file written in the worktree actually appeared in the root checkout, confirmed `GET .../spotlight/` reported `active: true`, stopped it, confirmed the root was restored to a clean `git status` and the mirrored file was gone.
- **Also verified against the real `adelaide` repo** (not just a synthetic one): registered it, created a worktree, called `spotlight/start` — got a real `409`, because `adelaide`'s actual root checkout genuinely has an uncommitted `yarn.lock` change right now. Confirmed nothing in the real repo changed as a result (`git status --porcelain` unchanged before/after). This is exactly the safety behavior the tool exists to provide, demonstrated against real state rather than a contrived test.
- All test worktrees, temp repos, and a temporary debug binary/module were cleaned up; no server process left running.

**Not built (explicitly out of scope for this step):** worktree status dashboard, Monaco editor, git-diff/comment-to-agent.

**Process notes:**
- Before writing any spotlight code, checked disk space and discovered the *original* (wrong) node_modules-copying design would have needed ~11.5GB against 15GB free — this constraint evaporated once the design was corrected, since source-only mirroring of `adelaide`'s tracked files is only ~22MB. Worth remembering: check assumptions about *what* a feature does before sizing *how* to test it.
- Followed the commit-checkpoint policy from `PLAN.md`/memory throughout this step (bun migration, `PLAN.md` correction, `internal/spotlight`, REST endpoints, frontend, docs — each its own commit) — no repeat of step 2's earlier accidental-combined-commit slip.

**Next up:** Step 4 — worktree monitoring dashboard (git-status polling pushed over ws, dirty/ahead-behind/spotlight-status badges in `WorktreeList.tsx`). Spotlight's own status is already surfaced per-row; step 4 adds git dirty/ahead-behind on top of that.

---

## 2026-08-07 — Step 2: terminal via tmux

**Completed this session:**

- `internal/store`: added `TerminalSession` CRUD (`AddTerminalSession`, `ListTerminalSessions`, `ListAllTerminalSessions`, `GetTerminalSession`, `RemoveTerminalSession`) — the `terminal_sessions` table existed as schema-only since step 1, now actually used.
- `internal/term`: new package. `Manager.CreateSession` runs `tmux new-session -d -s wts-<id> -c <worktreePath>` and records it; `CloseSession` kills the tmux session and removes the row; `ListLiveTmuxSessionNames` (treats "no server running" as an empty set, not an error); `Reconcile` prunes DB rows whose tmux session is no longer live (never kills a live one); `Attach`/`Resize` wrap `creack/pty` for `tmux attach-session` + `pty.Setsize`.
- `internal/api/terminals.go`: REST CRUD (`GET/POST .../terminals/`, `DELETE .../terminals/:id`) plus `GET /ws/terminals/:id`, a `gorilla/websocket` endpoint that attaches a pty to the tmux session and relays: binary ws messages = raw bytes into the pty, text ws messages = JSON control frames (currently just `{"type":"resize",...}`). `cmd/worktree-studio/main.go` now calls `term.Reconcile` at startup before the server starts accepting requests.
- Frontend: `Terminal.tsx` (xterm.js + `@xterm/addon-fit`, binary-input/JSON-resize protocol matching the server), `WorktreeDetail.tsx` (new route `/repo/:repoId/worktree/:worktreeId` — terminal tabs, "+ New Terminal", tab close, and a "⧉ New tab" button that does `window.open(window.location.href)`, satisfying the multi-repo/multi-window ask without any shared client state). `WorktreeList.tsx` links each row to its detail page.
- `docs/session-persistence.md` (new): the tmux rationale, wire protocol, manual restart-survival recipe, and a documented simplification vs. `PLAN.md`'s original sketch (one dedicated ws connection per terminal tab, not one multiplexed channel across tabs — there's no other consumer of a shared channel yet). `docs/architecture.md` and `docs/running-locally.md` updated for step 2 (tmux added as a prerequisite). `.claude/skills/worktree-studio/SKILL.md` got a new "Using terminals" section.

**Verified working (real commands run, not just claimed):**

- `go build ./...`, `go vet ./...`, `gofmt -l .` all clean. `go test ./...` — 22/22 passing (added `internal/term/term_test.go`: create/list/close, reconcile-drops-dead-sessions, reconcile-keeps-live-sessions, attach+resize+real-command-via-pty, using real `tmux`/pty, skipped gracefully if `tmux` isn't on `PATH`).
- `npm run build` in `web/` succeeds with the new terminal UI (xterm.js bundle included; only warning is bundle-size, expected for an internal tool).
- Built a small throwaway Go websocket test client (no `websocat` available) and drove a real running server end-to-end: registered the `adelaide` repo, created a worktree, created a terminal (confirmed `tmux list-sessions` shows `wts-<id>`), sent a command over the ws connection, confirmed the output (matching `tmux capture-pane` exactly) and the correct working directory.
- **The actual persistence claim**, verified for real: created a terminal, ran a command in it, `pkill`ed the Go server process, confirmed via `tmux list-sessions` the tmux session was still alive with the server gone, restarted the Go server, confirmed `GET .../terminals/` still listed the session, reattached over a fresh ws connection and saw the pre-restart scrollback plus successfully ran a new command (`POST_RESTART_MARKER`) — full round-trip proof the server restart didn't touch the running shell.
- Re-verified against the real production server (built binary + real embedded frontend, not `go run`): confirmed the served JS bundle actually contains the new terminal code (`New Terminal`, `ws/terminals`, `xterm-` strings present), SPA fallback works for the new `/repo/:id/worktree/:id` route, and ran the same create-terminal → ws command → cleanup flow against it successfully.
- Audit log gained correct `terminal.create`/`terminal.close` lines with `terminal_id`/`worktree_id`/`tmux_session_name`/`tab_label` fields.
- Honest limitation: no browser automation tool was connected this session, so the frontend was verified via clean `tsc`+`vite build`, bundle content inspection, and the real backend wire protocol — not an actual click-through in a rendered browser. The React/xterm wiring follows the same patterns already proven working in step 1's `RepoPicker`/`Workspace`, but a visual/interactive pass is still worth doing next time a browser tool is available.
- Test tmux sessions, worktrees, and repos created during verification were all cleaned up (`tmux list-sessions | grep wts-` empty at the end); no server process left running.

**Not built (explicitly out of scope for this step):** spotlight sync, worktree status dashboard, Monaco editor, git-diff/comment-to-agent.

**Process note:** the first attempt at this step accidentally landed steps `internal/store` + `go.mod` + `internal/term` in one combined commit despite the just-added commit-checkpoint policy in `PLAN.md` — caught immediately via `git show --stat HEAD`, undone with `git reset --soft HEAD~1`, and re-split into three properly-scoped commits before continuing. Worth double-checking `git status` right after any `git add` that follows an earlier one in the same session.

**Next up:** Step 3 — spotlight sync (manifest + staging + atomic swap + lock, triggered on worktree creation, manual re-sync button). Open question to resolve first: confirm this machine's/adelaide repo's `.yarnrc.yml` `nodeLinker` setting (`node-modules` vs `pnp`) before finalizing the exact allow-list of what gets synced.

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

## Post-step-7.5 fixes: worktree-switching bug, sidebar colors, auto-claude terminal (2026-08-07)

Three issues reported after reviewing the completed step 7 UI overhaul:

- **Bug: terminal arrangement didn't persist across reload, and worktrees appeared to share/leak each other's terminals.** Root cause: `WorktreeDetail` sat directly on the routed element, and React Router doesn't unmount/remount a route element when only its URL params change (same route pattern matched) — so navigating from one worktree to another kept the same dockview instance, the same terminal-list state, and the same "have I loaded the saved layout yet" flag from the *previous* worktree. Fixed by splitting `WorktreeDetail` into a thin outer component (extracts `repoId`/`worktreeId` from params) and an inner component keyed on `` `${repoId}:${worktreeId}` `` — the key change forces a full React remount (fresh dockview instance, fresh terminal fetch, fresh layout fetch) every time the worktree actually changes.
  - Getting the regression test right took a real diagnosis pass: an initial single-hop test (navigate w1 → w2, assert w1's terminal gone / w2's present) *passed even against the unfixed, buggy code* — dockview's own tab-visibility hiding masks the shared-instance bug on the very first navigation. Only a **round-trip** test (w1 → w2 → w1, asserting w1's terminal is back and w2's is gone) discriminates: verified by hand against both the buggy code (fails — stale terminal from w2 persists) and the fixed code (passes) before keeping the test.
- **Sidebar row colors simplified.** Previously every row color-coded dirty (red) / clean (green), and the active row got a third amber tint on top — reported as reading like noise rather than signal. Now every row is neutral gray by default and only the selected worktree gets an accent (a green left-edge tab + subtle inset glow); dirty/clean is still visible via the existing ahead/behind ticks, just not as the row's border color. `docs/design.md`'s flight-strip section updated to match.
- **New: creating a worktree auto-starts a `claude` terminal in it.** `internal/term.Manager.CreateSession` gained an `initialCommand` parameter, sent via `tmux send-keys -t <session> <command> Enter` as a real argv element right after the session is created (no shell involved, so no injection risk from the command string). Wired through `POST .../terminals/`'s new `initial_command` field, and a shared `createWorktreeWithClaudeTerminal` helper in `web/src/worktreeActions.ts` used by both worktree-creation entry points (the sidebar/command-palette flow in `Layout.tsx`, and `Workspace.tsx`'s own "+ New worktree" button). If the auto-terminal step fails, the worktree is still created successfully — the error is only logged, not surfaced as a blocking failure.

**Verified:**
- `go build ./... && go vet ./... && gofmt -l . && go test ./...` — clean, 42 tests passing (includes new `internal/term.TestCreateSessionRunsInitialCommand` and `internal/api.TestCreateTerminalWithInitialCommand`, both of which poll a real tmux pane's captured output for the expected string rather than trusting the API response alone).
- Frontend: `WorktreeDetail.test.tsx`'s new round-trip test, `worktreeActions.test.ts` (new file, covers the happy path and the swallow-the-error-but-still-return-the-worktree path).
- Real end-to-end pass against a built binary + real git repo (not just unit tests): registered a throwaway repo, created a worktree, created a terminal via the real HTTP endpoint with `initial_command: "claude"`, and confirmed via `tmux capture-pane` that `claude` was actually typed and run in the session — not just that the API returned 201. Also fetched the served production CSS bundle directly and confirmed it contains the new `--green-dim` token and the simplified `.sidebar-worktree.active` rule, with no leftover `data-dirty`-keyed color rules.
- Test repo, worktree, terminal, and tmux session all cleaned up afterward; test server killed; production binary rebuilt in place.
- **Not verified**: actual visual appearance in a browser (no browser tool available this session) — the CSS/behavior is confirmed correct in the served bundle and via the API, but no screenshot was taken.
- **Noted, not touched**: found roughly a dozen idle tmux `wts-*` sessions (beyond the two known real ones) with dead/deleted working directories, consistent with orphaned test sessions from earlier verification passes in this project's history. Left alone rather than killed, since tmux-session deletion is destructive and irreversible and none were confirmed with certainty to be disposable — worth a manual look (`tmux list-sessions`) and cleanup at your convenience.
