import { createContext, useContext, useEffect, useRef, useState } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import {
  DockviewApi,
  DockviewDefaultTab,
  DockviewReact,
  DockviewReadyEvent,
  IDockviewPanelHeaderProps,
  IDockviewPanelProps,
} from "dockview-react";
import "dockview-react/dist/styles/dockview.css";
import {
  createTerminal,
  deleteTerminal,
  getDependencyStatus,
  getWorktreeLayout,
  listTerminals,
  openInVSCode,
  saveWorktreeLayout,
  TerminalSession,
} from "./api";
import Terminal from "./Terminal";
import WorktreeAuditLog from "./WorktreeAuditLog";
import FileTree from "./FileTree";
import EditorPanel, { EditorPanelParams } from "./EditorPanel";
import { useRepoContext } from "./RepoContext";
import VSCodeIcon from "./icons/VSCodeIcon";
import ClaudeIcon from "./icons/ClaudeIcon";
import { SplitHorizontalIcon, SplitVerticalIcon } from "./icons/SplitIcons";
import { registerActiveFileOpener } from "./activeWorktreeFileOpener";
import { detectTerminalApp, TerminalAppKind } from "./terminalAppDetection";

interface TerminalPanelParams {
  terminalId: string;
  // What the tab reverts to once the pane's title no longer matches a
  // known app — the tab_label this terminal was created with (see
  // createTerminal/handleNewTerminal), not something recomputed later.
  baseLabel: string;
  // Set once a known interactive app is detected running in this pane
  // (see terminalAppDetection.ts) — drives which icon TerminalTab shows.
  // Undefined until/unless a matching title arrives, and cleared again if
  // the title stops matching (e.g. the app exits back to a plain shell).
  appKind?: TerminalAppKind;
}

// Icon shown in a terminal tab once its pane is known to be running that
// app (see terminalAppDetection.ts) — kept separate from that module so
// terminalAppDetection.ts itself stays framework-free. New entries here
// are how this grows to cover more "persisting" apps beyond claude later.
const TERMINAL_APP_ICONS: Record<TerminalAppKind, (props: { size?: number }) => JSX.Element> = {
  claude: ClaudeIcon,
};

// A thin wrapper so dockview can host the existing Terminal component as a
// panel. Terminal.tsx itself needs no changes — dockview panels are just
// React components in the tree; adding/splitting/resizing panels doesn't
// touch this component's mount state, so the xterm/websocket lifecycle in
// Terminal.tsx's own useEffect is unaffected.
//
// Also wires Terminal's onTitleChange to this panel's own dockview tab:
// detects a known app from the pane's title and, if it changed, updates
// both the tab's label (api.setTitle) and its params (api.updateParameters)
// so TerminalTab below knows which icon to render. Reverts to baseLabel
// with no icon once the title stops matching anything known (e.g. the app
// exited back to a plain shell).
function TerminalPanel(props: IDockviewPanelProps<TerminalPanelParams>) {
  function handleTitleChange(title: string) {
    const app = detectTerminalApp(title);
    if (props.params.appKind !== app?.kind) {
      props.api.updateParameters({ ...props.params, appKind: app?.kind });
    }
    props.api.setTitle(app ? app.label : props.params.baseLabel);
  }

  return <Terminal terminalId={props.params.terminalId} onTitleChange={handleTitleChange} />;
}

// Renders the same look as dockview's own default tab, plus a leading
// app icon when TerminalPanel above has detected one. Delegating to
// DockviewDefaultTab (rather than reimplementing the tab's close button
// etc. from scratch) is what keeps this in sync with dockview's own
// look/behavior for free.
function TerminalTab(props: IDockviewPanelHeaderProps<TerminalPanelParams>) {
  const Icon = props.params.appKind ? TERMINAL_APP_ICONS[props.params.appKind] : null;
  return (
    <div className="terminal-tab-with-icon">
      {Icon && <Icon size={13} />}
      <DockviewDefaultTab {...props} />
    </div>
  );
}

const tabComponents = { "terminal-tab": TerminalTab };

const components = { terminal: TerminalPanel, editor: EditorPanel };

// dockview's watermarkComponent (like `components` above) is read once at
// construction, not re-passed fresh on every render — so it can't close
// over WorktreeDetailInner's own handleNewTerminal directly the way an
// inline function would, the same reason `components` is a stable
// module-level object instead of built fresh per render. This tiny
// context is the bridge: dockview-react renders panels/watermark via a
// shared "ReactPortalStore" that's still part of the normal React tree
// (that's what lets Terminal.tsx/EditorPanel.tsx work as plain components
// at all), so context set up in WorktreeDetailInner's render still reaches
// this module-scope Watermark component through the portal.
const WatermarkActionsContext = createContext<{
  onOpenShell: () => void;
  onOpenClaude: () => void;
} | null>(null);

function Watermark() {
  const actions = useContext(WatermarkActionsContext);
  return (
    <div className="dockview-watermark">
      <p>Nothing open in this worktree yet.</p>
      {actions && (
        <div className="dockview-watermark-actions">
          <button type="button" onClick={actions.onOpenShell}>
            Open shell
          </button>
          <button type="button" onClick={actions.onOpenClaude}>
            Open claude
          </button>
        </div>
      )}
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
  const { worktreesByRepo } = useRepoContext();
  const worktree = worktreesByRepo[repoId]?.find((w) => w.id === worktreeId);
  const [terminals, setTerminals] = useState<TerminalSession[]>([]);
  // Distinguishes "haven't fetched terminals yet" from "fetched, there are
  // none" — the initial-layout effect below must not run until this is
  // true, or it can seed panels from a still-empty `terminals` and then
  // never get another chance (see that effect's own doc comment for why).
  const [terminalsLoaded, setTerminalsLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [searchParams] = useSearchParams();
  const deepLinkTerminalId = searchParams.get("terminal");
  const deepLinkAppliedRef = useRef(false);
  const [logOpen, setLogOpen] = useState(false);
  const [filesOpen, setFilesOpen] = useState(true);
  const [vscodeAvailable, setVscodeAvailable] = useState(false);
  const [vscodeError, setVscodeError] = useState<string | null>(null);
  const [activeFilePath, setActiveFilePath] = useState<string | null>(null);
  // State (not a ref) deliberately: the effect that applies the initial
  // saved layout needs to re-run once this becomes available, and refs
  // don't trigger effect re-runs when mutated.
  const [dockviewApi, setDockviewApi] = useState<DockviewApi | null>(null);
  // undefined = not fetched yet, null = fetched, nothing saved.
  const [savedLayout, setSavedLayout] = useState<unknown | null | undefined>(undefined);
  const initialLayoutAppliedRef = useRef(false);
  const saveDebounceRef = useRef<number | undefined>(undefined);
  // onDockviewReady below subscribes handlePanelClosed to dockview's
  // onDidRemovePanel exactly once (dockview-react only calls onReady
  // once per instance) — a plain read of the `terminals` state variable
  // inside that closure would be frozen at whatever `terminals` was
  // during that one render (likely still `[]`, since the terminals fetch
  // is async and hasn't resolved yet at that point), silently breaking
  // every future terminal-close call. This ref is kept current via the
  // effect below and is what handlePanelClosed actually reads.
  const terminalsRef = useRef<TerminalSession[]>([]);

  useEffect(() => {
    listTerminals(repoId, worktreeId)
      .then(setTerminals)
      .catch((err) => setError(err.message))
      .finally(() => setTerminalsLoaded(true));
    // repoId/worktreeId never change within this component's lifetime —
    // the outer WorktreeDetail remounts a fresh instance (new `key`) per
    // worktree instead. Empty deps: this is mount-only, deliberately.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    terminalsRef.current = terminals;
  }, [terminals]);

  // Fetch the saved layout (if any) for this worktree — mount-only, same
  // reasoning as above.
  useEffect(() => {
    getWorktreeLayout(repoId, worktreeId)
      .then(setSavedLayout)
      .catch(() => setSavedLayout(null));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Gates the "Open in VS Code" button — `code` requires a one-time manual
  // install step even when VS Code itself is installed, so this can be
  // false on an otherwise normal machine. See settings.go's checkOnPath
  // for "code" and SettingsModal.tsx's Installation tab, which surfaces
  // the same status with an install hint.
  useEffect(() => {
    getDependencyStatus()
      .then((status) => setVscodeAvailable(status.vscode_cli?.installed ?? false))
      .catch(() => setVscodeAvailable(false));
  }, []);

  async function handleOpenInVSCode() {
    setVscodeError(null);
    try {
      await openInVSCode(repoId, worktreeId);
    } catch (err) {
      // A real error, not a silent no-op — this check passing doesn't
      // guarantee the exec call succeeds (e.g. `code` uninstalled between
      // the check above and this click).
      setVscodeError((err as Error).message);
    }
  }

  // Seed dockview with a panel per known terminal that doesn't already
  // have one — used both as the fallback when there's no saved layout,
  // and to pick up any terminal created after the layout was last saved.
  function seedPanels(api: DockviewApi, sessions: TerminalSession[]) {
    for (const ts of sessions) {
      if (!api.getPanel(ts.id)) {
        api.addPanel<TerminalPanelParams>({
          id: ts.id,
          component: "terminal",
          tabComponent: "terminal-tab",
          title: ts.tab_label,
          params: { terminalId: ts.id, baseLabel: ts.tab_label },
        });
      }
    }
  }

  // Apply the initial layout exactly once dockview is ready AND both the
  // saved-layout and terminals fetches have resolved (order between any of
  // these is not guaranteed — this effect is what makes it not matter).
  // Gating on terminalsLoaded (not just a truthy `terminals`) matters: a
  // worktree with terminals but no saved layout would otherwise let this
  // fire while `terminals` was still its initial `[]`, seed nothing, and —
  // because of the ref guard below — never get a second chance once the
  // fetch actually resolved. Found for real: a fresh worktree page showed
  // the "nothing open" watermark despite the worktree having live terminal
  // sessions, reproducing on every cold load, not just occasionally.
  //
  // If a saved layout references a terminal id that no longer exists
  // server-side (e.g. deleted through some path that didn't get a chance
  // to save an updated layout first), that panel is left to dockview/
  // Terminal.tsx's own graceful failure (a pane showing a connection
  // error) rather than hand-pruning dockview's serialized grid structure —
  // a deliberate simplification, not an oversight; see docs/architecture.md.
  useEffect(() => {
    if (!dockviewApi || savedLayout === undefined || !terminalsLoaded || initialLayoutAppliedRef.current) return;
    initialLayoutAppliedRef.current = true;
    if (savedLayout) {
      try {
        dockviewApi.fromJSON(savedLayout as Parameters<DockviewApi["fromJSON"]>[0]);
      } catch (err) {
        console.error("failed to restore saved terminal layout, falling back to default", err);
      }
    }
    seedPanels(dockviewApi, terminals);
  }, [dockviewApi, savedLayout, terminalsLoaded, terminals]);

  // Deep link support: a settings-page "Shells" row links to
  // /repo/:repoId/worktree/:worktreeId?terminal=<id>, and landing here
  // should focus that terminal's panel — once, after the initial
  // layout/seed above has actually created it (a fresh navigation to an
  // already-mounted instance doesn't re-run this: see the `key` remount
  // comment above WorktreeDetail).
  useEffect(() => {
    if (!dockviewApi || !deepLinkTerminalId || deepLinkAppliedRef.current) return;
    const panel = dockviewApi.getPanel(deepLinkTerminalId);
    if (!panel) return;
    deepLinkAppliedRef.current = true;
    panel.api.setActive();
  }, [dockviewApi, deepLinkTerminalId, terminals]);

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
    // Keeps the file tree's highlighted/scrolled-to node in sync with
    // whichever editor panel is currently active — setActiveFilePath is a
    // state setter, so this closure never goes stale the way a plain
    // variable read would (see handlePanelClosed's terminalsRef comment
    // above for the general version of that pitfall).
    event.api.onDidActivePanelChange((e) => {
      const id = e.panel?.id;
      setActiveFilePath(id?.startsWith("editor:") ? id.slice("editor:".length) : null);
    });
  }

  async function handlePanelClosed(panelId: string) {
    // dockview's onDidRemovePanel fires for every panel kind (terminals
    // AND editor panels), but only terminals have a server-side session to
    // tear down. Without this check, closing an editor panel (id
    // "editor:<path>") fell through to deleteTerminal, which 404'd
    // ("terminal session not found") and surfaced that as a top-level
    // error banner — a real bug, found from a user report — for an action
    // (closing a file) that has nothing to clean up server-side.
    if (!terminalsRef.current.some((t) => t.id === panelId)) return;

    // The panel is already gone from dockview's own layout by the time
    // this fires (that's what triggered the event) — this just tells the
    // server to actually kill the tmux session and drops our own
    // bookkeeping copy of the list.
    try {
      await deleteTerminal(repoId, worktreeId, panelId);
    } catch (err) {
      setError((err as Error).message);
    }
    setTerminals((prev) => prev.filter((t) => t.id !== panelId));
  }

  async function handleNewTerminal(direction: PlacementDirection, tabLabel?: string, initialCommand?: string) {
    try {
      const ts = await createTerminal(repoId, worktreeId, tabLabel, initialCommand);
      setTerminals((prev) => [...prev, ts]);

      if (!dockviewApi) return; // the seed effect will pick it up once ready
      const reference = dockviewApi.activePanel;
      dockviewApi.addPanel<TerminalPanelParams>({
        id: ts.id,
        component: "terminal",
        tabComponent: "terminal-tab",
        title: ts.tab_label,
        params: { terminalId: ts.id, baseLabel: ts.tab_label },
        position: reference ? { referencePanel: reference.id, direction } : undefined,
      });
    } catch (err) {
      setError((err as Error).message);
    }
  }

  // Opens a file's editor panel, reusing an already-open panel for the same
  // path instead of creating a second one — see EditorPanel.tsx's doc
  // comment for why this is what makes the "two panels on the same file"
  // question moot rather than something needing a shared-model design.
  function handleOpenFile(path: string) {
    if (!dockviewApi) return;
    const id = `editor:${path}`;
    const existing = dockviewApi.getPanel(id);
    if (existing) {
      existing.api.setActive();
      return;
    }
    const reference = dockviewApi.activePanel;
    dockviewApi.addPanel<EditorPanelParams>({
      id,
      component: "editor",
      title: path.split("/").pop() ?? path,
      params: { repoId, worktreeId, path },
      position: reference ? { referencePanel: reference.id, direction: "within" } : undefined,
    });
  }

  // Registers this worktree as the one the command palette's file search
  // opens into — see activeWorktreeFileOpener.ts for why this needs to be
  // a plain module-level registration rather than React context.
  useEffect(() => {
    registerActiveFileOpener(handleOpenFile);
    return () => registerActiveFileOpener(null);
  });

  return (
    <div className="container worktree-detail">
      <div className="terminal-toolbar">
        <div className="terminal-toolbar-left">
          <button
            title="Toggle the file tree sidebar"
            onClick={() => setFilesOpen((o) => !o)}
            aria-pressed={filesOpen}
          >
            📁 Files
          </button>
          <button
            title={
              vscodeAvailable
                ? "Open this worktree in VS Code"
                : "VS Code CLI ('code') not detected — see Settings > Installation"
            }
            disabled={!vscodeAvailable}
            onClick={handleOpenInVSCode}
            className="button-with-icon"
          >
            <VSCodeIcon /> VS Code
          </button>
          <button title="View this worktree's audit log" onClick={() => setLogOpen(true)}>
            🕐 Log
          </button>
        </div>
        <div className="terminal-toolbar-actions">
          <button
            aria-label="Open this worktree in a new browser tab"
            title="Open this worktree in a new browser tab"
            onClick={() => window.open(window.location.href, "_blank")}
          >
            ⧉
          </button>
          <button
            aria-label="New terminal tab"
            title="New terminal tab"
            onClick={() => handleNewTerminal("within")}
          >
            +
          </button>
          <button
            aria-label="Split right (new terminal)"
            title="Split right (new terminal)"
            className="button-with-icon"
            onClick={() => handleNewTerminal("right")}
          >
            <SplitVerticalIcon />
          </button>
          <button
            aria-label="Split down (new terminal)"
            title="Split down (new terminal)"
            className="button-with-icon"
            onClick={() => handleNewTerminal("below")}
          >
            <SplitHorizontalIcon />
          </button>
        </div>
      </div>
      {error && <p className="error">{error}</p>}
      {vscodeError && <p className="error">Failed to open VS Code: {vscodeError}</p>}

      <div className="worktree-body">
        {filesOpen && (
          <div className="worktree-sidebar">
            <FileTree
              repoId={repoId}
              worktreeId={worktreeId}
              folderName={worktree?.name ?? worktreeId}
              onOpenFile={handleOpenFile}
              activePath={activeFilePath}
            />
          </div>
        )}
        <div className="terminal-area">
          <WatermarkActionsContext.Provider
            value={{
              onOpenShell: () => handleNewTerminal("within"),
              onOpenClaude: () => handleNewTerminal("within", "claude", "claude"),
            }}
          >
            <DockviewReact
              components={components}
              tabComponents={tabComponents}
              watermarkComponent={Watermark}
              onReady={onDockviewReady}
              className="dockview-theme-abyss command-deck-dockview"
            />
          </WatermarkActionsContext.Provider>
        </div>
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
