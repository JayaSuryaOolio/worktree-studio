// Shared worktree action logic (delete-with-confirm-and-force-retry,
// spotlight start/stop with a friendly conflict message, create-with-an-
// auto-started-claude-terminal) — used by Workspace.tsx's table,
// Sidebar.tsx's per-worktree kebab menu, and Layout.tsx's shared
// creation flow, so none of this is duplicated across those surfaces.
import { archiveWorktree, ConflictError, createTerminal, createWorktree, deleteWorktree, startSpotlight, stopSpotlight, Worktree } from "./api";

interface ActionCallbacks {
  onDone: () => void;
  onError: (message: string) => void;
}

export async function archiveWorktreeWithConfirm(wt: Worktree, { onDone, onError }: ActionCallbacks) {
  if (
    !confirm(
      `Archive worktree "${wt.name}" (branch ${wt.branch})? It'll be hidden from the list — the git worktree and branch stay on disk, and any claude session in it can still be resumed later.`
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
// The claude session gets an id we generate ourselves (rather than one
// `claude` would pick on its own) specifically so it's known *before* the
// session starts — passed via `--session-id` and recorded in the audit log
// in the same request that creates the terminal. That's what makes
// `claude --resume <id>` possible later, independent of whether this
// terminal/tmux session is still alive. `-n <name>` sets a human-readable
// title (the worktree's own adjective-noun name is already human-friendly
// and has no spaces/special characters, so it's safe to embed directly in
// the command line typed into the pane's shell — no quoting needed).
export async function createWorktreeWithClaudeTerminal(repoId: string, name: string): Promise<Worktree> {
  const wt = await createWorktree(repoId, name);
  try {
    const claudeSessionId = crypto.randomUUID();
    await createTerminal(
      repoId,
      wt.id,
      "claude",
      `claude --session-id ${claudeSessionId} -n ${wt.name}`,
      claudeSessionId,
      wt.name
    );
  } catch (err) {
    console.error("worktree created, but failed to auto-start a claude terminal", err);
  }
  return wt;
}
