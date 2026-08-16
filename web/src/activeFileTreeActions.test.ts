import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { registerActiveFileTreeActions, useActiveFileTreeActions } from "./activeFileTreeActions";

function makeActions(worktreeId: string, overrides: Partial<Parameters<typeof registerActiveFileTreeActions>[0]> = {}) {
  return {
    worktreeId,
    filterToChanged: false,
    changedFilesAvailable: true,
    toggleChangedFilesFilter: vi.fn(),
    ...overrides,
  };
}

afterEach(() => {
  registerActiveFileTreeActions(null);
});

describe("activeFileTreeActions", () => {
  it("returns null when nothing is registered", () => {
    const { result } = renderHook(() => useActiveFileTreeActions());
    expect(result.current).toBeNull();
  });

  it("reflects the currently registered actions", () => {
    const { result } = renderHook(() => useActiveFileTreeActions());
    const actions = makeActions("w1");
    act(() => {
      registerActiveFileTreeActions(actions);
    });
    expect(result.current).toBe(actions);
  });

  it("goes back to null once the file tree unmounts (unregisters)", () => {
    const { result } = renderHook(() => useActiveFileTreeActions());
    act(() => {
      registerActiveFileTreeActions(makeActions("w1"));
    });
    expect(result.current).not.toBeNull();

    act(() => {
      registerActiveFileTreeActions(null);
    });
    expect(result.current).toBeNull();
  });
});
