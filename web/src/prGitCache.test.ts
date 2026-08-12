import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getCachedSummary, setCachedSummary, SUMMARY_CACHE_TTL_MS } from "./prGitCache";
import { WorktreeSummary } from "./api";

const summary: WorktreeSummary = {
  branch: "feature",
  ahead: 1,
  behind: 0,
  has_upstream: true,
  dirty: true,
  changed_files: ["a.go"],
  pr: null,
};

beforeEach(() => {
  localStorage.clear();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("prGitCache", () => {
  it("returns null for a worktree that's never been cached", () => {
    expect(getCachedSummary("wt1")).toBeNull();
  });

  it("round-trips a cached summary, fresh (not stale) right after setting it", () => {
    setCachedSummary("wt1", summary);
    const cached = getCachedSummary("wt1");
    expect(cached?.data).toEqual(summary);
    expect(cached?.stale).toBe(false);
  });

  it("marks a cached summary stale once it's older than the TTL", () => {
    vi.useFakeTimers();
    setCachedSummary("wt1", summary);
    vi.advanceTimersByTime(SUMMARY_CACHE_TTL_MS + 1);
    expect(getCachedSummary("wt1")?.stale).toBe(true);
  });

  it("keeps separate entries per worktree id", () => {
    setCachedSummary("wt1", summary);
    setCachedSummary("wt2", { ...summary, branch: "other" });
    expect(getCachedSummary("wt1")?.data.branch).toBe("feature");
    expect(getCachedSummary("wt2")?.data.branch).toBe("other");
  });

  it("tolerates corrupt localStorage content instead of throwing", () => {
    localStorage.setItem("worktree-studio-worktree-summary-cache", "{not json");
    expect(getCachedSummary("wt1")).toBeNull();
  });
});
