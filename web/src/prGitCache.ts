// A localStorage-backed, TTL'd cache for WorktreeSummary (git status +
// changed files + PR, via getWorktreeSummary) — the sidebar's hover
// popover reads/writes this instead of hitting the backend (which itself
// shells out to `gh`, a real GitHub API call) on every hover. A person
// moving their mouse across several rows shouldn't cost several `gh`
// calls; this is what keeps that from happening — see
// WorktreeHoverPopover.tsx's stale-while-revalidate use of it.
import { WorktreeSummary } from "./api";

const STORAGE_KEY = "worktree-studio-worktree-summary-cache";

// How long a cached summary is considered fresh enough to skip a
// background refetch — per direct request: not fetched fresh on every
// hover (rate-limit risk), refreshed "time to time" instead.
export const SUMMARY_CACHE_TTL_MS = 5 * 60 * 1000;

interface CacheEntry {
  data: WorktreeSummary;
  fetchedAt: number;
}

function readAll(): Record<string, CacheEntry> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? (JSON.parse(raw) as Record<string, CacheEntry>) : {};
  } catch {
    return {}; // corrupt/unavailable storage — treat as empty rather than throwing
  }
}

function writeAll(all: Record<string, CacheEntry>): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(all));
  } catch {
    // Storage full/unavailable — the cache is a pure optimization, so a
    // failed write just means the next hover re-fetches; nothing else to
    // do about it here.
  }
}

export interface CachedSummary {
  data: WorktreeSummary;
  /** True once fetchedAt is more than SUMMARY_CACHE_TTL_MS old — callers
   * still get the data (better a stale summary than none while a
   * background refresh is in flight), just told it may be outdated. */
  stale: boolean;
}

export function getCachedSummary(worktreeId: string): CachedSummary | null {
  const entry = readAll()[worktreeId];
  if (!entry) return null;
  return { data: entry.data, stale: Date.now() - entry.fetchedAt > SUMMARY_CACHE_TTL_MS };
}

export function setCachedSummary(worktreeId: string, data: WorktreeSummary): void {
  const all = readAll();
  all[worktreeId] = { data, fetchedAt: Date.now() };
  writeAll(all);
}
