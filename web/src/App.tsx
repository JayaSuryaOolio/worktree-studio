import { Routes, Route } from "react-router-dom";
import Layout from "./Layout";
import RepoPicker from "./RepoPicker";
import Workspace from "./Workspace";
import WorktreeDetail from "./WorktreeDetail";

export default function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route path="/" element={<RepoPicker />} />
        <Route path="/repo/:repoId" element={<Workspace />} />
        <Route
          path="/repo/:repoId/worktree/:worktreeId"
          element={<WorktreeDetail />}
        />
      </Route>
    </Routes>
  );
}
