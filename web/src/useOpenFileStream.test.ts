import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useOpenFileStream } from "./useOpenFileStream";

vi.mock("./api", () => ({
  openFileWsUrl: vi.fn(() => "ws://localhost/ws/open-file"),
}));

// Same fake-socket harness as useAttentionStream.test.ts.
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

describe("useOpenFileStream", () => {
  it("invokes the callback for an open-file message", () => {
    const onOpenFile = vi.fn();
    renderHook(() => useOpenFileStream(onOpenFile));

    act(() => latestSocket().emit({ type: "open-file", worktree_id: "wt1", path: "src/main.go" }));

    expect(onOpenFile).toHaveBeenCalledWith("wt1", "src/main.go");
  });

  it("ignores messages of an unknown type", () => {
    const onOpenFile = vi.fn();
    renderHook(() => useOpenFileStream(onOpenFile));

    act(() => latestSocket().emit({ type: "something-else" }));

    expect(onOpenFile).not.toHaveBeenCalled();
  });

  it("reconnects after the socket closes", () => {
    vi.useFakeTimers();
    renderHook(() => useOpenFileStream(vi.fn()));
    expect(FakeWebSocket.instances).toHaveLength(1);

    act(() => {
      latestSocket().onclose?.();
      vi.advanceTimersByTime(3000);
    });

    expect(FakeWebSocket.instances).toHaveLength(2);
  });

  it("does not reconnect after unmount", () => {
    vi.useFakeTimers();
    const { unmount } = renderHook(() => useOpenFileStream(vi.fn()));
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
