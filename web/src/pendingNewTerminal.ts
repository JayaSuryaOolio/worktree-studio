// A tiny module-level singleton for handing an "open a new terminal here"
// instruction across a route navigation — same idiom as pendingFileOpen.ts
// (the sender, e.g. the spotlight-start handler in Sidebar.tsx that needs
// the repo-root worktree's own tab to exist before it can add a panel to
// it, and the receiver, the WorktreeDetail instance that mounts after
// navigate(), aren't in an ancestor/descendant relationship at the moment
// the instruction needs to be handed off).
//
// Keyed by worktreeId defensively, same reasoning as pendingFileOpen.ts: if
// two "open a terminal" requests for different worktrees race a
// navigation, the mounting WorktreeDetail should only ever consume the one
// addressed to it.
let pendingWorktreeId: string | null = null;

export function setPendingNewTerminal(worktreeId: string) {
  pendingWorktreeId = worktreeId;
}

/** Returns true and clears the flag if a new terminal is pending for
 * worktreeId, false otherwise (including if it belongs to a different
 * worktree) — call once from the mount effect that's ready to open one. */
export function takePendingNewTerminal(worktreeId: string): boolean {
  if (pendingWorktreeId !== worktreeId) return false;
  pendingWorktreeId = null;
  return true;
}
