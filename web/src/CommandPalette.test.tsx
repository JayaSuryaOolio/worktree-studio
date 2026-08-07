import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import CommandPalette from "./CommandPalette";
import { RepoProvider } from "./RepoContext";

vi.mock("./api", () => ({
  listRepos: vi.fn(),
  listWorktrees: vi.fn(),
  getWorktreeStatus: vi.fn(),
  getSpotlightStatus: vi.fn(),
}));

import { getSpotlightStatus, getWorktreeStatus, listRepos, listWorktrees } from "./api";

function LocationProbe() {
  const location = useLocation();
  return <div data-testid="location">{location.pathname}</div>;
}

function Harness() {
  return (
    <MemoryRouter initialEntries={["/"]}>
      <RepoProvider>
        <CommandPalette onAddRepo={vi.fn()} onNewWorktree={vi.fn()} />
        <LocationProbe />
      </RepoProvider>
    </MemoryRouter>
  );
}

beforeEach(() => {
  vi.mocked(listRepos).mockResolvedValue([{ id: "r1", name: "adelaide", path: "/tmp/adelaide" }]);
  vi.mocked(listWorktrees).mockResolvedValue([
    { id: "w1", repo_id: "r1", name: "feature", branch: "feature", path: "/tmp/adelaide-wt/feature", created_at: "2026-01-01T00:00:00Z", status: "active" },
  ]);
  vi.mocked(getWorktreeStatus).mockResolvedValue({ branch: "feature", dirty: false, has_upstream: false, ahead: 0, behind: 0 });
  vi.mocked(getSpotlightStatus).mockResolvedValue({ available: false, active: false });
});

describe("CommandPalette", () => {
  it("is closed by default and opens on Cmd+K", async () => {
    render(<Harness />);
    await waitFor(() => expect(listWorktrees).toHaveBeenCalled());

    expect(screen.queryByPlaceholderText(/jump to a repo or worktree/i)).not.toBeInTheDocument();

    act(() => {
      document.dispatchEvent(new KeyboardEvent("keydown", { key: "k", metaKey: true, bubbles: true }));
    });

    expect(screen.getByPlaceholderText(/jump to a repo or worktree/i)).toBeInTheDocument();
  });

  it("navigates to a worktree when its item is selected, and closes", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await waitFor(() => expect(listWorktrees).toHaveBeenCalled());

    act(() => {
      document.dispatchEvent(new KeyboardEvent("keydown", { key: "k", metaKey: true, bubbles: true }));
    });

    const item = await screen.findByText("feature");
    await user.click(item);

    expect(screen.getByTestId("location")).toHaveTextContent("/repo/r1/worktree/w1");
    expect(screen.queryByPlaceholderText(/jump to a repo or worktree/i)).not.toBeInTheDocument();
  });

  it("triggers onNewWorktree for the right repo and closes", async () => {
    const user = userEvent.setup();
    const onNewWorktree = vi.fn();
    render(
      <MemoryRouter initialEntries={["/"]}>
        <RepoProvider>
          <CommandPalette onAddRepo={vi.fn()} onNewWorktree={onNewWorktree} />
        </RepoProvider>
      </MemoryRouter>
    );
    await waitFor(() => expect(listWorktrees).toHaveBeenCalled());

    act(() => {
      document.dispatchEvent(new KeyboardEvent("keydown", { key: "k", ctrlKey: true, bubbles: true }));
    });

    const item = await screen.findByText(/new worktree in adelaide/i);
    await user.click(item);

    expect(onNewWorktree).toHaveBeenCalledWith("r1");
    expect(screen.queryByPlaceholderText(/jump to a repo or worktree/i)).not.toBeInTheDocument();
  });
});
