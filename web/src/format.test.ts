import { describe, expect, it } from "vitest";
import { commonValue, dominantValue, leafName, parentDir, relativeTime, shortenPath } from "./format";

const now = new Date("2026-08-25T12:00:00Z");
const ago = (ms: number) => new Date(now.getTime() - ms).toISOString();

const SECOND = 1000;
const MINUTE = 60 * SECOND;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

describe("relativeTime", () => {
  it("collapses anything very recent to 'just now'", () => {
    expect(relativeTime(ago(0), now)).toBe("just now");
    expect(relativeTime(ago(20 * SECOND), now)).toBe("just now");
  });

  it("steps up through minutes, hours, days, weeks and months", () => {
    expect(relativeTime(ago(5 * MINUTE), now)).toBe("5 minutes ago");
    expect(relativeTime(ago(3 * HOUR), now)).toBe("3 hours ago");
    expect(relativeTime(ago(4 * DAY), now)).toBe("4 days ago");
    expect(relativeTime(ago(14 * DAY), now)).toBe("2 weeks ago");
    expect(relativeTime(ago(120 * DAY), now)).toBe("4 months ago");
  });

  it("singularises exactly one unit", () => {
    expect(relativeTime(ago(1 * HOUR), now)).toBe("1 hour ago");
    expect(relativeTime(ago(1 * DAY), now)).toBe("1 day ago");
  });

  it("does not render a negative age for a timestamp slightly in the future", () => {
    // Clock skew between the Go server and the browser is real and
    // "-1 minutes ago" is worse than a small lie.
    expect(relativeTime(new Date(now.getTime() + 5 * SECOND).toISOString(), now)).toBe("just now");
  });

  it("returns an empty string for an unparseable timestamp", () => {
    expect(relativeTime("not a date", now)).toBe("");
  });
});

describe("shortenPath", () => {
  it("keeps the last two segments and marks the elision", () => {
    expect(shortenPath("/Users/j/.worktree-studio/worktrees/9af16fe11ec0cf6b/orders-new-id")).toBe(
      "…/9af16fe11ec0cf6b/orders-new-id"
    );
  });

  it("leaves a path that is already short enough alone", () => {
    expect(shortenPath("/tmp/repo")).toBe("/tmp/repo");
    expect(shortenPath("repo")).toBe("repo");
  });

  it("honours a different segment count", () => {
    expect(shortenPath("/a/b/c/d/e", 3)).toBe("…/c/d/e");
  });
});

describe("commonValue", () => {
  const rows = (...vals: (string | undefined)[]) => vals.map((v) => ({ v }));

  it("returns the shared value when every row agrees", () => {
    expect(commonValue(rows("origin/master", "origin/master"), (r) => r.v)).toBe("origin/master");
  });

  it("returns null the moment one row differs", () => {
    expect(commonValue(rows("origin/master", "origin/main"), (r) => r.v)).toBeNull();
  });

  it("returns null for an empty list or a missing value", () => {
    expect(commonValue([], (r: { v?: string }) => r.v)).toBeNull();
    expect(commonValue(rows(undefined, undefined), (r) => r.v)).toBeNull();
  });
});

describe("dominantValue", () => {
  const rows = (...vals: (string | undefined)[]) => vals.map((v) => ({ v }));
  const M = "origin/master";

  it("hoists what most rows share and counts the ones that don't", () => {
    // The real case: nine worktrees created from origin/master, two
    // attached from elsewhere. commonValue gives up here and the column
    // keeps printing origin/master nine times.
    const items = rows(...Array(9).fill(M), undefined, undefined);
    expect(commonValue(items, (r) => r.v)).toBeNull();
    expect(dominantValue(items, (r) => r.v)).toEqual({ value: M, exceptions: 2 });
  });

  it("reports zero exceptions when every row agrees", () => {
    expect(dominantValue(rows(M, M, M), (r) => r.v)).toEqual({ value: M, exceptions: 0 });
  });

  it("gives up when no value covers 60% — the column is carrying real information", () => {
    expect(dominantValue(rows("a", "b", "c", "d"), (r) => r.v)).toBeNull();
    expect(dominantValue(rows("a", "a", "b", "c", "d"), (r) => r.v)).toBeNull();
  });

  it("gives up below three items, where a caption costs more than it saves", () => {
    expect(dominantValue(rows(M, M), (r) => r.v)).toBeNull();
    expect(dominantValue([], (r: { v?: string }) => r.v)).toBeNull();
  });

  it("counts only real values, so a column of blanks hoists nothing", () => {
    expect(dominantValue(rows(undefined, undefined, undefined), (r) => r.v)).toBeNull();
  });

  it("breaks ties deterministically rather than on Map ordering", () => {
    const a = dominantValue(rows("beta", "beta", "alpha", "alpha"), (r) => r.v);
    const b = dominantValue(rows("alpha", "alpha", "beta", "beta"), (r) => r.v);
    expect(a).toEqual(b);
  });
});

describe("parentDir / leafName", () => {
  it("splits a path at its last separator", () => {
    expect(parentDir("/a/b/c")).toBe("/a/b");
    expect(leafName("/a/b/c")).toBe("c");
  });

  it("treats a bare name as having no directory part", () => {
    expect(parentDir("c")).toBe("");
    expect(leafName("c")).toBe("c");
  });

  it("does not claim the root slash as a parent", () => {
    expect(parentDir("/c")).toBe("");
  });
});
