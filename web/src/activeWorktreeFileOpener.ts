// A tiny module-level singleton bridging CommandPalette (a sibling of the
// routed page content, mounted once in Layout.tsx — see its own comment)
// to whichever WorktreeDetail instance is currently mounted, so selecting
// a file in the command palette can open it the same way clicking it in
// the file tree sidebar does. React context wouldn't reach across this
// gap: CommandPalette isn't a descendant of the routed <Outlet/> content,
// it's a sibling of it, so nothing rendered inside WorktreeDetail can hand
// it something via context. A plain mutable module variable set on mount/
// cleared on unmount is the whole mechanism — there's only ever at most
// one WorktreeDetail mounted at a time (each worktree route remounts a
// fresh instance, see WorktreeDetail.tsx's outer/inner split), so there's
// no ambiguity about which one "the active worktree" refers to.
type FileOpener = (path: string) => void;

let activeOpener: FileOpener | null = null;

export function registerActiveFileOpener(fn: FileOpener | null) {
  activeOpener = fn;
}

export function openFileInActiveWorktree(path: string): boolean {
  if (!activeOpener) return false;
  activeOpener(path);
  return true;
}
