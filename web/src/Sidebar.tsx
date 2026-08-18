import { useState } from "react";
import { Link, useMatch } from "react-router-dom";
import { SpotlightStatus, Worktree, WorktreeStatus } from "./api";
import { useRepoContext } from "./RepoContext";
import SettingsModal from "./SettingsModal";
import WorktreeAuditLog from "./WorktreeAuditLog";
import {
  archiveWorktreeWithConfirm,
  deleteWorktreeWithConfirm,
  startSpotlightWithFriendlyError,
  stopSpotlightSafe,
} from "./worktreeActions";
import { TransientIndicatorPhase, useTransientIndicator } from "./useTransientIndicator";
import { rootWorktreeId } from "./rootWorktree";
import WorktreeHoverPopover from "./WorktreeHoverPopover";
import { ChevronIcon, SpotlightIcon } from "./icons/StatusIcons";
import { GitBranchIcon } from "./icons/FileTreeIcons";
import { SplitHorizontalIcon, SplitVerticalIcon } from "./icons/SplitIcons";
import VSCodeIcon from "./icons/VSCodeIcon";
import { useAttentionBlink } from "./useAttentionBlink";
import { useActiveWorktreeActions } from "./activeWorktreeActions";
import { useActiveFileTreeActions } from "./activeFileTreeActions";

interface Props {
  onAddRepo: () => void;
  onNewWorktree: (repoId: string) => void;
}

interface RowProps {
  repoId: string;
  wt: Worktree;
  isActive: boolean;
  status: WorktreeStatus | undefined;
  spot: SpotlightStatus | undefined;
  spotlightPending: boolean;
  attentionMessage: string | undefined;
  attentionBlinking: boolean;
  refreshingPhase: TransientIndicatorPhase;
  onSpotlightStart: () => void;
  onSpotlightStop: () => void;
  onViewLog: () => void;
  onArchive: () => void;
  onDelete: () => void;
  activeWorktreeActions: ReturnType<typeof useActiveWorktreeActions>;
  activeFileTreeActions: ReturnType<typeof useActiveFileTreeActions>;
}

// A worktree row that expands, accordion-style, into a card with the full
// worktree name, copyable branch name, and every per-worktree action: the
// icons that used to live in WorktreeDetail.tsx's toolbar and
// FileTree.tsx's header (files/VS Code/terminal split/git-filter), plus
// the items that used to live behind the row's kebab menu (spotlight,
// view log, archive, delete). Only the first group depends on this
// worktree actually being open right now (see
// activeWorktreeActions.ts/activeFileTreeActions.ts) — the rest are plain
// API calls or local modal state, so they work regardless.
function SidebarWorktreeRow({
  repoId,
  wt,
  isActive,
  status,
  spot,
  spotlightPending,
  attentionMessage,
  attentionBlinking,
  refreshingPhase,
  onSpotlightStart,
  onSpotlightStop,
  onViewLog,
  onArchive,
  onDelete,
  activeWorktreeActions,
  activeFileTreeActions,
}: RowProps) {
  const [expanded, setExpanded] = useState(false);

  const forThisWorktree = activeWorktreeActions?.worktreeId === wt.id ? activeWorktreeActions : null;
  const fileTreeForThisWorktree = activeFileTreeActions?.worktreeId === wt.id ? activeFileTreeActions : null;
  const inactiveTitle = "Open this worktree first";

  return (
    <li className="sidebar-worktree-card">
      <WorktreeHoverPopover wt={wt}>
        <Link
          to={`/repo/${repoId}/worktree/${wt.id}`}
          className={isActive ? "sidebar-worktree active" : "sidebar-worktree"}
          // Navigating to the worktree you're already on is a no-op (same
          // route, same mounted WorktreeDetail instance) — repurposed as
          // the expand/collapse toggle instead, so a second click on the
          // already-focused row does something instead of nothing. This is
          // meant to become a muscle-memory habit: click once to open,
          // click again (now focused) to expand.
          onClick={(e) => {
            if (!isActive) return;
            e.preventDefault();
            setExpanded((v) => !v);
          }}
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
            {(spotlightPending || spot?.active) && (
              <span
                className={spotlightPending ? "sidebar-spotlight-icon pending" : "sidebar-spotlight-icon"}
                title={spotlightPending ? "Updating spotlight status…" : "Spotlight active"}
              >
                <SpotlightIcon size={13} />
              </span>
            )}
            {attentionMessage !== undefined && (
              <span
                className={
                  attentionBlinking ? "sidebar-dot sidebar-dot-attention blinking" : "sidebar-dot sidebar-dot-attention"
                }
                title={attentionMessage || "Claude is waiting for your input"}
              />
            )}
            {isActive && refreshingPhase !== "hidden" && (
              <span
                className={refreshingPhase === "fading" ? "sidebar-status-refreshing fading" : "sidebar-status-refreshing"}
                title="Refreshing status…"
                aria-label="Refreshing status"
              />
            )}
            <button
              type="button"
              className="sidebar-worktree-expand-toggle"
              aria-label={expanded ? `Collapse ${wt.branch}` : `Expand ${wt.branch}`}
              aria-expanded={expanded}
              onClick={(e) => {
                e.preventDefault();
                e.stopPropagation();
                setExpanded((v) => !v);
              }}
            >
              <ChevronIcon />
            </button>
          </span>
        </Link>
      </WorktreeHoverPopover>

      <div
        className={expanded ? "sidebar-worktree-card-wrapper expanded" : "sidebar-worktree-card-wrapper"}
        style={{ gridTemplateRows: expanded ? "1fr" : "0fr" }}
      >
        <div className="sidebar-worktree-card-inner">
          <div className="sidebar-worktree-card-name">{wt.name}</div>
          <div className="sidebar-worktree-card-icons">
            <button
              type="button"
              className={fileTreeForThisWorktree?.filterToChanged ? "active" : undefined}
              disabled={!fileTreeForThisWorktree?.changedFilesAvailable}
              title={fileTreeForThisWorktree ? "Filter to only files changed in this branch" : inactiveTitle}
              onClick={() => fileTreeForThisWorktree?.toggleChangedFilesFilter()}
            >
              <GitBranchIcon size={14} />
            </button>
            <button
              type="button"
              className={forThisWorktree?.filesOpen ? "active" : undefined}
              disabled={!forThisWorktree}
              title={forThisWorktree ? "Toggle the file tree sidebar" : inactiveTitle}
              onClick={() => forThisWorktree?.toggleFiles()}
            >
              📁
            </button>
            <button
              type="button"
              disabled={!forThisWorktree || !forThisWorktree.vscodeAvailable}
              title={
                !forThisWorktree
                  ? inactiveTitle
                  : forThisWorktree.vscodeAvailable
                    ? "Open this worktree in VS Code"
                    : "VS Code CLI ('code') not detected — see Settings > Installation"
              }
              onClick={() => forThisWorktree?.openVSCode()}
            >
              <VSCodeIcon size={14} />
            </button>
            <button
              type="button"
              disabled={!forThisWorktree}
              title={forThisWorktree ? "New terminal tab" : inactiveTitle}
              onClick={() => forThisWorktree?.newTerminal()}
            >
              +
            </button>
            <button
              type="button"
              disabled={!forThisWorktree}
              title={forThisWorktree ? "Split right (new terminal)" : inactiveTitle}
              onClick={() => forThisWorktree?.splitRight()}
            >
              <SplitVerticalIcon size={14} />
            </button>
            <button
              type="button"
              disabled={!forThisWorktree}
              title={forThisWorktree ? "Split down (new terminal)" : inactiveTitle}
              onClick={() => forThisWorktree?.splitDown()}
            >
              <SplitHorizontalIcon size={14} />
            </button>
            {/* Used to live behind the row's kebab menu — none of these
                depend on this worktree being the currently-open one (unlike
                the icons above), so they're always enabled. */}
            {spot?.available &&
              (spot.active ? (
                <button type="button" title="Stop spotlight" onClick={onSpotlightStop}>
                  <SpotlightIcon size={14} />
                </button>
              ) : (
                <button
                  type="button"
                  title={spot.active_worktree_path ? "Start spotlight (replaces active mirror)" : "Start spotlight"}
                  onClick={onSpotlightStart}
                >
                  <SpotlightIcon size={14} />
                </button>
              ))}
            <button type="button" title="View worktree log" onClick={onViewLog}>
              🕐
            </button>
            <button type="button" title="Archive" onClick={onArchive}>
              🗄
            </button>
            <button type="button" className="danger" title="Delete" onClick={onDelete}>
              🗑
            </button>
          </div>
        </div>
      </div>
    </li>
  );
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

  // Whichever worktree is actually open right now (if any) — shared by
  // every row's expanded card so only that one gets live, enabled action
  // icons. See activeWorktreeActions.ts/activeFileTreeActions.ts.
  const activeWorktreeActions = useActiveWorktreeActions();
  const activeFileTreeActions = useActiveFileTreeActions();

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
                    {/* The repo root is a real (synthetic) worktree row
                        server-side (see rootWorktree.ts/EnsureRootWorktree),
                        so a claude session running there can page this the
                        same way a real worktree's row does — it just has no
                        card of its own to show it next to. */}
                    {attentionPending[rootWorktreeId(r.id)] !== undefined && (
                      <span
                        className={
                          attentionBlinking.has(rootWorktreeId(r.id))
                            ? "sidebar-dot sidebar-dot-attention blinking"
                            : "sidebar-dot sidebar-dot-attention"
                        }
                        title={attentionPending[rootWorktreeId(r.id)] || "Claude is waiting for your input"}
                      />
                    )}
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
                  {/* "Flight strip" row — see docs/design.md. Only the
                      currently-selected worktree gets a color (green);
                      dirty/clean isn't encoded via the row's own border
                      color anymore (see style.css for why — per direct
                      feedback that was noise, not signal), just the
                      ahead/behind ticks below. Expands, accordion-style,
                      into a card with the full name/PR/action icons — see
                      SidebarWorktreeRow above. */}
                  {(worktreesByRepo[r.id] ?? []).map((wt) => (
                    <SidebarWorktreeRow
                      key={wt.id}
                      repoId={r.id}
                      wt={wt}
                      isActive={wt.id === activeWorktreeId}
                      status={gitStatus[wt.id]}
                      spot={spotlightStatus[wt.id]}
                      spotlightPending={!!spotlightPending[wt.id]}
                      attentionMessage={attentionPending[wt.id]}
                      attentionBlinking={attentionBlinking.has(wt.id)}
                      refreshingPhase={wt.id === activeWorktreeId ? activeStatusRefreshingPhase : "hidden"}
                      onSpotlightStart={() => handleSpotlightStart(wt)}
                      onSpotlightStop={() => handleSpotlightStop(wt)}
                      onViewLog={() => setLogWorktree(wt)}
                      onArchive={() =>
                        archiveWorktreeWithConfirm(wt, { onDone: refreshWorktrees, onError: setError })
                      }
                      onDelete={() =>
                        deleteWorktreeWithConfirm(wt, { onDone: refreshWorktrees, onError: setError })
                      }
                      activeWorktreeActions={activeWorktreeActions}
                      activeFileTreeActions={activeFileTreeActions}
                    />
                  ))}
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
