import { ReactNode, useEffect, useRef, useState } from "react";
import { getWorktreeSummary, Worktree, WorktreeSummary } from "./api";
import { getCachedSummary, setCachedSummary } from "./prGitCache";

// Delay before the popover appears — long enough that scanning down the
// sidebar (which passes over several rows) doesn't fire it for every one,
// per direct request. Hiding is immediate (no symmetric delay) — a
// popover that lingers after the mouse has already left reads as stuck,
// not helpful.
const SHOW_DELAY_MS = 900;

interface Props {
  wt: Worktree;
  children: ReactNode;
}

// Wraps a sidebar worktree row: on hover (after SHOW_DELAY_MS), shows the
// worktree's full name (the row itself truncates it) plus its git/PR
// summary — the row itself has no room for that detail. Reads/writes
// prGitCache.ts's localStorage cache rather than calling
// getWorktreeSummary on every hover: a fresh cache entry is shown as-is, a
// stale or missing one is shown (if present) while a background refetch
// updates it, which is what keeps repeated hovers from hitting GitHub's
// API rate limits through the backend's own `gh` call.
export default function WorktreeHoverPopover({ wt, children }: Props) {
  const [visible, setVisible] = useState(false);
  const [summary, setSummary] = useState<WorktreeSummary | null>(null);
  const [error, setError] = useState<string | null>(null);
  const showTimerRef = useRef<number | undefined>(undefined);

  useEffect(() => {
    return () => {
      if (showTimerRef.current) window.clearTimeout(showTimerRef.current);
    };
  }, []);

  function handleMouseEnter() {
    showTimerRef.current = window.setTimeout(() => {
      setVisible(true);
      setError(null);

      const cached = getCachedSummary(wt.id);
      if (cached) setSummary(cached.data);
      if (!cached || cached.stale) {
        getWorktreeSummary(wt.repo_id, wt.id)
          .then((data) => {
            setCachedSummary(wt.id, data);
            setSummary(data);
          })
          .catch((err) => {
            // Keep showing cached/stale data (set above) if there is any
            // — only surface an error when there's nothing else to show.
            if (!cached) setError((err as Error).message);
          });
      }
    }, SHOW_DELAY_MS);
  }

  function handleMouseLeave() {
    if (showTimerRef.current) {
      window.clearTimeout(showTimerRef.current);
      showTimerRef.current = undefined;
    }
    setVisible(false);
  }

  return (
    <div className="worktree-hover-target" onMouseEnter={handleMouseEnter} onMouseLeave={handleMouseLeave}>
      {children}
      {visible && (
        <div className="worktree-hover-popover" role="tooltip">
          <div className="worktree-hover-popover-name">{wt.name}</div>
          <div className="worktree-hover-popover-branch">{wt.branch}</div>
          {error && <p className="error">{error}</p>}
          {!error && !summary && <p className="muted">Loading…</p>}
          {summary && (
            <>
              <div className="worktree-hover-popover-pr">
                {summary.pr ? (
                  <a href={summary.pr.url} target="_blank" rel="noopener noreferrer">
                    PR #{summary.pr.number}
                    {summary.pr.is_draft ? " (draft)" : ""} · {summary.pr.state} — {summary.pr.title}
                  </a>
                ) : (
                  <span className="muted">No pull request for this branch</span>
                )}
              </div>
              <div className="worktree-hover-popover-git">
                {summary.has_upstream && (summary.ahead > 0 || summary.behind > 0) && (
                  <span className="sidebar-ticks">
                    {summary.ahead > 0 && `↑${summary.ahead} `}
                    {summary.behind > 0 && `↓${summary.behind}`}
                  </span>
                )}
                <span className={summary.dirty ? "badge badge-dirty" : "badge badge-clean"}>
                  {summary.dirty ? `${summary.changed_files.length} changed file(s)` : "clean"}
                </span>
              </div>
              {summary.changed_files.length > 0 && (
                <ul className="worktree-hover-popover-files">
                  {summary.changed_files.slice(0, 8).map((f) => (
                    <li key={f}>
                      <code>{f}</code>
                    </li>
                  ))}
                  {summary.changed_files.length > 8 && (
                    <li className="muted">+{summary.changed_files.length - 8} more</li>
                  )}
                </ul>
              )}
            </>
          )}
        </div>
      )}
    </div>
  );
}
