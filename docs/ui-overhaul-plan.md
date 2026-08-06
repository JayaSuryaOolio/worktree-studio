# Worktree Studio — UI overhaul: sidebar shell, Command Deck visual design, split-based terminal arrangement

## Context

The current UI (built across steps 1–4) is functionally complete but visually and structurally minimal by design — three flat routes, no shared navigation, plain unthemed CSS, and terminals as a plain tab strip. Now that the core features work, the ask is to make this feel like a real coordination tool for parallel worktree-driven work: a persistent left sidebar that always shows what's open (so switching between parallel tasks doesn't mean navigating back through list views), a deliberate visual identity instead of generic default styling, and a terminal-arrangement model that goes beyond tabs — explicit split-right/split-down/new-tab placement, not just one-at-a-time switching.

Explored the current frontend in full (routes, API client, styling, dependencies — nothing missing on the data side, this is a structural+visual rework, not a backend-capability gap except for one new piece: persisting the terminal layout). Confirmed via research that **dockview** (`github.com/mathuo/dockview`, MIT, zero runtime deps) is the right library for the split/tile terminal arrangement — it directly supports adding a panel positioned relative to another (`right`/`below`/`within` an existing panel, i.e. exactly "split right / split down / new tab"), plus full layout serialization for persistence. This matches the project's standing "wrap the best available tool, don't reinvent" philosophy (same call already made for git, tmux, spotlight).

## Decisions locked in from clarification

- **Terminal arrangement UX**: a "+ New Terminal ▾" dropdown on each worktree's terminal view with three explicit actions — **New tab**, **Split right**, **Split down**. No free-form drag-to-float, and **no OS-level popout window support at all — not built, not a deferred TODO, simply out of scope permanently.**
  - **New tab** = classic single-visible-at-a-time tab behavior: adds the terminal as a tab within the *currently active* dockview group (`direction: 'within'`). Selecting that tab shows it **full size**, other tabs in the same group hidden — this is a tab strip, not a tile.
  - **Split right** / **Split down** = tiled/simultaneously-visible panels (`direction: 'right'` / `'below'`), each independently a tab group (so a split panel can itself grow its own tabs later).
  - Maps directly to dockview's `addPanel({ position: { referencePanel, direction } })` API. Dockview's native drag-to-rearrange still works once panels exist, but the dropdown is the primary, required interaction — not a "figure out drag gestures" affordance.
  - **Terminals must be resizable**: dragging the boundary between two split panels resizes them — this is dockview's native split-view behavior, not new code, but it's a hard requirement to verify (see build order) rather than an assumption.
  - **Theme switching**: also not built now — Command Deck is the only theme. Record as a `PLAN.md` TODO (the CSS-custom-property token structure makes adding alternates cheap later), but don't build a switcher UI now.
- **Layout persistence**: backend/SQLite, not localStorage. New `worktree_layouts` table + `GET/PUT /api/repos/:repoId/worktrees/:worktreeId/layout`. Matches this project's actual persistence precedent (repos/worktrees/terminal_sessions are all SQLite; nothing meaningful is ever localStorage-only) and survives across browsers/machines for free.
- **Visual direction: "Command Deck"** — dark, high-density, mission-control-for-parallel-tasks. Concrete token system below; this is a deliberate choice, self-checked against the frontend-design skill's warning about generic "near-black + single neon accent" defaults (see "Self-critique" below).
- **This is an SPA, not a multi-page-feeling app**: no page-transition animations between routes — the persistent sidebar/chrome never remounts, `<Outlet/>` content swaps instantly. New interactions beyond the sidebar should be **modals, dropdowns, and a command palette** — not new full "pages." Concretely this adds a **command palette (Cmd+K)** as a first-class navigation/action surface (see below), scoped to: fuzzy-jump to any registered repo or open worktree, trigger "New worktree," jump into the current worktree's terminal view. Built with **`cmdk`** (small, MIT-licensed, headless — the same "wrap a proven primitive" call already made for dockview/tmux/spotlight/git), not hand-rolled fuzzy-matching.

## Command Deck: design tokens

**Color** (named, not generic grays):
```
--bg:          #0B0F14   deep blue-black ("cockpit at night" — not pure #000/near-black)
--bg-elevated: #12181F   sidebar / panel surfaces / dockview group headers
--border:      #232B35   hairline seams between panels, avionics-panel-seam feel
--text:        #D9E1E8   soft instrument-white (not pure #fff — reduces the "screen glow" cliché)
--text-dim:    #6B7684   secondary labels, timestamps, muted chrome
--amber:       #E8A33D   primary accent — selected/active/focus (avionics amber, used affirmatively)
--cyan:        #4FB6C7   secondary accent — links, info, spotlight-active indicator (radar-scope cyan)
--green:       #5FA777   status: clean / in-sync (subdued, not neon)
--red:         #D9534F   status: dirty / error / conflict (desaturated alarm red, not pure #f00)
```
Multiple grounded semantic accents (amber/cyan/green/red, each tied to a real avionics/radar reference) rather than one arbitrary neon pop color is the deliberate departure from the skill's named generic-default #2 ("near-black + single bright accent").

**Type** (3-tier, deliberately not "one display font slapped on top"):
- **Display** (repo names, page/section headers only, used with restraint): a technical/geometric face with real character — **Space Grotesk**.
- **UI chrome** (buttons, nav labels, form labels — the neutral workhorse): **Inter** (or system-ui fallback stack).
- **Data** (branch names, paths, status figures, ahead/behind counts, sidebar worktree rows, all terminal content): the existing monospace stack already used in `Terminal.tsx` (`ui-monospace, SFMono-Regular, Menlo, Consolas, monospace`) — **extended to every data-shaped piece of UI, not just the terminal itself**. This is what actually ties the whole app together: everything that's *data* reads like telemetry, everything that's *chrome* reads like a normal app, only page titles get the display face.

**Signature element**: sidebar worktree rows styled as **flight strips** — the real paper strips air-traffic controllers use to track simultaneous in-progress flights (a thin horizontal card, callsign on the left, status as a colored tab at the edge, small tick-mark annotations). Here: branch name as the "callsign" (mono, prominent), a colored left-edge tab for dirty/clean/spotlight-active state (amber/green/cyan), ahead/behind as small tick counts. This is subject-specific — git worktrees genuinely are multiple simultaneous in-progress things being coordinated — not a generic colored-dot-in-a-list-item.

**Self-critique (per the frontend-design skill's required pass)**: checked this against "what would the generic version of this brief produce" — a generic AI pass would land on near-black + one neon green/vermilion accent with a colored dot per list row. Differentiated by: (a) blue-black not near-black, (b) four grounded semantic colors instead of one decorative accent, (c) a real signature element (flight strips) derived from the subject (coordinating parallel work) rather than decoration, (d) the mono-for-data/sans-for-chrome split as a functional system, not just a font pairing choice.

## Layout architecture (sidebar shell)

- **`web/src/Layout.tsx`** (new): wraps all routes via `<Outlet/>`, renders `<Sidebar/>` + `<main>` in a CSS grid (`280px 1fr`). `App.tsx` becomes:
  ```tsx
  <Routes>
    <Route element={<Layout />}>
      <Route path="/" element={<RepoPicker />} />
      <Route path="/repo/:repoId" element={<Workspace />} />
      <Route path="/repo/:repoId/worktree/:worktreeId" element={<WorktreeDetail />} />
    </Route>
  </Routes>
  ```
- **`web/src/RepoContext.tsx`** (new): `RepoProvider` mounted in `Layout.tsx`, above `<Outlet/>`. Owns: registered repos list, `selectedRepoId` (derived via `useMatch("/repo/:repoId/*")` — verify this resolves correctly from a provider mounted above the matched route, since plain `useParams` would not), the selected repo's worktrees, and the existing 5s git-status/spotlight-status poll **moved here from `Workspace.tsx`** so `Sidebar.tsx` and `Workspace.tsx` read the same fetched data instead of polling twice. `useRepoContext()` hook exported for consumers. Create/delete/spotlight-toggle *action* handlers stay local to `Workspace.tsx` (they call `api.ts` directly, then `refreshWorktrees()` from context) — only the read/poll side moves.
- **`web/src/Sidebar.tsx`** (new): consumes `useRepoContext()`. Registered repos (compact), and under the selected repo, its worktrees rendered as flight-strip rows (mono branch name, colored status tab, ahead/behind ticks), linking to `/repo/:id/worktree/:wtId`. Auto-loads whenever `selectedRepoId` changes — including a fresh tab opened straight at a worktree URL (the "⧉ New tab" button already does this), satisfying "loads automatically when I open a new tab and select the repo."
- **Trim in the same commit** (avoid a double-listing window): `RepoPicker.tsx` stops rendering its own repo table (keeps the "add a repo" form); `Workspace.tsx` keeps its worktree table (it's the detailed view — create/delete/spotlight actions live there) but no longer owns the polling state, only consumes it from context.

## Command palette (Cmd+K)

- **`web/src/CommandPalette.tsx`** (new), built on **`cmdk`**. Global keyboard shortcut (Cmd/Ctrl+K) mounted once near the root (`Layout.tsx`), rendered as a modal overlay (consistent with the "modals/dropdowns/palettes, no new pages" interaction language) — not a route, not something that changes the URL until an item is actually chosen.
- Data source: `useRepoContext()` — lists every registered repo and (for the selected repo) every worktree as jump targets; navigating via the palette is just `navigate()` to the existing routes, no new pages created.
- Actions included now: jump to a repo, jump to a worktree, "New worktree" (opens the existing `NewWorktreeDialog` modal rather than duplicating that form), jump to the current worktree's terminal view. Not building arbitrary command execution or terminal-targeted actions (e.g. "new terminal split right" from the palette) in this pass — the dropdown already covers that, and scope creep here isn't worth it.

## Terminal arrangement (dockview)

- Add `dockview` + `dockview-react` to `web/package.json`.
- **`web/src/WorktreeDetail.tsx`**: replace the current tab-strip (manual `activeId` + `display:none` toggling) with `DockviewReact`. One panel per terminal session, panel `id` == `TerminalSession.id` (ties saved-layout panel refs directly to real terminal sessions — no separate id space). Panel component is a thin wrapper rendering the existing `<Terminal terminalId={...}/>` **unchanged** — dockview panels are just React components in the tree, so the xterm/websocket lifecycle in `Terminal.tsx` needs no changes.
- **"+ New Terminal ▾" dropdown**, three actions:
  - *New tab* → `dockviewApi.addPanel({..., position: { referenceGroup: activeGroup, direction: 'within' }})`
  - *Split right* → `direction: 'right'`
  - *Split down* → `direction: 'below'`
  Each also calls the existing `createTerminal(repoId, worktreeId)` first to get a real `TerminalSession`, then adds the panel with that id.
- Closing a panel (dockview's own close button, or `onDidRemovePanel`) calls the existing `deleteTerminal`.
- On mount: `getWorktreeLayout(repoId, worktreeId)` — if present, filter out any panel referencing a `terminalId` no longer in `listTerminals` (e.g. a dead tmux session pruned by server-side reconciliation) before `dockviewApi.fromJSON(layout)`; if absent, lay out whatever `listTerminals` currently returns as a simple default tiling.
- Save: debounce (~500ms) on dockview's `onDidLayoutChange`, `PUT` `dockviewApi.toJSON()` via a new `saveWorktreeLayout`.
- Styling: start from dockview's dark theme CSS, override its `--dv-*` custom properties with the Command Deck tokens above rather than fighting its DOM structure.

### Backend: `worktree_layouts`
- `internal/store/store.go`: new table
  ```sql
  CREATE TABLE IF NOT EXISTS worktree_layouts (
    worktree_id TEXT PRIMARY KEY REFERENCES worktrees(id) ON DELETE CASCADE,
    layout_json TEXT NOT NULL,
    updated_at  TEXT NOT NULL
  );
  ```
  plus `GetWorktreeLayout(worktreeID string) (string, error)` (returns `sql.ErrNoRows` if none saved) and `SaveWorktreeLayout(worktreeID, layoutJSON string) error` (upsert).
- `internal/api/api.go`: `r.Get("/layout", s.handleGetLayout)` / `r.Put("/layout", s.handleSaveLayout)` under the worktree sub-router — the project's first `PUT` (correct verb for "replace this resource's state," no need to force it into the existing POST/DELETE pattern).
- `web/src/api.ts`: `getWorktreeLayout`/`saveWorktreeLayout`.

## Build order (checkpointed commits, per this project's established rhythm)

1. **Layout shell + sidebar + `RepoContext`, no visual redesign yet.** Verify in a real browser (via the `run` skill): sidebar worktree list appears automatically on `/repo/:id` and on a fresh tab opened at `/repo/:id/worktree/:wtId`; only one 5s-poll network pattern visible in devtools, not two; confirm no full-page-reload/transition happens when navigating between repos/worktrees (sidebar/chrome persist, only `<Outlet/>` content swaps). Commit.
2. **Command palette (Cmd+K)** — `cmdk` package add, `CommandPalette.tsx`, wired to `RepoContext` for repo/worktree jump + "New worktree." Verify: Cmd+K opens from any route, fuzzy-filters repos/worktrees, selecting one navigates without a full reload, Escape closes. Commit.
3. **Command Deck visual pass** — CSS custom properties for the token system above, applied across `Sidebar.tsx`/`Layout.tsx`/`WorktreeList.tsx`/`CommandPalette.tsx`/existing components; flight-strip sidebar rows. Before dockview, so its panels get styled once against real tokens. Commit (likely 2-3: tokens+base, then component polish).
4. **Dockview integration** — package add, `WorktreeDetail.tsx` rework, the New Terminal ▾ dropdown with the three placement actions, styled against Command Deck tokens from the start. No persistence yet (resets on reload). Verify explicitly: (a) each dropdown action places correctly — new tab shows full-size and hides sibling tabs in the same group, split-right/split-down show simultaneously as tiles; (b) dragging a split boundary actually resizes both sides (real requirement, not assumed); (c) devtools network tab shows no websocket reconnects when panels are rearranged. Commit.
5. **Layout persistence** — Go store/route commit (with tests matching `internal/store/store_test.go`/`internal/api/api_test.go` patterns) first, then frontend load/save wiring as a second commit. Verify: arrange a non-default split layout, reload → restored; kill and restart the Go server → still restored (this project's standing bar for persistence claims, per the tmux-restart precedent in PROGRESS.md's step 2 entry).

Each step ends with a docs update (`docs/ui-layout.md` new, covering the sidebar/context/command-palette model and the dockview/layout-persistence design; `.claude/skills/worktree-studio/SKILL.md` gets sections on the command palette and arranging terminals) and a `PROGRESS.md` entry — this project's standing "docs are part of done" convention. Add theme-switching as an explicit TODO in `PLAN.md`. OS-level popout is not recorded anywhere as a TODO — it's not wanted, full stop.

## Risks flagged, not blocking

- `useMatch("/repo/:repoId/*")` from a provider mounted above the matched route is the one piece of the context design that isn't a mechanical extension of existing patterns — verify early in step 1.
- Dockview's `--dv-*` variable theming should be low-override (confirm in step 3/4, don't expect to fight its DOM).
- Panel moves/splits within the same window shouldn't remount `Terminal.tsx` (same-document DOM moves) — confirm empirically in step 4's verification rather than assuming.
- `cmdk` is headless (no default visual styling) — it's a state/filtering primitive, the palette's actual look is entirely step 3's Command Deck tokens; don't expect it to look reasonable before that pass.

### Critical files
`web/src/App.tsx`, `web/src/Workspace.tsx`, `web/src/WorktreeDetail.tsx`, `web/src/api.ts`, `web/src/style.css`, `internal/api/api.go`, `internal/store/store.go`.
