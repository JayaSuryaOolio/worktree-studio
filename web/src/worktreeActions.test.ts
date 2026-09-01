import { describe, expect, it, vi } from "vitest";

vi.mock("./api", () => ({
  ConflictError: class ConflictError extends Error {},
  deleteWorktree: vi.fn(),
  pinWorktree: vi.fn(),
  startSpotlight: vi.fn(),
  stopSpotlight: vi.fn(),
  unpinWorktree: vi.fn(),
}));

import { ConflictError, pinWorktree, startSpotlight, unpinWorktree } from "./api";
import { startSpotlightWithFriendlyError, toggleWorktreePinSafe } from "./worktreeActions";

const worktree = {
  id: "wt1",
  repo_id: "r1",
  name: "feature",
  branch: "feature",
  path: "/tmp/wt1",
  created_at: "2026-01-01T00:00:00Z",
  status: "active" as const,
};

describe("startSpotlightWithFriendlyError", () => {
  it("calls onDone on a plain success", async () => {
    vi.mocked(startSpotlight).mockResolvedValue({ root: "/tmp/root" });
    const onDone = vi.fn();
    const onError = vi.fn();

    await startSpotlightWithFriendlyError(worktree, { onDone, onError });

    expect(startSpotlight).toHaveBeenCalledWith("r1", "wt1");
    expect(onDone).toHaveBeenCalled();
    expect(onError).not.toHaveBeenCalled();
  });

  it("on a dirty-root conflict, confirms then retries with stash=true, and calls onDone on success", async () => {
    vi.mocked(startSpotlight).mockRejectedValueOnce(new ConflictError("dirty"));
    vi.mocked(startSpotlight).mockResolvedValueOnce({ root: "/tmp/root" });
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    const onDone = vi.fn();
    const onError = vi.fn();

    await startSpotlightWithFriendlyError(worktree, { onDone, onError });

    expect(confirmSpy).toHaveBeenCalled();
    // Second call is the stash=true retry — the non-interactive "yes" to
    // the prompt confirm() just stood in for.
    expect(startSpotlight).toHaveBeenNthCalledWith(2, "r1", "wt1", true);
    expect(onDone).toHaveBeenCalled();
    expect(onError).not.toHaveBeenCalled();
    confirmSpy.mockRestore();
  });

  it("on a dirty-root conflict, does nothing further if the user declines the confirm", async () => {
    vi.mocked(startSpotlight).mockRejectedValueOnce(new ConflictError("dirty"));
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(false);
    const onDone = vi.fn();
    const onError = vi.fn();

    await startSpotlightWithFriendlyError(worktree, { onDone, onError });

    expect(startSpotlight).toHaveBeenCalledTimes(1); // no retry attempted
    expect(onDone).not.toHaveBeenCalled();
    expect(onError).not.toHaveBeenCalled();
    confirmSpy.mockRestore();
  });

  it("surfaces the retry's own error if the stash=true retry itself fails", async () => {
    vi.mocked(startSpotlight).mockRejectedValueOnce(new ConflictError("dirty"));
    vi.mocked(startSpotlight).mockRejectedValueOnce(new Error("stash failed"));
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    const onDone = vi.fn();
    const onError = vi.fn();

    await startSpotlightWithFriendlyError(worktree, { onDone, onError });

    expect(onDone).not.toHaveBeenCalled();
    expect(onError).toHaveBeenCalledWith("stash failed");
    confirmSpy.mockRestore();
  });
});

describe("toggleWorktreePinSafe", () => {
  it("calls pinWorktree when nextPinned is true, and onDone on success", async () => {
    vi.mocked(pinWorktree).mockResolvedValue({ pinned: true });
    const onDone = vi.fn();
    const onError = vi.fn();

    await toggleWorktreePinSafe(worktree, true, { onDone, onError });

    expect(pinWorktree).toHaveBeenCalledWith("r1", "wt1");
    expect(unpinWorktree).not.toHaveBeenCalled();
    expect(onDone).toHaveBeenCalled();
    expect(onError).not.toHaveBeenCalled();
  });

  it("calls unpinWorktree when nextPinned is false", async () => {
    vi.mocked(unpinWorktree).mockResolvedValue({ pinned: false });
    const onDone = vi.fn();
    const onError = vi.fn();

    await toggleWorktreePinSafe(worktree, false, { onDone, onError });

    expect(unpinWorktree).toHaveBeenCalledWith("r1", "wt1");
    expect(pinWorktree).not.toHaveBeenCalled();
    expect(onDone).toHaveBeenCalled();
  });

  it("surfaces an error via onError without throwing, e.g. a pinned worktree already blocked from archive elsewhere", async () => {
    vi.mocked(pinWorktree).mockRejectedValueOnce(new Error("failed to update worktree pin state"));
    const onDone = vi.fn();
    const onError = vi.fn();

    await toggleWorktreePinSafe(worktree, true, { onDone, onError });

    expect(onDone).not.toHaveBeenCalled();
    expect(onError).toHaveBeenCalledWith("failed to update worktree pin state");
  });
});
