import { useState } from "react";
import { Link, useMatch } from "react-router-dom";
import { useRepoContext } from "./RepoContext";
import WorktreeActionsMenu from "./WorktreeActionsMenu";
import { deleteWorktreeWithConfirm, startSpotlightWithFriendlyError, stopSpotlightSafe } from "./worktreeActions";

interface Props {
  onAddRepo: () => void;
  onNewWorktree: (repoId: string) => void;
}

// The persistent left sidebar: every registered repo, with its worktrees
// nested directly underneath — the actual point of this component — so
// switching between parallel in-progress worktrees, across any repo,
// never means navigating back through a list view first. Auto-loads via
// RepoContext whenever the set of repos/worktrees changes, including a
// fresh browser tab opened straight at a worktree URL.
//
// TODO (future, not this pass): nest repos under a "project" grouping —
// for now a flat list of repos is enough.
export default function Sidebar({ onAddRepo, onNewWorktree }: Props) {
  const {
    repos,
    reposLoading,
    worktreesByRepo,
    worktreesLoading,
    refreshWorktrees,
    gitStatus,
    spotlightStatus,
  } = useRepoContext();

  const worktreeMatch = useMatch("/repo/:repoId/worktree/:worktreeId");
  const activeWorktreeId = worktreeMatch?.params.worktreeId ?? null;

  const [error, setError] = useState<string | null>(null);

  return (
    <nav className="sidebar" aria-label="Repos and worktrees">
      <div className="sidebar-header">
        <Link to="/" className="sidebar-brand">
          worktree-studio
        </Link>
      </div>

      <div className="sidebar-section">
        <div className="sidebar-section-title">
          <span>Repos</span>
          <button
            type="button"
            className="icon-button"
            aria-label="Add repo"
            title="Add repo"
            onClick={onAddRepo}
          >
            +
          </button>
        </div>

        {reposLoading ? (
          <p className="sidebar-empty muted">Loading…</p>
        ) : repos.length === 0 ? (
          <p className="sidebar-empty muted">None registered yet.</p>
        ) : (
          <ul className="sidebar-repo-tree">
            {repos.map((r) => (
              <li key={r.id} className="sidebar-repo-group">
                <div className="sidebar-repo-row">
                  <Link to={`/repo/${r.id}`} className="sidebar-repo">
                    {r.name}
                  </Link>
                  <button
                    type="button"
                    className="icon-button"
                    aria-label={`New worktree in ${r.name}`}
                    title="New worktree"
                    onClick={() => onNewWorktree(r.id)}
                  >
                    +
                  </button>
                </div>

                <ul className="sidebar-worktree-list">
                  {(worktreesByRepo[r.id] ?? []).map((wt) => {
                    const status = gitStatus[wt.id];
                    const spot = spotlightStatus[wt.id];
                    return (
                      <li key={wt.id}>
                        {/* "Flight strip" row — see docs/design.md. The
                            left-edge accent color is driven explicitly by
                            data-dirty (not by sniffing a decorative dot's
                            presence via :has()), so it's one obvious
                            thing to grep for if the mapping ever needs to
                            change. */}
                        <Link
                          to={`/repo/${r.id}/worktree/${wt.id}`}
                          data-dirty={status ? String(status.dirty) : undefined}
                          className={
                            wt.id === activeWorktreeId
                              ? "sidebar-worktree active"
                              : "sidebar-worktree"
                          }
                        >
                          <span className="sidebar-worktree-branch">{wt.branch}</span>
                          <span className="sidebar-worktree-meta">
                            {status?.has_upstream && (status.ahead > 0 || status.behind > 0) && (
                              <span
                                className="sidebar-ticks"
                                title={`${status.ahead} ahead, ${status.behind} behind upstream`}
                              >
                                {status.ahead > 0 && `↑${status.ahead}`}
                                {status.behind > 0 && `↓${status.behind}`}
                              </span>
                            )}
                            {spot?.active && (
                              <span className="sidebar-dot sidebar-dot-spotlight" title="Spotlight active" />
                            )}
                            <WorktreeActionsMenu
                              wt={wt}
                              spotlightStatus={spot}
                              onSpotlightStart={() =>
                                startSpotlightWithFriendlyError(wt, { onDone: refreshWorktrees, onError: setError })
                              }
                              onSpotlightStop={() =>
                                stopSpotlightSafe(wt, { onDone: refreshWorktrees, onError: setError })
                              }
                              onDelete={() =>
                                deleteWorktreeWithConfirm(wt, { onDone: refreshWorktrees, onError: setError })
                              }
                            />
                          </span>
                        </Link>
                      </li>
                    );
                  })}
                  {worktreesLoading && (worktreesByRepo[r.id]?.length ?? 0) === 0 && (
                    <li className="sidebar-empty muted">Loading…</li>
                  )}
                  {!worktreesLoading && (worktreesByRepo[r.id]?.length ?? 0) === 0 && (
                    <li className="sidebar-empty muted">No worktrees yet.</li>
                  )}
                </ul>
              </li>
            ))}
          </ul>
        )}
      </div>

      {error && <p className="error sidebar-error">{error}</p>}
    </nav>
  );
}
