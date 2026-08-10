import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StatusScheduler } from "./statusScheduler";

// tickMs is set small in every test below so the fake-timer advances stay
// readable, independent of the real intervalMs/idleTimeoutMs under test.
const TICK_MS = 10;

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("StatusScheduler", () => {
  it("fetches immediately on first subscribe, and pushes the result to the listener", async () => {
    const fetcher = vi.fn().mockResolvedValue({ dirty: true });
    const scheduler = new StatusScheduler(fetcher, { intervalMs: 1000, idleTimeoutMs: 60_000, tickMs: TICK_MS });
    const listener = vi.fn();

    scheduler.subscribe("wt1", listener);
    await vi.waitFor(() => expect(fetcher).toHaveBeenCalledTimes(1));
    await vi.waitFor(() => expect(listener).toHaveBeenLastCalledWith({ data: { dirty: true }, fetchedAt: expect.any(Number), refreshing: false }));

    scheduler.dispose();
  });

  it("keeps refreshing on the interval while touched (e.g. subscribed)", async () => {
    const fetcher = vi.fn().mockResolvedValue({ n: 1 });
    const scheduler = new StatusScheduler(fetcher, { intervalMs: 100, idleTimeoutMs: 60_000, tickMs: TICK_MS });

    scheduler.subscribe("wt1", () => {});
    await vi.waitFor(() => expect(fetcher).toHaveBeenCalledTimes(1));

    await vi.advanceTimersByTimeAsync(150);
    expect(fetcher.mock.calls.length).toBeGreaterThanOrEqual(2);

    scheduler.dispose();
  });

  it("stops background refreshes once idleTimeoutMs has passed since the last touch", async () => {
    const fetcher = vi.fn().mockResolvedValue({ n: 1 });
    const scheduler = new StatusScheduler(fetcher, { intervalMs: 20, idleTimeoutMs: 50, tickMs: TICK_MS });

    scheduler.subscribe("wt1", () => {});
    await vi.waitFor(() => expect(fetcher).toHaveBeenCalledTimes(1));

    await vi.advanceTimersByTimeAsync(200); // well past idleTimeoutMs with no further touch
    const callsAfterIdle = fetcher.mock.calls.length;

    await vi.advanceTimersByTimeAsync(200);
    expect(fetcher.mock.calls.length).toBe(callsAfterIdle); // no further calls once idle

    scheduler.dispose();
  });

  it("un-idles and resumes background refresh once touched again", async () => {
    const fetcher = vi.fn().mockResolvedValue({ n: 1 });
    const scheduler = new StatusScheduler(fetcher, { intervalMs: 20, idleTimeoutMs: 50, tickMs: TICK_MS });

    scheduler.subscribe("wt1", () => {});
    await vi.waitFor(() => expect(fetcher).toHaveBeenCalledTimes(1));
    await vi.advanceTimersByTimeAsync(200);
    const callsAfterIdle = fetcher.mock.calls.length;

    scheduler.touch("wt1");
    await vi.advanceTimersByTimeAsync(100);
    expect(fetcher.mock.calls.length).toBeGreaterThan(callsAfterIdle);

    scheduler.dispose();
  });

  it("still returns last-known data via peek() for an idle id, without refetching", async () => {
    const fetcher = vi.fn().mockResolvedValue({ n: 42 });
    const scheduler = new StatusScheduler(fetcher, { intervalMs: 20, idleTimeoutMs: 50, tickMs: TICK_MS });

    scheduler.subscribe("wt1", () => {});
    await vi.waitFor(() => expect(fetcher).toHaveBeenCalledTimes(1));
    await vi.advanceTimersByTimeAsync(200);
    const callsAfterIdle = fetcher.mock.calls.length;

    expect(scheduler.peek("wt1")?.data).toEqual({ n: 42 });
    expect(fetcher.mock.calls.length).toBe(callsAfterIdle);

    scheduler.dispose();
  });

  it("refreshNow fetches immediately and defers the next scheduled tick by intervalMs", async () => {
    const fetcher = vi.fn().mockResolvedValue({ n: 1 });
    const scheduler = new StatusScheduler(fetcher, { intervalMs: 100, idleTimeoutMs: 60_000, tickMs: TICK_MS });

    scheduler.subscribe("wt1", () => {});
    await vi.waitFor(() => expect(fetcher).toHaveBeenCalledTimes(1));

    await scheduler.refreshNow("wt1");
    expect(fetcher).toHaveBeenCalledTimes(2);

    // Immediately after refreshNow, the background heartbeat should NOT
    // fire again right away — the whole point of refreshNow is that it
    // doesn't get redundantly followed by a background tick.
    await vi.advanceTimersByTimeAsync(30);
    expect(fetcher).toHaveBeenCalledTimes(2);

    scheduler.dispose();
  });

  it("reports refreshing:true to listeners while a fetch is in flight", async () => {
    let resolveFetch: (value: { ok: boolean }) => void = () => {};
    const fetcher = vi.fn().mockImplementation(
      () => new Promise<{ ok: boolean }>((resolve) => (resolveFetch = resolve))
    );
    const scheduler = new StatusScheduler(fetcher, { intervalMs: 1000, idleTimeoutMs: 60_000, tickMs: TICK_MS });
    const snapshots: boolean[] = [];

    scheduler.subscribe("wt1", (snap) => snapshots.push(snap.refreshing));
    expect(snapshots).toContain(true);

    resolveFetch({ ok: true });
    await vi.waitFor(() => expect(snapshots[snapshots.length - 1]).toBe(false));

    scheduler.dispose();
  });

  it("does not overlap two fetches for the same id if one is already in flight", async () => {
    let resolveFetch: (value: { ok: boolean }) => void = () => {};
    const fetcher = vi.fn().mockImplementation(
      () => new Promise<{ ok: boolean }>((resolve) => (resolveFetch = resolve))
    );
    const scheduler = new StatusScheduler(fetcher, { intervalMs: 5, idleTimeoutMs: 60_000, tickMs: TICK_MS });

    scheduler.subscribe("wt1", () => {});
    await vi.advanceTimersByTimeAsync(50); // several heartbeats' worth, first fetch still unresolved
    expect(fetcher).toHaveBeenCalledTimes(1);

    resolveFetch({ ok: true });
    await vi.advanceTimersByTimeAsync(0); // flush the resolved fetch's .finally()
    expect(scheduler.peek("wt1")?.refreshing).toBe(false);

    scheduler.dispose();
  });

  it("leaves last-known data in place and keeps retrying after a failed fetch", async () => {
    const fetcher = vi.fn().mockRejectedValueOnce(new Error("boom")).mockResolvedValue({ n: 7 });
    const scheduler = new StatusScheduler(fetcher, { intervalMs: 20, idleTimeoutMs: 60_000, tickMs: TICK_MS });

    scheduler.subscribe("wt1", () => {});
    await vi.waitFor(() => expect(fetcher).toHaveBeenCalledTimes(1));
    expect(scheduler.peek("wt1")?.data).toBeNull();

    await vi.advanceTimersByTimeAsync(30);
    await vi.waitFor(() => expect(scheduler.peek("wt1")?.data).toEqual({ n: 7 }));

    scheduler.dispose();
  });

  it("setPaused(true) stops the heartbeat; setPaused(false) resumes and catches up immediately", async () => {
    const fetcher = vi.fn().mockResolvedValue({ n: 1 });
    const scheduler = new StatusScheduler(fetcher, { intervalMs: 20, idleTimeoutMs: 60_000, tickMs: TICK_MS });

    scheduler.subscribe("wt1", () => {});
    await vi.waitFor(() => expect(fetcher).toHaveBeenCalledTimes(1));

    scheduler.setPaused(true);
    await vi.advanceTimersByTimeAsync(200);
    expect(fetcher).toHaveBeenCalledTimes(1); // no background ticks while paused

    scheduler.setPaused(false);
    await vi.waitFor(() => expect(fetcher).toHaveBeenCalledTimes(2)); // catches up right away on resume

    scheduler.dispose();
  });
});
