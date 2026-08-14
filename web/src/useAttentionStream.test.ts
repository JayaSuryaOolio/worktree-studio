import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useAttentionStream } from "./useAttentionStream";

vi.mock("./api", () => ({
  attentionWsUrl: vi.fn(() => "ws://localhost/ws/attention"),
}));

// A minimal fake WebSocket — real browser sockets can't be driven
// synchronously from a test, so this stands in for one: it records itself
// on `instances` so a test can grab the most recent one and fire its
// onmessage/onclose handlers directly, the same way a real socket would
// call them asynchronously.
class FakeWebSocket {
  static instances: FakeWebSocket[] = [];
  onmessage: ((event: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  closed = false;

  constructor(public url: string) {
    FakeWebSocket.instances.push(this);
  }
  close() {
    this.closed = true;
  }
  emit(data: unknown) {
    this.onmessage?.({ data: JSON.stringify(data) });
  }
}

beforeEach(() => {
  FakeWebSocket.instances = [];
  vi.stubGlobal("WebSocket", FakeWebSocket);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

function latestSocket(): FakeWebSocket {
  const ws = FakeWebSocket.instances[FakeWebSocket.instances.length - 1];
  if (!ws) throw new Error("no WebSocket was constructed");
  return ws;
}

describe("useAttentionStream", () => {
  it("adopts the initial snapshot", () => {
    const { result } = renderHook(() => useAttentionStream());
    act(() => latestSocket().emit({ type: "snapshot", pending: { wt1: "waiting" } }));
    expect(result.current).toEqual({ wt1: "waiting" });
  });

  it("adds a worktree on a pending update and calls onUpdate", () => {
    const onUpdate = vi.fn();
    const { result } = renderHook(() => useAttentionStream(onUpdate));
    act(() => latestSocket().emit({ type: "snapshot", pending: {} }));
    act(() => latestSocket().emit({ type: "update", worktree_id: "wt1", pending: true, message: "waiting" }));

    expect(result.current).toEqual({ wt1: "waiting" });
    expect(onUpdate).toHaveBeenCalledWith("wt1", true, "waiting");
  });

  it("removes a worktree on a pending:false update", () => {
    const { result } = renderHook(() => useAttentionStream());
    act(() => latestSocket().emit({ type: "snapshot", pending: { wt1: "waiting" } }));
    act(() => latestSocket().emit({ type: "update", worktree_id: "wt1", pending: false }));

    expect(result.current).toEqual({});
  });

  it("reconnects after the socket closes", () => {
    vi.useFakeTimers();
    renderHook(() => useAttentionStream());
    expect(FakeWebSocket.instances).toHaveLength(1);

    act(() => {
      latestSocket().onclose?.();
      vi.advanceTimersByTime(3000);
    });

    expect(FakeWebSocket.instances).toHaveLength(2);
  });

  it("does not reconnect after unmount", () => {
    vi.useFakeTimers();
    const { unmount } = renderHook(() => useAttentionStream());
    const ws = latestSocket();

    unmount();
    expect(ws.closed).toBe(true);

    act(() => {
      ws.onclose?.();
      vi.advanceTimersByTime(5000);
    });
    expect(FakeWebSocket.instances).toHaveLength(1);
  });
});
