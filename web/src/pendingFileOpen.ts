// A tiny module-level singleton for handing a file path across a route
// navigation, same idiom as activeWorktreeFileOpener.ts's registry (a
// mutable module variable rather than React context, since the sender —
// RepoContext, reacting to a /ws/open-file push for a worktree that isn't
// the currently open tab — and the receiver — the WorktreeDetail instance
// that mounts after navigate() — aren't in an ancestor/descendant
// relationship at the moment the value needs to be handed off).
//
// Keyed by worktreeId defensively: if two open-file events for different
// worktrees race a navigation, the mounting WorktreeDetail should only ever
// consume the one addressed to it.
let pending: { worktreeId: string; path: string } | null = null;

export function setPendingFileOpen(worktreeId: string, path: string) {
  pending = { worktreeId, path };
}

/** Returns and clears the pending path for worktreeId, or null if there
 * isn't one (or it belongs to a different worktree) — call once from the
 * mount effect that's ready to open a file. */
export function takePendingFileOpen(worktreeId: string): string | null {
  if (!pending || pending.worktreeId !== worktreeId) return null;
  const path = pending.path;
  pending = null;
  return path;
}
