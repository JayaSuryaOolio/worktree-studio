import { useRepoContext } from "./RepoContext";

// Adding a repo is now a modal reachable from the sidebar's "+" button —
// this page is just the welcome/empty state shown before anything's
// selected. Not a form of its own anymore.
export default function RepoPicker() {
  const { repos, reposLoading } = useRepoContext();

  return (
    <div className="container">
      <h1>worktree-studio</h1>
      {reposLoading ? (
        <p>Loading…</p>
      ) : repos.length === 0 ? (
        <p>Add a repo from the sidebar's "+" to get started.</p>
      ) : (
        <p>Pick a repo or worktree from the sidebar.</p>
      )}
    </div>
  );
}
