import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  ConflictError,
  createWorktree,
  deleteWorktree,
  getSpotlightStatus,
  listWorktrees,
  SpotlightStatus,
  startSpotlight,
  stopSpotlight,
  Worktree,
} from "./api";
import WorktreeList from "./WorktreeList";
import NewWorktreeDialog from "./NewWorktreeDialog";

export default function Workspace() {
  const { repoId } = useParams<{ repoId: string }>();
  const [worktrees, setWorktrees] = useState<Worktree[]>([]);
  const [spotlight, setSpotlight] = useState<Record<string, SpotlightStatus>>({});
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [dialogOpen, setDialogOpen] = useState(false);

  function refreshSpotlight(wts: Worktree[]) {
    if (!repoId) return;
    Promise.all(
      wts.map((wt) =>
        getSpotlightStatus(repoId, wt.id)
          .then((s) => [wt.id, s] as const)
          .catch(() => [wt.id, { available: false, active: false }] as const)
      )
    ).then((entries) => setSpotlight(Object.fromEntries(entries)));
  }

  function refresh() {
    if (!repoId) return;
    listWorktrees(repoId)
      .then((wts) => {
        setWorktrees(wts);
        refreshSpotlight(wts);
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }

  useEffect(refresh, [repoId]);

  async function handleSpotlightStart(wt: Worktree) {
    if (!repoId) return;
    try {
      await startSpotlight(repoId, wt.id);
      refreshSpotlight(worktrees);
    } catch (err) {
      if (err instanceof ConflictError) {
        setError(
          `Can't start spotlight for "${wt.name}": the repo's root checkout has uncommitted changes. Commit or stash them first.`
        );
        return;
      }
      setError((err as Error).message);
    }
  }

  async function handleSpotlightStop(wt: Worktree) {
    if (!repoId) return;
    try {
      await stopSpotlight(repoId, wt.id);
      refreshSpotlight(worktrees);
    } catch (err) {
      setError((err as Error).message);
    }
  }

  async function handleCreate(name: string) {
    if (!repoId) return;
    await createWorktree(repoId, name);
    refresh();
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
      refresh();
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
            refresh();
          } catch (retryErr) {
            setError((retryErr as Error).message);
          }
        }
        return;
      }
      setError((err as Error).message);
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

      {loading ? (
        <p>Loading…</p>
      ) : (
        <WorktreeList
          worktrees={worktrees}
          onDelete={handleDelete}
          spotlight={spotlight}
          onSpotlightStart={handleSpotlightStart}
          onSpotlightStop={handleSpotlightStop}
        />
      )}
      {error && <p className="error">{error}</p>}

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
