import { Routes, Route } from "react-router-dom";
import RepoPicker from "./RepoPicker";
import Workspace from "./Workspace";

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<RepoPicker />} />
      <Route path="/repo/:repoId" element={<Workspace />} />
    </Routes>
  );
}
