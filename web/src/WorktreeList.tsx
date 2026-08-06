import { Link } from "react-router-dom";
import { SpotlightStatus, Worktree, WorktreeStatus } from "./api";

interface Props {
  worktrees: Worktree[];
  onDelete: (wt: Worktree) => void;
  spotlight: Record<string, SpotlightStatus>;
  onSpotlightStart: (wt: Worktree) => void;
  onSpotlightStop: (wt: Worktree) => void;
  gitStatus: Record<string, WorktreeStatus>;
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

function SpotlightCell({
  status,
  onStart,
  onStop,
}: {
  status: SpotlightStatus | undefined;
  onStart: () => void;
  onStop: () => void;
}) {
  if (!status || !status.available) {
    return <span className="muted">unavailable</span>;
  }
  if (status.active) {
    return (
      <button title="Stop mirroring this worktree into the repo root" onClick={onStop}>
        ● in focus — Stop
      </button>
    );
  }
  if (status.active_worktree_path) {
    return (
      <button
        title="Another worktree is currently mirrored into this repo's root; starting will take over"
        onClick={onStart}
      >
        Start (will replace active mirror)
      </button>
    );
  }
  return <button onClick={onStart}>Start</button>;
}

export default function WorktreeList({
  worktrees,
  onDelete,
  spotlight,
  onSpotlightStart,
  onSpotlightStop,
  gitStatus,
}: Props) {
  if (worktrees.length === 0) {
    return <p>No worktrees yet for this repo.</p>;
  }

  return (
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
              <SpotlightCell
                status={spotlight[wt.id]}
                onStart={() => onSpotlightStart(wt)}
                onStop={() => onSpotlightStop(wt)}
              />
            </td>
            <td>
              <Link to={`/repo/${wt.repo_id}/worktree/${wt.id}`}>Open</Link>{" "}
              <button className="danger" onClick={() => onDelete(wt)}>
                Delete
              </button>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
