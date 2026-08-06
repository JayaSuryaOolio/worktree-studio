import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
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
}));

import { createTerminal, listTerminals } from "./api";

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
});
