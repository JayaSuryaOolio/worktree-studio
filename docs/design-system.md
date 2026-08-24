# Worktree Studio — design system

The source of truth is `web/src/styles/tokens.css`. This document is the
rationale, the rules, and the things a stylesheet can't tell you.

Superseded `docs/design.md`'s single "Command Deck" theme — Command Deck
still exists, as one selectable option among three, mapped onto the
contract below rather than being the contract itself.

## The problem this solves

The first version of this UI was functionally complete and visually
exhausting. Concretely, from a real screenshot of ~20 worktrees:

- Every worktree row was a card — border, radius, background lift, chevron.
  Twenty entries in one list rendered as twenty separate objects.
- Four accents (amber/cyan/green/red) were in play simultaneously, and
  green meant "selected" in the sidebar, "clean" in the status column, and
  "active" in a badge printed on 100% of rows. A colour with three
  meanings has none.
- The worktrees table printed the same 62-character path prefix on every
  row, wrapping over three lines, while the branch name — the only thing
  that differed — was head-truncated (`hotfix-backend-se…`) exactly where
  branch names diverge.
- The most prominent string in the workspace was
  `[No pull request for this branch]`: an empty state, bracketed, in first
  position.
- `--text-dim` (#6b7684 on #0b0f14) measured 4.19:1 — below the 4.5:1
  floor — and carried every label, timestamp and secondary value in the app.

None of that is a taste disagreement; each item is a measurable property
that costs attention on every glance. This tool is looked at hundreds of
times a day and read maybe five, so the whole system optimises for the
glance.

## Principles

Each has a rule attached, because a principle you can't fail a review
against is a mood board.

1. **The terminal is the app.** Everything else is a frame, and frames are
   quiet. *Rule: chrome sits at `--text-2`, never `--text`.*
2. **Silence is the default state.** A row earns pixels only when
   something about it is abnormal. *Rule: if it renders on every row, it
   isn't a signal — delete it.*
3. **One accent; semantics are separate and rare.** `--accent` is
   selection, focus and the active tab, nothing else. `--attention` /
   `--danger` / `--ok` appear only on the exception. *Rule: at most one
   coloured element per row.*
4. **Rows, not cards.** Repeating items are separated by rhythm, not
   outlines. *Rule: zero borders on any repeatable item; selection is a
   wash plus a 2px rail, and the rail is on exactly one row at a time.*
5. **Show the difference, not the data.** *Rule: never print the same
   string twice in a column — elide shared prefixes, middle-truncate
   identifiers, use relative times with the exact value on hover.*
6. **The sidebar answers "what needs me?", ⌘K answers "take me there".**
   *Rule: sort by attention, then recency. Navigation features go in the
   palette, not in more sidebar.*
7. **Motion is a signal, not decoration.** *Rule: one blinking thing in
   the whole app (the attention dot, for its first 10s), everything else
   ≤ `--dur-slow`, and `prefers-reduced-motion` honoured.*

## The token contract

Every palette block defines **exactly** the same token names. A theme
that needs a token the others don't have isn't a theme, it's a redesign —
that guardrail is what stops this file sprawling back into per-component
colour soup.

| Token | Role | Rule |
|---|---|---|
| `--surface-0` | Workspace + terminal ground | Never pure black or pure white |
| `--surface-1` | Sidebar, dialogs, tab strips | Within ~5% lightness of `--surface-0` |
| `--surface-2` | Hover / pressed | The only per-row background that exists |
| `--surface-sunk` | Inputs, code blocks, insets | |
| `--rule` | Structural seams between panels | One per seam, never between sibling rows |
| `--rule-soft` | The faintest allowed division | |
| `--text` | Content: branches, terminal, filenames | ≥ 7:1 on `--surface-0` |
| `--text-2` | Chrome: labels, headers, secondary values | ≥ 4.5:1 |
| `--text-3` | Counts, timestamps, placeholders | ≥ 3:1, never for anything you must read |
| `--accent` | Selection, focus ring, active tab | Exactly one hue per theme |
| `--accent-soft` | Selected-row wash | Same hue, ≤ 12% alpha |
| `--accent-on` | Text drawn on an `--accent` fill | |
| `--attention` | A session is waiting on you | The one saturated fill in the app |
| `--ok` / `--warn` / `--danger` / `--info` | Status | Only on the exception |
| `--danger-soft` | Destructive hover | |
| `--overlay`, `--shadow-*` | Backdrops, elevation | |
| `--font-ui` | Chrome | One face. No display face. |
| `--font-mono` | Everything data-shaped | Branches, paths, counts, terminal |
| `--row-h`, `--row-h-lg` | List row heights | A theme picks these; components never override |
| `--radius`, `--radius-lg` | Corners | One value each; no per-component radii |

### Scales, fixed

- **Type**: `--fs-1` 11px · `--fs-2` 12px · `--fs-3` 13px · `--fs-4` 15px ·
  `--fs-5` 20px. Five sizes, no others. A sixth is a bug, which makes
  "is this on the scale?" reviewable instead of arguable.
- **Space**: `--space-1..6` = 4 / 8 / 12 / 16 / 24 / 32px.
- **Motion**: `--dur-fast` 120ms · `--dur` 160ms · `--dur-slow` 200ms,
  all on `--ease`. `--dur-slow` is a ceiling, not a default.

### No display face

There deliberately isn't one. Headings take their hierarchy from the type
scale and weight — one less font to load, one less voice competing with
the terminal, and one less thing that renders differently offline.
`--font-display` survives as an alias of `--font-ui` so nothing
downstream broke on the way through.

## Themes

Two axes, both stamped on `<html>`: `data-theme` (family) and `data-mode`
(dark/light).

- **Graphite** — modern, the default. Neutral graphite with a faint cool
  bias and one warm amber accent. Surfaces sit within ~4% lightness of
  each other on purpose: depth comes from spacing and type weight, not
  from stacking progressively lighter greys, which is what produced the
  "everything is a card" look in the first place. `--radius: 4px`.
- **Ledger** — classic. Warm paper and ink, square corners, one deep
  editor blue, in the Solarized/Sublime lineage without copying any of
  them. Deliberately more structured than Graphite: hairlines are meant
  to be *visible* here and rows are a step taller. That structure is what
  makes it read as classic rather than unfinished. `--radius: 0`.
- **Command Deck** — the original, kept so nothing is lost. Its four
  full-strength accents collapse onto the contract's one-accent-plus-
  exceptions shape, and its `--text-dim` is lifted from #6b7684 (4.19:1)
  to #7c8694 (5.25:1), with the original demoted to `--text-3` where it
  only carries counts and timestamps.

### Why palettes are element-scopable, not `:root`-only

The palette blocks are plain `[data-theme][data-mode]` selectors rather
than `:root[data-theme=...]`. That's what lets the Appearance tab's theme
swatches be *real palettes on a nested element* instead of hand-copied
hex values — a preview can't drift out of sync with the thing it
previews. The bare `:root` block carries the structural tokens plus a
complete Graphite-dark palette, so the app renders correctly even if the
attributes are never stamped (JS disabled, a script error, a stale
cached `index.html`).

### Mode resolution

`data-mode` is **always** a concrete `dark` or `light`. The stored mode
can be `system`, but resolution happens in `theme.ts` / the inline
pre-paint snippet in `index.html`, never in CSS. Two reasons:

- The stylesheet stays a flat set of palette blocks with no
  `prefers-color-scheme` branch, so there's no third code path where a
  token might resolve differently than the swatch that previewed it.
- "Follow the OS" remains something the user explicitly chose, which was
  the original argument for having no media query at all. `main.tsx`
  subscribes via `watchSystemMode` so the choice stays live rather than
  resolving once per page load.

The pre-paint snippet in `index.html` is duplicated from `theme.ts` on
purpose: it has to run synchronously before any CSS paints, and importing
a module means waiting on the module graph — which is exactly the flash
it exists to prevent.

### Migration from the old key

Pre-redesign, the whole theme was one `worktree-studio-theme` key holding
`"dark"` or `"light"`. It's read once, for the mode axis only, so an
existing install keeps the light/dark it had chosen and picks up
Graphite as its family. It is never written back to.

## No compatibility aliases

The migration ran through the old token names (`--bg`, `--border`,
`--text-dim`, `--amber`, `--cyan`, `--green`, `--red`, `--font-display`)
kept as aliases pointing at the contract, so each stylesheet could be
converted one commit at a time without anything going unstyled in
between. Every one of them is now gone: the audit that removed them
(`grep` for legacy token names and for raw hex across `web/src`) came
back empty except for `tokens.css` itself.

That audit is worth re-running before adding a token. Two invariants it
checks, both of which the pre-redesign code violated:

- **No stylesheet outside `tokens.css` contains a raw hex colour.** That's
  what makes a new theme a matter of adding one palette block.
- **No stylesheet uses a colour name that isn't in the contract.** A
  component reaching for a colour the contract doesn't define is the
  first step back toward the four-overloaded-accents problem.

Note in particular that `--green` no longer means "selected" anywhere.
Selection is `--accent` in every list in the app; `--ok` is the status
colour, and it appears only on an exception.
