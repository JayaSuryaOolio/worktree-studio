# Task grouping — design

Status: **proposed, not built.** This plan is written before the build, the same way `docs/editor-plan.md` and `docs/ui-overhaul-plan.md` were. Once it ships, update `docs/architecture.md`, `README.md` and `internal/skillasset/SKILL.md`, and keep this file as the design record.

## Problem

The sidebar lists worktrees grouped by repo. One piece of work often needs several worktrees: a backend change and a frontend change in two repos, a spike next to the real attempt, or a hotfix next to its main-line fix. Nothing ties them together today, so the user has to remember which worktrees belong to which ticket.

Tickets already live in Jira (or elsewhere), but Jira can't link back into this tool. Building a separate kanban product for this (`~/work/conveyor`) means a second binary, a second database and a second UI, all for what is mostly a **label on a worktree**.

## Principle: a task is a field on a worktree, not a new building block

The app is called *worktree-studio*, and it should stay close to that name. So:

- The **worktree stays the primary object.** A task is two nullable columns on it: `task_source` and `task_key`.
- A worktree with no task is an **ad-hoc worktree**. That is the default and remains fully supported. Nothing about today's flow changes for someone who never turns tasks on.
- worktree-studio does **not** become a task tracker. It shows tasks only when at least one worktree references them, or when they were created in worktree-studio's own local source. It never lists a Jira backlog.
- **Task sources** are pluggable. worktree-studio's own SQLite table is one source (`local`). External sources (Jira and others) are small commands that print JSON. More than one source can be active at once.
- Other apps (Conveyor, scripts, Claude) control the label through the existing CLI and API: they set `task_source` and `task_key` on a worktree. worktree-studio doesn't need to know who set them.

## Data model

### `worktrees`: two new columns

```sql
ALTER TABLE worktrees ADD COLUMN task_source TEXT NOT NULL DEFAULT '';
ALTER TABLE worktrees ADD COLUMN task_key    TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_worktrees_task ON worktrees(task_source, task_key);
```

Both columns are added with the existing `migrateAddX` and `hasColumn` pattern in `internal/store/store.go`. Empty values mean ad-hoc. A worktree has **at most one task**. Several tasks per worktree is out of scope (see below).

### `tasks`: one table for every source

```sql
CREATE TABLE IF NOT EXISTS tasks (
	source       TEXT NOT NULL,          -- 'local', or a task_sources.id
	key          TEXT NOT NULL,          -- source-native id: 'PROJ-123', or a generated id for local
	title        TEXT NOT NULL,
	status       TEXT NOT NULL,          -- one of the buckets below
	status_label TEXT NOT NULL DEFAULT '', -- the source's own wording, e.g. "Code Review"
	url          TEXT NOT NULL DEFAULT '',
	updated_at   TEXT NOT NULL,
	PRIMARY KEY (source, key)
);
```

- For `source = 'local'`, this row **is** the task, and worktree-studio owns it.
- For an external source, this row is a **read-through cache**, like `claude_session_titles`. It lets the sidebar and the board render instantly and keep working when the source is offline. It is refreshed on demand (see "Refresh").
- There is deliberately no foreign key from `worktrees` to `tasks`. A worktree can reference a task whose cache row hasn't been fetched yet; the UI shows the key until the title arrives.

### Status buckets

All sources map their statuses onto one fixed set, which is also the set of board columns:

| bucket | meaning |
|---|---|
| `todo` | not started |
| `in_progress` | being worked |
| `in_review` | PR open, awaiting review or QA |
| `done` | finished |

The fixed set keeps the board and sidebar sort simple. A source's own wording is kept in `status_label` and shown on the card. Mapping is the **source's** job (see the command contract), so Go code never learns Jira workflow names.

### `task_sources`: configured external sources

```sql
CREATE TABLE IF NOT EXISTS task_sources (
	id      TEXT PRIMARY KEY,  -- short slug used in task_source, e.g. 'jira'
	name    TEXT NOT NULL,     -- display name
	command TEXT NOT NULL      -- absolute path to the adapter executable
);
```

`local` is built in and is never stored here.

## Task sources

### Local source (built in)

Local tasks have CRUD through the API: title and status are editable, and the key comes from `newID()` or a short human slug. This is the "I don't use Jira" path, and it also covers personal tasks that don't belong in Jira.

### Command sources (external)

This follows the project's usual approach: shell out to a real tool instead of reimplementing it, as `internal/gh` does for `gh` and `internal/spotlight` does for `spotlight`. An external source is an executable that follows this contract:

```
<command> list [--query <text>]      → JSON array of Task, used by the picker search
<command> get <key> [<key>...]       → JSON array of Task, used for cache refresh (batched)
```

```json
{ "key": "PROJ-123", "title": "Split invoice export", "status": "in_progress",
  "status_label": "In Development", "url": "https://acme.atlassian.net/browse/PROJ-123" }
```

- `status` must be one of the four buckets. The adapter does the mapping.
- A non-zero exit code means an error, with the message on stderr. This is the same convention `gh.PRForBranch` handles.
- **Jira** ships as an example adapter script (`install/task-sources/jira`) built on `acli` (or `curl` with an API token). It is an example, not Go code in this binary. Linear, GitHub Issues and others follow the same pattern without any change here.
- **Conveyor**, or any app with its own task store, can be a source the same way: it exposes `list` and `get` through its CLI.

Trust: the command is user-configured and runs as the user, with the same trust level as the installed Claude hooks. It can only be added through an explicit settings action, never by auto-discovery.

### Refresh

- The **picker** calls `list --query` live (debounced) and upserts the results into `tasks`.
- The **sidebar and board** read only from `tasks`. A background refresh calls `get` with every external key that a non-archived worktree references, batched per source. It uses the same on-demand, TTL-bounded approach as the PR-status lookup (`web/src/prGitCache.ts`, `statusScheduler.ts`), not a server-side poller.
- When a source fails, the cached row stays and the UI shows a small "stale" marker. It never blanks the group.

## Who sets the task on a worktree

1. **New-worktree dialog**: an optional **Task** field (details below).
2. **Existing worktree**: a context-menu "Set task…" or "Clear task" action in the sidebar and the hover popover.
3. **CLI**, for integrated apps and Claude:
   - `worktree-studio create-worktree <name> --task jira:PROJ-123` extends the existing subcommand.
   - `worktree-studio set-task <source>:<key> [path]` and `worktree-studio set-task --clear [path]` resolve the worktree from the path or cwd, the same way `open-file` and `spotlight` do.
4. **API**: `PUT /api/repos/{repoID}/worktrees/{worktreeID}/task` with body `{source, key}`; empty values clear the task. Every change writes an audit-log entry, as other worktree mutations do.

When a task is set from the CLI or API with an unknown external key, the store accepts it and the next refresh fills in the title. Integrated apps don't have to pre-register tasks.

## UI

### New-worktree dialog

- A **Task** field placed above "Name". It is a searchable picker over every source: local tasks first, then each external source's `list --query`. It is optional, defaults to "None (ad-hoc)", and remembers the last-used source.
- A "+ New local task" option creates a local task inline from the typed text.
- Picking a task **pre-fills the name** from the key and a title slug (`proj-123-split-invoice-export`), replacing the random word suggestion. The user can still edit it.
- Opening the dialog from inside a task group (see the sidebar) pre-selects that task. This is how "add another worktree to this task" works.

### Sidebar

- A new **Group by: Repo | Task** toggle, saved in `sidebarPreferences.ts`. **Repo** is today's view, unchanged.
- In **Task** mode:
  - One group per task, headed by the key, title and a status pill. Clicking the key opens `url` in the browser.
  - Groups are sorted by bucket (`in_progress`, then `in_review`, then `todo`, then `done`), then by most recent worktree activity.
  - Worktree rows inside a group show a **repo badge**, because tasks cross repos. That cross-repo view is the main thing this mode adds.
  - An **Ad-hoc** group at the bottom holds every worktree without a task.
  - `done` groups are collapsed by default.
  - The existing filter, attention indicators and collapse behaviour apply unchanged.

### Board (`/board`)

- A plain kanban with four columns, one per bucket. **Cards are tasks**, and each card lists its worktrees (repo, name, PR pill), which open on click.
- The board shows only tasks that have at least one non-archived worktree, plus all local tasks. Ad-hoc worktrees don't appear; the sidebar already covers them.
- **Status changes** (drag a card, or use a menu) work for **local tasks only**. External tasks are read-only here; their status changes in the source. Writing back to Jira is out of scope.
- This replaces the board Conveyor was going to build. It is a view over worktrees, not a dispatcher.

### TUI

Phase 1 adds `task_key` as a column or label in the TUI's worktree list. Group-by-task in the TUI is a follow-up, and must work through ctrl+space like every other TUI action.

## API surface (new)

```
GET    /api/tasks?with_worktrees=1           tasks referenced by worktrees, plus all local tasks (board/sidebar)
GET    /api/tasks/search?source=&q=          picker search (local LIKE query, or command `list`)
POST   /api/tasks                            create a local task {title, status}
PATCH  /api/tasks/local/{key}                update a local task {title?, status?}
DELETE /api/tasks/local/{key}                delete a local task (only if no worktree references it, or clear them first)
POST   /api/tasks/refresh                    refresh the external cache for referenced keys
PUT    /api/repos/{repoID}/worktrees/{worktreeID}/task   set or clear {source, key}
GET    /api/task-sources                     list (local is always included)
POST   /api/task-sources                     add {id, name, command}
DELETE /api/task-sources/{id}
POST   /api/task-sources/{id}/test           run `list` once and report the result (explicit action, never a background probe)
```

`POST /api/worktrees` (the CLI create endpoint) and the UI create endpoint accept optional `task_source` and `task_key` fields.

## Package layout

- `internal/store`: migrations, `tasks` and `task_sources` CRUD, `SetWorktreeTask`, and `ListWorktreesByTask`.
- `internal/tasksrc` (new, small): `Run(command, args) ([]Task, error)` executes a command source and validates the bucket values. There is one function, not an interface, because `local` is answered directly from the store.
- `internal/api/tasks.go`: the handlers above.
- `cmd/worktree-studio/settask.go`: the `set-task` subcommand, plus `--task` in `createworktree.go`.
- `web/src`: the `TaskPicker.tsx` component used by `NewWorktreeDialog`, a group-by-task branch in `Sidebar.tsx`, `Board.tsx`, and a task-sources section in `SettingsModal.tsx`.
- `install/task-sources/jira`: the example adapter.

## Build order (one commit per step)

1. Store: the columns, the `tasks` and `task_sources` tables, CRUD and tests.
2. API: set or clear a worktree's task, local task CRUD, audit entries, and tests.
3. CLI: `create-worktree --task` and `set-task`.
4. New-worktree dialog: the task picker (local only) and name pre-fill.
5. Sidebar: group-by-task mode.
6. Board view (local status edits).
7. Command sources: `internal/tasksrc`, the settings UI, Test, search and refresh.
8. Example Jira adapter.
9. Docs: README, architecture.md, SKILL.md (CLI flags and `set-task`), running-locally.md if needed.

Steps 1–6 are useful without any external source. Step 7 is where Jira comes in.

## Out of scope (separate PRs if wanted)

- **Several tasks per worktree.** Add a join table only if a real case appears.
- **Writing status back** to Jira or other sources (moving a Jira ticket from the board).
- **Auto-deriving task status** from PR state (open PR → `in_review`, all merged → `done`). It is cheap to add later, since PR status is already fetched. Do it as a follow-up for local tasks.
- **Dispatch, automation or agents picking up tasks**: Conveyor's dispatcher. That still belongs outside this app, if anywhere.
- **Task descriptions, comments or attachments.** Those belong in the source.
- **Per-repo source configuration.** Sources are global for now.

## Open questions

1. **Composite display key.** If two sources ever share a key format (for example, Jira and Linear both use `ABC-12`), the sidebar should show `source:key`. Should it always show the source, or only when the key is ambiguous?
2. **Archiving.** When every worktree of a task is archived, the task leaves the sidebar and board. Should a local task also move to `done` automatically?
3. **Local key format.** A random id, or a typed slug like `invoice-export`? A typed slug reads better in branch names but needs a uniqueness check.
