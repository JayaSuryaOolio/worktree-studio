// The synthetic worktree id for a repo's own root checkout — mirrors
// store.RootWorktreeID on the backend (internal/api/api.go's
// EnsureRootWorktree). Kept as a shared helper so the frontend's few call
// sites (the sidebar's repo-name link, the `/repo/:repoId` redirect) can't
// drift out of sync with each other or with the backend's id format.
export function rootWorktreeId(repoId: string): string {
  return `root-${repoId}`;
}

export function isRootWorktreeId(worktreeId: string): boolean {
  return worktreeId.startsWith("root-");
}
