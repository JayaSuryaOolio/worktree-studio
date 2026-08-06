import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  createWorktree,
  deleteWorktree,
  listWorktrees,
  Worktree,
} from "./api";
import WorktreeList from "./WorktreeList";
import NewWorktreeDialog from "./NewWorktreeDialog";

export default function Workspace() {
  const { repoId } = useParams<{ repoId: string }>();
  const [worktrees, setWorktrees] = useState<Worktree[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [dialogOpen, setDialogOpen] = useState(false);

  function refresh() {
    if (!repoId) return;
    listWorktrees(repoId)
      .then(setWorktrees)
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }

  useEffect(refresh, [repoId]);

  async function handleCreate(name: string) {
    if (!repoId) return;
    await createWorktree(repoId, name);
    refresh();
  }

  async function handleDelete(wt: Worktree) {
    if (!repoId) return;
    if (!confirm(`Remove worktree "${wt.name}" (branch ${wt.branch})?`)) return;
    try {
      await deleteWorktree(repoId, wt.id);
      refresh();
    } catch (err) {
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
        <WorktreeList worktrees={worktrees} onDelete={handleDelete} />
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
