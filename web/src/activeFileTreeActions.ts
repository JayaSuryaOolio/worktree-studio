import { useSyncExternalStore } from "react";

// Same bridge pattern as activeWorktreeActions.ts, but registered by
// FileTree.tsx itself rather than WorktreeDetail — this action (filter to
// changed files) only makes sense while a file tree instance actually
// exists, i.e. only while the files panel is open, so this is
// unregistered whenever FileTree unmounts (files panel closed or the
// worktree itself changes), independently of whether
// activeWorktreeActions' registration is still live. Collapse-all lives
// directly in FileTree.tsx's own header instead — it only makes sense
// right next to the tree it acts on, no cross-tree bridge needed there.
export interface ActiveFileTreeActions {
  worktreeId: string;
  filterToChanged: boolean;
  changedFilesAvailable: boolean;
  toggleChangedFilesFilter: () => void;
}

let current: ActiveFileTreeActions | null = null;
const listeners = new Set<() => void>();

export function registerActiveFileTreeActions(actions: ActiveFileTreeActions | null) {
  current = actions;
  listeners.forEach((fn) => fn());
}

function subscribe(fn: () => void): () => void {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

export function getActiveFileTreeActions(): ActiveFileTreeActions | null {
  return current;
}

export function useActiveFileTreeActions(): ActiveFileTreeActions | null {
  return useSyncExternalStore(subscribe, getActiveFileTreeActions);
}
