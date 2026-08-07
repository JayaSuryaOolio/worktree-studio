import { render, screen, waitFor } from "@testing-library/react";
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

vi.mock("./api", () => ({
  listTerminals: vi.fn(),
  createTerminal: vi.fn(),
  deleteTerminal: vi.fn(),
  getWorktreeLayout: vi.fn(),
  saveWorktreeLayout: vi.fn(),
}));

import { createTerminal, getWorktreeLayout, listTerminals, saveWorktreeLayout } from "./api";

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
});

describe("WorktreeDetail", () => {
  it("renders each existing terminal as a dockview panel", async () => {
    renderPage();
    await waitFor(() => expect(listTerminals).toHaveBeenCalledWith("r1", "w1"));
    expect(await screen.findByTestId("terminal-t1")).toBeInTheDocument();
  });

  it("creates a terminal via the dropdown's 'New tab' action and renders it", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByTestId("terminal-t1");

    await user.click(screen.getByRole("button", { name: /new terminal/i }));
    await user.click(screen.getByRole("button", { name: "New tab" }));

    await waitFor(() => expect(createTerminal).toHaveBeenCalledWith("r1", "w1"));
    expect(await screen.findByTestId("terminal-t2")).toBeInTheDocument();
    // Menu closes after the action.
    expect(screen.queryByRole("button", { name: "New tab" })).not.toBeInTheDocument();
  });

  it("creates a terminal via 'Split right' and via 'Split down'", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByTestId("terminal-t1");

    await user.click(screen.getByRole("button", { name: /new terminal/i }));
    await user.click(screen.getByRole("button", { name: "Split right" }));
    expect(await screen.findByTestId("terminal-t2")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /new terminal/i }));
    await user.click(screen.getByRole("button", { name: "Split down" }));
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
      const user = userEvent.setup();
      renderPage();
      await screen.findByTestId("terminal-t1");
      await waitFor(() => expect(getWorktreeLayout).toHaveBeenCalled());

      await user.click(screen.getByRole("button", { name: /new terminal/i }));
      await user.click(screen.getByRole("button", { name: "New tab" }));
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
});
