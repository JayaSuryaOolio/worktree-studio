import { beforeEach, describe, expect, it } from "vitest";
import { getStoredFilesOpen, setStoredFilesOpen } from "./filesPanelPreference";

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
