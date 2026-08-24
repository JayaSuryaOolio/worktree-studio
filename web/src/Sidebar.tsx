import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useMatch, useNavigate } from "react-router-dom";
import { SpotlightStatus, Worktree, WorktreeStatus } from "./api";
import { splitBranchLabel } from "./branchLabel";
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
import { setPendingNewTerminal } from "./pendingNewTerminal";
import { isTextEntryTarget } from "./keyboard";
import { getCollapsedRepos, setCollapsedRepos } from "./sidebarPreferences";

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
  onDelete: () => Promise<void>;
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
  // Guards against a double-click firing two overlapping delete flows —
  // each stacks its own confirm() dialog and, if both are answered, sends
  // two concurrent DELETE requests for the same worktree. A real reported
  // bug: the second request's confirm()/await chain could still be running
  // when the row unmounts (delete succeeded, worktree gone from the list),
  // so this is intentionally local state, not something threaded back into
  // an already-gone row.
  const [deleting, setDeleting] = useState(false);

  const { head: branchHead, tail: branchTail } = splitBranchLabel(wt.branch);

  const forThisWorktree = activeWorktreeActions?.worktreeId === wt.id ? activeWorktreeActions : null;
  const fileTreeForThisWorktree = activeFileTreeActions?.worktreeId === wt.id ? activeFileTreeActions : null;
  const inactiveTitle = "Open this worktree first";

  async function handleDelete() {
    if (deleting) return;
    setDeleting(true);
    try {
      await onDelete();
    } finally {
      setDeleting(false);
    }
  }

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
          {/* Split so the ellipsis lands in the MIDDLE of the name, not
              at its end — branch names are prefix-clustered, so the tail
              is what tells rows apart (see branchLabel.ts and the
              .sidebar-worktree-branch rules). The two spans are written
              without whitespace between them on purpose: JSX would render
              it as a real space inside the branch name. */}
          <span className="sidebar-worktree-branch" title={wt.branch}>
            <span className="sidebar-worktree-branch-head">{branchHead}</span>
            {branchTail !== "" && <span className="sidebar-worktree-branch-tail">{branchTail}</span>}
          </span>
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
            <button type="button" className="danger" title="Delete" disabled={deleting} onClick={handleDelete}>
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
  const navigate = useNavigate();

  async function handleSpotlightStart(wt: Worktree) {
    setSpotlightPending((p) => ({ ...p, [wt.id]: true }));
    try {
      await startSpotlightWithFriendlyError(wt, {
        onDone: () => {
          refreshWorktrees();
          openShellAtRepoRoot(wt.repo_id);
        },
        onError: setError,
      });
      await refreshSpotlightStatus(wt.id);
    } finally {
      setSpotlightPending((p) => ({ ...p, [wt.id]: false }));
    }
  }

  // Turning spotlight on mirrors the repo's root checkout from this
  // worktree — direct user request: also open a shell there, since the
  // whole point is to have the root's dependencies/build output available
  // to work with. Always opens a fresh terminal tab rather than checking
  // for one already sitting at the root (a "focus existing instead"
  // version is a possible follow-up, deliberately skipped for now). If the
  // root worktree's own tab is already open, adds the panel directly via
  // activeWorktreeActions (same bridge the sidebar's per-worktree terminal
  // icons use); otherwise navigates there first and leaves a
  // pendingNewTerminal instruction for that page to pick up once its
  // dockview is ready (same idiom as pendingFileOpen.ts).
  function openShellAtRepoRoot(repoId: string) {
    const rootId = rootWorktreeId(repoId);
    if (activeWorktreeActions?.worktreeId === rootId) {
      activeWorktreeActions.newTerminal();
      return;
    }
    setPendingNewTerminal(rootId);
    navigate(`/repo/${repoId}/worktree/${rootId}`);
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

  // --- Status board state -------------------------------------------
  //
  // The sidebar's job is "what needs me?", not "list everything I own"
  // (that's what the command palette is for). Three pieces make that
  // work at 20+ worktrees: a filter, collapsible repo groups, and a
  // waiting-count you can filter by.
  //
  // Deliberately NOT sorting attention-first, despite that being the
  // obvious reading of the principle: rows that reorder themselves
  // underneath the pointer cause misclicks, and attention arrives
  // asynchronously from a websocket, so it would happen exactly while
  // you were reaching for something. Order stays stable; attention is
  // surfaced by the count, the dot, and the filter below instead.
  const [filter, setFilter] = useState("");
  const [attentionOnly, setAttentionOnly] = useState(false);
  const [collapsed, setCollapsed] = useState<Set<string>>(getCollapsedRepos);
  const filterRef = useRef<HTMLInputElement>(null);

  function toggleCollapsed(repoId: string) {
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(repoId)) next.delete(repoId);
      else next.add(repoId);
      setCollapsedRepos(next);
      return next;
    });
  }

  // "/" focuses the filter — but only when the keystroke isn't already
  // going somewhere that wants a literal slash. Without the guard this
  // would steal every "/" typed into a shell, which is most of them.
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key !== "/" || e.metaKey || e.ctrlKey || e.altKey) return;
      if (isTextEntryTarget(e.target)) return;
      e.preventDefault();
      filterRef.current?.focus();
      filterRef.current?.select();
    }
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, []);

  const attentionCount = Object.keys(attentionPending).length;

  // Filtering runs across every repo at once, so a match in a collapsed
  // group still surfaces — a filter that silently skipped collapsed
  // groups would be worse than no filter.
  const query = filter.trim().toLowerCase();
  const visibleByRepo = useMemo(() => {
    const out: Record<string, Worktree[]> = {};
    for (const repo of repos) {
      const all = worktreesByRepo[repo.id] ?? [];
      const repoMatches = repo.name.toLowerCase().includes(query);
      out[repo.id] = all.filter((wt) => {
        if (attentionOnly && attentionPending[wt.id] === undefined) return false;
        if (query === "" || repoMatches) return true;
        return (
          wt.branch.toLowerCase().includes(query) || wt.name.toLowerCase().includes(query)
        );
      });
    }
    return out;
  }, [repos, worktreesByRepo, query, attentionOnly, attentionPending]);

  // A repo drops out entirely when nothing in it matches — unless its own
  // name is the match, in which case it stays as an (empty) heading you
  // can still create a worktree in.
  const narrowing = query !== "" || attentionOnly;
  const visibleRepos = narrowing
    ? repos.filter(
        (r) => visibleByRepo[r.id].length > 0 || (query !== "" && r.name.toLowerCase().includes(query))
      )
    : repos;

  return (
    <nav className="sidebar" aria-label="Repos and worktrees">
      {/* Pinned. The brand, the filter and the "Repos" heading used to sit
          inside the sidebar's single scroll container, so scrolling to a
          repo near the bottom of a long list scrolled the search box off
          the top — the one control you reach for *because* the list is
          long. Only the tree scrolls now. */}
      <div className="sidebar-top">
        <div className="sidebar-header">
          <Link to="/" className="sidebar-brand">
          worktree-studio
        </Link>
        {/* The one number worth putting in the chrome: how many claude
            sessions are waiting on you, anywhere. Renders nothing at zero
            — the normal state of this app is silence. Clicking it narrows
            the list to exactly those worktrees. */}
        {attentionCount > 0 && (
          <button
            type="button"
            className={attentionOnly ? "sidebar-waiting active" : "sidebar-waiting"}
            aria-pressed={attentionOnly}
            title={
              attentionOnly
                ? "Showing only worktrees waiting on you — click to show all"
                : "Show only the worktrees waiting on you"
            }
            onClick={() => setAttentionOnly((v) => !v)}
          >
            <span className="sidebar-dot sidebar-dot-attention" />
            {attentionCount} waiting
          </button>
        )}
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

      <div className="sidebar-filter">
        <input
          ref={filterRef}
          type="text"
          value={filter}
          placeholder="Filter worktrees"
          aria-label="Filter worktrees"
          onChange={(e) => setFilter(e.target.value)}
          onKeyDown={(e) => {
            if (e.key !== "Escape") return;
            // Escape clears first and only gives up focus once there's
            // nothing left to clear — otherwise a single Escape both
            // wipes the query and drops you out of the box, which makes
            // correcting a typo a two-step recovery.
            if (filter !== "") {
              e.stopPropagation();
              setFilter("");
            } else {
              e.currentTarget.blur();
            }
          }}
        />
          {/* A hint, not a control. This was a bordered keycap, which read
              as a button sitting inside the text field — something you
              could click, and which did nothing. It's a dim glyph now,
              and it clears out of the way the moment the field is
              focused. */}
          <span className="sidebar-filter-hint" aria-hidden="true">
            /
          </span>
        </div>

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
      </div>

      <div className="sidebar-scroll">
        {reposLoading ? (
          <p className="sidebar-empty muted">Loading…</p>
        ) : repos.length === 0 ? (
          <p className="sidebar-empty muted">None registered yet.</p>
        ) : visibleRepos.length === 0 ? (
          <p className="sidebar-empty muted">
            {attentionOnly ? "Nothing waiting on you." : "No worktree matches that."}
          </p>
        ) : (
          <ul className="sidebar-repo-tree">
            {visibleRepos.map((r) => (
              <li key={r.id} className="sidebar-repo-group">
                <div className="sidebar-repo-row">
                  {/* Collapse toggle. A narrowed list is always expanded
                      regardless of the stored state — a filter that
                      silently hid its own matches inside a collapsed
                      group would be worse than no filter — so the toggle
                      hides while narrowing rather than lying about
                      what it would do. */}
                  {!narrowing && (
                    <button
                      type="button"
                      className="sidebar-repo-collapse"
                      aria-label={collapsed.has(r.id) ? `Expand ${r.name}` : `Collapse ${r.name}`}
                      aria-expanded={!collapsed.has(r.id)}
                      onClick={() => toggleCollapsed(r.id)}
                    >
                      <ChevronIcon />
                    </button>
                  )}
                  {/* Opens the repo's own root checkout through the same
                      worktree-detail UI as a real worktree (terminals,
                      files, layout) — see rootWorktree.ts and
                      EnsureRootWorktree. Takes the same accent rail as any
                      other selected row whenever that's what's open. */}
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
                  {/* What a collapsed group still has to tell you: how
                      much is in it, and whether anything inside is
                      waiting. Both render only while collapsed — visible
                      rows already say it themselves. */}
                  {collapsed.has(r.id) && !narrowing && (
                    <span className="sidebar-repo-collapsed-meta">
                      {(worktreesByRepo[r.id] ?? []).some(
                        (wt) => attentionPending[wt.id] !== undefined
                      ) && <span className="sidebar-dot sidebar-dot-attention" />}
                      {/* No "0". An empty collapsed group printing a zero
                          is the exact rule this redesign is built on
                          being broken in miniature: nothing to report
                          should look like nothing. */}
                      {(worktreesByRepo[r.id] ?? []).length > 0 && (
                        <span className="sidebar-repo-count">{(worktreesByRepo[r.id] ?? []).length}</span>
                      )}
                    </span>
                  )}
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

                {!(collapsed.has(r.id) && !narrowing) && (
                <ul className="sidebar-worktree-list">
                  {/* A plain row — no border, no card. Only the currently
                      selected worktree is coloured, via an accent rail
                      (see docs/design-system.md: "rows, not cards", and
                      one railed row on screen at a time). Ahead/behind
                      ticks render only when non-zero. Expands,
                      accordion-style, into a panel with the full name and
                      the per-worktree actions — see SidebarWorktreeRow. */}
                  {(visibleByRepo[r.id] ?? []).map((wt) => (
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
                  {!worktreesLoading && (visibleByRepo[r.id]?.length ?? 0) === 0 && (
                    <li className="sidebar-empty muted">
                      {narrowing ? "No match here." : "No worktrees yet."}
                    </li>
                  )}
                </ul>
                )}
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
