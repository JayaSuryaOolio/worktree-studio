import { Task, TaskStatus, Worktree } from "./api";

// Helpers for the task label on worktrees (see docs/tasks-plan.md).

export function taskId(source: string, key: string): string {
  return `${source}:${key}`;
}

/** Local keys are sequential numbers, shown as "#12"; external keys
 * (PROJ-123) are shown as they are. */
export function taskKeyLabel(t: { source: string; key: string }): string {
  return t.source === "local" ? `#${t.key}` : t.key;
}

export function findTask<T extends Task>(tasks: T[], wt: Worktree | undefined): T | undefined {
  if (!wt || !wt.task_key) return undefined;
  return tasks.find((t) => t.source === wt.task_source && t.key === wt.task_key);
}

/** Status position first, then most recently touched — the sidebar's and
 * the board's order. */
export function sortTasks<T extends Task>(tasks: T[], statuses: TaskStatus[]): T[] {
  const pos = new Map(statuses.map((s) => [s.id, s.position]));
  return [...tasks].sort(
    (a, b) =>
      (pos.get(a.status_id) ?? 0) - (pos.get(b.status_id) ?? 0) || b.updated_at.localeCompare(a.updated_at)
  );
}

/** A worktree name pre-filled from a task: "proj-123-split-invoice-export". */
export function taskWorktreeName(t: { key: string; title: string }): string {
  return `${t.key} ${t.title}`
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 48)
    .replace(/-+$/, "");
}
