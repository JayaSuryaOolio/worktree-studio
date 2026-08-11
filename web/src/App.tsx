import { Navigate, Routes, Route, useParams } from "react-router-dom";
import Layout from "./Layout";
import RepoPicker from "./RepoPicker";
import WorktreeDetail from "./WorktreeDetail";
import RepoSettings from "./RepoSettings";
import { rootWorktreeId } from "./rootWorktree";

// `/repo/:repoId` on its own has nothing useful left to show (the settings
// page's Worktrees tab already covers the "list every worktree" view) — it
// just redirects straight to the repo's own root-checkout worktree page, the
// same place clicking the repo name in the sidebar links to.
function RepoRootRedirect() {
  const { repoId } = useParams<{ repoId: string }>();
  if (!repoId) return null;
  return <Navigate to={`/repo/${repoId}/worktree/${rootWorktreeId(repoId)}`} replace />;
}

export default function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route path="/" element={<RepoPicker />} />
        <Route path="/repo/:repoId" element={<RepoRootRedirect />} />
        <Route path="/repo/:repoId/settings" element={<RepoSettings />} />
        <Route
          path="/repo/:repoId/worktree/:worktreeId"
          element={<WorktreeDetail />}
        />
      </Route>
    </Routes>
  );
}
