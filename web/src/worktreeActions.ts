// Shared worktree action logic (delete-with-confirm-and-force-retry,
// spotlight start/stop with a friendly conflict message) — used by both
// Workspace.tsx's table and Sidebar.tsx's per-worktree kebab menu, so the
// confirm/retry flow isn't duplicated between the two surfaces.
import { ConflictError, deleteWorktree, startSpotlight, stopSpotlight, Worktree } from "./api";

interface ActionCallbacks {
  onDone: () => void;
  onError: (message: string) => void;
}

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
      onError(
        `Can't start spotlight for "${wt.name}": the repo's root checkout has uncommitted changes. Commit or stash them first.`
      );
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
