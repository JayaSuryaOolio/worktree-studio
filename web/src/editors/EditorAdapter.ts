import type { ComponentType } from "react";

// The seam this whole directory exists for: EditorPanel.tsx (the dockview
// panel shell) depends only on this interface, never on a specific editing
// engine's own types. Swapping the registered engine, or adding a second
// one alongside it, means writing a new component that satisfies
// EditorProps and adding it to registry.ts — nothing in EditorPanel.tsx or
// FileTree.tsx changes. See docs/editor-plan.md's "Editor abstraction"
// section for the full rationale, including pitfall #10 (why this
// interface must never grow a field that's really "an engine-specific
// option in disguise" — anything CodeMirror-specific belongs inside
// CodeMirrorEditor.tsx, not here).
export interface EditorProps {
  /** The file's initial content when this component mounts. Engines here
   * are uncontrolled after mount (like a native <textarea defaultValue>,
   * not a controlled <input value>) — to force a reset to different
   * content (e.g. after the user accepts an external-change reload
   * prompt), the caller remounts via a changed `key`, the same pattern
   * WorktreeDetail.tsx already uses to reset dockview state per worktree.
   * This keeps every engine's own state management (undo history, cursor
   * position, selection) simple and internal, rather than fighting a
   * controlled-value re-render on every keystroke. */
  content: string;
  /** The file's path (relative to the worktree root is fine — only the
   * basename/extension matter). Purely a language-detection hint: engines
   * map this to their own syntax-highlighting grammar however they need
   * to (e.g. via @codemirror/language-data's matchFilename, which also
   * handles extensionless conventional names like "Dockerfile" or
   * "Makefile"). Never a CodeMirror-specific (or any other engine's own)
   * language identifier — that mapping lives inside the adapter, not here. */
  path: string;
  /** The one theme that exists right now (matches Command Deck's dark
   * palette) — a real union once a second theme is ever built, per
   * PLAN.md's explicitly-deferred theme-switching TODO. */
  theme: "vscode-dark";
  /** Called on every content change with the buffer's current full text.
   * The caller (EditorPanel) is responsible for dirty-tracking and save
   * debouncing/throttling — this callback fires on every edit, unthrottled. */
  onChange: (value: string) => void;
  /** Called when the user invokes the engine's own "save" keybinding
   * (Cmd/Ctrl+S). The engine must preventDefault the browser's native
   * "Save Page As" for this key — see docs/editor-plan.md pitfall #4. */
  onSaveRequested: () => void;
}

export type EditorAdapter = ComponentType<EditorProps>;
