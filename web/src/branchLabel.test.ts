import { describe, expect, it } from "vitest";
import { splitBranchLabel } from "./branchLabel";

describe("splitBranchLabel", () => {
  it("leaves short names whole", () => {
    expect(splitBranchLabel("hotfix-1")).toEqual({ head: "hotfix-1", tail: "" });
    expect(splitBranchLabel("issue-217")).toEqual({ head: "issue-217", tail: "" });
  });

  it("keeps the trailing segment of a long hyphenated name", () => {
    // The real complaint: this used to render as "hotfix-backend-se…",
    // which is indistinguishable from any other hotfix-backend-* branch.
    expect(splitBranchLabel("hotfix-backend-services")).toEqual({
      head: "hotfix-backend",
      tail: "-services",
    });
  });

  it("cuts at the boundary nearest the end, keeping the head as long as possible", () => {
    // Scanning back from the end (rather than forward from the middle)
    // means the least possible text goes behind the ellipsis while the
    // tail still lands on a real segment.
    expect(splitBranchLabel("feature/fetch-users-and-roles")).toEqual({
      head: "feature/fetch-users-and",
      tail: "-roles",
    });
  });

  it("treats a slash as a boundary too", () => {
    expect(splitBranchLabel("migrate/syncposusers")).toEqual({
      head: "migrate",
      tail: "/syncposusers",
    });
  });

  it("recomposes to exactly the original name", () => {
    const names = [
      "oc-6308-deleted-user-crashes-open-orders",
      "migrate/sync-pos-users",
      "feature-bypass-central-users-flag",
      "resolve-users-in-worklogs",
      "kanban-board-auto-work",
      "earnest-juniper",
    ];
    for (const n of names) {
      const { head, tail } = splitBranchLabel(n);
      expect(head + tail).toBe(n);
    }
  });

  it("falls back to a fixed tail when there's no usable boundary", () => {
    const { head, tail } = splitBranchLabel("averyveryverylongsingletoken");
    expect(tail).toHaveLength(8);
    expect(head + tail).toBe("averyveryverylongsingletoken");
  });

  it("never returns a tail so short it defeats the point", () => {
    for (const n of ["a-really-long-branch-x", "feature/some-thing-ab"]) {
      const { tail } = splitBranchLabel(n);
      expect(tail.length).toBeGreaterThanOrEqual(3);
    }
  });
});
