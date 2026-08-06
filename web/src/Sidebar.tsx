import { Link, useMatch } from "react-router-dom";
import { useRepoContext } from "./RepoContext";

// The persistent left sidebar: registered repos, and — the actual point of
// this component — the selected repo's worktrees, always visible, so
// switching between parallel in-progress worktrees doesn't mean navigating
// back through a list view first. Auto-loads via RepoContext whenever the
// selected repo changes, including a fresh browser tab opened straight at
// a worktree URL.
export default function Sidebar() {
  const {
    repos,
    reposLoading,
    selectedRepoId,
    worktrees,
    worktreesLoading,
    gitStatus,
    spotlightStatus,
  } = useRepoContext();

  const worktreeMatch = useMatch("/repo/:repoId/worktree/:worktreeId");
  const activeWorktreeId = worktreeMatch?.params.worktreeId ?? null;

  return (
    <nav className="sidebar" aria-label="Repos and worktrees">
      <div className="sidebar-header">
        <Link to="/" className="sidebar-brand">
          worktree-studio
        </Link>
      </div>

      <div className="sidebar-section">
        <div className="sidebar-section-title">Repos</div>
        {reposLoading ? (
          <p className="sidebar-empty muted">Loading…</p>
        ) : repos.length === 0 ? (
          <p className="sidebar-empty muted">None registered yet.</p>
        ) : (
          <ul className="sidebar-repo-list">
            {repos.map((r) => (
              <li key={r.id}>
                <Link
                  to={`/repo/${r.id}`}
                  className={r.id === selectedRepoId ? "sidebar-repo active" : "sidebar-repo"}
                >
                  {r.name}
                </Link>
              </li>
            ))}
          </ul>
        )}
      </div>

      {selectedRepoId && (
        <div className="sidebar-section sidebar-worktrees">
          <div className="sidebar-section-title">Worktrees</div>
          {worktreesLoading ? (
            <p className="sidebar-empty muted">Loading…</p>
          ) : worktrees.length === 0 ? (
            <p className="sidebar-empty muted">No worktrees yet.</p>
          ) : (
            <ul className="sidebar-worktree-list">
              {worktrees.map((wt) => {
                const status = gitStatus[wt.id];
                const spot = spotlightStatus[wt.id];
                return (
                  <li key={wt.id}>
                    <Link
                      to={`/repo/${selectedRepoId}/worktree/${wt.id}`}
                      className={
                        wt.id === activeWorktreeId
                          ? "sidebar-worktree active"
                          : "sidebar-worktree"
                      }
                    >
                      <span className="sidebar-worktree-branch">{wt.branch}</span>
                      <span className="sidebar-worktree-meta">
                        {status?.dirty && <span className="sidebar-dot sidebar-dot-dirty" title="Dirty" />}
                        {!status?.dirty && status && (
                          <span className="sidebar-dot sidebar-dot-clean" title="Clean" />
                        )}
                        {spot?.active && (
                          <span className="sidebar-dot sidebar-dot-spotlight" title="Spotlight active" />
                        )}
                      </span>
                    </Link>
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      )}
    </nav>
  );
}
