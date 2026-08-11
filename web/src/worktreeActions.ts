// Shared worktree action logic (delete-with-confirm-and-force-retry,
// spotlight start/stop with a friendly conflict message) — used by
// Workspace.tsx's table and Sidebar.tsx's per-worktree kebab menu, so
// none of this is duplicated across those surfaces.
import { archiveWorktree, ConflictError, deleteWorktree, startSpotlight, stopSpotlight, unarchiveWorktree, Worktree } from "./api";

interface ActionCallbacks {
  onDone: () => void;
  onError: (message: string) => void;
}

// Mirrors internal/api.ArchivedWorktreeRetention on the backend — kept as
// a plain constant here (not fetched from the server) since it's a fixed
// policy, the same reason rootWorktree.ts's id format is duplicated rather
// than round-tripped through an API call.
export const ARCHIVED_RETENTION_DAYS = 60;

export async function archiveWorktreeWithConfirm(wt: Worktree, { onDone, onError }: ActionCallbacks) {
  if (
    !confirm(
      `Archive worktree "${wt.name}" (branch ${wt.branch})? It'll be hidden from the list — the git worktree and branch stay on disk, and any claude session in it can still be resumed later. If left archived for ${ARCHIVED_RETENTION_DAYS} days, it's permanently deleted.`
    )
  )
    return;
  try {
    await archiveWorktree(wt.repo_id, wt.id);
    onDone();
  } catch (err) {
    onError((err as Error).message);
  }
}

export async function unarchiveWorktreeSafe(wt: Worktree, { onDone, onError }: ActionCallbacks) {
  try {
    await unarchiveWorktree(wt.repo_id, wt.id);
    onDone();
  } catch (err) {
    onError((err as Error).message);
  }
}

/** Days left before an archived worktree's auto-delete sweep hard-removes
 * it, given its archived_at — negative/zero once it's actually eligible
 * (the sweep runs on its own schedule server-side, so a worktree can sit
 * at 0 for a while before it's actually picked up). Returns null for a
 * worktree that isn't archived (archived_at === ""). */
export function daysUntilAutoDelete(archivedAt: string): number | null {
  if (!archivedAt) return null;
  const archivedMs = new Date(archivedAt).getTime();
  const deadlineMs = archivedMs + ARCHIVED_RETENTION_DAYS * 24 * 60 * 60 * 1000;
  return Math.ceil((deadlineMs - Date.now()) / (24 * 60 * 60 * 1000));
}

// Kept for a future settings-modal bulk-delete view (filter by repo/
// status, then actually remove) — not wired to any UI today, since the
// per-worktree kebab menu now offers Archive instead (see
// archiveWorktreeWithConfirm above).
export async function deleteWorktreeWithConfirm(wt: Worktree, { onDone, onError }: ActionCallbacks) {
  if (
    !confirm(
      `Remove worktree "${wt.name}" (branch ${wt.branch})? Any uncommitted changes in it will be lost.`
    )
  )
    return;
  try {
    await deleteWorktree(wt.repo_id, wt.id);
    onDone();
  } catch (err) {
    if (err instanceof ConflictError) {
      // The backend refused because the worktree has uncommitted changes
      // or untracked files — git's own safety check. Give the user an
      // explicit second chance to discard them, rather than silently
      // force-removing (or silently failing) the first time around.
      if (
        confirm(
          `Worktree "${wt.name}" has uncommitted changes or untracked files.\n\nRemove it anyway? This will permanently discard those changes.`
        )
      ) {
        try {
          await deleteWorktree(wt.repo_id, wt.id, true);
          onDone();
        } catch (retryErr) {
          onError((retryErr as Error).message);
        }
      }
      return;
    }
    onError((err as Error).message);
  }
}

export async function startSpotlightWithFriendlyError(wt: Worktree, { onDone, onError }: ActionCallbacks) {
  try {
    await startSpotlight(wt.repo_id, wt.id);
    onDone();
  } catch (err) {
    if (err instanceof ConflictError) {
      // The spotlight CLI itself supports stashing the root's uncommitted
      // changes and proceeding, but only via an interactive prompt this
      // server-side call has no terminal to show — so it refuses outright
      // instead. This is the browser-side stand-in for that prompt: ask
      // the same yes/no question a real terminal would have, and retry
      // with stash=true (the non-interactive "yes") only if confirmed —
      // same "refuse first, retry with an explicit flag once confirmed"
      // shape as deleteWorktreeWithConfirm's force-retry above.
      if (
        confirm(
          `Can't start spotlight for "${wt.name}": the repo's root checkout has uncommitted changes.\n\nStash them and start spotlight?`
        )
      ) {
        try {
          await startSpotlight(wt.repo_id, wt.id, true);
          onDone();
        } catch (retryErr) {
          onError((retryErr as Error).message);
        }
      }
      return;
    }
    onError((err as Error).message);
  }
}

export async function stopSpotlightSafe(wt: Worktree, { onDone, onError }: ActionCallbacks) {
  try {
    await stopSpotlight(wt.repo_id, wt.id);
    onDone();
  } catch (err) {
    onError((err as Error).message);
  }
}
