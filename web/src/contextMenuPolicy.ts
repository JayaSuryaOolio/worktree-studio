// Decides whether the browser's native right-click context menu should be
// allowed through — used by main.tsx's document-wide `contextmenu`
// listener. Extracted as a pure function (same reasoning as Terminal.tsx's
// classifyTerminalKeyEvent) so the actual decision logic is unit-testable
// without a real browser/DOM event.
//
// Real text-editing surfaces keep the native menu — it's the only
// right-click copy/paste a person has there. `.xterm` is xterm.js's own
// root class (Terminal.tsx) — a real reported regression was this
// blanket-disabling the terminal's own right-click paste/copy, a common
// terminal habit and Terminal.tsx's explicit documented fallback for when
// the programmatic clipboard write fails. `.cm-editor` is CodeMirror's own
// root class (EditorPanel.tsx). Plain `<input>`/`<textarea>`/
// `contenteditable` covers every other text field (dialogs, the command
// palette, etc.).
const ALLOWED_SELECTOR = "input, textarea, [contenteditable='true'], .xterm, .cm-editor";

export function shouldAllowNativeContextMenu(target: EventTarget | null): boolean {
  return target instanceof Element && target.closest(ALLOWED_SELECTOR) !== null;
}
