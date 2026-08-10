import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useTransientIndicator } from "./useTransientIndicator";

const OPTS = { showDelayMs: 900, visibleMs: 1000, fadeMs: 300 };

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("useTransientIndicator", () => {
  it("stays hidden if active flips back to false before showDelayMs", () => {
    const { result, rerender } = renderHook(({ active }) => useTransientIndicator(active, OPTS), {
      initialProps: { active: true },
    });
    expect(result.current).toBe("hidden");

    act(() => vi.advanceTimersByTime(500));
    rerender({ active: false });
    act(() => vi.advanceTimersByTime(500));

    expect(result.current).toBe("hidden");
  });

  it("becomes visible only after active has held true for showDelayMs", () => {
    const { result } = renderHook(({ active }) => useTransientIndicator(active, OPTS), {
      initialProps: { active: true },
    });

    act(() => vi.advanceTimersByTime(899));
    expect(result.current).toBe("hidden");

    act(() => vi.advanceTimersByTime(1));
    expect(result.current).toBe("visible");
  });

  it("stays visible for visibleMs even if active goes false immediately after showing", () => {
    const { result, rerender } = renderHook(({ active }) => useTransientIndicator(active, OPTS), {
      initialProps: { active: true },
    });

    act(() => vi.advanceTimersByTime(900));
    expect(result.current).toBe("visible");

    rerender({ active: false });
    act(() => vi.advanceTimersByTime(999));
    expect(result.current).toBe("visible");

    act(() => vi.advanceTimersByTime(1));
    expect(result.current).toBe("fading");
  });

  it("hides after fading out for fadeMs", () => {
    const { result, rerender } = renderHook(({ active }) => useTransientIndicator(active, OPTS), {
      initialProps: { active: true },
    });

    act(() => vi.advanceTimersByTime(900));
    rerender({ active: false });
    act(() => vi.advanceTimersByTime(1000));
    expect(result.current).toBe("fading");

    act(() => vi.advanceTimersByTime(299));
    expect(result.current).toBe("fading");

    act(() => vi.advanceTimersByTime(1));
    expect(result.current).toBe("hidden");
  });

  it("re-activating while fading brings it back to visible instead of hiding then re-debouncing", () => {
    const { result, rerender } = renderHook(({ active }) => useTransientIndicator(active, OPTS), {
      initialProps: { active: true },
    });

    act(() => vi.advanceTimersByTime(900));
    rerender({ active: false });
    act(() => vi.advanceTimersByTime(1000));
    expect(result.current).toBe("fading");

    rerender({ active: true });
    expect(result.current).toBe("visible");

    // The visible-duration timer restarted too — still visible well past
    // the original fade point.
    act(() => vi.advanceTimersByTime(999));
    expect(result.current).toBe("visible");
  });
});
