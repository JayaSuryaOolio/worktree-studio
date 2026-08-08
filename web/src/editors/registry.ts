import type { EditorAdapter } from "./EditorAdapter";
import CodeMirrorEditor from "./CodeMirrorEditor";

// Mirrors the `const components = { terminal: TerminalPanel }` pattern
// already established in WorktreeDetail.tsx — same shape, same place in
// the codebase's mental model. Adding a second engine later (or letting a
// user/setting pick between two) means adding an entry here and nowhere
// else; EditorPanel.tsx only ever reads from this map by kind, never
// imports an adapter directly.
export const EDITOR_REGISTRY: Record<string, EditorAdapter> = {
  codemirror: CodeMirrorEditor,
};

// The only engine that exists today. Not user-facing yet — see
// docs/editor-plan.md's "explicitly out of scope" note on the
// engine-picker UI: building a dropdown for a one-item registry is
// premature, but the registry itself is what makes adding a second engine
// (or a picker) additive rather than a rewrite.
export const DEFAULT_EDITOR_KIND = "codemirror";
