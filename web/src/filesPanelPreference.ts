// Whether the worktree-detail file tree panel is shown, and which side of
// the terminal area it sits on — both single global preferences
// (localStorage), not per-worktree. `filesOpen` was previously plain
// component state in WorktreeDetail.tsx, which reset to "open" every time
// `key={...}` remounted the component on switching worktrees (see that
// file's own doc comment on why that remount happens) — a real reported
// bug: collapsing the file tree didn't stay collapsed across worktrees.
//
// The side defaults to "right", where the tree's own toggle button lives
// in the worktree header's right end: the control and the thing it opens
// are on the same edge, so the panel appears where you were already
// pointing. Kept as a setting rather than a hardcoded flip because the
// left-hand file tree is what VS Code and every editor before it trained
// people to expect — see SettingsModal.tsx's Appearance tab.
import { useSyncExternalStore } from "react";

const STORAGE_KEY = "worktree-studio-files-open";
const SIDE_KEY = "worktree-studio-files-side";
const WIDTH_KEY = "worktree-studio-files-width";

export type FilesPanelSide = "left" | "right";

// Narrower than MIN and nested paths have nothing left to show; wider than
// MAX and the tree starts competing with the terminal for the screen. The
// same reasoning (and roughly the same feel) as the app sidebar's own
// bounds in sidebarPreferences.ts, with more headroom at the top end —
// this one holds deep directory trees, not a flat list of branch names.
export const FILES_PANEL_MIN_WIDTH = 160;
export const FILES_PANEL_MAX_WIDTH = 720;
export const FILES_PANEL_DEFAULT_WIDTH = 240;

export function clampFilesPanelWidth(px: number): number {
  if (!Number.isFinite(px)) return FILES_PANEL_DEFAULT_WIDTH;
  return Math.min(FILES_PANEL_MAX_WIDTH, Math.max(FILES_PANEL_MIN_WIDTH, Math.round(px)));
}

// Global, like every other preference in this module — the file tree is
// one panel that swaps its contents per worktree, not a different panel
// per worktree, so a width set in one is the width you meant everywhere.
export function getStoredFilesWidth(): number {
  try {
    const raw = localStorage.getItem(WIDTH_KEY);
    if (raw === null) return FILES_PANEL_DEFAULT_WIDTH;
    return clampFilesPanelWidth(Number(raw));
  } catch {
    return FILES_PANEL_DEFAULT_WIDTH;
  }
}

export function setStoredFilesWidth(px: number): void {
  try {
    localStorage.setItem(WIDTH_KEY, String(clampFilesPanelWidth(px)));
  } catch {
    // See setStoredFilesOpen.
  }
}

export function getStoredFilesOpen(): boolean {
  try {
    return localStorage.getItem(STORAGE_KEY) !== "false"; // default: open
  } catch {
    return true;
  }
}

export function setStoredFilesOpen(open: boolean): void {
  try {
    localStorage.setItem(STORAGE_KEY, String(open));
  } catch {
    // A private window / disabled storage just means the panel doesn't
    // remember its state, which is much smaller than failing to toggle it.
  }
}

export function getStoredFilesSide(): FilesPanelSide {
  try {
    return localStorage.getItem(SIDE_KEY) === "left" ? "left" : "right";
  } catch {
    return "right";
  }
}

// The settings modal and the worktree detail page are in different parts
// of the tree with no context between them, so changing the side has to
// reach an already-mounted WorktreeDetail some other way. Same
// module-level-singleton + useSyncExternalStore bridge as
// activeWorktreeActions.ts; the snapshot is a plain string, so it stays
// referentially stable across reads.
const listeners = new Set<() => void>();

export function setStoredFilesSide(side: FilesPanelSide): void {
  try {
    localStorage.setItem(SIDE_KEY, side);
  } catch {
    // See setStoredFilesOpen.
  }
  listeners.forEach((fn) => fn());
}

function subscribe(fn: () => void): () => void {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

export function useFilesPanelSide(): FilesPanelSide {
  return useSyncExternalStore(subscribe, getStoredFilesSide);
}
