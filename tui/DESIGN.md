# wts-tui design system

Status: build steps 1–5 done (keymap, theme, status bar, sidebar tree,
splits, resizing, mouse, command palette, saved layouts). See *Build order*.

## The problem it solves

The prototype has two layers of keys that fight each other. Bare letters
(`j`, `c`, `s`, `1`) are shortcuts in the sidebar but text inside a terminal.
Leaving a terminal therefore takes `ctrl+]` first, and nothing on screen says
which layer you're in. Every rule below comes from one principle:

> **Bare keys and `ctrl+letter` always belong to whatever has focus.**
> The app only takes keys that shells and Claude don't use, and the
> current mode and its keys are always shown on screen.

The browser already works this way (`web/src/keyboard.ts` never steals the
tmux prefix, and Cmd+K works from inside a terminal). The TUI follows the
same contract.

## Layout

```
 worktree-studio      1 claude   2 shell   3 claude+shell   +                    ● 2 need you
 ▾ Accounts(SSO)        ━ claude ──────────────────────────────┬─ shell ───────────────────────
 ▌feat/serve…OOLIO-59 ↓3│ > fix the failing auth test           │ $ go test ./...
  get-user-orgs     ↓11 │                                       │ ok   worktree-studio/tui  0.4s
  loading-state         │ ● Reading internal/api/terminals.go   │
 ▸ Accounts             │                                       ├─ shell ───────────────────────
 ▸ Backoffice         ● │                                       │ $
 ▸ POS                  │                                       │
 TERMINAL  alt+arrows move  alt+1-9 tab  ctrl+space menu                        server :8787 ✓
```

Four regions, always in the same place:

| Region | What it is |
|---|---|
| **Sidebar** (left, resizable) | Repos → worktrees as one collapsible tree. Replaces `[`/`]` repo paging. Attention `●` and git ticks stay. |
| **Tab bar** (top of main) | Tabs of the **selected worktree**. Each server terminal session lives in exactly one tab. |
| **Panes** (main) | A tab is a split tree of panes, and each pane is one live session (claude or shell). Splits are resizable. |
| **Status bar** (bottom row) | Mode chip, then the keys that work *right now*, then server/attention on the right. |

### Tab model

- **Tab = a layout. Pane = a session.** A new session made in the browser
  appears as its own tab. Splitting puts a new or existing session beside
  the current one.
- Each worktree keeps its own tabs. Selecting another worktree switches the
  whole tab bar, and coming back restores the last tab and focused pane.
- The layout (which sessions are in which tab, and the split ratios) is
  TUI-local state in `~/.config/wts-tui/layouts.json`. The server only
  knows the session list, so the browser is unaffected.
- **Closing a pane** removes it from the layout. **Killing** the tmux
  session is a separate action that asks for confirmation.
- **A new empty split asks what to run:** claude, shell, or any of this
  worktree's sessions not currently shown. That is also how you bring back
  a session you closed.

## Color: one accent

The palette is the browser's **Graphite dark** (`web/src/styles/tokens.css`),
mapped to truecolor with 256-color fallbacks.

| Token | Hex | Used for |
|---|---|---|
| `surface-0` | `#131517` | background (terminal default, not painted) |
| `surface-2` | `#202429` | selected sidebar row, active tab, modal body |
| `rule` | `#24282c` → `#3a3f45`* | dividers, unfocused pane headers |
| `text` | `#dfe1e4` | primary text |
| `text-2` | `#9aa1a8` | secondary: inactive tabs, branch names |
| `text-3` | `#6c747c` | hints, counts, ticks |
| **`accent`** | **`#e0913c`** | **the focused thing, and nothing else** |
| `attention` | `#f0b429` | `●` only: Claude is waiting on you |
| `ok` / `danger` | `#6fae7c` / `#d1685f` | git ↑↓ and errors, as glyphs only, never fills |

\* Dividers need to be lighter than the web's `rule` to show up in a terminal.

**The accent rule:** at any moment exactly one thing on screen uses
`accent`. That is where your keystrokes go:

- focused pane: its header line is drawn `━` in accent with an accent title; other pane headers are `─` in `rule`
- focused sidebar: the `▌` bar on the selected row is accent (the row background stays `surface-2` either way)
- a modal or palette: the selected row is accent, and the panes behind it drop to `text-3`

The active tab is shown with a `surface-2` background and bold `text`, not
accent. Tabs say *where you are*; the accent says *where you're typing*.
The same mixing up of those two is why the prototype felt ambiguous.

Semantic colors (`attention`, `ok`, `danger`) appear only as one-cell glyphs,
never as backgrounds, so they can't be mistaken for focus.

## Keys

### Rules

1. **Never steal from a terminal.** No bare letters, no `ctrl+letter`
   (that would break readline, Claude's own keys and the tmux prefix
   `ctrl+b`).
2. **The app owns `alt` and one leader key, `ctrl+space`.** Both reach the
   TUI from inside Claude.
3. **Arrows mean space.** Moving focus and resizing both use arrows, so
   there are no `h`/`j`/`k`/`l` or brackets to learn.
4. **The leader shows a menu.** You never have to memorize what comes
   after `ctrl+space`.

### Direct keys (work everywhere, including inside Claude)

| Key | Action |
|---|---|
| `alt+←` `alt+→` `alt+↑` `alt+↓` | move focus to the neighbouring pane; `alt+←` from the leftmost pane goes to the sidebar |
| `alt+shift+arrows` | grow the focused pane in that direction (at the screen edge, its nearest divider on that axis moves instead, shrinking it) |
| `alt+1` … `alt+9` | go to tab N |
| `ctrl+space` | leader menu (below) |
| mouse | click a tab, pane or sidebar row to focus it; drag a divider to resize; scroll wheel goes to the pane (scrolls if the tmux session has `mouse on`) |

### Leader menu (`ctrl+space`, then one key; the menu appears at the bottom right)

```
╭ ctrl+space ──────────────────╮
│  c      new Claude tab        │
│  t      new shell tab         │
│  →  ↓   split right / down    │
│  x      close pane            │
│  z      zoom pane (toggle)    │
│  r      resize mode           │
│  n      new worktree          │
│  space  command palette       │
│  q      quit (sessions keep running) │
╰───────────────────────────────╯
```

`r` enters **resize mode**: arrows resize, `shift+arrows` resize in bigger
steps, and `esc` or `enter` leaves. The status bar chip reads `RESIZE` the
whole time.

### Sidebar (when it has focus)

| Key | Action |
|---|---|
| `↑` `↓` | move |
| `→` / `←` | expand / collapse a repo (on a worktree, `→` focuses its panes) |
| `enter` | open the worktree's tabs and focus the last-used pane |
| type letters | filter worktrees across all repos; `esc` clears the filter |

Letters filter instead of acting, because a typed letter should always be
text wherever focus is.

### Command palette (`ctrl+space space`)

A fuzzy list of every worktree, session and action, matching the browser's
Cmd+K. It's the fallback when you've forgotten a key, and the fastest way to
jump to a worktree in another repo. Substring matches come first, then the ones
whose letters appear in order (`spd` finds "split down"). Worktrees are listed
from repos the sidebar has loaded.

## Status bar

```
 TERMINAL  alt+arrows move  alt+1-9 tab  ctrl+space menu            ● 2 need you   server :8787 ✓
 SIDEBAR   type filter  ↑↓ move  →← open/close  enter open  ctrl+space menu
 LEADER    c claude  t shell  →↓ split  x close  z zoom  r resize  space palette
 RESIZE    arrows resize  shift bigger  esc done
 NEW PANE  ↑↓ choose  enter open  esc close pane
```

The chip is the only text with a filled background (`surface-2` with accent
text). The hints are generated from the same keymap table that dispatches
keys, so they can't drift from what the keys actually do.

## Platform notes

- **`alt` on macOS** needs Option to send Meta: Ghostty `macos-option-as-alt = true`,
  iTerm2 Profile → Keys → Option = Esc+, or Terminal.app "Use Option as
  Meta key". Without it, the leader menu and the mouse still do everything.
- **Known trade-off:** `alt+←`/`alt+→` are word-jump keys in some shells.
  We take them for pane focus, the same choice zellij makes. Word-jump still
  works with `ctrl+←`/`ctrl+→` or `esc b`/`esc f`.
- **Text selection** while the mouse is captured: hold `shift` (iTerm, Ghostty,
  most terminals) or `fn` (Terminal.app) while dragging.
- **Shared sessions resize:** every visible pane is a live tmux client. If
  the browser shows the same session, tmux sizes it to the most recent
  client. This is already true today.

## Code shape (DDD)

| Layer | New |
|---|---|
| `domain` | `layout.go`: `Pane`, an immutable split tree (`Split`, `Close`, `Replace`, `Rects`, `Neighbor`; `Resize`, `DividerAt`, `DragTo`), and `Workbench`, one worktree's tabs reconciled against the server's session list (`Reconcile`, `Split`, `Place`, `ClosePane`, `Move`, `Zoom`, `Hidden`). This holds all the geometry logic and is fully unit-tested with no UI. |
| `app` | `Terminals` (list/create/attach sessions); a `LayoutStore` port arrives with persistence in step 5. |
| `infra` | `layoutfile` adapter (JSON); `ptyterm` unchanged (one attach per visible pane). |
| `ui` | `theme.go` (the tokens above, the only place colors exist), `keymap.go` (one table that drives dispatch, status-bar hints and the leader menu), `panes.go`, `tabbar.go`, `statusbar.go`, `palette.go`; the sidebar becomes a tree. |

## Build order (one commit each, each usable on its own)

1. `theme.go` + `keymap.go` + status bar + the new focus rules. `j`/`k`/`[`/`]`/`ctrl+]` go away; `alt` keys and the leader menu arrive.
2. Sidebar tree with type-to-filter.
3. `domain.Pane`/`Workbench` + split rendering + focus moves + close/zoom; the empty-pane "what to run" picker.
4. Resizing by keyboard (`alt+shift+arrows`, resize mode) and mouse (click to focus, drag dividers, wheel to the pane).
5. Command palette + layout persistence.

## Out of scope (separate PRs if wanted)

Light mode and the Ledger/Command Deck themes · moving the shared logic out
of React into a library · the file tree / editor panels · closing (killing)
sessions server-side beyond the confirm prompt.
