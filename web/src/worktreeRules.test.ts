import { describe, expect, it } from "vitest";
import { canArchiveWorktree } from "./worktreeRules";

describe("canArchiveWorktree", () => {
  it("is true for an unpinned worktree", () => {
    expect(canArchiveWorktree({ pinned: false })).toBe(true);
  });

  it("is false for a pinned worktree — mirrors internal/api.CanArchiveWorktree", () => {
    expect(canArchiveWorktree({ pinned: true })).toBe(false);
  });
});
