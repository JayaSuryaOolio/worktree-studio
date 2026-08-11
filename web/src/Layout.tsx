import { useState } from "react";
import { Outlet, useNavigate } from "react-router-dom";
import AddRepoDialog from "./AddRepoDialog";
import AttachWorktreeDialog from "./AttachWorktreeDialog";
import { createWorktree, importWorktree, Repo } from "./api";
import CommandPalette from "./CommandPalette";
import NewWorktreeDialog from "./NewWorktreeDialog";
import { RepoProvider, useRepoContext } from "./RepoContext";
import Sidebar from "./Sidebar";

// The persistent app shell: a left sidebar (worktree list, auto-loading per
// selected repo), the routed page content, and a command palette — all
// mounted once above the three routes so switching between them never
// remounts the sidebar or re-triggers its data fetching. This app is an
// SPA in the literal sense, not a set of independently-loading pages.
export default function Layout() {
  return (
    <RepoProvider>
      <LayoutShell />
    </RepoProvider>
  );
}

// Split out from Layout so it can call useRepoContext() — RepoProvider has
// to be an ancestor, not a sibling, of anything that reads the context.
function LayoutShell() {
  const { repos, refreshRepos, refreshWorktrees } = useRepoContext();
  const navigate = useNavigate();

  // Add-repo/new-worktree/attach-worktree dialogs are triggered from both
  // the sidebar and the command palette — the state lives here, one level
  // up, so there's exactly one dialog instance either surface can open,
  // not two independent copies.
  const [addRepoOpen, setAddRepoOpen] = useState(false);
  const [newWorktreeRepoId, setNewWorktreeRepoId] = useState<string | null>(null);
  const [attachWorktreeRepoId, setAttachWorktreeRepoId] = useState<string | null>(null);

  function handleRepoCreated(repo: Repo) {
    refreshRepos();
    navigate(`/repo/${repo.id}`);
  }

  async function handleWorktreeCreate(repoId: string, name: string) {
    const wt = await createWorktree(repoId, name);
    refreshWorktrees();
    navigate(`/repo/${repoId}/worktree/${wt.id}`);
  }

  async function handleWorktreeAttach(repoId: string, path: string, name: string) {
    const wt = await importWorktree(repoId, path, name || undefined);
    refreshWorktrees();
    navigate(`/repo/${repoId}/worktree/${wt.id}`);
  }

  const attachWorktreeRepo = repos.find((r) => r.id === attachWorktreeRepoId);

  return (
    <div className="app-shell">
      <Sidebar onAddRepo={() => setAddRepoOpen(true)} onNewWorktree={setNewWorktreeRepoId} />
      <main className="app-main">
        <Outlet />
      </main>

      <CommandPalette
        onAddRepo={() => setAddRepoOpen(true)}
        onNewWorktree={setNewWorktreeRepoId}
        onAttachWorktree={setAttachWorktreeRepoId}
      />

      {addRepoOpen && (
        <AddRepoDialog onCreated={handleRepoCreated} onClose={() => setAddRepoOpen(false)} />
      )}
      {newWorktreeRepoId && (
        <NewWorktreeDialog
          repoId={newWorktreeRepoId}
          onCreate={(name) => handleWorktreeCreate(newWorktreeRepoId, name)}
          onClose={() => setNewWorktreeRepoId(null)}
        />
      )}
      {attachWorktreeRepoId && attachWorktreeRepo && (
        <AttachWorktreeDialog
          repoName={attachWorktreeRepo.name}
          onAttach={(path, name) => handleWorktreeAttach(attachWorktreeRepoId, path, name)}
          onClose={() => setAttachWorktreeRepoId(null)}
        />
      )}
    </div>
  );
}
