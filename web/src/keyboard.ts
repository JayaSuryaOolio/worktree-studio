// Shared guards for document-level keyboard shortcuts.
//
// This app is mostly a terminal. A bare-key shortcut registered on
// `document` fires while you're typing into xterm just as readily as
// anywhere else, so every such shortcut needs to know when the keystroke
// belongs to something else.

/**
 * True when the keystroke is going into a text-entry surface — a real
 * input/textarea, a contenteditable, or xterm's hidden helper textarea
 * (which is how the terminal receives keys, so it matches TEXTAREA and is
 * covered by the same check).
 *
 * Bare-key shortcuts must bail on this. Cmd-modified ones generally
 * shouldn't: Cmd+K is expected to work while the terminal has focus.
 */
export function isTextEntryTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  const tag = target.tagName;
  if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return true;
  // === true, not a bare return: isContentEditable is undefined rather
  // than false on elements that aren't editable in jsdom, and the
  // declared `boolean` return type doesn't catch that at runtime.
  return target.isContentEditable === true;
}

/**
 * True when a shortcut's modifier is pressed, in a way that doesn't steal
 * tmux's prefix.
 *
 * The naive `e.metaKey || e.ctrlKey` is wrong for a letter tmux binds:
 * Ctrl+B is tmux's default prefix, and every terminal in this app is a
 * tmux session. Cmd never reaches tmux, so it's always safe; Ctrl is only
 * honoured when the keystroke isn't headed into a terminal — which keeps
 * the shortcut working on Linux and Windows without breaking the prefix
 * for anyone actually typing in a shell.
 */
export function hasSafeModifier(e: KeyboardEvent): boolean {
  if (e.metaKey) return true;
  return e.ctrlKey && !isTextEntryTarget(e.target);
}
