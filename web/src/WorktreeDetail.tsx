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
  getTerminalCwd,
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
import ClaudeIcon from "./icons/ClaudeIcon";
import { registerActiveFileOpener } from "./activeWorktreeFileOpener";
import { takePendingFileOpen } from "./pendingFileOpen";
import { takePendingNewTerminal } from "./pendingNewTerminal";
import { registerActiveWorktreeActions } from "./activeWorktreeActions";
import { detectTerminalApp, TerminalAppKind } from "./terminalAppDetection";
import { isRootWorktreeId } from "./rootWorktree";
import { getStoredFilesOpen, setStoredFilesOpen } from "./filesPanelPreference";
import { useWorktreeSummary } from "./useWorktreeSummary";

interface TerminalPanelParams {
  terminalId: string;
  repoId: string;
  worktreeId: string;
  // The worktree's own path, for TerminalPanel's one-shot cwd-mismatch
  // check below — undefined only in the brief window before the root
  // worktree's repo has loaded (see WorktreeDetailInner's worktreePath).
  worktreePath: string | undefined;
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
//
// The .terminal-panel-inset wrapper below gives every pane its own inset,
// not just the whole dockview area's outer edge (.terminal-area's padding
// in style.css) — without it, a pane created via split-right/split-down
// sits flush against the internal sash with zero left/top padding, since
// that boundary is an internal dockview split, not the outer container edge.
function TerminalPanel(props: IDockviewPanelProps<TerminalPanelParams>) {
  const { terminalId, repoId, worktreeId, worktreePath } = props.params;
  // A one-shot check (not polled — see getTerminalCwd's own doc comment):
  // null until fetched, then true/false. Left null (no border either way)
  // on a fetch failure or while worktreePath itself isn't known yet,
  // rather than guessing.
  const [cwdMismatch, setCwdMismatch] = useState<boolean | null>(null);

  useEffect(() => {
    if (!worktreePath) return;
    let cancelled = false;
    getTerminalCwd(repoId, worktreeId, terminalId)
      .then(({ cwd }) => {
        if (cancelled) return;
        setCwdMismatch(!(cwd === worktreePath || cwd.startsWith(worktreePath + "/")));
      })
      .catch(() => {
        // Best-effort: a lookup failure just means no border either way,
        // same as this check never having run.
      });
    return () => {
      cancelled = true;
    };
    // terminalId alone identifies this pane for the lifetime of the
    // component; repoId/worktreeId/worktreePath are constant for as long
    // as this panel exists (its parent WorktreeDetailInner instance is
    // itself remounted whole on worktree switch — see WorktreeDetail's own
    // doc comment on why).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [terminalId]);

  function handleTitleChange(title: string) {
    const app = detectTerminalApp(title);
    if (props.params.appKind !== app?.kind) {
      props.api.updateParameters({ ...props.params, appKind: app?.kind });
    }
    // Falls back to "shell" (not an empty string) if this pane's baseLabel
    // is itself blank — e.g. a terminal_sessions row from before the
    // backend started defaulting an empty tab_label to "shell" on create
    // (internal/api/terminals.go), which would otherwise render as a
    // blank tab title forever once the pane's OSC title stops matching a
    // known app.
    props.api.setTitle(app ? app.label : props.params.baseLabel || "shell");
  }

  return (
    <div
      className={cwdMismatch ? "terminal-panel-inset cwd-mismatch" : "terminal-panel-inset"}
      title={cwdMismatch ? "This shell's directory is outside the worktree" : undefined}
    >
      <Terminal terminalId={terminalId} onTitleChange={handleTitleChange} />
    </div>
  );
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
const DockviewActionsContext = createContext<{
  onOpenShell: () => void;
  onOpenClaude: () => void;
} | null>(null);

function Watermark() {
  const actions = useContext(DockviewActionsContext);
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

// A "+" pinned to the far left of the tab strip (dockview's
// prefixHeaderActionsComponent, which mounts into dv-pre-actions-container
// — the first child, before any tabs; leftHeaderActionsComponent despite
// its name renders AFTER the tabs, not before them). This is the one way
// to open another shell once at least one panel already exists — the
// per-worktree "new terminal" icon used to live in this component's own
// toolbar, but that moved to the sidebar's expandable worktree card, which
// the synthetic root worktree (a repo's own checkout) has no row/card for
// at all. Reuses DockviewActionsContext (same bridge Watermark uses above)
// since dockview-react renders this via the same portal-based tree.
function NewTerminalPrefixAction() {
  const actions = useContext(DockviewActionsContext);
  if (!actions) return null;
  return (
    <button
      type="button"
      className="dockview-prefix-new-terminal"
      title="New terminal tab"
      onClick={actions.onOpenShell}
    >
      +
    </button>
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
  const { repos, worktreesByRepo } = useRepoContext();
  const worktree = worktreesByRepo[repoId]?.find((w) => w.id === worktreeId);
  // The synthetic root worktree (see rootWorktree.ts) is never in
  // worktreesByRepo — it's not a real git worktree, so the backend excludes
  // it from the normal per-repo listing; its path is the repo's own path
  // instead of a worktree's. TerminalPanel uses this to flag (a faint red
  // border) a shell whose cwd has drifted outside it.
  const repo = repos.find((r) => r.id === repoId);
  const worktreePath = isRootWorktreeId(worktreeId) ? repo?.path : worktree?.path;
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
  const [filesOpen, setFilesOpen] = useState(getStoredFilesOpen);
  const [vscodeAvailable, setVscodeAvailable] = useState(false);
  const [vscodeError, setVscodeError] = useState<string | null>(null);
  const [branchCopied, setBranchCopied] = useState(false);
  const branchCopiedTimeoutRef = useRef<number | undefined>(undefined);
  const { summary } = useWorktreeSummary(repoId, worktreeId, true);
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

  function copyBranch() {
    const branch = worktree?.branch;
    if (!branch) return;
    navigator.clipboard.writeText(branch).then(() => {
      setBranchCopied(true);
      if (branchCopiedTimeoutRef.current) window.clearTimeout(branchCopiedTimeoutRef.current);
      branchCopiedTimeoutRef.current = window.setTimeout(() => setBranchCopied(false), 1500);
    });
  }

  function toggleFiles() {
    setFilesOpen((o) => {
      const next = !o;
      setStoredFilesOpen(next);
      return next;
    });
  }

  // Ctrl/Cmd+B toggles the file tree — the same shortcut VS Code and most
  // other editors use for their own file explorer, so it's already muscle
  // memory rather than something new to learn. Deliberately NOT intercepted
  // while focus is inside a terminal pane: tmux's own default prefix key is
  // literally Ctrl+B, so stealing it there would break every tmux binding
  // (pane switching, copy-mode, etc.) for every shell in the app — a much
  // worse regression than not having this shortcut work from a terminal.
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (!(e.metaKey || e.ctrlKey)) return;
      if (e.key.toLowerCase() !== "b") return;
      if (e.target instanceof Element && e.target.closest(".xterm")) return;
      e.preventDefault();
      toggleFiles();
    }
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
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
          tabComponent: "terminal-tab",
          title: ts.tab_label || "shell",
          params: { terminalId: ts.id, repoId, worktreeId, worktreePath, baseLabel: ts.tab_label || "shell" },
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
        title: ts.tab_label || "shell",
        params: { terminalId: ts.id, repoId, worktreeId, worktreePath, baseLabel: ts.tab_label || "shell" },
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

  // Consumes a file left for this worktree by RepoContext's /ws/open-file
  // handler when it had to navigate here first (the target worktree wasn't
  // already the open tab) — see pendingFileOpen.ts. Gated on dockviewApi so
  // this fires once panels can actually be added, not on mount; takePendingFileOpen
  // clears itself after one read, so this is safe to depend on dockviewApi
  // alone rather than needing its own "already consumed" guard.
  useEffect(() => {
    if (!dockviewApi) return;
    const path = takePendingFileOpen(worktreeId);
    if (path) handleOpenFile(path);
  }, [dockviewApi, worktreeId]);

  // Consumes a "open a new terminal here" instruction left for this
  // worktree by Sidebar.tsx's spotlight-start handler when it had to
  // navigate here first (the repo-root worktree wasn't already the open
  // tab) — see pendingNewTerminal.ts. Same gating as the pendingFileOpen
  // effect above.
  useEffect(() => {
    if (!dockviewApi) return;
    if (takePendingNewTerminal(worktreeId)) handleNewTerminal("within");
  }, [dockviewApi, worktreeId]);

  // Same "no deps, re-register every render" idiom as the file-opener
  // registration above — the sidebar's action icons for the currently-open
  // worktree call straight through into this instance's own handlers, and
  // need to see fresh values (e.g. filesOpen/vscodeAvailable flipping)
  // immediately, not just at mount. See activeWorktreeActions.ts.
  useEffect(() => {
    registerActiveWorktreeActions({
      worktreeId,
      filesOpen,
      toggleFiles,
      vscodeAvailable,
      openVSCode: handleOpenInVSCode,
      openLog: () => setLogOpen(true),
      newTerminal: () => handleNewTerminal("within"),
      splitRight: () => handleNewTerminal("right"),
      splitDown: () => handleNewTerminal("below"),
    });
    return () => registerActiveWorktreeActions(null);
  });

  return (
    <div className="container worktree-detail">
      {error && <p className="error">{error}</p>}
      {vscodeError && <p className="error">Failed to open VS Code: {vscodeError}</p>}

      <div className="worktree-body">
        {filesOpen && (
          <div className="worktree-sidebar">
            <FileTree
              repoId={repoId}
              worktreeId={worktreeId}
              onOpenFile={handleOpenFile}
              activePath={activeFilePath}
              folderPath={worktreePath}
            />
          </div>
        )}
        <div className="terminal-area">
          {/* Sits directly above the shell tabs, not spanning the file
              tree column too — a title for this worktree's terminal/editor
              area specifically. */}
          {/* Branch name first, PR trailing and quiet. This used to open
              with "[No pull request for this branch]" — bracketed, in
              first position, ahead of the branch name — which made the
              most prominent string in the workspace a statement that
              nothing exists. No PR now renders nothing at all: the normal
              state of the UI is silence (docs/design-system.md). */}
          {/* Branch name, then the state of that branch, then its PR.
              This bar used to hold the branch name and nothing else,
              sitting directly beside a file-tree panel whose header showed
              the same string (a worktree directory is named after its
              branch) — two headers, one piece of information between
              them. It now carries what you'd otherwise run `git status`
              to find out, and each part renders only when there's
              something to say. */}
          <div className="worktree-header">
            <span className="worktree-header-branch-name">{worktree?.branch}</span>
            <button
              type="button"
              className={branchCopied ? "worktree-header-copy copied" : "worktree-header-copy"}
              title="Copy branch name"
              onClick={copyBranch}
            >
              {branchCopied ? "Copied" : "⧉"}
            </button>

            {summary?.has_upstream && (summary.ahead > 0 || summary.behind > 0) && (
              <span
                className="worktree-header-ticks"
                title={`${summary.ahead} ahead, ${summary.behind} behind upstream`}
              >
                {summary.ahead > 0 && `↑${summary.ahead}`}
                {summary.ahead > 0 && summary.behind > 0 && " "}
                {summary.behind > 0 && `↓${summary.behind}`}
              </span>
            )}
            {summary && summary.changed_files.length > 0 && (
              <span
                className="worktree-header-changed"
                title={summary.changed_files.join("\n")}
              >
                {summary.changed_files.length} changed
              </span>
            )}

            <span className="worktree-header-spacer" />

            {summary?.pr ? (
              <a
                className="worktree-header-pr"
                href={summary.pr.url}
                target="_blank"
                rel="noopener noreferrer"
                title={summary.pr.title}
              >
                <span className="worktree-header-pr-number">#{summary.pr.number}</span>
                {summary.pr.is_draft && <span className="worktree-header-pr-draft">draft</span>}
                <span className="worktree-header-pr-title">{summary.pr.title}</span>
              </a>
            ) : null}
          </div>
          <DockviewActionsContext.Provider
            value={{
              onOpenShell: () => handleNewTerminal("within"),
              onOpenClaude: () => handleNewTerminal("within", "claude", "claude"),
            }}
          >
            <DockviewReact
              components={components}
              tabComponents={tabComponents}
              watermarkComponent={Watermark}
              prefixHeaderActionsComponent={NewTerminalPrefixAction}
              onReady={onDockviewReady}
              className="dockview-theme-abyss app-dockview"
            />
          </DockviewActionsContext.Provider>
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
