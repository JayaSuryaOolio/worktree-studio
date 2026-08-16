import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter, MemoryRouter, Route, RouterProvider, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import WorktreeDetail from "./WorktreeDetail";

// Stub out the real Terminal component (xterm + a real WebSocket) — this
// test is about dockview panel orchestration and terminal CRUD wiring,
// not xterm/websocket internals (those are exercised by manual/real-server
// verification elsewhere, and Terminal.tsx itself is unchanged by this step).
vi.mock("./Terminal", () => ({
  default: ({ terminalId }: { terminalId: string }) => (
    <div data-testid={`terminal-${terminalId}`}>term:{terminalId}</div>
  ),
}));

// Stub out the real EditorPanel (CodeMirror + its own file-content fetch)
// for the same reason Terminal is stubbed above — this test is about
// dockview panel orchestration (does opening a file create/reuse the right
// panel?), not the editor engine's own internals.
vi.mock("./EditorPanel", () => ({
  default: ({ params }: { params: { path: string } }) => (
    <div data-testid={`editor-${params.path}`}>editor:{params.path}</div>
  ),
}));

vi.mock("./api", () => ({
  listTerminals: vi.fn(),
  createTerminal: vi.fn(),
  deleteTerminal: vi.fn(),
  getWorktreeLayout: vi.fn(),
  saveWorktreeLayout: vi.fn(),
  getFileTree: vi.fn(),
  getDependencyStatus: vi.fn(),
  openInVSCode: vi.fn(),
  getTerminalCwd: vi.fn(),
  getWorktreeSummary: vi.fn(),
}));

// Stubbed rather than wrapped in the real RepoProvider (which would need
// listRepos/listWorktrees/getWorktreeStatus/getSpotlightStatus mocked too,
// none of which this test cares about) — WorktreeDetail only reads
// worktreesByRepo from this context, to resolve the current worktree's
// path (for TerminalPanel's cwd-mismatch check).
vi.mock("./RepoContext", () => ({
  useRepoContext: vi.fn(),
}));

import {
  createTerminal,
  deleteTerminal,
  getDependencyStatus,
  getFileTree,
  getTerminalCwd,
  getWorktreeLayout,
  getWorktreeSummary,
  listTerminals,
  saveWorktreeLayout,
} from "./api";
import { useRepoContext } from "./RepoContext";
import { getActiveWorktreeActions } from "./activeWorktreeActions";

function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/repo/r1/worktree/w1"]}>
      <Routes>
        <Route path="/repo/:repoId/worktree/:worktreeId" element={<WorktreeDetail />} />
      </Routes>
    </MemoryRouter>
  );
}

beforeEach(() => {
  vi.mocked(listTerminals).mockResolvedValue([
    { id: "t1", worktree_id: "w1", tmux_session_name: "wts-t1", tab_label: "shell" },
  ]);
  let nextId = 2;
  vi.mocked(createTerminal).mockImplementation(async () => ({
    id: `t${nextId++}`,
    worktree_id: "w1",
    tmux_session_name: `wts-t${nextId}`,
    tab_label: "shell",
  }));
  vi.mocked(getWorktreeLayout).mockResolvedValue(null);
  vi.mocked(saveWorktreeLayout).mockResolvedValue(undefined);
  vi.mocked(getFileTree).mockResolvedValue([]);
  vi.mocked(deleteTerminal).mockResolvedValue(undefined);
  // Matches the active worktree's own path by default, i.e. no
  // cwd-mismatch border — individual tests override this to exercise the
  // mismatch case.
  vi.mocked(getTerminalCwd).mockResolvedValue({ cwd: "/tmp/feature" });
  vi.mocked(getWorktreeSummary).mockResolvedValue({
    branch: "feature",
    ahead: 0,
    behind: 0,
    has_upstream: true,
    dirty: false,
    changed_files: [],
    pr: null,
  });
  vi.mocked(useRepoContext).mockReturnValue({
    repos: [],
    reposLoading: false,
    reposError: null,
    refreshRepos: vi.fn(),
    selectedRepoId: "r1",
    worktreesByRepo: {
      r1: [
        { id: "w1", repo_id: "r1", name: "feature-worktree", branch: "feature", path: "/tmp/feature", created_at: "", status: "active" },
        { id: "w2", repo_id: "r1", name: "other-worktree", branch: "other", path: "/tmp/other", created_at: "", status: "active" },
      ],
    },
    worktreesLoading: false,
    worktreesError: null,
    refreshWorktrees: vi.fn(),
    gitStatus: {},
    spotlightStatus: {},
    statusRefreshing: {},
  });
  vi.mocked(getDependencyStatus).mockResolvedValue({
    tmux: { installed: true },
    spotlight: { installed: true },
    skill: { installed: true },
    vscode_cli: { installed: false },
  });
});

describe("WorktreeDetail", () => {
  it("renders each existing terminal as a dockview panel", async () => {
    renderPage();
    await waitFor(() => expect(listTerminals).toHaveBeenCalledWith("r1", "w1"));
    expect(await screen.findByTestId("terminal-t1")).toBeInTheDocument();
  });

  it("flags a terminal whose cwd has drifted outside the worktree with a faint border", async () => {
    vi.mocked(getTerminalCwd).mockResolvedValue({ cwd: "/tmp/somewhere-else" });
    renderPage();
    const term = await screen.findByTestId("terminal-t1");
    await waitFor(() => expect(term.closest(".terminal-panel-inset")).toHaveClass("cwd-mismatch"));
  });

  it("does not flag a terminal whose cwd is a subdirectory of the worktree", async () => {
    vi.mocked(getTerminalCwd).mockResolvedValue({ cwd: "/tmp/feature/src" });
    renderPage();
    const term = await screen.findByTestId("terminal-t1");
    await waitFor(() => expect(getTerminalCwd).toHaveBeenCalled());
    expect(term.closest(".terminal-panel-inset")).not.toHaveClass("cwd-mismatch");
  });

  // The "+"/split-right/split-down buttons that used to live in this
  // component's own toolbar now live in the sidebar's expandable worktree
  // card and call through the activeWorktreeActions registry instead — see
  // activeWorktreeActions.ts. Driven here the same way that consumer would.
  it("creates a terminal via the registered newTerminal action and renders it", async () => {
    renderPage();
    await screen.findByTestId("terminal-t1");

    await act(async () => {
      getActiveWorktreeActions()?.newTerminal();
    });

    await waitFor(() => expect(createTerminal).toHaveBeenCalledWith("r1", "w1", undefined, undefined));
    expect(await screen.findByTestId("terminal-t2")).toBeInTheDocument();
  });

  it("creates a terminal via the registered splitRight and splitDown actions", async () => {
    renderPage();
    await screen.findByTestId("terminal-t1");

    await act(async () => {
      getActiveWorktreeActions()?.splitRight();
    });
    expect(await screen.findByTestId("terminal-t2")).toBeInTheDocument();

    await act(async () => {
      getActiveWorktreeActions()?.splitDown();
    });
    expect(await screen.findByTestId("terminal-t3")).toBeInTheDocument();

    expect(createTerminal).toHaveBeenCalledTimes(2);
  });

  it("fetches the saved layout for this worktree on mount", async () => {
    renderPage();
    await screen.findByTestId("terminal-t1");
    await waitFor(() => expect(getWorktreeLayout).toHaveBeenCalledWith("r1", "w1"));
  });

  it(
    "saves the layout (debounced) after a real dockview layout change",
    async () => {
      // Real timers, not fake ones — vi.useFakeTimers() interacts badly
      // with testing-library's internal polling (findBy/waitFor use real
      // setTimeout under the hood too), so faking time here risked
      // deadlocking the test rather than actually speeding it up. A 600ms
      // real wait for one test is a fine price for exercising the real
      // debounce path end-to-end instead of mocking it away.
      renderPage();
      await screen.findByTestId("terminal-t1");
      await waitFor(() => expect(getWorktreeLayout).toHaveBeenCalled());

      await act(async () => {
        getActiveWorktreeActions()?.newTerminal();
      });
      await screen.findByTestId("terminal-t2");

      // Nothing saved yet — still inside the debounce window.
      expect(saveWorktreeLayout).not.toHaveBeenCalled();

      await new Promise((resolve) => setTimeout(resolve, 600));
      await waitFor(() => expect(saveWorktreeLayout).toHaveBeenCalled());

      const lastCall = vi.mocked(saveWorktreeLayout).mock.calls.at(-1);
      expect(lastCall?.[0]).toBe("r1");
      expect(lastCall?.[1]).toBe("w1");
      // The third argument is dockview's own real toJSON() output (not a
      // fixture) — assert it's shaped like a real serialized layout
      // rather than pinning its exact contents, which would make this
      // test brittle against dockview's own internal schema.
      expect(lastCall?.[2]).toHaveProperty("panels");
      expect(lastCall?.[2]).toHaveProperty("grid");
    },
    10000
  );

  // Regression test for a real user-reported bug ("all worktrees show the
  // same terminals" / "terminals not switching"): react-router reuses the
  // same WorktreeDetail component instance across navigations between two
  // different worktree URLs (same route element, just new params) rather
  // than unmounting it — so without an explicit remount, dockview's
  // long-lived panel set from previously-visited worktrees just
  // accumulated. Uses real client-side navigation (createMemoryRouter +
  // router.navigate), not separate render() calls — those would trivially
  // "pass" regardless of the bug, since each always creates a fresh tree.
  //
  // A single w1->w2 hop alone does NOT discriminate here: dockview's own
  // default tab-visibility handling unmounts the now-inactive w1 panel's
  // DOM even on the *buggy* code, so it looks fine by accident on the
  // first hop. The bug only becomes visible on a round trip back to a
  // *previously*-visited worktree — confirmed by hand: this exact
  // assertion failed (showing "terminal-t2" while on /worktree/w1) against
  // the pre-fix component, and passes against the fixed one.
  it("shows the right worktree's terminal after navigating away and back (round trip)", async () => {
    vi.mocked(listTerminals).mockImplementation(async (_repoId, worktreeId) => {
      if (worktreeId === "w1") {
        return [{ id: "t1", worktree_id: "w1", tmux_session_name: "wts-t1", tab_label: "shell" }];
      }
      if (worktreeId === "w2") {
        return [{ id: "t2", worktree_id: "w2", tmux_session_name: "wts-t2", tab_label: "shell" }];
      }
      return [];
    });

    const router = createMemoryRouter(
      [{ path: "/repo/:repoId/worktree/:worktreeId", element: <WorktreeDetail /> }],
      { initialEntries: ["/repo/r1/worktree/w1"] }
    );
    render(<RouterProvider router={router} />);
    await screen.findByTestId("terminal-t1");

    router.navigate("/repo/r1/worktree/w2");
    await screen.findByTestId("terminal-t2");

    router.navigate("/repo/r1/worktree/w1");

    expect(await screen.findByTestId("terminal-t1")).toBeInTheDocument();
    expect(screen.queryByTestId("terminal-t2")).not.toBeInTheDocument();
  });

  it("empty-state watermark's 'Open claude' button creates a claude terminal", async () => {
    vi.mocked(listTerminals).mockResolvedValue([]);
    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole("button", { name: "Open claude" }));
    await waitFor(() =>
      expect(createTerminal).toHaveBeenCalledWith("r1", "w1", "claude", "claude")
    );
  });

  it("empty-state watermark's 'Open shell' button creates a plain terminal", async () => {
    vi.mocked(listTerminals).mockResolvedValue([]);
    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole("button", { name: "Open shell" }));
    await waitFor(() =>
      expect(createTerminal).toHaveBeenCalledWith("r1", "w1", undefined, undefined)
    );
  });

  it("opens a file from the file tree into an editor panel, and reuses it on a second click", async () => {
    vi.mocked(getFileTree).mockResolvedValue([
      { name: "main.go", path: "src/main.go", type: "file" },
    ]);
    const user = userEvent.setup();
    renderPage();
    await screen.findByTestId("terminal-t1");

    const fileButton = await screen.findByText("main.go");
    await user.click(fileButton);
    expect(await screen.findByTestId("editor-src/main.go")).toBeInTheDocument();

    // Clicking the same file again must not open a second panel — see
    // EditorPanel.tsx's doc comment: one panel per file per worktree,
    // reused rather than duplicated.
    await user.click(fileButton);
    expect(screen.getAllByTestId("editor-src/main.go")).toHaveLength(1);
  });

  // Regression test for a real user-reported bug: dockview's
  // onDidRemovePanel fires for every closed panel, terminals AND editor
  // panels alike, but only terminals have a server-side session to tear
  // down. Before the fix, closing an editor panel fell through to
  // deleteTerminal(editorPanelId), which 404'd ("terminal session not
  // found") and surfaced that as a top-level error banner sitting right
  // above the editor — for an action (closing a file) that has nothing to
  // clean up server-side.
  it("closing a file's editor panel does not call deleteTerminal or show an error", async () => {
    vi.mocked(getFileTree).mockResolvedValue([{ name: "main.go", path: "main.go", type: "file" }]);
    const user = userEvent.setup();
    const { container } = renderPage();
    await screen.findByTestId("terminal-t1");

    await user.click(await screen.findByText("main.go"));
    await screen.findByTestId("editor-main.go");

    const editorTab = Array.from(container.querySelectorAll(".dv-tab")).find((el) =>
      el.textContent?.includes("main.go")
    );
    const closeBtn = editorTab?.querySelector(".dv-default-tab-action");
    expect(closeBtn).toBeTruthy();
    await user.click(closeBtn as Element);

    await waitFor(() => expect(screen.queryByTestId("editor-main.go")).not.toBeInTheDocument());
    expect(deleteTerminal).not.toHaveBeenCalled();
    expect(screen.queryByText(/terminal session not found/i)).not.toBeInTheDocument();
  });
});
