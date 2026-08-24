import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  clampSidebarWidth,
  getCollapsedRepos,
  getSidebarHidden,
  getSidebarWidth,
  setCollapsedRepos,
  setSidebarHidden,
  setSidebarWidth,
  SIDEBAR_DEFAULT_WIDTH,
  SIDEBAR_MAX_WIDTH,
  SIDEBAR_MIN_WIDTH,
} from "./sidebarPreferences";

beforeEach(() => localStorage.clear());
afterEach(() => {
  localStorage.clear();
  vi.restoreAllMocks();
});

describe("collapsed repos", () => {
  it("round-trips a set", () => {
    setCollapsedRepos(new Set(["r1", "r2"]));
    expect(getCollapsedRepos()).toEqual(new Set(["r1", "r2"]));
  });

  it("defaults to nothing collapsed", () => {
    expect(getCollapsedRepos().size).toBe(0);
  });

  it("survives a corrupted value rather than throwing on first render", () => {
    localStorage.setItem("worktree-studio-collapsed-repos", "{not json");
    expect(getCollapsedRepos().size).toBe(0);

    localStorage.setItem("worktree-studio-collapsed-repos", '{"r1":true}');
    expect(getCollapsedRepos().size).toBe(0);

    localStorage.setItem("worktree-studio-collapsed-repos", '["r1", 7, null]');
    expect(getCollapsedRepos()).toEqual(new Set(["r1"]));
  });
});

describe("sidebar width", () => {
  it("defaults, round-trips, and clamps to the readable range", () => {
    expect(getSidebarWidth()).toBe(SIDEBAR_DEFAULT_WIDTH);

    setSidebarWidth(320);
    expect(getSidebarWidth()).toBe(320);

    setSidebarWidth(20);
    expect(getSidebarWidth()).toBe(SIDEBAR_MIN_WIDTH);

    setSidebarWidth(9000);
    expect(getSidebarWidth()).toBe(SIDEBAR_MAX_WIDTH);
  });

  it("clampSidebarWidth rejects NaN rather than propagating it into a style", () => {
    expect(clampSidebarWidth(Number.NaN)).toBe(SIDEBAR_DEFAULT_WIDTH);
    expect(clampSidebarWidth(Number.POSITIVE_INFINITY)).toBe(SIDEBAR_DEFAULT_WIDTH);
  });

  it("falls back to the default for a non-numeric stored value", () => {
    localStorage.setItem("worktree-studio-sidebar-width", "wide please");
    expect(getSidebarWidth()).toBe(SIDEBAR_DEFAULT_WIDTH);
  });
});

describe("sidebar hidden", () => {
  it("defaults to visible and round-trips", () => {
    expect(getSidebarHidden()).toBe(false);
    setSidebarHidden(true);
    expect(getSidebarHidden()).toBe(true);
  });
});

// A private window (or a browser set to block site data) throws on every
// access. None of these are important enough to break a render over.
describe("with storage unavailable", () => {
  beforeEach(() => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("denied");
    });
  });

  it("every accessor falls back to its default instead of throwing", () => {
    expect(() => setCollapsedRepos(new Set(["r1"]))).not.toThrow();
    expect(() => setSidebarWidth(300)).not.toThrow();
    expect(() => setSidebarHidden(true)).not.toThrow();
    expect(getCollapsedRepos().size).toBe(0);
    expect(getSidebarWidth()).toBe(SIDEBAR_DEFAULT_WIDTH);
    expect(getSidebarHidden()).toBe(false);
  });
});
