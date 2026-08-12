import { act, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import WorktreeHoverPopover from "./WorktreeHoverPopover";
import { setCachedSummary } from "./prGitCache";
import { Worktree, WorktreeSummary } from "./api";

vi.mock("./api", () => ({
  getWorktreeSummary: vi.fn(),
}));

import { getWorktreeSummary } from "./api";

const wt: Worktree = {
  id: "wt1",
  repo_id: "r1",
  name: "a-very-long-worktree-name-that-gets-clipped-in-the-row",
  branch: "feature/long-branch-name",
  path: "/tmp/wt1",
  created_at: "",
  status: "active",
  source: "created",
  archived_at: "",
  source_branch: "main",
};

const summary: WorktreeSummary = {
  branch: "feature/long-branch-name",
  ahead: 2,
  behind: 0,
  has_upstream: true,
  dirty: true,
  changed_files: ["a.go", "b.go"],
  pr: { number: 42, title: "Add the thing", state: "OPEN", url: "https://example.com/pr/42", is_draft: false },
};

beforeEach(() => {
  localStorage.clear();
  vi.mocked(getWorktreeSummary).mockResolvedValue(summary);
});

describe("WorktreeHoverPopover", () => {
  it("does not show the popover before the hover delay elapses", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    render(
      <WorktreeHoverPopover wt={wt}>
        <span>row</span>
      </WorktreeHoverPopover>
    );

    const { fireEvent } = await import("@testing-library/react");
    fireEvent.mouseEnter(screen.getByText("row").parentElement!);

    act(() => {
      vi.advanceTimersByTime(500);
    });
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();

    await act(async () => {
      vi.advanceTimersByTime(500);
    });
    expect(screen.getByRole("tooltip")).toBeInTheDocument();
    vi.useRealTimers();
  });

  it("hides immediately on mouse leave, even mid-delay", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const { fireEvent } = await import("@testing-library/react");
    render(
      <WorktreeHoverPopover wt={wt}>
        <span>row</span>
      </WorktreeHoverPopover>
    );
    const target = screen.getByText("row").parentElement!;

    fireEvent.mouseEnter(target);
    act(() => {
      vi.advanceTimersByTime(500);
    });
    fireEvent.mouseLeave(target);
    await act(async () => {
      vi.advanceTimersByTime(1000);
    });

    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
    vi.useRealTimers();
  });

  it("shows the full worktree name and PR/git summary once fetched", async () => {
    const { fireEvent } = await import("@testing-library/react");
    render(
      <WorktreeHoverPopover wt={wt}>
        <span>row</span>
      </WorktreeHoverPopover>
    );
    fireEvent.mouseEnter(screen.getByText("row").parentElement!);

    expect(await screen.findByText(wt.name)).toBeInTheDocument();
    expect(await screen.findByText(/PR #42/)).toBeInTheDocument();
    expect(screen.getByText("a.go")).toBeInTheDocument();
    expect(getWorktreeSummary).toHaveBeenCalledWith("r1", "wt1");
  });

  it("shows a cached summary immediately and does not refetch while fresh", async () => {
    setCachedSummary("wt1", summary);
    const { fireEvent } = await import("@testing-library/react");
    render(
      <WorktreeHoverPopover wt={wt}>
        <span>row</span>
      </WorktreeHoverPopover>
    );
    fireEvent.mouseEnter(screen.getByText("row").parentElement!);

    expect(await screen.findByText(/PR #42/)).toBeInTheDocument();
    await waitFor(() => expect(getWorktreeSummary).not.toHaveBeenCalled());
  });
});
