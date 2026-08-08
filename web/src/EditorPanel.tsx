import { useEffect, useRef, useState } from "react";
import type { IDockviewPanelProps } from "dockview-react";
import { filesWsUrl, getFileContent, saveFileContent } from "./api";
import { EDITOR_REGISTRY, DEFAULT_EDITOR_KIND } from "./editors/registry";

export interface EditorPanelParams {
  repoId: string;
  worktreeId: string;
  path: string;
}

// The dockview panel shell for the file editor — the only place in the
// frontend (besides registry.ts) that touches the editor abstraction.
// Deliberately imports nothing from "codemirror"/"@codemirror/*" directly;
// swapping the registered engine never requires touching this file. See
// docs/editor-plan.md's "Editor abstraction" section.
//
// One panel is dedicated to exactly one file for its whole lifetime —
// WorktreeDetail.tsx's handleOpenFile reuses an already-open panel instead
// of creating a second one for the same path, which is what makes the
// "multiple panels on the same file" question (docs/editor-plan.md pitfall
// #8) moot by construction: there's only ever one buffer per file.
export default function EditorPanel({ params }: IDockviewPanelProps<EditorPanelParams>) {
  const { repoId, worktreeId, path } = params;
  const [content, setContent] = useState<string | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  // Set when the ws push reports this file changed on disk while the
  // buffer has unsaved edits — see docs/editor-plan.md pitfall #5. Not set
  // for a clean buffer, which reloads silently instead (see the ws
  // effect below).
  const [externalChangePending, setExternalChangePending] = useState(false);
  // Bumped to force the (uncontrolled, see EditorAdapter.ts) editor
  // component to remount with fresh content — the same key-remount
  // pattern WorktreeDetail.tsx already uses to reset dockview state.
  const [reloadKey, setReloadKey] = useState(0);
  // The editor component is uncontrolled after mount (see EditorAdapter.ts)
  // — it reports changes via onChange, but this ref (not React state) is
  // what handleSave actually reads, so a fast typist's save doesn't race a
  // stale closure over `content` from an earlier render.
  const latestContentRef = useRef("");
  // Read inside the ws message handler (set up once, see the effect
  // below) — a ref instead of the `dirty` state directly so that effect
  // doesn't need `dirty` in its deps and reconnect the socket on every
  // keystroke.
  const dirtyRef = useRef(false);

  function loadContent() {
    return getFileContent(repoId, worktreeId, path).then((res) => {
      latestContentRef.current = res.content;
      setContent(res.content);
      setReloadKey((k) => k + 1);
    });
  }

  useEffect(() => {
    loadContent().catch((err) => setLoadError((err as Error).message));
    // Mount-only: this panel is dedicated to one file for its entire
    // lifetime (see the reuse-existing-panel note above) — repoId/
    // worktreeId/path never change out from under an existing instance.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // fsnotify-driven external-change push (see internal/files.Watcher).
  // Reload silently if the buffer is clean; otherwise flag it and let the
  // user decide, rather than either silently clobbering unsaved edits or
  // silently ignoring a real external change.
  useEffect(() => {
    const ws = new WebSocket(filesWsUrl(worktreeId));
    ws.onmessage = (event) => {
      let msg: { type?: string; path?: string };
      try {
        msg = JSON.parse(event.data);
      } catch {
        return;
      }
      if (msg.type !== "changed" || msg.path !== path) return;
      if (dirtyRef.current) {
        setExternalChangePending(true);
      } else {
        loadContent().catch((err) => setLoadError((err as Error).message));
      }
    };
    return () => ws.close();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [worktreeId, path]);

  async function handleSave() {
    setSaving(true);
    setSaveError(null);
    try {
      await saveFileContent(repoId, worktreeId, path, latestContentRef.current);
      setDirty(false);
      dirtyRef.current = false;
    } catch (err) {
      setSaveError((err as Error).message);
    } finally {
      setSaving(false);
    }
  }

  function handleReloadFromDisk() {
    setExternalChangePending(false);
    setDirty(false);
    dirtyRef.current = false;
    loadContent().catch((err) => setLoadError((err as Error).message));
  }

  if (loadError) {
    return <div className="editor-panel-error">Failed to open {path}: {loadError}</div>;
  }
  if (content === null) {
    return <div className="editor-panel-loading">Loading {path}…</div>;
  }

  const Editor = EDITOR_REGISTRY[DEFAULT_EDITOR_KIND];

  return (
    <div className="editor-panel">
      <div className="editor-panel-toolbar">
        <span className="editor-panel-path">{path}</span>
        {dirty && (
          <span className="editor-panel-dirty" title="unsaved changes">
            ●
          </span>
        )}
        <button type="button" onClick={handleSave} disabled={saving || !dirty}>
          {saving ? "Saving…" : "Save"}
        </button>
        {saveError && <span className="editor-panel-save-error">{saveError}</span>}
      </div>
      {externalChangePending && (
        <div className="editor-panel-external-change">
          <span>This file changed on disk. Reloading will discard your unsaved edits.</span>
          <button type="button" onClick={handleReloadFromDisk}>
            Reload
          </button>
          <button type="button" onClick={() => setExternalChangePending(false)}>
            Keep editing
          </button>
        </div>
      )}
      <div className="editor-panel-body">
        <Editor
          key={reloadKey}
          content={content}
          path={path}
          theme="vscode-dark"
          onChange={(value) => {
            latestContentRef.current = value;
            dirtyRef.current = true;
            setDirty(true);
          }}
          onSaveRequested={handleSave}
        />
      </div>
    </div>
  );
}
