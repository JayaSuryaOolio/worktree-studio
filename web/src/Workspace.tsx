import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  ConflictError,
  createWorktree,
  deleteWorktree,
  startSpotlight,
  stopSpotlight,
  Worktree,
} from "./api";
import { useRepoContext } from "./RepoContext";
import WorktreeList from "./WorktreeList";
import NewWorktreeDialog from "./NewWorktreeDialog";

export default function Workspace() {
  const { repoId } = useParams<{ repoId: string }>();
  const {
    worktrees,
    worktreesLoading,
    worktreesError,
    refreshWorktrees,
    gitStatus,
    spotlightStatus,
  } = useRepoContext();
  const [actionError, setActionError] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);

  async function handleSpotlightStart(wt: Worktree) {
    if (!repoId) return;
    try {
      await startSpotlight(repoId, wt.id);
      refreshWorktrees();
    } catch (err) {
      if (err instanceof ConflictError) {
        setActionError(
          `Can't start spotlight for "${wt.name}": the repo's root checkout has uncommitted changes. Commit or stash them first.`
        );
        return;
      }
      setActionError((err as Error).message);
    }
  }

  async function handleSpotlightStop(wt: Worktree) {
    if (!repoId) return;
    try {
      await stopSpotlight(repoId, wt.id);
      refreshWorktrees();
    } catch (err) {
      setActionError((err as Error).message);
    }
  }

  async function handleCreate(name: string) {
    if (!repoId) return;
    await createWorktree(repoId, name);
    refreshWorktrees();
  }

  async function handleDelete(wt: Worktree) {
    if (!repoId) return;
    if (
      !confirm(
        `Remove worktree "${wt.name}" (branch ${wt.branch})? Any uncommitted changes in it will be lost.`
      )
    )
      return;
    try {
      await deleteWorktree(repoId, wt.id);
      refreshWorktrees();
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
            await deleteWorktree(repoId, wt.id, true);
            refreshWorktrees();
          } catch (retryErr) {
            setActionError((retryErr as Error).message);
          }
        }
        return;
      }
      setActionError((err as Error).message);
    }
  }

  if (!repoId) return null;

  return (
    <div className="container">
      <p>
        <Link className="repo-link" to="/">
          ← all repos
        </Link>
      </p>
      <h1>Worktrees</h1>
      <button onClick={() => setDialogOpen(true)}>+ New worktree</button>

      {worktreesLoading ? (
        <p>Loading…</p>
      ) : (
        <WorktreeList
          worktrees={worktrees}
          onDelete={handleDelete}
          spotlight={spotlightStatus}
          onSpotlightStart={handleSpotlightStart}
          onSpotlightStop={handleSpotlightStop}
          gitStatus={gitStatus}
        />
      )}
      {(worktreesError || actionError) && (
        <p className="error">{worktreesError || actionError}</p>
      )}

      {dialogOpen && (
        <NewWorktreeDialog
          repoId={repoId}
          onCreate={handleCreate}
          onClose={() => setDialogOpen(false)}
        />
      )}
    </div>
  );
}
