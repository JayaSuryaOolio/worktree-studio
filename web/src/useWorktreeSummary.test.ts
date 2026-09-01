import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useWorktreeSummary } from "./useWorktreeSummary";
import { setCachedSummary } from "./prGitCache";
import { WorktreeSummary } from "./api";

vi.mock("./api", () => ({
  getWorktreeSummary: vi.fn(),
}));

import { getWorktreeSummary } from "./api";

const summary: WorktreeSummary = {
  branch: "feature",
  ahead: 0,
  behind: 0,
  has_upstream: true,
  dirty: false,
  changed_files: [],
  pr: null,
};

beforeEach(() => {
  localStorage.clear();
  vi.mocked(getWorktreeSummary).mockResolvedValue(summary);
});

describe("useWorktreeSummary", () => {
  it("does nothing while disabled", () => {
    renderHook(() => useWorktreeSummary("r1", "wt1", false));
    expect(getWorktreeSummary).not.toHaveBeenCalled();
  });

  it("fetches and returns the summary once enabled", async () => {
    const { result } = renderHook(() => useWorktreeSummary("r1", "wt1", true));
    await waitFor(() => expect(result.current.summary).toEqual(summary));
    expect(getWorktreeSummary).toHaveBeenCalledWith("r1", "wt1");
  });

  it("serves a fresh cache hit without calling the API", async () => {
    setCachedSummary("wt1", summary);
    const { result } = renderHook(() => useWorktreeSummary("r1", "wt1", true));
    await waitFor(() => expect(result.current.summary).toEqual(summary));
    expect(getWorktreeSummary).not.toHaveBeenCalled();
  });
});

// The worktree header's branch name and PR link go stale the moment
// someone checks out a branch or opens a PR from inside one of its own
// shells, and nothing tells the page about it — hence a poll and a manual
// nudge, both of which must ignore the cache TTL that exists to keep the
// sidebar's hovers off GitHub's rate limit.
describe("useWorktreeSummary refresh", () => {
  it("refresh() re-fetches even when the cache is still fresh", async () => {
    setCachedSummary("wt1", summary);
    const { result } = renderHook(() => useWorktreeSummary("r1", "wt1", true));
    await waitFor(() => expect(result.current.summary).toEqual(summary));
    expect(getWorktreeSummary).not.toHaveBeenCalled();

    act(() => result.current.refresh());
    await waitFor(() => expect(getWorktreeSummary).toHaveBeenCalledTimes(1));
  });

  it("refresh() is inert while disabled", async () => {
    const { result } = renderHook(() => useWorktreeSummary("r1", "wt1", false));
    act(() => result.current.refresh());
    expect(getWorktreeSummary).not.toHaveBeenCalled();
  });

  it("polls on the given interval, and not at all without one", async () => {
    vi.useFakeTimers();
    try {
      setCachedSummary("wt1", summary);
      const { rerender } = renderHook(
        ({ poll }: { poll?: number }) => useWorktreeSummary("r1", "wt1", true, { pollMs: poll }),
        { initialProps: {} as { poll?: number } }
      );
      await vi.advanceTimersByTimeAsync(120_000);
      expect(getWorktreeSummary).not.toHaveBeenCalled();

      rerender({ poll: 60_000 });
      await vi.advanceTimersByTimeAsync(60_000);
      expect(getWorktreeSummary).toHaveBeenCalledTimes(1);
      await vi.advanceTimersByTimeAsync(60_000);
      expect(getWorktreeSummary).toHaveBeenCalledTimes(2);
    } finally {
      vi.useRealTimers();
    }
  });
});
