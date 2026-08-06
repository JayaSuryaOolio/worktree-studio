import { useState } from "react";
import { useParams } from "react-router-dom";
import { createWorktree } from "./api";
import { useRepoContext } from "./RepoContext";
import WorktreeList from "./WorktreeList";
import NewWorktreeDialog from "./NewWorktreeDialog";

export default function Workspace() {
  const { repoId } = useParams<{ repoId: string }>();
  const {
    worktreesByRepo,
    worktreesLoading,
    worktreesError,
    refreshWorktrees,
    gitStatus,
    spotlightStatus,
  } = useRepoContext();
  const worktrees = (repoId && worktreesByRepo[repoId]) || [];
  const [actionError, setActionError] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);

  async function handleCreate(name: string) {
    if (!repoId) return;
    await createWorktree(repoId, name);
    refreshWorktrees();
  }

  if (!repoId) return null;

  return (
    <div className="container">
      <h1>Worktrees</h1>
      <button onClick={() => setDialogOpen(true)}>+ New worktree</button>

      {worktreesLoading ? (
        <p>Loading…</p>
      ) : (
        <WorktreeList
          worktrees={worktrees}
          spotlight={spotlightStatus}
          gitStatus={gitStatus}
          onActionDone={refreshWorktrees}
          onActionError={setActionError}
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
