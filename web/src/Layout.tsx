import { Outlet } from "react-router-dom";
import { RepoProvider } from "./RepoContext";
import Sidebar from "./Sidebar";

// The persistent app shell: a left sidebar (worktree list, auto-loading per
// selected repo) plus the routed page content. Mounted once above all
// three routes so switching between them never remounts the sidebar or
// re-triggers its data fetching — this app is an SPA in the literal sense,
// not a set of independently-loading pages. Visual styling (Command Deck
// tokens) lands in a later step; this is the structural shell only.
export default function Layout() {
  return (
    <RepoProvider>
      <div className="app-shell">
        <Sidebar />
        <main className="app-main">
          <Outlet />
        </main>
      </div>
    </RepoProvider>
  );
}
