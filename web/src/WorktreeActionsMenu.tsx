import { useEffect, useRef, useState } from "react";
import { SpotlightStatus, Worktree } from "./api";

interface Props {
  wt: Worktree;
  spotlightStatus: SpotlightStatus | undefined;
  onOpen?: () => void; // optional — the sidebar's row is already a nav link, so it omits this
  onSpotlightStart: () => void;
  onSpotlightStop: () => void;
  onViewLog: () => void;
  onArchive: () => void;
}

// A per-worktree kebab menu — the single place every operational action on
// a worktree lives (spotlight start/stop, delete, optionally open), rather
// than a row of separate always-visible buttons.
export default function WorktreeActionsMenu({
  wt,
  spotlightStatus,
  onOpen,
  onSpotlightStart,
  onSpotlightStop,
  onViewLog,
  onArchive,
}: Props) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    function onDocMouseDown(e: MouseEvent) {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", onDocMouseDown);
    return () => document.removeEventListener("mousedown", onDocMouseDown);
  }, [open]);

  function act(fn: () => void) {
    setOpen(false);
    fn();
  }

  return (
    <div
      className="actions-menu"
      ref={rootRef}
      // The sidebar renders this inside a <Link> for the row itself.
      // stopPropagation alone is NOT enough: react-router's <Link> does
      // its client-side-navigation preventDefault() inside ITS OWN onClick
      // handler on the <a> — stopping propagation here means that handler
      // never runs at all, so the browser falls through to the anchor's
      // native default action (a real full-page navigation to href, i.e.
      // exactly the "clicking the menu reloads the page" bug). Calling
      // preventDefault() ourselves, from anywhere in the bubble path,
      // suppresses that default action regardless of whether the Link's
      // own handler ever ran.
      onClick={(e) => {
        e.preventDefault();
        e.stopPropagation();
      }}
    >
      <button
        type="button"
        className="actions-menu-trigger"
        aria-label={`Actions for ${wt.branch}`}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
      >
        ⋮
      </button>
      {open && (
        <div className="actions-menu-list" role="menu">
          {onOpen && (
            <button type="button" role="menuitem" onClick={() => act(onOpen)}>
              Open
            </button>
          )}
          {spotlightStatus?.available &&
            (spotlightStatus.active ? (
              <button type="button" role="menuitem" onClick={() => act(onSpotlightStop)}>
                Stop spotlight
              </button>
            ) : (
              <button type="button" role="menuitem" onClick={() => act(onSpotlightStart)}>
                {spotlightStatus.active_worktree_path
                  ? "Start spotlight (replaces active mirror)"
                  : "Start spotlight"}
              </button>
            ))}
          <button type="button" role="menuitem" onClick={() => act(onViewLog)}>
            View worktree log
          </button>
          <button type="button" role="menuitem" onClick={() => act(onArchive)}>
            Archive
          </button>
        </div>
      )}
    </div>
  );
}
