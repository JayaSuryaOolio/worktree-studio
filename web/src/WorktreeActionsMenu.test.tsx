import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import WorktreeActionsMenu from "./WorktreeActionsMenu";
import { Worktree } from "./api";

const wt: Worktree = {
  id: "w1",
  repo_id: "r1",
  name: "feature",
  branch: "feature",
  path: "/tmp/x",
  created_at: "2026-01-01T00:00:00Z",
  status: "active",
};

// Regression test for a real reported bug: the sidebar renders this menu
// nested inside a react-router <Link> (the worktree row itself). The
// original code only called stopPropagation() in its click handler, which
// stops the click from reaching the Link's own onClick (the handler that
// would normally call preventDefault() and do client-side navigation
// instead) — but does NOT itself suppress the anchor's native default
// action. Net effect: clicking the kebab menu fell through to a real
// full-page navigation to the row's href. Tested here at the raw DOM
// event level (defaultPrevented), independent of react-router, since
// that's the exact mechanism of the bug — a MemoryRouter-based test
// wouldn't distinguish buggy from fixed (its navigation is driven
// entirely by the Link's own handler running at all, not by the
// underlying native default action).
describe("WorktreeActionsMenu nested inside an anchor", () => {
  it("prevents the ancestor anchor's native default navigation when the trigger is clicked", () => {
    render(
      <a href="/somewhere">
        <WorktreeActionsMenu
          wt={wt}
          spotlightStatus={undefined}
          onSpotlightStart={() => {}}
          onSpotlightStop={() => {}}
          onViewLog={() => {}}
          onArchive={() => {}}
        />
      </a>
    );
    const trigger = screen.getByRole("button", { name: /actions for feature/i });
    const event = new MouseEvent("click", { bubbles: true, cancelable: true });
    trigger.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
  });

  it("prevents the ancestor anchor's native default navigation when a menu item is clicked", () => {
    const onArchive = vi.fn();
    render(
      <a href="/somewhere">
        <WorktreeActionsMenu
          wt={wt}
          spotlightStatus={undefined}
          onSpotlightStart={() => {}}
          onSpotlightStop={() => {}}
          onViewLog={() => {}}
          onArchive={onArchive}
        />
      </a>
    );
    // Open the menu via a real click (state change only — no ancestor
    // concern for the trigger itself, already covered above). Wrapped in
    // act() so React 18's state update actually flushes before the next
    // query runs (plain .click() outside act() doesn't guarantee that).
    act(() => {
      screen.getByRole("button", { name: /actions for feature/i }).click();
    });
    const archiveItem = screen.getByRole("menuitem", { name: /archive/i });

    // Dispatch manually (rather than user-event) so we can inspect the
    // same event object's defaultPrevented immediately after dispatch —
    // by the time a synchronous dispatchEvent() call returns, React's
    // root-delegated handler has already run, so this reflects the real
    // outcome (unlike attaching our own listener directly to the target
    // node, which fires during native bubbling BEFORE the event reaches
    // React's actual listener up at the root — an ordering trap, not a
    // real bug, that an earlier version of this test fell into).
    const event = new MouseEvent("click", { bubbles: true, cancelable: true });
    act(() => {
      archiveItem.dispatchEvent(event);
    });

    expect(onArchive).toHaveBeenCalledTimes(1);
    expect(event.defaultPrevented).toBe(true);
  });

  it("closes when clicking outside", async () => {
    const user = userEvent.setup();
    render(
      <div>
        <a href="/somewhere">
          <WorktreeActionsMenu
            wt={wt}
            spotlightStatus={{ available: true, active: false }}
            onSpotlightStart={() => {}}
            onSpotlightStop={() => {}}
            onViewLog={() => {}}
            onArchive={() => {}}
          />
        </a>
        <button>outside</button>
      </div>
    );
    await user.click(screen.getByRole("button", { name: /actions for feature/i }));
    expect(screen.getByRole("menuitem", { name: /archive/i })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "outside" }));
    expect(screen.queryByRole("menuitem", { name: /archive/i })).not.toBeInTheDocument();
  });

  it("shows a spotlight start/stop item only when spotlight is available", async () => {
    const user = userEvent.setup();
    const { rerender } = render(
      <WorktreeActionsMenu
        wt={wt}
        spotlightStatus={{ available: false, active: false }}
        onSpotlightStart={() => {}}
        onSpotlightStop={() => {}}
        onViewLog={() => {}}
        onArchive={() => {}}
      />
    );
    await user.click(screen.getByRole("button", { name: /actions for feature/i }));
    expect(screen.queryByRole("menuitem", { name: /spotlight/i })).not.toBeInTheDocument();

    rerender(
      <WorktreeActionsMenu
        wt={wt}
        spotlightStatus={{ available: true, active: true }}
        onSpotlightStart={() => {}}
        onSpotlightStop={() => {}}
        onViewLog={() => {}}
        onArchive={() => {}}
      />
    );
    expect(screen.getByRole("menuitem", { name: /stop spotlight/i })).toBeInTheDocument();
  });
});
