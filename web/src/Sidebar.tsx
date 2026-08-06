import { useState } from "react";
import { Link, useMatch, useNavigate } from "react-router-dom";
import { createWorktree, Repo } from "./api";
import AddRepoDialog from "./AddRepoDialog";
import NewWorktreeDialog from "./NewWorktreeDialog";
import { useRepoContext } from "./RepoContext";
import WorktreeActionsMenu from "./WorktreeActionsMenu";
import { deleteWorktreeWithConfirm, startSpotlightWithFriendlyError, stopSpotlightSafe } from "./worktreeActions";

// The persistent left sidebar: every registered repo, with its worktrees
// nested directly underneath — the actual point of this component — so
// switching between parallel in-progress worktrees, across any repo,
// never means navigating back through a list view first. Auto-loads via
// RepoContext whenever the set of repos/worktrees changes, including a
// fresh browser tab opened straight at a worktree URL.
//
// TODO (future, not this pass): nest repos under a "project" grouping —
// for now a flat list of repos is enough.
export default function Sidebar() {
  const {
    repos,
    reposLoading,
    refreshRepos,
    worktreesByRepo,
    worktreesLoading,
    refreshWorktrees,
    gitStatus,
    spotlightStatus,
  } = useRepoContext();
  const navigate = useNavigate();

  const worktreeMatch = useMatch("/repo/:repoId/worktree/:worktreeId");
  const activeWorktreeId = worktreeMatch?.params.worktreeId ?? null;

  const [addRepoOpen, setAddRepoOpen] = useState(false);
  const [newWorktreeRepoId, setNewWorktreeRepoId] = useState<string | null>(null);
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
            onClick={() => setAddRepoOpen(true)}
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
                    onClick={() => setNewWorktreeRepoId(r.id)}
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
                        <Link
                          to={`/repo/${r.id}/worktree/${wt.id}`}
                          className={
                            wt.id === activeWorktreeId
                              ? "sidebar-worktree active"
                              : "sidebar-worktree"
                          }
                        >
                          <span className="sidebar-worktree-branch">{wt.branch}</span>
                          <span className="sidebar-worktree-meta">
                            {status && (
                              <span
                                className={status.dirty ? "sidebar-dot sidebar-dot-dirty" : "sidebar-dot sidebar-dot-clean"}
                                title={status.dirty ? "Dirty" : "Clean"}
                              />
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

      {addRepoOpen && (
        <AddRepoDialog
          onCreated={(repo: Repo) => {
            refreshRepos();
            navigate(`/repo/${repo.id}`);
          }}
          onClose={() => setAddRepoOpen(false)}
        />
      )}

      {newWorktreeRepoId && (
        <NewWorktreeDialog
          repoId={newWorktreeRepoId}
          onCreate={async (name) => {
            const wt = await createWorktree(newWorktreeRepoId, name);
            refreshWorktrees();
            navigate(`/repo/${newWorktreeRepoId}/worktree/${wt.id}`);
          }}
          onClose={() => setNewWorktreeRepoId(null)}
        />
      )}
    </nav>
  );
}
