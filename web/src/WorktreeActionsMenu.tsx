import { useEffect, useRef, useState } from "react";
import { SpotlightStatus, Worktree } from "./api";

interface Props {
  wt: Worktree;
  spotlightStatus: SpotlightStatus | undefined;
  onOpen?: () => void; // optional — the sidebar's row is already a nav link, so it omits this
  onSpotlightStart: () => void;
  onSpotlightStop: () => void;
  onDelete: () => void;
}

// A per-worktree kebab menu — the single place every operational action on
// a worktree lives (spotlight start/stop, delete, optionally open), rather
// than a row of separate always-visible buttons. Used by both Sidebar.tsx
// and WorktreeList.tsx so the two surfaces share one menu implementation.
export default function WorktreeActionsMenu({
  wt,
  spotlightStatus,
  onOpen,
  onSpotlightStart,
  onSpotlightStop,
  onDelete,
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
      // The sidebar renders this inside a <Link> for the row itself —
      // stop clicks here from bubbling up into that link's navigation.
      onClick={(e) => e.stopPropagation()}
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
          <button
            type="button"
            role="menuitem"
            className="danger"
            onClick={() => act(onDelete)}
          >
            Delete
          </button>
        </div>
      )}
    </div>
  );
}
