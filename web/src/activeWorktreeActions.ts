import { useSyncExternalStore } from "react";

// Same module-level-singleton bridge as activeWorktreeFileOpener.ts — the
// sidebar's per-worktree action icons (files/VS Code/log/terminal) are
// rendered outside WorktreeDetail's tree entirely, so there's no React
// context path from a sidebar row down into whichever WorktreeDetail
// instance is actually mounted. Unlike the file-opener bridge, the sidebar
// also needs to reactively mirror a bit of that instance's state (e.g. is
// the files panel currently open) rather than just fire-and-forget an
// action, hence useSyncExternalStore instead of a bare function call.
export interface ActiveWorktreeActions {
  worktreeId: string;
  filesOpen: boolean;
  toggleFiles: () => void;
  vscodeAvailable: boolean;
  openVSCode: () => void;
  openLog: () => void;
  newTerminal: () => void;
  splitRight: () => void;
  splitDown: () => void;
}

let current: ActiveWorktreeActions | null = null;
const listeners = new Set<() => void>();

export function registerActiveWorktreeActions(actions: ActiveWorktreeActions | null) {
  current = actions;
  listeners.forEach((fn) => fn());
}

function subscribe(fn: () => void): () => void {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

export function getActiveWorktreeActions(): ActiveWorktreeActions | null {
  return current;
}

export function useActiveWorktreeActions(): ActiveWorktreeActions | null {
  return useSyncExternalStore(subscribe, getActiveWorktreeActions);
}
