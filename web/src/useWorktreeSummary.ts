import { useCallback, useEffect, useRef, useState } from "react";
import { getWorktreeSummary, WorktreeSummary } from "./api";
import { getCachedSummary, setCachedSummary } from "./prGitCache";

export interface UseWorktreeSummaryOptions {
  /** Re-fetch this often (ms) while the document is visible, ignoring the
   * prGitCache TTL. Off (0/undefined) by default: only the worktree header
   * — one worktree, the one you're looking at — polls, because a summary
   * costs a real `gh pr view` call and the sidebar's dozens of rows must
   * not each be doing this on a timer. */
  pollMs?: number;
}

export interface UseWorktreeSummaryResult {
  summary: WorktreeSummary | null;
  error: string | null;
  /** Force a fetch now, bypassing the cache TTL — for the moments a
   * summary is known to have just gone stale (a shell opening in the
   * worktree, where the branch is about to be switched or a PR pushed). */
  refresh: () => void;
}

// Shared stale-while-revalidate logic for WorktreeSummary (git status +
// changed files + PR) — used by WorktreeHoverPopover.tsx, FileTree.tsx's
// header and WorktreeDetail.tsx's worktree header, so the caching policy
// (prGitCache.ts's 5-minute TTL) lives in exactly one place rather than
// being reimplemented per consumer. `enabled` gates the effect entirely —
// the popover only wants this running while it's actually visible, not
// from the moment its (always-mounted) row exists.
export function useWorktreeSummary(
  repoId: string,
  worktreeId: string,
  enabled: boolean,
  options: UseWorktreeSummaryOptions = {}
): UseWorktreeSummaryResult {
  const { pollMs } = options;
  const [summary, setSummary] = useState<WorktreeSummary | null>(null);
  const [error, setError] = useState<string | null>(null);
  // Guards against a slow fetch resolving after the component is gone, and
  // against an in-flight fetch's result being applied out of order behind
  // a newer one (a poll tick landing after a manual refresh).
  const requestSeqRef = useRef(0);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const fetchNow = useCallback(
    (haveCached: boolean) => {
      const seq = ++requestSeqRef.current;
      getWorktreeSummary(repoId, worktreeId)
        .then((data) => {
          if (!mountedRef.current || seq !== requestSeqRef.current) return;
          setCachedSummary(worktreeId, data);
          setSummary(data);
          setError(null);
        })
        .catch((err) => {
          if (!mountedRef.current || seq !== requestSeqRef.current) return;
          // Keep showing cached/stale data if there is any — only surface
          // an error when there's nothing else to show.
          if (!haveCached) setError((err as Error).message);
        });
    },
    [repoId, worktreeId]
  );

  const refresh = useCallback(() => {
    if (!enabled) return;
    fetchNow(true);
  }, [enabled, fetchNow]);

  useEffect(() => {
    if (!enabled) return;

    const cached = getCachedSummary(worktreeId);
    if (cached) setSummary(cached.data);
    if (!cached || cached.stale) fetchNow(!!cached);
  }, [repoId, worktreeId, enabled, fetchNow]);

  // Periodic refresh, for the things that change underneath this page
  // without the app ever hearing about it: a branch checked out in a
  // shell, a PR opened from the command line. Paused while the document is
  // hidden — a backgrounded tab polling `gh` forever is pure waste — and
  // caught up immediately when it becomes visible again, so coming back to
  // the window shows current state rather than whatever was true when you
  // left it.
  useEffect(() => {
    if (!enabled || !pollMs) return;

    const timer = window.setInterval(() => {
      if (document.hidden) return;
      fetchNow(true);
    }, pollMs);
    function onVisibility() {
      if (!document.hidden) fetchNow(true);
    }
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [enabled, pollMs, fetchNow]);

  return { summary, error, refresh };
}
