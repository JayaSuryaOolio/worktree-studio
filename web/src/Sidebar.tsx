import { useState } from "react";
import { Link, useMatch } from "react-router-dom";
import { Worktree } from "./api";
import { useRepoContext } from "./RepoContext";
import WorktreeActionsMenu from "./WorktreeActionsMenu";
import SettingsModal from "./SettingsModal";
import WorktreeAuditLog from "./WorktreeAuditLog";
import { archiveWorktreeWithConfirm, startSpotlightWithFriendlyError, stopSpotlightSafe } from "./worktreeActions";
import { useTransientIndicator } from "./useTransientIndicator";
import { rootWorktreeId } from "./rootWorktree";
import WorktreeHoverPopover from "./WorktreeHoverPopover";
import { SpotlightIcon } from "./icons/StatusIcons";
import { useAttentionBlink } from "./useAttentionBlink";

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
    statusRefreshing,
    refreshSpotlightStatus,
    attentionPending,
  } = useRepoContext();

  // True for a worktree id from the moment its spotlight start/stop is
  // clicked until that worktree's spotlight status has actually been
  // re-fetched — drives the blinking dot below. Without an explicit
  // refreshSpotlightStatus call (see the two handlers below), the sidebar
  // had no way to know the action had finished short of waiting for the
  // background scheduler's own next naturally-due tick, which read as the
  // action itself being slow (a real reported bug) when it wasn't.
  const [spotlightPending, setSpotlightPending] = useState<Record<string, boolean>>({});

  async function handleSpotlightStart(wt: Worktree) {
    setSpotlightPending((p) => ({ ...p, [wt.id]: true }));
    try {
      await startSpotlightWithFriendlyError(wt, { onDone: refreshWorktrees, onError: setError });
      await refreshSpotlightStatus(wt.id);
    } finally {
      setSpotlightPending((p) => ({ ...p, [wt.id]: false }));
    }
  }

  async function handleSpotlightStop(wt: Worktree) {
    setSpotlightPending((p) => ({ ...p, [wt.id]: true }));
    try {
      await stopSpotlightSafe(wt, { onDone: refreshWorktrees, onError: setError });
      await refreshSpotlightStatus(wt.id);
    } finally {
      setSpotlightPending((p) => ({ ...p, [wt.id]: false }));
    }
  }

  const worktreeMatch = useMatch("/repo/:repoId/worktree/:worktreeId");
  const activeWorktreeId = worktreeMatch?.params.worktreeId ?? null;

  // Debounced/held/fade-out presence for the "refreshing" dot below — see
  // useTransientIndicator.ts. Only the active worktree's row can ever show
  // it (RepoContext.tsx's stale-on-focus refresh only fires for the
  // currently-viewed worktree), so this one hook call covers every row.
  const activeStatusRefreshingPhase = useTransientIndicator(
    activeWorktreeId ? !!statusRefreshing[activeWorktreeId] : false
  );

  // Which pending worktrees are still within their first 10s of blinking —
  // see useAttentionBlink.ts. The dot itself never disappears on its own
  // (only the explicit clear-on-focus in RepoContext.tsx removes it); this
  // only controls whether it's currently animating or holding steady.
  const attentionBlinking = useAttentionBlink(Object.keys(attentionPending));

  const [error, setError] = useState<string | null>(null);
  const [logWorktree, setLogWorktree] = useState<Worktree | null>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);

  return (
    <nav className="sidebar" aria-label="Repos and worktrees">
      <div className="sidebar-header">
        <Link to="/" className="sidebar-brand">
          worktree-studio
        </Link>
        <button
          type="button"
          className="icon-button"
          aria-label="Settings"
          title="Settings"
          onClick={() => setSettingsOpen(true)}
        >
          ⚙
        </button>
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
                  {/* Opens the repo's own root checkout through the same
                      worktree-detail UI as a real worktree (terminals,
                      files, layout) — see rootWorktree.ts and
                      EnsureRootWorktree. Highlighted green, like an active
                      worktree row, whenever that's what's currently open. */}
                  <Link
                    to={`/repo/${r.id}/worktree/${rootWorktreeId(r.id)}`}
                    className={
                      activeWorktreeId === rootWorktreeId(r.id) ? "sidebar-repo active" : "sidebar-repo"
                    }
                  >
                    {r.name}
                  </Link>
                  {/* Grouped in their own right-aligned cluster (rather than
                      as two siblings of the row's justify-content:
                      space-between) so the gear sits directly next to
                      "+ new worktree" instead of floating in the middle of
                      the row. */}
                  <div className="sidebar-repo-row-actions">
                    <Link
                      to={`/repo/${r.id}/settings`}
                      className="icon-button"
                      aria-label={`${r.name} settings`}
                      title="Repo settings"
                    >
                      ⚙
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
                </div>

                <ul className="sidebar-worktree-list">
                  {(worktreesByRepo[r.id] ?? []).map((wt) => {
                    const status = gitStatus[wt.id];
                    const spot = spotlightStatus[wt.id];
                    return (
                      <li key={wt.id}>
                        {/* "Flight strip" row — see docs/design.md. Only
                            the currently-selected worktree gets a color
                            (green); dirty/clean isn't encoded via the
                            row's own border color anymore (see style.css
                            for why — per direct feedback that was noise,
                            not signal), just the ahead/behind ticks below.
                            Wrapped in WorktreeHoverPopover for the full
                            name + PR/git-summary popover — the row itself
                            has no room to show either, hence hover. */}
                        <WorktreeHoverPopover wt={wt}>
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
                            {status?.has_upstream && (status.ahead > 0 || status.behind > 0) && (
                              <span
                                className="sidebar-ticks"
                                title={`${status.ahead} ahead, ${status.behind} behind upstream`}
                              >
                                {status.ahead > 0 && `↑${status.ahead}`}
                                {status.behind > 0 && `↓${status.behind}`}
                              </span>
                            )}
                            {/* Blinks (via opacity) for the full duration of
                                a spotlight start/stop (see
                                handleSpotlightStart/Stop above) — a real
                                reported bug was that this only ever caught
                                up on whatever the background poller's own
                                next tick happened to be, up to 15s later,
                                which read as the action itself being slow.
                                A lamp icon, not a dot — see StatusIcons.tsx;
                                the plain dot look moved to the attention
                                badge below instead. */}
                            {(spotlightPending[wt.id] || spot?.active) && (
                              <span
                                className={
                                  spotlightPending[wt.id]
                                    ? "sidebar-spotlight-icon pending"
                                    : "sidebar-spotlight-icon"
                                }
                                title={spotlightPending[wt.id] ? "Updating spotlight status…" : "Spotlight active"}
                              >
                                <SpotlightIcon size={13} />
                              </span>
                            )}
                            {/* A claude session in this worktree is waiting
                                on a permission prompt or user input — see
                                useAttentionStream.ts/internal/attention.
                                Persists (regardless of focus) until the
                                worktree's detail page is opened, which
                                clears it (see RepoContext.tsx) — it never
                                disappears on its own. Blinks for its first
                                10s (useAttentionBlink.ts), then holds
                                steady for as long as it stays pending —
                                blinking forever read as more alarming than
                                useful. */}
                            {wt.id in attentionPending && (
                              <span
                                className={
                                  attentionBlinking.has(wt.id)
                                    ? "sidebar-dot sidebar-dot-attention blinking"
                                    : "sidebar-dot sidebar-dot-attention"
                                }
                                title={attentionPending[wt.id] || "Claude is waiting for your input"}
                              />
                            )}
                            {/* Only meaningful for the just-focused worktree in
                                practice (see RepoContext.tsx's stale-on-focus
                                refresh) — a brief, non-blocking cue that a
                                just-opened worktree's status was stale and is
                                being refreshed, not a permanent loading state.
                                Debounced/held/fade-out via
                                useTransientIndicator so a fast refresh (the
                                common case) doesn't just flash on and off. */}
                            {wt.id === activeWorktreeId && activeStatusRefreshingPhase !== "hidden" && (
                              <span
                                className={
                                  activeStatusRefreshingPhase === "fading"
                                    ? "sidebar-status-refreshing fading"
                                    : "sidebar-status-refreshing"
                                }
                                title="Refreshing status…"
                                aria-label="Refreshing status"
                              />
                            )}
                            <WorktreeActionsMenu
                              wt={wt}
                              spotlightStatus={spot}
                              onSpotlightStart={() => handleSpotlightStart(wt)}
                              onSpotlightStop={() => handleSpotlightStop(wt)}
                              onViewLog={() => setLogWorktree(wt)}
                              onArchive={() =>
                                archiveWorktreeWithConfirm(wt, { onDone: refreshWorktrees, onError: setError })
                              }
                            />
                          </span>
                        </Link>
                        </WorktreeHoverPopover>
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

      {logWorktree && (
        <WorktreeAuditLog
          repoId={logWorktree.repo_id}
          worktreeId={logWorktree.id}
          title={logWorktree.branch}
          onClose={() => setLogWorktree(null)}
        />
      )}

      {settingsOpen && <SettingsModal onClose={() => setSettingsOpen(false)} />}
    </nav>
  );
}
