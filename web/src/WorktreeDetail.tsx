import { useEffect, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { DockviewApi, DockviewReact, DockviewReadyEvent, IDockviewPanelProps } from "dockview-react";
import "dockview-react/dist/styles/dockview.css";
import {
  createTerminal,
  deleteTerminal,
  getWorktreeLayout,
  listTerminals,
  saveWorktreeLayout,
  TerminalSession,
} from "./api";
import Terminal from "./Terminal";
import WorktreeAuditLog from "./WorktreeAuditLog";

interface TerminalPanelParams {
  terminalId: string;
}

// A thin wrapper so dockview can host the existing Terminal component as a
// panel. Terminal.tsx itself needs no changes — dockview panels are just
// React components in the tree; adding/splitting/resizing panels doesn't
// touch this component's mount state, so the xterm/websocket lifecycle in
// Terminal.tsx's own useEffect is unaffected.
function TerminalPanel(props: IDockviewPanelProps<TerminalPanelParams>) {
  return <Terminal terminalId={props.params.terminalId} />;
}

const components = { terminal: TerminalPanel };

function Watermark() {
  return (
    <div className="dockview-watermark">
      No terminals yet — click "+ New Terminal" to start a shell in this worktree.
    </div>
  );
}

type PlacementDirection = "within" | "right" | "below";

// Debounce layout saves — dockview's onDidLayoutChange fires on every
// drag/resize/tab-switch, and a save on every single event would hammer
// the DB for no benefit (the layout is a single opaque JSON blob per
// worktree, upserted wholesale, not something worth writing 30 times a
// second while someone drags a sash).
const LAYOUT_SAVE_DEBOUNCE_MS = 500;

// react-router reuses the same WorktreeDetail component instance across
// navigations between /repo/:repoId/worktree/:w1 and .../w2 (same route
// element — it re-renders with new params rather than unmounting), but
// the *whole point* of the inner component is a long-lived dockview
// instance whose panels persist across renders. Left unguarded, that
// means switching worktrees left every previous worktree's terminal
// panels sitting in the shared dockview grid, and a since-corrupted mix
// of "layouts" from multiple worktrees fighting over the same instance
// (which is also why saved layouts looked like they weren't restoring —
// fromJSON was reconciling against already-populated state, not a clean
// one). Forcing a full remount via `key` on worktreeId is the standard
// fix: a real bug found from a user report, not a hypothetical.
export default function WorktreeDetail() {
  const { repoId, worktreeId } = useParams<{
    repoId: string;
    worktreeId: string;
  }>();
  if (!repoId || !worktreeId) return null;
  return <WorktreeDetailInner key={`${repoId}:${worktreeId}`} repoId={repoId} worktreeId={worktreeId} />;
}

function WorktreeDetailInner({ repoId, worktreeId }: { repoId: string; worktreeId: string }) {
  const [terminals, setTerminals] = useState<TerminalSession[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [menuOpen, setMenuOpen] = useState(false);
  const [logOpen, setLogOpen] = useState(false);
  // State (not a ref) deliberately: the effect that applies the initial
  // saved layout needs to re-run once this becomes available, and refs
  // don't trigger effect re-runs when mutated.
  const [dockviewApi, setDockviewApi] = useState<DockviewApi | null>(null);
  // undefined = not fetched yet, null = fetched, nothing saved.
  const [savedLayout, setSavedLayout] = useState<unknown | null | undefined>(undefined);
  const initialLayoutAppliedRef = useRef(false);
  const saveDebounceRef = useRef<number | undefined>(undefined);

  useEffect(() => {
    listTerminals(repoId, worktreeId)
      .then(setTerminals)
      .catch((err) => setError(err.message));
    // repoId/worktreeId never change within this component's lifetime —
    // the outer WorktreeDetail remounts a fresh instance (new `key`) per
    // worktree instead. Empty deps: this is mount-only, deliberately.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Fetch the saved layout (if any) for this worktree — mount-only, same
  // reasoning as above.
  useEffect(() => {
    getWorktreeLayout(repoId, worktreeId)
      .then(setSavedLayout)
      .catch(() => setSavedLayout(null));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Seed dockview with a panel per known terminal that doesn't already
  // have one — used both as the fallback when there's no saved layout,
  // and to pick up any terminal created after the layout was last saved.
  function seedPanels(api: DockviewApi, sessions: TerminalSession[]) {
    for (const ts of sessions) {
      if (!api.getPanel(ts.id)) {
        api.addPanel<TerminalPanelParams>({
          id: ts.id,
          component: "terminal",
          title: ts.tab_label,
          params: { terminalId: ts.id },
        });
      }
    }
  }

  // Apply the initial layout exactly once dockview is ready AND the
  // saved-layout fetch has resolved (order between those two is not
  // guaranteed — this effect is what makes it not matter). If a saved
  // layout references a terminal id that no longer exists server-side
  // (e.g. deleted through some path that didn't get a chance to save an
  // updated layout first), that panel is left to dockview/Terminal.tsx's
  // own graceful failure (a pane showing a connection error) rather than
  // hand-pruning dockview's serialized grid structure — a deliberate
  // simplification, not an oversight; see docs/architecture.md.
  useEffect(() => {
    if (!dockviewApi || savedLayout === undefined || initialLayoutAppliedRef.current) return;
    initialLayoutAppliedRef.current = true;
    if (savedLayout) {
      try {
        dockviewApi.fromJSON(savedLayout as Parameters<DockviewApi["fromJSON"]>[0]);
      } catch (err) {
        console.error("failed to restore saved terminal layout, falling back to default", err);
      }
    }
    seedPanels(dockviewApi, terminals);
  }, [dockviewApi, savedLayout, terminals]);

  // Debounced layout save on every layout change, once the initial
  // load/seed above has happened (so restoring the saved layout doesn't
  // immediately re-trigger a save of the same thing it just loaded).
  useEffect(() => {
    if (!dockviewApi) return;
    const disposable = dockviewApi.onDidLayoutChange(() => {
      if (!initialLayoutAppliedRef.current) return;
      if (saveDebounceRef.current) window.clearTimeout(saveDebounceRef.current);
      saveDebounceRef.current = window.setTimeout(() => {
        saveWorktreeLayout(repoId, worktreeId, dockviewApi.toJSON()).catch((err) => {
          console.error("failed to save terminal layout", err);
        });
      }, LAYOUT_SAVE_DEBOUNCE_MS);
    });
    return () => disposable.dispose();
  }, [dockviewApi]);

  function onDockviewReady(event: DockviewReadyEvent) {
    setDockviewApi(event.api);
    event.api.onDidRemovePanel((panel) => {
      handlePanelClosed(panel.id);
    });
  }

  async function handlePanelClosed(terminalId: string) {
    // The panel is already gone from dockview's own layout by the time
    // this fires (that's what triggered the event) — this just tells the
    // server to actually kill the tmux session and drops our own
    // bookkeeping copy of the list.
    try {
      await deleteTerminal(repoId, worktreeId, terminalId);
    } catch (err) {
      setError((err as Error).message);
    }
    setTerminals((prev) => prev.filter((t) => t.id !== terminalId));
  }

  async function handleNewTerminal(direction: PlacementDirection) {
    setMenuOpen(false);
    try {
      const ts = await createTerminal(repoId, worktreeId);
      setTerminals((prev) => [...prev, ts]);

      if (!dockviewApi) return; // the seed effect will pick it up once ready
      const reference = dockviewApi.activePanel;
      dockviewApi.addPanel<TerminalPanelParams>({
        id: ts.id,
        component: "terminal",
        title: ts.tab_label,
        params: { terminalId: ts.id },
        position: reference ? { referencePanel: reference.id, direction } : undefined,
      });
    } catch (err) {
      setError((err as Error).message);
    }
  }

  return (
    <div className="container worktree-detail">
      <div className="terminal-toolbar">
        <div className="new-terminal-menu">
          <button type="button" onClick={() => setMenuOpen((o) => !o)}>
            + New Terminal ▾
          </button>
          {menuOpen && (
            <div className="actions-menu-list new-terminal-menu-list">
              <button type="button" onClick={() => handleNewTerminal("within")}>
                New tab
              </button>
              <button type="button" onClick={() => handleNewTerminal("right")}>
                Split right
              </button>
              <button type="button" onClick={() => handleNewTerminal("below")}>
                Split down
              </button>
            </div>
          )}
        </div>
        <div className="terminal-toolbar-actions">
          <button title="View this worktree's audit log" onClick={() => setLogOpen(true)}>
            🕐 Log
          </button>
          <button
            title="Open this worktree in a new browser tab"
            onClick={() => window.open(window.location.href, "_blank")}
          >
            ⧉ New tab
          </button>
        </div>
      </div>
      {error && <p className="error">{error}</p>}

      <div className="terminal-area">
        <DockviewReact
          components={components}
          watermarkComponent={Watermark}
          onReady={onDockviewReady}
          className="dockview-theme-abyss command-deck-dockview"
        />
      </div>

      {logOpen && (
        <WorktreeAuditLog
          repoId={repoId}
          worktreeId={worktreeId}
          title="this worktree"
          onClose={() => setLogOpen(false)}
        />
      )}
    </div>
  );
}
