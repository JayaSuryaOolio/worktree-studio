import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { applyTheme, getStoredTheme, resolveMode, setTheme, watchSystemMode } from "./theme";

// jsdom's matchMedia always reports matches:false, so systemMode() resolves
// to "dark" unless a test stubs it. Several tests below stub it explicitly
// rather than relying on that default, since the point is the resolution
// step, not jsdom's opinion about the OS.
function stubMatchMedia(prefersLight: boolean, listeners?: (() => void)[]) {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: prefersLight && query.includes("light"),
    media: query,
    addEventListener: (_: string, fn: () => void) => listeners?.push(fn),
    removeEventListener: (_: string, fn: () => void) => {
      if (!listeners) return;
      const i = listeners.indexOf(fn);
      if (i >= 0) listeners.splice(i, 1);
    },
  }));
}

function reset() {
  localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
  document.documentElement.removeAttribute("data-mode");
  vi.unstubAllGlobals();
}

beforeEach(reset);
afterEach(reset);

describe("theme", () => {
  it("defaults to graphite + system with nothing stored", () => {
    expect(getStoredTheme()).toEqual({ family: "graphite", mode: "system" });
  });

  it("setTheme persists both axes and getStoredTheme reflects them", () => {
    setTheme({ family: "ledger", mode: "light" });
    expect(getStoredTheme()).toEqual({ family: "ledger", mode: "light" });
    expect(localStorage.getItem("worktree-studio-theme-family")).toBe("ledger");
    expect(localStorage.getItem("worktree-studio-theme-mode")).toBe("light");
  });

  it("applies both attributes, always with a resolved mode", () => {
    applyTheme({ family: "ledger", mode: "light" });
    expect(document.documentElement.getAttribute("data-theme")).toBe("ledger");
    expect(document.documentElement.getAttribute("data-mode")).toBe("light");

    applyTheme({ family: "deck", mode: "dark" });
    expect(document.documentElement.getAttribute("data-theme")).toBe("deck");
    expect(document.documentElement.getAttribute("data-mode")).toBe("dark");
  });

  it("never leaves data-mode as the literal string 'system'", () => {
    stubMatchMedia(true);
    applyTheme({ family: "graphite", mode: "system" });
    expect(document.documentElement.getAttribute("data-mode")).toBe("light");

    stubMatchMedia(false);
    applyTheme({ family: "graphite", mode: "system" });
    expect(document.documentElement.getAttribute("data-mode")).toBe("dark");
  });

  it("resolveMode passes explicit choices through untouched", () => {
    stubMatchMedia(true);
    expect(resolveMode("dark")).toBe("dark");
    expect(resolveMode("light")).toBe("light");
    expect(resolveMode("system")).toBe("light");
  });

  it("migrates the pre-redesign dark/light key into the mode axis", () => {
    localStorage.setItem("worktree-studio-theme", "light");
    // Family is new, so it takes the default; the mode they had chosen is
    // preserved rather than silently reset to "system".
    expect(getStoredTheme()).toEqual({ family: "graphite", mode: "light" });
  });

  it("prefers an explicitly stored mode over the legacy key", () => {
    localStorage.setItem("worktree-studio-theme", "light");
    localStorage.setItem("worktree-studio-theme-mode", "dark");
    expect(getStoredTheme().mode).toBe("dark");
  });

  it("ignores garbage stored values and falls back to the defaults", () => {
    localStorage.setItem("worktree-studio-theme-family", "solarized");
    localStorage.setItem("worktree-studio-theme-mode", "sepia");
    expect(getStoredTheme()).toEqual({ family: "graphite", mode: "system" });
  });

  it("watchSystemMode subscribes and unsubscribes", () => {
    const listeners: (() => void)[] = [];
    stubMatchMedia(false, listeners);

    let calls = 0;
    const stop = watchSystemMode(() => calls++);
    expect(listeners).toHaveLength(1);

    listeners[0]();
    expect(calls).toBe(1);

    stop();
    expect(listeners).toHaveLength(0);
  });

  it("watchSystemMode is a safe no-op where matchMedia is missing", () => {
    vi.stubGlobal("matchMedia", undefined);
    expect(() => watchSystemMode(() => {})()).not.toThrow();
  });
});
