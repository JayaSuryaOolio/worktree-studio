import { describe, expect, it } from "vitest";
import { setPendingNewTerminal, takePendingNewTerminal } from "./pendingNewTerminal";

describe("pendingNewTerminal", () => {
  it("returns true and clears the flag for a matching worktree", () => {
    setPendingNewTerminal("wt1");
    expect(takePendingNewTerminal("wt1")).toBe(true);
    expect(takePendingNewTerminal("wt1")).toBe(false);
  });

  it("returns false for a different worktree", () => {
    setPendingNewTerminal("wt3");
    expect(takePendingNewTerminal("wt4")).toBe(false);
  });

  it("returns false when nothing is pending", () => {
    expect(takePendingNewTerminal("wt5")).toBe(false);
  });
});
