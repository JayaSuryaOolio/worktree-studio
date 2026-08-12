// Whether the worktree-detail file tree sidebar is shown — a single
// global preference (localStorage), not per-worktree. Previously plain
// component state in WorktreeDetail.tsx, which reset to "open" every time
// `key={...}` remounted the component on switching worktrees (see that
// file's own doc comment on why that remount happens) — a real reported
// bug: collapsing the file tree didn't stay collapsed across worktrees.
const STORAGE_KEY = "worktree-studio-files-open";

export function getStoredFilesOpen(): boolean {
  return localStorage.getItem(STORAGE_KEY) !== "false"; // default: open
}

export function setStoredFilesOpen(open: boolean): void {
  localStorage.setItem(STORAGE_KEY, String(open));
}
