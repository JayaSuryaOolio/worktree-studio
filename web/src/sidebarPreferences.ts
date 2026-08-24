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
const WIDTH_KEY = "worktree-studio-sidebar-width";

// Narrower than MIN and the branch names have nothing left to show; wider
// than MAX and the sidebar starts competing with the terminal for the
// screen, which is the opposite of what it's for.
export const SIDEBAR_MIN_WIDTH = 180;
export const SIDEBAR_MAX_WIDTH = 460;
export const SIDEBAR_DEFAULT_WIDTH = 250;

export function clampSidebarWidth(px: number): number {
  if (!Number.isFinite(px)) return SIDEBAR_DEFAULT_WIDTH;
  return Math.min(SIDEBAR_MAX_WIDTH, Math.max(SIDEBAR_MIN_WIDTH, Math.round(px)));
}

export function getSidebarWidth(): number {
  try {
    const raw = localStorage.getItem(WIDTH_KEY);
    if (raw === null) return SIDEBAR_DEFAULT_WIDTH;
    return clampSidebarWidth(Number(raw));
  } catch {
    return SIDEBAR_DEFAULT_WIDTH;
  }
}

export function setSidebarWidth(px: number): void {
  try {
    localStorage.setItem(WIDTH_KEY, String(clampSidebarWidth(px)));
  } catch {
    // See the note on setCollapsedRepos.
  }
}

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
