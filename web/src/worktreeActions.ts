// Shared worktree action logic (delete-with-confirm-and-force-retry,
// spotlight start/stop with a friendly conflict message, create-with-an-
// auto-started-claude-terminal) — used by Workspace.tsx's table,
// Sidebar.tsx's per-worktree kebab menu, and Layout.tsx's shared
// creation flow, so none of this is duplicated across those surfaces.
import { ConflictError, createTerminal, createWorktree, deleteWorktree, startSpotlight, stopSpotlight, Worktree } from "./api";

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

// Creates a worktree, then immediately creates a terminal in it with
// `claude` auto-run as the first command — every worktree-creation
// surface (sidebar "+", command palette, Workspace's own button) wants
// this same behavior, not just the git-level worktree creation. Errors
// from the terminal-creation step are swallowed (logged only) rather
// than failing the whole operation: the worktree itself was created
// successfully at that point, and the user can always open a terminal
// manually — losing the auto-claude convenience isn't worth surfacing
// a scary error for what's otherwise a successful worktree creation.
export async function createWorktreeWithClaudeTerminal(repoId: string, name: string): Promise<Worktree> {
  const wt = await createWorktree(repoId, name);
  try {
    await createTerminal(repoId, wt.id, "claude", "claude");
  } catch (err) {
    console.error("worktree created, but failed to auto-start a claude terminal", err);
  }
  return wt;
}
