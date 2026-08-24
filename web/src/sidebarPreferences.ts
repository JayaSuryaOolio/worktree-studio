// Sidebar layout preferences — which repo groups are collapsed, and
// whether the sidebar is showing at all. localStorage rather than SQLite:
// unlike the terminal layout (which is per-worktree state you'd want back
// on another machine), these are per-screen viewing habits, and the
// nearest precedent in this codebase is filesPanelPreference.ts.
//
// Every accessor is total — a private window, disabled storage or a
// corrupted value falls back to the default rather than throwing on the
// first render.

const COLLAPSED_KEY = "worktree-studio-collapsed-repos";
const HIDDEN_KEY = "worktree-studio-sidebar-hidden";

export function getCollapsedRepos(): Set<string> {
  try {
    const raw = localStorage.getItem(COLLAPSED_KEY);
    if (!raw) return new Set();
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return new Set();
    return new Set(parsed.filter((v): v is string => typeof v === "string"));
  } catch {
    return new Set();
  }
}

export function setCollapsedRepos(ids: Set<string>): void {
  try {
    localStorage.setItem(COLLAPSED_KEY, JSON.stringify([...ids]));
  } catch {
    // Not remembering a collapsed group is a much smaller problem than
    // failing to collapse it.
  }
}

export function getSidebarHidden(): boolean {
  try {
    return localStorage.getItem(HIDDEN_KEY) === "true";
  } catch {
    return false;
  }
}

export function setSidebarHidden(hidden: boolean): void {
  try {
    localStorage.setItem(HIDDEN_KEY, String(hidden));
  } catch {
    // See above.
  }
}
