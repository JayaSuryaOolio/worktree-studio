import { useEffect, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { DockviewApi, DockviewReact, DockviewReadyEvent, IDockviewPanelProps } from "dockview-react";
import "dockview-react/dist/styles/dockview.css";
import { createTerminal, deleteTerminal, listTerminals, TerminalSession } from "./api";
import Terminal from "./Terminal";

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

export default function WorktreeDetail() {
  const { repoId, worktreeId } = useParams<{
    repoId: string;
    worktreeId: string;
  }>();
  const [terminals, setTerminals] = useState<TerminalSession[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [menuOpen, setMenuOpen] = useState(false);
  const dockviewApiRef = useRef<DockviewApi | null>(null);

  function refresh() {
    if (!repoId || !worktreeId) return;
    listTerminals(repoId, worktreeId)
      .then(setTerminals)
      .catch((err) => setError(err.message));
  }

  useEffect(refresh, [repoId, worktreeId]);

  // Seed dockview with a panel per known terminal — runs whenever the
  // terminal list changes (including the initial load) or dockview
  // becomes ready, whichever happens second. Only adds panels that don't
  // already exist, so this is safe to re-run on every terminals update.
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

  useEffect(() => {
    const api = dockviewApiRef.current;
    if (!api) return;
    seedPanels(api, terminals);
  }, [terminals]);

  function onDockviewReady(event: DockviewReadyEvent) {
    dockviewApiRef.current = event.api;
    seedPanels(event.api, terminals);
    event.api.onDidRemovePanel((panel) => {
      handlePanelClosed(panel.id);
    });
  }

  async function handlePanelClosed(terminalId: string) {
    if (!repoId || !worktreeId) return;
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
    if (!repoId || !worktreeId) return;
    setMenuOpen(false);
    try {
      const ts = await createTerminal(repoId, worktreeId);
      setTerminals((prev) => [...prev, ts]);

      const api = dockviewApiRef.current;
      if (!api) return; // seedPanels' effect will pick it up once ready
      const reference = api.activePanel;
      api.addPanel<TerminalPanelParams>({
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

  if (!repoId || !worktreeId) return null;

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
    </div>
  );
}
