import { beforeEach, describe, expect, it } from "vitest";
import {
  clampFilesPanelWidth,
  FILES_PANEL_DEFAULT_WIDTH,
  FILES_PANEL_MAX_WIDTH,
  FILES_PANEL_MIN_WIDTH,
  getStoredFilesOpen,
  getStoredFilesSide,
  getStoredFilesWidth,
  setStoredFilesOpen,
  setStoredFilesSide,
  setStoredFilesWidth,
} from "./filesPanelPreference";

beforeEach(() => {
  localStorage.clear();
});

describe("filesPanelPreference", () => {
  it("defaults to open with nothing stored", () => {
    expect(getStoredFilesOpen()).toBe(true);
  });

  it("persists false and reads it back", () => {
    setStoredFilesOpen(false);
    expect(getStoredFilesOpen()).toBe(false);
  });

  it("persists true and reads it back", () => {
    setStoredFilesOpen(false);
    setStoredFilesOpen(true);
    expect(getStoredFilesOpen()).toBe(true);
  });
});

describe("filesPanelPreference side", () => {
  it("defaults to the right with nothing stored", () => {
    expect(getStoredFilesSide()).toBe("right");
  });

  it("persists left and reads it back", () => {
    setStoredFilesSide("left");
    expect(getStoredFilesSide()).toBe("left");
  });

  it("falls back to the right for a junk stored value", () => {
    localStorage.setItem("worktree-studio-files-side", "sideways");
    expect(getStoredFilesSide()).toBe("right");
  });
});

describe("filesPanelPreference width", () => {
  it("defaults with nothing stored", () => {
    expect(getStoredFilesWidth()).toBe(FILES_PANEL_DEFAULT_WIDTH);
  });

  it("persists a width and reads it back", () => {
    setStoredFilesWidth(333);
    expect(getStoredFilesWidth()).toBe(333);
  });

  it("clamps to the allowed range, on the way in and on the way out", () => {
    expect(clampFilesPanelWidth(10)).toBe(FILES_PANEL_MIN_WIDTH);
    expect(clampFilesPanelWidth(9999)).toBe(FILES_PANEL_MAX_WIDTH);
    expect(clampFilesPanelWidth(Number.NaN)).toBe(FILES_PANEL_DEFAULT_WIDTH);

    setStoredFilesWidth(9999);
    expect(getStoredFilesWidth()).toBe(FILES_PANEL_MAX_WIDTH);
  });

  it("falls back to the default for a junk stored value", () => {
    localStorage.setItem("worktree-studio-files-width", "wide-ish");
    expect(getStoredFilesWidth()).toBe(FILES_PANEL_DEFAULT_WIDTH);
  });
});
