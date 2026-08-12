import { renderHook, waitFor } from "@testing-library/react";
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
