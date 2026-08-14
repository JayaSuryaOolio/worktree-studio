import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useAttentionBlink } from "./useAttentionBlink";

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("useAttentionBlink", () => {
  it("starts a newly-pending worktree blinking", () => {
    const { result } = renderHook(() => useAttentionBlink(["wt1"]));
    expect(result.current.has("wt1")).toBe(true);
  });

  it("stops blinking after 10 seconds while still pending", () => {
    const { result, rerender } = renderHook(({ ids }) => useAttentionBlink(ids), {
      initialProps: { ids: ["wt1"] },
    });
    expect(result.current.has("wt1")).toBe(true);

    act(() => {
      vi.advanceTimersByTime(10_000);
    });
    rerender({ ids: ["wt1"] });

    expect(result.current.has("wt1")).toBe(false);
  });

  it("clears blink state when a worktree is no longer pending", () => {
    const { result, rerender } = renderHook(({ ids }) => useAttentionBlink(ids), {
      initialProps: { ids: ["wt1"] },
    });
    expect(result.current.has("wt1")).toBe(true);

    rerender({ ids: [] });
    expect(result.current.has("wt1")).toBe(false);
  });

  it("gives a re-pending worktree a fresh blink window", () => {
    const { result, rerender } = renderHook(({ ids }) => useAttentionBlink(ids), {
      initialProps: { ids: ["wt1"] },
    });

    act(() => {
      vi.advanceTimersByTime(10_000);
    });
    rerender({ ids: ["wt1"] });
    expect(result.current.has("wt1")).toBe(false);

    // Cleared, then pending again — should blink anew.
    rerender({ ids: [] });
    rerender({ ids: ["wt1"] });
    expect(result.current.has("wt1")).toBe(true);
  });

  it("tracks multiple worktrees independently", () => {
    const { result, rerender } = renderHook(({ ids }) => useAttentionBlink(ids), {
      initialProps: { ids: ["wt1"] },
    });

    act(() => {
      vi.advanceTimersByTime(5_000);
    });
    rerender({ ids: ["wt1", "wt2"] });
    expect(result.current.has("wt1")).toBe(true);
    expect(result.current.has("wt2")).toBe(true);

    act(() => {
      vi.advanceTimersByTime(5_000);
    });
    rerender({ ids: ["wt1", "wt2"] });
    // wt1 has been pending 10s total and should have stopped; wt2 has
    // only had 5s and should still be blinking.
    expect(result.current.has("wt1")).toBe(false);
    expect(result.current.has("wt2")).toBe(true);
  });
});
