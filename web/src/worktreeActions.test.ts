import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("./api", () => ({
  ConflictError: class ConflictError extends Error {},
  createWorktree: vi.fn(),
  createTerminal: vi.fn(),
  deleteWorktree: vi.fn(),
  startSpotlight: vi.fn(),
  stopSpotlight: vi.fn(),
}));

import { createTerminal, createWorktree } from "./api";
import { createWorktreeWithClaudeTerminal } from "./worktreeActions";

const worktree = {
  id: "wt1",
  repo_id: "r1",
  name: "feature",
  branch: "feature",
  path: "/tmp/wt1",
  created_at: "2026-01-01T00:00:00Z",
};

beforeEach(() => {
  vi.mocked(createWorktree).mockResolvedValue(worktree);
  vi.mocked(createTerminal).mockResolvedValue({
    id: "t1",
    worktree_id: "wt1",
    tmux_session_name: "wts-t1",
    tab_label: "claude",
  });
});

describe("createWorktreeWithClaudeTerminal", () => {
  it("creates the worktree, then a terminal with claude as tab label and initial command", async () => {
    const result = await createWorktreeWithClaudeTerminal("r1", "feature");

    expect(createWorktree).toHaveBeenCalledWith("r1", "feature");
    expect(createTerminal).toHaveBeenCalledWith("r1", "wt1", "claude", "claude");
    expect(result).toEqual(worktree);
  });

  it("still returns the worktree even if creating the auto-claude terminal fails", async () => {
    vi.mocked(createTerminal).mockRejectedValue(new Error("tmux exploded"));
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});

    const result = await createWorktreeWithClaudeTerminal("r1", "feature");

    expect(result).toEqual(worktree);
    expect(consoleError).toHaveBeenCalled();
    consoleError.mockRestore();
  });
});
