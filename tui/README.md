# tui — terminal frontend (prototype)

A Bubble Tea client for a running worktree-studio server, alongside the browser UI in `web/`.
Sidebar tree (repos → active worktrees, pinned first) + tabbed terminal pane: ↑/↓ ahead/behind, dirty `*`,
live "Claude needs you" `●` from `/ws/attention` (a collapsed repo shows `●` if any of its worktrees needs you). Git status loads when a repo is expanded.

```bash
go run ./cmd/worktree-studio          # server (or already running)
go run ./tui/cmd/wts-tui              # honours WORKTREE_STUDIO_ADDR
```

Design system and the full keymap plan: [`DESIGN.md`](DESIGN.md). The rule: bare keys and `ctrl+letter` always go to whatever has focus (so typing into Claude never triggers the app); the app only owns `alt` combos and `ctrl+space`. The bottom status bar shows the current mode and the keys that work in it; amber marks where your keys are going.

| Key | Where | Action |
|---|---|---|
| `alt+←→↑↓` | anywhere | move to the neighbouring pane; `alt+←` from the leftmost pane goes to the sidebar, `alt+→` from the sidebar back |
| `alt+1`–`alt+9` | anywhere | go to tab N |
| `ctrl+space` | anywhere | menu: `c` new Claude tab, `t` new shell tab, `→`/`↓` split right/down, `x` close pane, `z` zoom pane, `n` new worktree, `q` quit |
| `↑`/`↓`, `enter`, `esc` | empty pane | pick what runs there: new Claude, new shell, or a running session that isn't shown; `esc` closes the pane |
| `↑`/`↓` | sidebar | move |
| `→` / `←` | sidebar | expand / collapse a repo; `→` on a worktree opens its terminals, `←` jumps to its repo |
| `enter` | sidebar | open worktree's terminals (toggles on a repo) |
| type letters | sidebar | filter worktrees across all repos by branch or name; `backspace` edits, `esc` clears |

On macOS, `alt` needs Option to send Meta (Ghostty `macos-option-as-alt = true`, iTerm2 Option key = Esc+, Terminal.app "Use Option as Meta key"); `ctrl+space` works without it.

Tabs are the worktree's terminal sessions (the same tmux sessions the browser shows; a shell is created if there are none). Closing wts-tui only detaches; sessions keep running. A tab is a split layout and each pane one session. Splitting opens an empty pane that asks what to run. Closing a pane only hides its session (pick it back into any empty pane); killing sessions is still done in the browser. Each worktree keeps its own tabs while wts-tui runs (not saved yet).
New-worktree dialog: type a name, `tab` to the branch picker, `←`/`→` to cycle, `enter` create, `esc` cancel.

## Layout (DDD, dependencies point inward)

```
domain/           pure model: Repo, Worktree, GitStatus, Attention (+ events). No I/O.
app/              use cases (Sidebar) + ports (Workspace, AttentionFeed).
infra/httpapi/    adapter: REST + websocket against the server.
infra/ptyterm/    adapter: `tmux attach` in a pty + in-process emulator (charmbracelet/x/vt).
ui/               Bubble Tea presentation. Depends on app/domain only.
cmd/wts-tui/      composition root.
```

Terminal limits: no resizing splits, mouse or scrollback yet (DESIGN.md build steps 4–5). Not yet: spotlight badge, expandable row actions, hover summary, other dialogs (add repo, attach, settings), branch filtering in the new-worktree picker, terminals/editor.
