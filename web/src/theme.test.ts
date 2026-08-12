import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { applyTheme, getStoredTheme, setTheme } from "./theme";

beforeEach(() => {
  localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
});

afterEach(() => {
  localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
});

describe("theme", () => {
  it("defaults to dark with nothing stored", () => {
    expect(getStoredTheme()).toBe("dark");
  });

  it("setTheme persists the choice and getStoredTheme reflects it", () => {
    setTheme("light");
    expect(getStoredTheme()).toBe("light");
    expect(localStorage.getItem("worktree-studio-theme")).toBe("light");
  });

  it("applyTheme sets data-theme=light for light, and removes it for dark", () => {
    applyTheme("light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");

    applyTheme("dark");
    expect(document.documentElement.hasAttribute("data-theme")).toBe(false);
  });

  it("ignores garbage stored values and falls back to dark", () => {
    localStorage.setItem("worktree-studio-theme", "solarized");
    expect(getStoredTheme()).toBe("dark");
  });
});
