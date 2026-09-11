import { describe, expect, it } from "vitest";
import { setPendingNewTerminal, takePendingNewTerminal } from "./pendingNewTerminal";

describe("pendingNewTerminal", () => {
  it("returns the pending options and clears the flag for a matching worktree", () => {
    setPendingNewTerminal("wt1");
    expect(takePendingNewTerminal("wt1")).toEqual({});
    expect(takePendingNewTerminal("wt1")).toBeNull();
  });

  it("carries tabLabel/initialCommand through", () => {
    setPendingNewTerminal("wt2", { tabLabel: "claude (resumed)", initialCommand: "claude --resume abc" });
    expect(takePendingNewTerminal("wt2")).toEqual({
      tabLabel: "claude (resumed)",
      initialCommand: "claude --resume abc",
    });
  });

  it("returns null for a different worktree", () => {
    setPendingNewTerminal("wt3");
    expect(takePendingNewTerminal("wt4")).toBeNull();
  });

  it("returns null when nothing is pending", () => {
    expect(takePendingNewTerminal("wt5")).toBeNull();
  });
});
