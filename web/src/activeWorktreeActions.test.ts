import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { registerActiveWorktreeActions, useActiveWorktreeActions } from "./activeWorktreeActions";

function makeActions(worktreeId: string, overrides: Partial<Parameters<typeof registerActiveWorktreeActions>[0]> = {}) {
  return {
    worktreeId,
    filesOpen: true,
    toggleFiles: vi.fn(),
    vscodeAvailable: true,
    openVSCode: vi.fn(),
    openLog: vi.fn(),
    newTerminal: vi.fn(),
    splitRight: vi.fn(),
    splitDown: vi.fn(),
    ...overrides,
  };
}

afterEach(() => {
  registerActiveWorktreeActions(null);
});

describe("activeWorktreeActions", () => {
  it("returns null when nothing is registered", () => {
    const { result } = renderHook(() => useActiveWorktreeActions());
    expect(result.current).toBeNull();
  });

  it("reflects the currently registered actions", () => {
    const { result } = renderHook(() => useActiveWorktreeActions());
    const actions = makeActions("w1");
    act(() => {
      registerActiveWorktreeActions(actions);
    });
    expect(result.current).toBe(actions);
  });

  it("picks up a re-registration (e.g. filesOpen flipping) without remounting", () => {
    const { result } = renderHook(() => useActiveWorktreeActions());
    act(() => {
      registerActiveWorktreeActions(makeActions("w1", { filesOpen: false }));
    });
    expect(result.current?.filesOpen).toBe(false);

    act(() => {
      registerActiveWorktreeActions(makeActions("w1", { filesOpen: true }));
    });
    expect(result.current?.filesOpen).toBe(true);
  });

  it("goes back to null once unregistered", () => {
    const { result } = renderHook(() => useActiveWorktreeActions());
    act(() => {
      registerActiveWorktreeActions(makeActions("w1"));
    });
    expect(result.current).not.toBeNull();

    act(() => {
      registerActiveWorktreeActions(null);
    });
    expect(result.current).toBeNull();
  });
});
