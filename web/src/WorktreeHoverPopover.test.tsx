import { act, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import WorktreeHoverPopover, { computePosition } from "./WorktreeHoverPopover";
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

function fakeRect(overrides: Partial<DOMRect>): DOMRect {
  return {
    left: 0,
    right: 0,
    top: 0,
    bottom: 0,
    width: 0,
    height: 0,
    x: 0,
    y: 0,
    toJSON: () => ({}),
    ...overrides,
  } as DOMRect;
}

describe("computePosition", () => {
  const originalInnerWidth = window.innerWidth;
  const originalInnerHeight = window.innerHeight;

  afterEach(() => {
    Object.defineProperty(window, "innerWidth", { value: originalInnerWidth, configurable: true });
    Object.defineProperty(window, "innerHeight", { value: originalInnerHeight, configurable: true });
  });

  it("places the popover to the right of the target when there's room", () => {
    Object.defineProperty(window, "innerWidth", { value: 1200, configurable: true });
    Object.defineProperty(window, "innerHeight", { value: 800, configurable: true });

    const pos = computePosition(fakeRect({ left: 20, right: 300, top: 100, bottom: 130 }));
    expect(pos.left).toBe(300 + 8);
    expect(pos.top).toBe(100);
  });

  it("flips below the row when there isn't enough room on the right", () => {
    Object.defineProperty(window, "innerWidth", { value: 500, configurable: true });
    Object.defineProperty(window, "innerHeight", { value: 800, configurable: true });

    // right=300 + margin(8) + popover width(320) > innerWidth(500)
    const pos = computePosition(fakeRect({ left: 20, right: 300, top: 100, bottom: 130 }));
    expect(pos.top).toBe(130 + 8);
    expect(pos.left).toBe(20);
  });
});
