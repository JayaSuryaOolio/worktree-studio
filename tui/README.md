# tui — terminal frontend (prototype)

A Bubble Tea client for a running worktree-studio server, alongside the browser UI in `web/`.
Currently only the **sidebar**: repos, active worktrees (pinned first), ↑/↓ ahead/behind, dirty `*`,
live "Claude needs you" `●` from `/ws/attention`.

```bash
go run ./cmd/worktree-studio          # server (or already running)
go run ./tui/cmd/wts-tui              # honours WORKTREE_STUDIO_ADDR
```

**Main window:** `enter` on a worktree attaches its terminal session (the same tmux session the browser shows; one is created if the worktree has none) in the right-hand pane and moves focus there — everything you type goes to the shell/Claude. `ctrl+]` returns focus to the sidebar. Closing wts-tui only detaches; the session keeps running.

Sidebar keys: `j`/`k` move, `[`/`]` switch repo, `n` new worktree, `enter` open worktree (also marks attention seen), `r` refresh, `q` quit.
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

Terminal limits: one session per worktree (the first), no mouse or scrollback, no `new terminal`/split. Not yet: spotlight badge, expandable row actions, hover summary, other dialogs (add repo, attach, settings), branch filtering in the new-worktree picker, terminals/editor.
