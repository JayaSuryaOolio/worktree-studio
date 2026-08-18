import { describe, expect, it } from "vitest";
import { setPendingFileOpen, takePendingFileOpen } from "./pendingFileOpen";

describe("pendingFileOpen", () => {
  it("returns and clears the pending path for a matching worktree", () => {
    setPendingFileOpen("wt1", "src/main.go");
    expect(takePendingFileOpen("wt1")).toBe("src/main.go");
    expect(takePendingFileOpen("wt1")).toBeNull();
  });

  it("returns null for a different worktree", () => {
    setPendingFileOpen("wt3", "src/main.go");
    expect(takePendingFileOpen("wt4")).toBeNull();
  });

  it("returns null when nothing is pending", () => {
    expect(takePendingFileOpen("wt5")).toBeNull();
  });
});
