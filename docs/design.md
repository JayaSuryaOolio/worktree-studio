# Command Deck — visual design system

The only theme worktree-studio has (see `PLAN.md`'s theme-switching TODO for why there's no switcher yet). Dark, high-density, mission-control-for-coordinating-parallel-worktrees. Defined as CSS custom properties in `web/src/style.css`'s `:root` block — this doc is the rationale, the stylesheet is the source of truth for exact values.

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

## Extending this later

- **Theme switching** (TODO, not built): the token structure already makes a second theme cheap — define an alternate `:root[data-theme="..."]` block with the same variable names, add a toggle. Not worth building until there's a second theme to switch to.
- **Dockview's own theme** (step 7.4): override its `--dv-*` custom properties with these same tokens rather than fighting its DOM structure — see `docs/ui-overhaul-plan.md`.
