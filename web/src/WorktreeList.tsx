import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { SpotlightStatus, Worktree, WorktreeStatus } from "./api";
import WorktreeActionsMenu from "./WorktreeActionsMenu";
import WorktreeAuditLog from "./WorktreeAuditLog";
import { archiveWorktreeWithConfirm, startSpotlightWithFriendlyError, stopSpotlightSafe } from "./worktreeActions";

interface Props {
  worktrees: Worktree[];
  spotlight: Record<string, SpotlightStatus>;
  gitStatus: Record<string, WorktreeStatus>;
  onActionDone: () => void;
  onActionError: (message: string) => void;
}

function GitStatusCell({ status }: { status: WorktreeStatus | undefined }) {
  if (!status) {
    return <span className="muted">…</span>;
  }
  return (
    <span className="git-status-badges">
      {status.dirty ? (
        <span className="badge badge-dirty" title="Uncommitted changes or untracked files">
          ● dirty
        </span>
      ) : (
        <span className="badge badge-clean" title="No uncommitted changes">
          clean
        </span>
      )}
      {status.has_upstream && (status.ahead > 0 || status.behind > 0) && (
        <span className="badge" title={`${status.ahead} commit(s) ahead, ${status.behind} behind its upstream`}>
          {status.ahead > 0 && `↑${status.ahead}`}
          {status.behind > 0 && `↓${status.behind}`}
        </span>
      )}
    </span>
  );
}

function SpotlightStatusBadge({ status }: { status: SpotlightStatus | undefined }) {
  if (!status || !status.available) {
    return <span className="muted">unavailable</span>;
  }
  if (status.active) {
    return <span className="badge" title="Mirroring this worktree into the repo root">● in focus</span>;
  }
  return <span className="muted">inactive</span>;
}

// All per-worktree actions (open, spotlight start/stop, delete) live
// behind each row's kebab menu (WorktreeActionsMenu) — the Status/
// Spotlight columns here are informational badges only, not buttons.
export default function WorktreeList({
  worktrees,
  spotlight,
  gitStatus,
  onActionDone,
  onActionError,
}: Props) {
  const navigate = useNavigate();
  const [logWorktree, setLogWorktree] = useState<Worktree | null>(null);

  if (worktrees.length === 0) {
    return <p>No worktrees yet for this repo.</p>;
  }

  return (
    <>
    <table>
      <thead>
        <tr>
          <th>Branch</th>
          <th>Path</th>
          <th>Created</th>
          <th>Status</th>
          <th>Spotlight</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        {worktrees.map((wt) => (
          <tr key={wt.id}>
            <td>{wt.branch}</td>
            <td>
              <code>{wt.path}</code>
            </td>
            <td>{new Date(wt.created_at).toLocaleString()}</td>
            <td>
              <GitStatusCell status={gitStatus[wt.id]} />
            </td>
            <td>
              <SpotlightStatusBadge status={spotlight[wt.id]} />
            </td>
            <td>
              <WorktreeActionsMenu
                wt={wt}
                spotlightStatus={spotlight[wt.id]}
                onOpen={() => navigate(`/repo/${wt.repo_id}/worktree/${wt.id}`)}
                onSpotlightStart={() =>
                  startSpotlightWithFriendlyError(wt, { onDone: onActionDone, onError: onActionError })
                }
                onSpotlightStop={() =>
                  stopSpotlightSafe(wt, { onDone: onActionDone, onError: onActionError })
                }
                onViewLog={() => setLogWorktree(wt)}
                onArchive={() =>
                  archiveWorktreeWithConfirm(wt, { onDone: onActionDone, onError: onActionError })
                }
              />
            </td>
          </tr>
        ))}
      </tbody>
    </table>
    {logWorktree && (
      <WorktreeAuditLog
        repoId={logWorktree.repo_id}
        worktreeId={logWorktree.id}
        title={logWorktree.branch}
        onClose={() => setLogWorktree(null)}
      />
    )}
    </>
  );
}
