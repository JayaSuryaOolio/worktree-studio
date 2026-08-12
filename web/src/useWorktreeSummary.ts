import { useEffect, useState } from "react";
import { getWorktreeSummary, WorktreeSummary } from "./api";
import { getCachedSummary, setCachedSummary } from "./prGitCache";

// Shared stale-while-revalidate logic for WorktreeSummary (git status +
// changed files + PR) — used by both WorktreeHoverPopover.tsx and
// FileTree.tsx's header, so the caching policy (prGitCache.ts's 5-minute
// TTL) lives in exactly one place rather than being reimplemented per
// consumer. `enabled` gates the effect entirely — the popover only wants
// this running while it's actually visible, not from the moment its
// (always-mounted) row exists.
export function useWorktreeSummary(
  repoId: string,
  worktreeId: string,
  enabled: boolean
): { summary: WorktreeSummary | null; error: string | null } {
  const [summary, setSummary] = useState<WorktreeSummary | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!enabled) return;

    const cached = getCachedSummary(worktreeId);
    if (cached) setSummary(cached.data);
    if (!cached || cached.stale) {
      getWorktreeSummary(repoId, worktreeId)
        .then((data) => {
          setCachedSummary(worktreeId, data);
          setSummary(data);
          setError(null);
        })
        .catch((err) => {
          // Keep showing cached/stale data (set above) if there is any —
          // only surface an error when there's nothing else to show.
          if (!cached) setError((err as Error).message);
        });
    }
  }, [repoId, worktreeId, enabled]);

  return { summary, error };
}
