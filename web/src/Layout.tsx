import { useCallback, useEffect, useRef, useState } from "react";
import { Outlet, useNavigate } from "react-router-dom";
import AddRepoDialog from "./AddRepoDialog";
import AttachWorktreeDialog from "./AttachWorktreeDialog";
import { createWorktree, importWorktree, Repo } from "./api";
import CommandPalette from "./CommandPalette";
import NewWorktreeDialog from "./NewWorktreeDialog";
import { RepoProvider, useRepoContext } from "./RepoContext";
import Sidebar from "./Sidebar";
import { hasSafeModifier } from "./keyboard";
import {
  clampSidebarWidth,
  getSidebarHidden,
  getSidebarWidth,
  setSidebarHidden,
  setSidebarWidth,
  SIDEBAR_DEFAULT_WIDTH,
  SIDEBAR_MAX_WIDTH,
  SIDEBAR_MIN_WIDTH,
} from "./sidebarPreferences";

// The persistent app shell: a left sidebar (worktree list, auto-loading per
// selected repo), the routed page content, and a command palette — all
// mounted once above the three routes so switching between them never
// remounts the sidebar or re-triggers its data fetching. This app is an
// SPA in the literal sense, not a set of independently-loading pages.
export default function Layout() {
  return (
    <RepoProvider>
      <LayoutShell />
    </RepoProvider>
  );
}

// Split out from Layout so it can call useRepoContext() — RepoProvider has
// to be an ancestor, not a sibling, of anything that reads the context.
function LayoutShell() {
  const { repos, refreshRepos, refreshWorktrees } = useRepoContext();
  const navigate = useNavigate();

  // Add-repo/new-worktree/attach-worktree dialogs are triggered from both
  // the sidebar and the command palette — the state lives here, one level
  // up, so there's exactly one dialog instance either surface can open,
  // not two independent copies.
  const [addRepoOpen, setAddRepoOpen] = useState(false);
  const [newWorktreeRepoId, setNewWorktreeRepoId] = useState<string | null>(null);
  const [attachWorktreeRepoId, setAttachWorktreeRepoId] = useState<string | null>(null);

  // Cmd/Ctrl+B hides the sidebar outright, for when the terminal is the
  // only thing you want on screen. hasSafeModifier, not a plain
  // metaKey||ctrlKey check: Ctrl+B is tmux's default prefix and every
  // terminal here is a tmux session, so Ctrl only counts when the
  // keystroke isn't headed into one (see keyboard.ts).
  const [sidebarHidden, setHidden] = useState(getSidebarHidden);
  const [sidebarWidth, setWidth] = useState(getSidebarWidth);
  const draggingRef = useRef(false);

  // Drag-to-resize. Pointer events (not mouse) so a trackpad, a touch
  // screen and a pen all work from one path, and setPointerCapture so the
  // drag keeps tracking when the pointer crosses into the terminal iframe
  // territory or leaves the window entirely — without capture, releasing
  // outside the handle leaves the sidebar stuck in drag state.
  //
  // Width is committed to storage on release, not on every move: this
  // fires at pointer frequency, and localStorage writes are synchronous.
  const handleResizeStart = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    draggingRef.current = true;
    e.currentTarget.setPointerCapture(e.pointerId);
    document.body.classList.add("resizing-sidebar");
  }, []);

  const handleResizeMove = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    if (!draggingRef.current) return;
    setWidth(clampSidebarWidth(e.clientX));
  }, []);

  const handleResizeEnd = useCallback(
    (e: React.PointerEvent<HTMLDivElement>) => {
      if (!draggingRef.current) return;
      draggingRef.current = false;
      e.currentTarget.releasePointerCapture(e.pointerId);
      document.body.classList.remove("resizing-sidebar");
      setSidebarWidth(sidebarWidth);
    },
    [sidebarWidth]
  );

  function nudgeWidth(delta: number) {
    setWidth((prev) => {
      const next = clampSidebarWidth(prev + delta);
      setSidebarWidth(next);
      return next;
    });
  }

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key.toLowerCase() !== "b" || !hasSafeModifier(e)) return;
      e.preventDefault();
      setHidden((prev) => {
        setSidebarHidden(!prev);
        return !prev;
      });
    }
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, []);

  function handleRepoCreated(repo: Repo) {
    refreshRepos();
    navigate(`/repo/${repo.id}`);
  }

  async function handleWorktreeCreate(repoId: string, name: string, sourceBranch: string) {
    const wt = await createWorktree(repoId, name, sourceBranch);
    refreshWorktrees();
    navigate(`/repo/${repoId}/worktree/${wt.id}`);
  }

  async function handleWorktreeAttach(repoId: string, path: string, name: string) {
    const wt = await importWorktree(repoId, path, name || undefined);
    refreshWorktrees();
    navigate(`/repo/${repoId}/worktree/${wt.id}`);
  }

  const attachWorktreeRepo = repos.find((r) => r.id === attachWorktreeRepoId);

  return (
    <div
      className={sidebarHidden ? "app-shell sidebar-hidden" : "app-shell"}
      style={{ ["--sidebar-w" as string]: `${sidebarWidth}px` }}
    >
      <Sidebar onAddRepo={() => setAddRepoOpen(true)} onNewWorktree={setNewWorktreeRepoId} />
      {/* A real separator, not decoration: arrow keys resize it and Home
          resets, so this isn't a pointer-only affordance. Double-click
          also resets, which is the convention people try first. */}
      {!sidebarHidden && (
        <div
          className="sidebar-resizer"
          role="separator"
          aria-orientation="vertical"
          aria-label="Resize sidebar"
          aria-valuenow={sidebarWidth}
          aria-valuemin={SIDEBAR_MIN_WIDTH}
          aria-valuemax={SIDEBAR_MAX_WIDTH}
          tabIndex={0}
          onPointerDown={handleResizeStart}
          onPointerMove={handleResizeMove}
          onPointerUp={handleResizeEnd}
          onPointerCancel={handleResizeEnd}
          onDoubleClick={() => {
            setWidth(SIDEBAR_DEFAULT_WIDTH);
            setSidebarWidth(SIDEBAR_DEFAULT_WIDTH);
          }}
          onKeyDown={(e) => {
            if (e.key === "ArrowLeft") nudgeWidth(-16);
            else if (e.key === "ArrowRight") nudgeWidth(16);
            else if (e.key === "Home") nudgeWidth(SIDEBAR_DEFAULT_WIDTH - sidebarWidth);
            else return;
            e.preventDefault();
          }}
        />
      )}
      {/* The only way back once the sidebar is hidden other than the
          keyboard — deliberately a hairline-width hit target pinned to the
          screen edge, so it costs nothing visually until you go looking
          for it. */}
      {sidebarHidden && (
        <button
          type="button"
          className="sidebar-reveal"
          aria-label="Show sidebar"
          title="Show sidebar (⌘B)"
          onClick={() => {
            setHidden(false);
            setSidebarHidden(false);
          }}
        />
      )}
      <main className="app-main">
        <Outlet />
      </main>

      <CommandPalette
        onAddRepo={() => setAddRepoOpen(true)}
        onNewWorktree={setNewWorktreeRepoId}
        onAttachWorktree={setAttachWorktreeRepoId}
      />

      {addRepoOpen && (
        <AddRepoDialog onCreated={handleRepoCreated} onClose={() => setAddRepoOpen(false)} />
      )}
      {newWorktreeRepoId && (
        <NewWorktreeDialog
          repoId={newWorktreeRepoId}
          onCreate={(name, sourceBranch) => handleWorktreeCreate(newWorktreeRepoId, name, sourceBranch)}
          onClose={() => setNewWorktreeRepoId(null)}
        />
      )}
      {attachWorktreeRepoId && attachWorktreeRepo && (
        <AttachWorktreeDialog
          repoName={attachWorktreeRepo.name}
          onAttach={(path, name) => handleWorktreeAttach(attachWorktreeRepoId, path, name)}
          onClose={() => setAttachWorktreeRepoId(null)}
        />
      )}
    </div>
  );
}
