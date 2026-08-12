# Command Deck — visual design system

Dark, high-density, mission-control-for-coordinating-parallel-worktrees — the default theme (and, until a light theme shipped, the only one). Defined as CSS custom properties in `web/src/styles/tokens.css`'s `:root` block — this doc is the rationale, the stylesheet is the source of truth for exact values. `web/src/style.css` itself is just an `@import` manifest over `web/src/styles/*.css`, split by feature area (tokens, base, dialogs, sidebar, worktree-detail, file-tree, ...) — see that file's own header comment.

## Why this direction

Checked against what a generic pass at this brief would produce: near-black background, one arbitrary neon accent (acid-green or vermilion), a colored dot per list row. Command Deck differs on purpose:

- **Blue-black, not near-black** (`--bg: #0b0f14`) — reads as "cockpit at night," not "default dark mode."
- **Four grounded semantic accents** (amber/cyan/green/red — `--amber`/`--cyan`/`--green`/`--red`), each tied to a real avionics/radar reference, rather than one decorative pop color with no meaning behind it.
- **A signature element derived from the actual subject**: sidebar worktree rows are styled as **flight strips** — the paper strips air-traffic controllers use to track multiple simultaneous in-progress flights. Branch name = the "callsign" (mono, prominent), small tick marks = ahead/behind counts. Git worktrees genuinely are multiple simultaneous in-progress things being coordinated, so this isn't decoration — it's the actual thing this tool does, rendered as UI. (Row color itself was simplified after real usage — see below — so the left-edge tab now marks *selection*, not dirty/clean status.)
- **A 3-tier type system**, not one display font slapped on top of existing chrome:
  - **Display** (`--font-display`, Space Grotesk) — page/section titles only, used with restraint.
  - **UI** (`--font-ui`, Inter) — buttons, labels, nav, form chrome. The neutral workhorse.
  - **Data** (`--font-mono`, the same stack already used in `Terminal.tsx`) — extended to *everything data-shaped*: branch names, paths, status figures, ahead/behind counts, sidebar rows, table cells, terminal content. This is what actually ties the whole app together — everything that's data reads like telemetry, everything that's chrome reads like a normal app.

## Color tokens

| Token | Value | Used for |
|---|---|---|
| `--bg` | `#0b0f14` | Page background, terminal area |
| `--bg-elevated` | `#12181f` | Sidebar, dialogs, dropdown menus, dockview group headers |
| `--bg-hover` | `#1a212b` | Hover state on rows/buttons |
| `--border` | `#232b35` | Hairline seams between panels |
| `--text` | `#d9e1e8` | Primary text (soft instrument-white, not pure `#fff`) |
| `--text-dim` | `#6b7684` | Secondary labels, timestamps, muted chrome |
| `--amber` | `#e8a33d` | Primary accent: selected / active / focus |
| `--cyan` | `#4fb6c7` | Secondary accent: links, spotlight-active indicator |
| `--green` | `#5fa777` | Status: clean / in-sync |
| `--red` | `#d9534f` | Status: dirty / error / conflict |

`--amber-dim`/`--cyan-dim`/`--red-dim` are the same hues at low alpha, for focus rings and selected-row backgrounds rather than a full-strength fill.

## Fonts and offline behavior

Space Grotesk and Inter load from the Google Fonts CDN (`web/index.html`) — every rule that uses them lists a real system-font fallback first in the stack (`--font-display`/`--font-ui`), so working offline means slightly-less-on-brand headings, not broken layout or missing text. The mono stack (`--font-mono`) is entirely system fonts already, no network dependency at all.

## Where the flight-strip row color comes from

Originally `Sidebar.tsx` set `data-dirty="true"|"false"` on each row and CSS colored the left-edge tab red/green from it, with the *active* row getting a third amber color on top. In practice all three colors on every row at once read as noise, not signal — reported directly after the first real look at the built UI. Simplified to: every row is neutral gray (`--border`) by default, and only `.sidebar-worktree.active` (the currently-selected worktree, via the router's `NavLink` active state) gets the accent — a `--green` left-edge tab plus a subtle `--green-dim` inset box-shadow. Dirty/clean is still visible in the row (via the ahead/behind ticks, sourced the same way from `internal/gitops.Status`), it's just no longer encoded as the row's border color.

## Light theme

Built exactly the way the section below once predicted it would be: a second block, `:root[data-theme="light"]` (also in `tokens.css`), redefining the same variable names — nothing else in the codebase needed to change, since every other stylesheet already consumed colors exclusively via these custom properties. Not a straight invert: the light palette's amber/cyan/green are noticeably darker/more saturated than their dark-theme counterparts, since the dark theme's versions wash out on a near-white background. Dark stays the default and needs no `data-theme` attribute at all; picking light sets it via `web/src/theme.ts` (`localStorage`-backed, not tied to `prefers-color-scheme` — an explicit choice, not one that silently follows the OS), switched from a toggle in the main `SettingsModal.tsx`'s Appearance tab. `web/index.html` has a small inline script that applies a stored light preference before first paint, to avoid a flash of dark-then-light.

## Extending this later

- **Dockview's own theme** (step 7.4): override its `--dv-*` custom properties with these same tokens rather than fighting its DOM structure — see `docs/ui-overhaul-plan.md`. Note this needs to cover the *inactive*-tab variables too (`--dv-activegroup-hiddenpanel-tab-*`/`--dv-inactivegroup-hiddenpanel-tab-*`), not just the active/visible ones — a real bug (missing those four) left every non-selected terminal tab on the built-in "abyss" theme's hardcoded near-black/white colors regardless of which theme was actually selected.
