# Task grouping — design

Status: **proposed, not built.** This plan is written before the build, the same way `docs/editor-plan.md` and `docs/ui-overhaul-plan.md` were. Once it ships, update `docs/architecture.md`, `README.md` and `internal/skillasset/SKILL.md`, and keep this file as the design record.

## Problem

The sidebar lists worktrees grouped by repo. One piece of work often needs several worktrees: a backend change and a frontend change in two repos, a spike next to the real attempt, or a hotfix next to its main-line fix. Nothing ties them together today, so the user has to remember which worktrees belong to which ticket.

Tickets already live in Jira (or elsewhere), but Jira can't link back into this tool. Building a separate kanban product for this (`~/work/conveyor`) means a second binary, a second database and a second UI, all for what is mostly a **label on a worktree**.

## Principle: a task is a label on a worktree, not a new building block

The app is called *worktree-studio*, and it should stay close to that name.

- The **worktree stays the primary object.** A task is a label on it, made of two columns: `task_source` and `task_key`.
- The hierarchy is **task > worktree**. Several worktrees can carry the same task label, and the UI groups by it. A worktree has at most one task.
- A worktree with no task is an **ad-hoc worktree**. That is the default and remains fully supported. Nothing about today's flow changes for someone who never uses tasks.
- worktree-studio does **not** become a task tracker. It shows tasks only when at least one worktree carries them, or when they were created in its own local source. It never lists a Jira backlog, and it **never writes anything back** to Jira or any other source.
- **Status is worktree-studio's own.** The user defines a short list of statuses in Settings and picks one per task. Sources provide only the key, title and link.
- **Task sources** are pluggable. worktree-studio's own SQLite table is one source (`local`). External sources are small commands that print JSON. More than one source can be active at once.
- **Other tools drive worktrees, not tasks.** A tool such as Conveyor, a script or Claude can create a worktree with a task label through the CLI or API, then start working in it. worktree-studio doesn't dispatch, deploy or automate anything itself.

## Data model

### `worktrees`: two new columns

```sql
ALTER TABLE worktrees ADD COLUMN task_source TEXT NOT NULL DEFAULT '';
ALTER TABLE worktrees ADD COLUMN task_key    TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_worktrees_task ON worktrees(task_source, task_key);
```

Both columns are added with the existing `migrateAddX` and `hasColumn` pattern in `internal/store/store.go`. Empty values mean ad-hoc.

### `tasks`: one row per labelled task, for every source

```sql
CREATE TABLE IF NOT EXISTS tasks (
	source     TEXT NOT NULL,              -- 'local', or a task_sources.id
	key        TEXT NOT NULL,              -- 'PROJ-123' for an external task; a sequential number for a local one
	title      TEXT NOT NULL DEFAULT '',   -- display name
	url        TEXT NOT NULL DEFAULT '',   -- optional task link
	status_id  INTEGER REFERENCES task_statuses(id),  -- NULL means no status set
	updated_at TEXT NOT NULL,
	PRIMARY KEY (source, key)
);
```

**Keys and names.** Each task has three identifiers:

| | local task | external task |
|---|---|---|
| id | `(source, key)`, the primary key | `(source, key)` |
| key | a sequential number (`1`, `2`, …), shown as `#12` | the source's own key, e.g. `PROJ-123` |
| display name | `title`, edited in worktree-studio | `title`, from the source |

**Who owns which column.**

- `status_id` always belongs to worktree-studio, for every source.
- `title` and `url` belong to worktree-studio for local tasks.
- For external tasks, `title` and `url` are filled in from the source. A refresh overwrites `title`, but it fills `url` only when the column is empty, so a link the user set by hand survives.

**The task link.** `url` is an optional link that can be set on any task: in the task popup, through the CLI (`--task-url`), or through the API. It means a task can point at a Jira ticket, a doc or a Slack thread even when no source adapter is configured. A local task with a link is enough for most uses.

There is deliberately no foreign key from `worktrees` to `tasks`. A worktree can carry a label whose `tasks` row doesn't exist yet. The UI then shows the bare key, and the row is created the first time the task is edited or refreshed.

### `task_statuses`: user-defined, ordered

```sql
CREATE TABLE IF NOT EXISTS task_statuses (
	id       INTEGER PRIMARY KEY,
	name     TEXT NOT NULL UNIQUE,
	position INTEGER NOT NULL
);
```

- The table is seeded on first run with `Todo`, `In progress`, `In review` and `Done`.
- Statuses are edited in **Settings → Tasks**: add, rename, reorder or delete.
- A status that tasks are using can't be deleted until those tasks are moved. Tasks reference statuses by `id`, so renaming one is just an update.
- `position` sets the board's column order and the sort order of sidebar groups.

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

A local task gets the next sequential number as its key, and its title and link are edited in worktree-studio. This covers people who don't use Jira, and personal tasks that don't belong in Jira.

### Command sources (external)

This follows the project's usual approach: shell out to a real tool instead of reimplementing it, as `internal/gh` does for `gh`. An adapter is a **read-only** executable that follows this contract:

```
<command> list [--query <text>]      → JSON array of Task, used by the picker search
<command> get <key> [<key>...]       → JSON array of Task, used for cache refresh (batched)
```

```json
{ "key": "PROJ-123", "title": "Split invoice export", "url": "https://acme.atlassian.net/browse/PROJ-123" }
```

- A non-zero exit code means an error, with the message on stderr.
- There is no status field. Status is local, so adapters never have to map workflow names.
- **Jira** ships as an example adapter script (`install/task-sources/jira`) built on `acli`. It is an example, not Go code in this binary.
- Trust: the command is user-configured and runs as the user, with the same trust level as the installed Claude hooks. It can only be added through an explicit settings action.

### Refresh

- The **picker** calls `list --query` live (debounced).
- The **sidebar, tab header and board** read only from `tasks`. An on-demand refresh, limited by a time-to-live, calls `get` with every external key that a non-archived worktree carries. It is batched per source, the same way the PR-status lookup works (`prGitCache.ts`, `statusScheduler.ts`).
- When a source fails, the cached title stays and a small "stale" marker appears.

## Who sets the task on a worktree

1. **New-worktree dialog**: an optional **Task** picker (details below).
2. **Existing worktree**: the task popup (below), or "Set task…" in the sidebar context menu.
3. **CLI**, for integrated tools and Claude:
   - `worktree-studio create-worktree <name> --task jira:PROJ-123 [--task-url <url>]` extends the existing subcommand.
   - `worktree-studio set-task <source>:<key> [--task-url <url>] [path]` and `worktree-studio set-task --clear [path]` resolve the worktree from the path or cwd, the same way `open-file` does.
   - `worktree-studio set-task --new "Invoice export"` creates a local task and assigns it in one step.
4. **API**: `PUT /api/repos/{repoID}/worktrees/{worktreeID}/task` with body `{source, key, url?}`; empty values clear the task. Every change writes an audit-log entry.

An unknown external key is accepted as is. The next refresh fills in its title, so integrated tools never have to register tasks first.

## UI

### Task popup

A small popover that opens when you click the **task icon**. The icon appears in the tab-panel header, on sidebar group headers and on board cards. The popup contains:

- **Status**: a dropdown of the statuses defined in Settings.
- **Link**: an editable URL field, with an "open" button.
- **Title**: editable for local tasks, read-only for external ones.
- **Worktrees**: the worktrees that carry this task, with an "add worktree" action that opens the new-worktree dialog with this task already selected.
- **Clear task from this worktree**, when the popup was opened from a worktree.

### Tab-panel header (breadcrumb)

The worktree detail view (`WorktreeDetail.tsx`) gets a breadcrumb above its dockview tab panels:

```
[icon] PROJ-123 Split invoice export (jira)  ›  invoice-export-api
```

- The task part opens the popup.
- An ad-hoc worktree shows just its name, plus a faint "+ task" affordance.

### New-worktree dialog

- An optional **Task** field above "Name". It is a searchable picker across every source, defaults to "None (ad-hoc)" and remembers the last-used source. It includes a "+ New local task" option.
- Picking a task pre-fills the name from the key and a title slug (`proj-123-split-invoice-export`). The name stays editable.

### Sidebar

- A **Group by: Repo | Task** setting, placed in **Settings**, not in the sidebar. It is stored through `sidebarPreferences.ts`, and **Repo** (today's view) is the default.
- In **Task** mode:
  - One group per task, headed by the task icon, key, title and status. Groups are sorted by status position, then by most recent worktree activity.
  - Worktree rows show a **repo badge**, because a task's worktrees can sit in different repos.
  - An **Ad-hoc** group at the bottom holds every worktree without a task.
  - The existing filter, attention indicators and per-group collapse apply unchanged.

### Board (`/board`)

- One column per configured status, plus a "No status" column. **Cards are tasks**, and each card lists its worktrees (repo, name, PR pill), which open on click.
- The board shows tasks that carry at least one non-archived worktree, plus all local tasks.
- You change a task's status by dragging its card or through the task popup. This only ever changes worktree-studio's own status.

### TUI

Phase 1 shows the task key in the TUI's worktree list. Group-by-task in the TUI is a follow-up, and must work through ctrl+space like every other TUI action.

## API surface (new)

```
GET    /api/tasks                             tasks carried by worktrees, plus all local tasks
GET    /api/tasks/search?source=&q=           picker search (local LIKE query, or adapter `list`)
POST   /api/tasks                             create a local task {title, url?, status_id?}
PATCH  /api/tasks/{source}/{key}              update {status_id?, url?, title? (local only)}
DELETE /api/tasks/local/{key}                 delete a local task that no worktree carries
POST   /api/tasks/refresh                     refresh external titles for carried keys
PUT    /api/repos/{repoID}/worktrees/{worktreeID}/task   set or clear {source, key, url?}
GET|POST|PATCH|DELETE /api/task-statuses      list, add, rename or reorder, delete (refused when in use)
GET|POST|DELETE /api/task-sources             manage adapters
POST   /api/task-sources/{id}/test            run `list` once (explicit action only)
```

The worktree create endpoints, both the UI one and `POST /api/worktrees` for the CLI, accept optional `task_source`, `task_key` and `task_url` fields.

## Package layout

- `internal/store`: migrations, CRUD for `tasks`, `task_statuses` and `task_sources`, plus `SetWorktreeTask`.
- `internal/tasksrc` (new, small): one `Run(command, args) ([]Task, error)` function. There is no interface, because `local` is answered directly from the store.
- `internal/api/tasks.go`: the handlers above.
- `cmd/worktree-studio/settask.go`: the `set-task` subcommand, plus `--task` and `--task-url` in `createworktree.go`.
- `web/src`:
  - `TaskPopup.tsx` and `TaskPicker.tsx`, both new.
  - The breadcrumb in `WorktreeDetail.tsx`.
  - The task-mode branch in `Sidebar.tsx`.
  - `Board.tsx`, new.
  - A **Tasks** section in `SettingsModal.tsx`: group-by, statuses and sources.
- `install/task-sources/jira`: the example adapter.

## Build order (one commit per step)

1. Store: the columns, the three tables, seeded statuses, CRUD and tests.
2. API: set or clear a worktree's task, local task CRUD, status CRUD, audit entries and tests.
3. CLI: `create-worktree --task/--task-url` and `set-task`.
4. Settings → Tasks: the group-by setting and the status list editor.
5. Task popup, and the tab-panel breadcrumb.
6. New-worktree dialog task picker (local only).
7. Sidebar group-by-task mode.
8. Board view.
9. Command sources: `internal/tasksrc`, the sources settings, Test, picker search and refresh.
10. Example Jira adapter.
11. Docs: README, architecture.md, SKILL.md (CLI flags and `set-task`).

Steps 1–8 are fully useful with local tasks and links alone. Step 9 is where Jira search comes in.

## Out of scope

- Several tasks per worktree.
- Any write-back to Jira or other sources: no status sync, no comments, no transitions.
- Dispatch, automation or auto-deployment. External tools create and drive worktrees through the existing CLI and API; worktree-studio only labels and shows them.
- Auto-deriving status from PR state. This is possible later, since PR status is already fetched.
- Task descriptions, comments or attachments.
- Per-repo sources or per-repo status lists.

## Open questions

1. **Display when keys collide.** If two sources use the same key shape (Jira and Linear both use `ABC-12`), the breadcrumb already shows `(source)`. Should the sidebar group headers show it too, or only when a key is ambiguous?
2. **Archiving.** When every worktree of a local task is archived, should the task move to the last status (`Done` by default) automatically, or stay where it is?
3. **New tasks.** Should a new task start with no status, or with the first status in the list?
