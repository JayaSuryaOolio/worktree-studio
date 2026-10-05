# tui — terminal frontend (prototype)

A Bubble Tea client for a running worktree-studio server, alongside the browser UI in `web/`.
Currently only the **sidebar**: repos, active worktrees (pinned first), ↑/↓ ahead/behind, dirty `*`,
live "Claude needs you" `●` from `/ws/attention`.

```bash
go run ./cmd/worktree-studio          # server (or already running)
go run ./tui/cmd/wts-tui              # honours WORKTREE_STUDIO_ADDR
```

Keys: `j`/`k` move, `[`/`]` switch repo, `enter` mark attention seen, `r` refresh, `q` quit.

## Layout (DDD, dependencies point inward)

```
domain/           pure model: Repo, Worktree, GitStatus, Attention (+ events). No I/O.
app/              use cases (Sidebar) + ports (Workspace, AttentionFeed).
infra/httpapi/    adapter: REST + websocket against the server.
ui/               Bubble Tea presentation. Depends on app/domain only.
cmd/wts-tui/      composition root.
```

Not yet: spotlight badge, expandable row actions, hover summary, dialogs, terminals/editor.
