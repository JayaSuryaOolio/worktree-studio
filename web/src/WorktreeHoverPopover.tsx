import { ReactNode, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Worktree } from "./api";
import { useWorktreeSummary } from "./useWorktreeSummary";

// Delay before the popover appears — long enough that scanning down the
// sidebar (which passes over several rows) doesn't fire it for every one,
// per direct request. Hiding has its own short grace period (HIDE_DELAY_MS
// below), not this same delay — the popover should feel responsive to
// leave, just not disappear mid-transit between the row and the popover
// itself.
const SHOW_DELAY_MS = 900;

// Grace period before actually hiding after the mouse leaves either the
// row or the popover. Without this, moving the mouse from the row toward
// the popover (they're not adjacent — there's a real gap, see
// .worktree-hover-popover's margin) leaves both elements for a moment
// while crossing that gap, hiding the popover before the mouse ever
// reaches it — a real reported bug ("closes immediately on mouse leave,
// so I can do nothing but view the details"). Short enough that
// deliberately moving away still reads as immediate, per the original
// "hide immediately on hover away" request — "away" just now means away
// from both elements, not only the row.
const HIDE_DELAY_MS = 150;

// The popover's own fixed width (see .worktree-hover-popover) — needed
// up front to clamp its position against the viewport's right edge below,
// not just read off the DOM after the fact.
const POPOVER_WIDTH_PX = 320;
const VIEWPORT_MARGIN_PX = 8;

interface Props {
  wt: Worktree;
  children: ReactNode;
}

interface Position {
  left: number;
  top: number;
}

// Where to place the popover relative to targetRect: to its right by
// default, but flipped below the row instead whenever the sidebar (fixed
// at 280px) doesn't leave enough room on the right — e.g. a narrow
// browser window. Clamped vertically against the viewport bottom either
// way, since this renders via a portal (see below) and isn't clipped by
// the sidebar's own overflow:auto the way an absolutely-positioned
// in-place popover was — a real reported bug, not a guess: the sidebar's
// scroll container clipped anything positioned past its own edge.
export function computePosition(targetRect: DOMRect): Position {
  const fitsToTheRight = targetRect.right + VIEWPORT_MARGIN_PX + POPOVER_WIDTH_PX <= window.innerWidth;
  const left = fitsToTheRight
    ? targetRect.right + VIEWPORT_MARGIN_PX
    : Math.max(VIEWPORT_MARGIN_PX, targetRect.left);
  const top = fitsToTheRight
    ? Math.min(targetRect.top, window.innerHeight - VIEWPORT_MARGIN_PX)
    : targetRect.bottom + VIEWPORT_MARGIN_PX;
  return { left, top };
}

// Wraps a sidebar worktree row: on hover (after SHOW_DELAY_MS), shows the
// worktree's full name (the row itself truncates it) plus its git/PR
// summary — the row itself has no room for that detail. Summary
// fetching/caching is useWorktreeSummary's job (shared with FileTree.tsx's
// header) — this component only cares about when to fetch (once visible)
// and where to render.
//
// Rendered via a portal into document.body (not inline where the hover
// target lives) precisely so it isn't clipped by the sidebar's own
// overflow:auto scroll container — an absolutely-positioned child can't
// escape an ancestor that clips overflow, no matter how it's positioned.
export default function WorktreeHoverPopover({ wt, children }: Props) {
  const [visible, setVisible] = useState(false);
  const [position, setPosition] = useState<Position | null>(null);
  const showTimerRef = useRef<number | undefined>(undefined);
  const hideTimerRef = useRef<number | undefined>(undefined);
  const targetRef = useRef<HTMLDivElement>(null);

  const { summary, error } = useWorktreeSummary(wt.repo_id, wt.id, visible);
  // Defaults to [] rather than reading summary.changed_files directly —
  // see the comment further down where it's used.
  const changedFiles = summary?.changed_files ?? [];

  useEffect(() => {
    return () => {
      if (showTimerRef.current) window.clearTimeout(showTimerRef.current);
      if (hideTimerRef.current) window.clearTimeout(hideTimerRef.current);
    };
  }, []);

  // Shared by both the row and the popover itself (see their own
  // onMouseEnter below) — entering either cancels any pending hide, which
  // is what lets the mouse cross the gap between them without the popover
  // disappearing first.
  function cancelHide() {
    if (hideTimerRef.current) {
      window.clearTimeout(hideTimerRef.current);
      hideTimerRef.current = undefined;
    }
  }

  function scheduleHide() {
    hideTimerRef.current = window.setTimeout(() => setVisible(false), HIDE_DELAY_MS);
  }

  function handleTargetMouseEnter() {
    cancelHide();
    showTimerRef.current = window.setTimeout(() => {
      if (targetRef.current) {
        setPosition(computePosition(targetRef.current.getBoundingClientRect()));
      }
      setVisible(true);
    }, SHOW_DELAY_MS);
  }

  function handleMouseLeave() {
    if (showTimerRef.current) {
      window.clearTimeout(showTimerRef.current);
      showTimerRef.current = undefined;
    }
    scheduleHide();
  }

  return (
    <div
      className="worktree-hover-target"
      ref={targetRef}
      onMouseEnter={handleTargetMouseEnter}
      onMouseLeave={handleMouseLeave}
    >
      {children}
      {visible &&
        position &&
        createPortal(
          <div
            className="worktree-hover-popover"
            role="tooltip"
            style={{ left: position.left, top: position.top }}
            onMouseEnter={cancelHide}
            onMouseLeave={scheduleHide}
          >
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
                {/* changedFiles defaults to [] rather than reading
                    summary.changed_files directly — a real reported crash
                    was a nil Go slice serializing to JSON `null` (fixed
                    server-side too, but this is what an API response not
                    matching its own declared type should cost: nothing,
                    not a blank sidebar). */}
                <div className="worktree-hover-popover-git">
                  {summary.has_upstream && (summary.ahead > 0 || summary.behind > 0) && (
                    <span className="sidebar-ticks">
                      {summary.ahead > 0 && `↑${summary.ahead} `}
                      {summary.behind > 0 && `↓${summary.behind}`}
                    </span>
                  )}
                  <span className={summary.dirty ? "badge badge-dirty" : "badge badge-clean"}>
                    {summary.dirty ? `${changedFiles.length} changed file(s)` : "clean"}
                  </span>
                </div>
                {changedFiles.length > 0 && (
                  <ul className="worktree-hover-popover-files">
                    {changedFiles.slice(0, 8).map((f) => (
                      <li key={f}>
                        <code>{f}</code>
                      </li>
                    ))}
                    {changedFiles.length > 8 && <li className="muted">+{changedFiles.length - 8} more</li>}
                  </ul>
                )}
              </>
            )}
          </div>,
          document.body
        )}
    </div>
  );
}
