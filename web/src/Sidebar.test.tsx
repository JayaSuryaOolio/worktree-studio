import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import Sidebar from "./Sidebar";
import { RepoProvider } from "./RepoContext";

vi.mock("./api", () => ({
  listRepos: vi.fn(),
  listWorktrees: vi.fn(),
  getWorktreeStatus: vi.fn(),
  getSpotlightStatus: vi.fn(),
  getWorktreeSummary: vi.fn(),
  attentionWsUrl: vi.fn(() => "ws://localhost/ws/attention"),
  openFileWsUrl: vi.fn(() => "ws://localhost/ws/open-file"),
  clearAttention: vi.fn(),
}));

import { getSpotlightStatus, getWorktreeStatus, getWorktreeSummary, listRepos, listWorktrees } from "./api";

function Harness({ initialPath = "/repo/r1/worktree/w1" }: { initialPath?: string }) {
  return (
    <MemoryRouter initialEntries={[initialPath]}>
      <RepoProvider>
        <Sidebar onAddRepo={vi.fn()} onNewWorktree={vi.fn()} />
      </RepoProvider>
    </MemoryRouter>
  );
}

beforeEach(() => {
  vi.mocked(listRepos).mockResolvedValue([{ id: "r1", name: "adelaide", path: "/tmp/adelaide" }]);
  vi.mocked(listWorktrees).mockResolvedValue([
    { id: "w1", repo_id: "r1", name: "feature-worktree", branch: "feature", path: "/tmp/adelaide-wt/feature", created_at: "2026-01-01T00:00:00Z", status: "active" },
  ]);
  vi.mocked(getWorktreeStatus).mockResolvedValue({ branch: "feature", dirty: false, has_upstream: false, ahead: 0, behind: 0 });
  vi.mocked(getSpotlightStatus).mockResolvedValue({ available: false, active: false });
  vi.mocked(getWorktreeSummary).mockResolvedValue({
    branch: "feature",
    ahead: 0,
    behind: 0,
    has_upstream: false,
    dirty: false,
    changed_files: [],
    pr: null,
  });
});

describe("Sidebar worktree row", () => {
  it("clicking the already-active row toggles the card open, then closed again", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    const row = await screen.findByText("feature");
    // The card's content is always in the DOM (the smooth expand/collapse
    // is a pure CSS grid-rows transition, not conditional rendering — see
    // sidebar.css), so the expand toggle's aria-expanded is what actually
    // reflects collapsed/expanded state, not element presence.
    const toggle = () => screen.getByRole("button", { name: /(expand|collapse) feature/i });
    expect(toggle()).toHaveAttribute("aria-expanded", "false");

    await user.click(row);
    expect(toggle()).toHaveAttribute("aria-expanded", "true");

    await user.click(row);
    expect(toggle()).toHaveAttribute("aria-expanded", "false");
  });

  it("clicking a non-active row navigates instead of expanding", async () => {
    vi.mocked(listWorktrees).mockResolvedValue([
      { id: "w1", repo_id: "r1", name: "feature-worktree", branch: "feature", path: "/tmp/adelaide-wt/feature", created_at: "2026-01-01T00:00:00Z", status: "active" },
      { id: "w2", repo_id: "r1", name: "other-worktree", branch: "other", path: "/tmp/adelaide-wt/other", created_at: "2026-01-01T00:00:00Z", status: "active" },
    ]);
    const user = userEvent.setup();
    render(<Harness />);

    const otherRow = await screen.findByText("other");
    await user.click(otherRow);

    // Navigating away from w1 to w2 is real navigation, not a toggle —
    // neither row's card should have expanded as a side effect.
    expect(screen.getByRole("button", { name: /expand feature/i })).toHaveAttribute(
      "aria-expanded",
      "false"
    );
    expect(screen.getByRole("button", { name: /expand other/i })).toHaveAttribute(
      "aria-expanded",
      "false"
    );
  });
});
