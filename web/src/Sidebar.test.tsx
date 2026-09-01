import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import Sidebar from "./Sidebar";
import { RepoProvider } from "./RepoContext";

vi.mock("./api", () => ({
  listRepos: vi.fn(),
  listWorktrees: vi.fn(),
  getWorktreeStatus: vi.fn(),
  getSpotlightStatus: vi.fn(),
  getWorktreeSummary: vi.fn(),
  startSpotlight: vi.fn(),
  deleteWorktree: vi.fn(),
  archiveWorktree: vi.fn(),
  pinWorktree: vi.fn(),
  unpinWorktree: vi.fn(),
  attentionWsUrl: vi.fn(() => "ws://localhost/ws/attention"),
  openFileWsUrl: vi.fn(() => "ws://localhost/ws/open-file"),
  clearAttention: vi.fn(),
}));

import {
  deleteWorktree,
  getSpotlightStatus,
  getWorktreeStatus,
  getWorktreeSummary,
  listRepos,
  listWorktrees,
  pinWorktree,
  startSpotlight,
  unpinWorktree,
} from "./api";

// Renders the current route so tests can assert a navigate() call actually
// happened, without needing a real Route/WorktreeDetail mounted for it.
function LocationDisplay() {
  const location = useLocation();
  return <div data-testid="location">{location.pathname}</div>;
}

function Harness({ initialPath = "/repo/r1/worktree/w1" }: { initialPath?: string }) {
  return (
    <MemoryRouter initialEntries={[initialPath]}>
      <RepoProvider>
        <Sidebar onAddRepo={vi.fn()} onNewWorktree={vi.fn()} />
        <LocationDisplay />
      </RepoProvider>
    </MemoryRouter>
  );
}

beforeEach(() => {
  localStorage.clear();
  vi.mocked(listRepos).mockResolvedValue([{ id: "r1", name: "adelaide", path: "/tmp/adelaide" }]);
  vi.mocked(listWorktrees).mockResolvedValue([
    { id: "w1", repo_id: "r1", name: "feature-worktree", branch: "feature", path: "/tmp/adelaide-wt/feature", created_at: "2026-01-01T00:00:00Z", status: "active" },
  ]);
  vi.mocked(getWorktreeStatus).mockResolvedValue({ branch: "feature", dirty: false, has_upstream: false, ahead: 0, behind: 0 });
  vi.mocked(getSpotlightStatus).mockResolvedValue({ available: false, active: false });
  vi.mocked(getWorktreeSummary).mockResolvedValue({
    branch: "feature",
    ahead: 0,
    behind: 0,
    has_upstream: false,
    dirty: false,
    changed_files: [],
    pr: null,
  });
});

describe("Sidebar worktree row", () => {
  // The filter is the control you reach for BECAUSE the list is long, so
  // it can't live in the same scroll container as the list.
  it("keeps the filter and the Repos heading out of the scrolling region", async () => {
    render(<Harness />);
    await screen.findByTitle("feature");

    const pinned = document.querySelector(".sidebar-top")!;
    const scroller = document.querySelector(".sidebar-scroll")!;

    expect(pinned.contains(screen.getByLabelText("Filter worktrees"))).toBe(true);
    expect(pinned.contains(screen.getByText("Repos"))).toBe(true);
    expect(scroller.contains(screen.getByTitle("feature"))).toBe(true);
    expect(scroller.contains(screen.getByLabelText("Filter worktrees"))).toBe(false);
  });

  it("the filter narrows the list across repos, and Escape clears it", async () => {
    vi.mocked(listWorktrees).mockResolvedValue([
      { id: "w1", repo_id: "r1", name: "feature-worktree", branch: "feature", path: "/tmp/adelaide-wt/feature", created_at: "2026-01-01T00:00:00Z", status: "active" },
      { id: "w2", repo_id: "r1", name: "hotfix-worktree", branch: "hotfix-1", path: "/tmp/adelaide-wt/hotfix", created_at: "2026-01-01T00:00:00Z", status: "active" },
    ]);
    const user = userEvent.setup();
    render(<Harness />);

    await screen.findByTitle("feature");
    const filter = screen.getByLabelText("Filter worktrees");

    await user.type(filter, "hotfix");
    expect(screen.queryByTitle("feature")).not.toBeInTheDocument();
    expect(screen.getByTitle("hotfix-1")).toBeInTheDocument();

    // First Escape clears, and deliberately does NOT also blur — losing
    // focus as well would make fixing a typo a two-step recovery.
    await user.keyboard("{Escape}");
    expect(filter).toHaveValue("");
    expect(await screen.findByTitle("feature")).toBeInTheDocument();
  });

  it("collapsing a repo hides its worktrees and shows a count instead", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await screen.findByTitle("feature");

    await user.click(screen.getByRole("button", { name: /collapse adelaide/i }));

    expect(screen.queryByTitle("feature")).not.toBeInTheDocument();
    expect(screen.getByText("1")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /expand adelaide/i }));
    expect(await screen.findByTitle("feature")).toBeInTheDocument();
  });

  // A filter that silently skipped collapsed groups would be worse than no
  // filter at all, so narrowing overrides the stored collapsed state.
  it("a filter match surfaces even from inside a collapsed repo", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await screen.findByTitle("feature");

    await user.click(screen.getByRole("button", { name: /collapse adelaide/i }));
    expect(screen.queryByTitle("feature")).not.toBeInTheDocument();

    await user.type(screen.getByLabelText("Filter worktrees"), "feat");
    expect(await screen.findByTitle("feature")).toBeInTheDocument();
  });

  // jsdom can't tell us where the ellipsis lands, but it can tell us the
  // row still spells the branch name exactly — which is the thing the
  // head/tail split could plausibly break (a stray JSX space between the
  // spans, a dropped or doubled character at the seam).
  it("renders a long branch name split for middle-truncation, without altering it", async () => {
    vi.mocked(listWorktrees).mockResolvedValue([
      {
        id: "w1",
        repo_id: "r1",
        name: "hotfix-worktree",
        branch: "hotfix-backend-services",
        path: "/tmp/adelaide-wt/hotfix",
        created_at: "2026-01-01T00:00:00Z",
        status: "active",
      },
    ]);
    render(<Harness />);

    const label = await screen.findByTitle("hotfix-backend-services");
    expect(label.textContent).toBe("hotfix-backend-services");
    expect(label.querySelector(".sidebar-worktree-branch-head")).toHaveTextContent("hotfix-backend");
    expect(label.querySelector(".sidebar-worktree-branch-tail")).toHaveTextContent("-services");
  });

  it("clicking the already-active row toggles the card open, then closed again", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    const row = await screen.findByText("feature");
    // The card's content is always in the DOM (the smooth expand/collapse
    // is a pure CSS grid-rows transition, not conditional rendering — see
    // sidebar.css), so the expand toggle's aria-expanded is what actually
    // reflects collapsed/expanded state, not element presence.
    const toggle = () => screen.getByRole("button", { name: /(expand|collapse) feature/i });
    expect(toggle()).toHaveAttribute("aria-expanded", "false");

    await user.click(row);
    expect(toggle()).toHaveAttribute("aria-expanded", "true");

    await user.click(row);
    expect(toggle()).toHaveAttribute("aria-expanded", "false");
  });

  it("clicking a non-active row navigates instead of expanding", async () => {
    vi.mocked(listWorktrees).mockResolvedValue([
      { id: "w1", repo_id: "r1", name: "feature-worktree", branch: "feature", path: "/tmp/adelaide-wt/feature", created_at: "2026-01-01T00:00:00Z", status: "active" },
      { id: "w2", repo_id: "r1", name: "other-worktree", branch: "other", path: "/tmp/adelaide-wt/other", created_at: "2026-01-01T00:00:00Z", status: "active" },
    ]);
    const user = userEvent.setup();
    render(<Harness />);

    const otherRow = await screen.findByText("other");
    await user.click(otherRow);

    // Navigating away from w1 to w2 is real navigation, not a toggle —
    // neither row's card should have expanded as a side effect.
    expect(screen.getByRole("button", { name: /expand feature/i })).toHaveAttribute(
      "aria-expanded",
      "false"
    );
    expect(screen.getByRole("button", { name: /expand other/i })).toHaveAttribute(
      "aria-expanded",
      "false"
    );
  });
});

describe("Sidebar spotlight start", () => {
  it("navigates to the repo root worktree to open a shell there once spotlight starts", async () => {
    vi.mocked(getSpotlightStatus).mockResolvedValue({ available: true, active: false });
    vi.mocked(startSpotlight).mockResolvedValue({ root: "/tmp/adelaide" });
    const user = userEvent.setup();
    render(<Harness />);

    const startButton = await screen.findByTitle("Start spotlight");
    await user.click(startButton);

    // No WorktreeDetail is mounted in this harness (the root worktree's tab
    // isn't already open), so the fallback path — navigate there and leave
    // a pendingNewTerminal instruction for it to pick up — is what should
    // fire; see Sidebar.tsx's openShellAtRepoRoot.
    await waitFor(() => {
      expect(screen.getByTestId("location")).toHaveTextContent("/repo/r1/worktree/root-r1");
    });
  });
});

describe("Sidebar worktree pin", () => {
  it("shows the pin indicator on the collapsed row for a pinned worktree, not an unpinned one", async () => {
    vi.mocked(listWorktrees).mockResolvedValue([
      { id: "w1", repo_id: "r1", name: "feature-worktree", branch: "feature", path: "/tmp/adelaide-wt/feature", created_at: "2026-01-01T00:00:00Z", status: "active", pinned: true },
    ]);
    render(<Harness />);

    expect(await screen.findByTitle("Pinned — never archived")).toBeInTheDocument();
  });

  it("has no pin indicator for an unpinned worktree", async () => {
    render(<Harness />);
    await screen.findByTitle("feature");

    expect(screen.queryByTitle("Pinned — never archived")).not.toBeInTheDocument();
  });

  it("clicking the pin toggle pins an unpinned worktree", async () => {
    vi.mocked(pinWorktree).mockResolvedValue({ pinned: true });
    const user = userEvent.setup();
    render(<Harness />);

    await user.click(await screen.findByText("feature"));
    await user.click(screen.getByTitle(/^Pin worktree/));

    expect(pinWorktree).toHaveBeenCalledWith("r1", "w1");
  });

  it("clicking the pin toggle unpins an already-pinned worktree", async () => {
    vi.mocked(listWorktrees).mockResolvedValue([
      { id: "w1", repo_id: "r1", name: "feature-worktree", branch: "feature", path: "/tmp/adelaide-wt/feature", created_at: "2026-01-01T00:00:00Z", status: "active", pinned: true },
    ]);
    vi.mocked(unpinWorktree).mockResolvedValue({ pinned: false });
    const user = userEvent.setup();
    render(<Harness />);

    await user.click(await screen.findByText("feature"));
    await user.click(screen.getByTitle("Unpin worktree"));

    expect(unpinWorktree).toHaveBeenCalledWith("r1", "w1");
  });

  // The actual lifecycle rule ("will never be archived") is enforced
  // server-side (see internal/api's CanArchiveWorktree) — this only
  // covers the frontend half: the button is disabled and explains why,
  // rather than letting a click reach the server just to be refused.
  it("disables Archive with an explanatory tooltip for a pinned worktree", async () => {
    vi.mocked(listWorktrees).mockResolvedValue([
      { id: "w1", repo_id: "r1", name: "feature-worktree", branch: "feature", path: "/tmp/adelaide-wt/feature", created_at: "2026-01-01T00:00:00Z", status: "active", pinned: true },
    ]);
    const user = userEvent.setup();
    render(<Harness />);

    await user.click(await screen.findByText("feature"));
    const archiveButton = screen.getByTitle("Pinned worktrees can't be archived — unpin it first");

    expect(archiveButton).toBeDisabled();
  });

  it("leaves Archive enabled for an unpinned worktree", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.click(await screen.findByText("feature"));

    expect(screen.getByTitle("Archive")).toBeEnabled();
  });
});

describe("Sidebar worktree delete", () => {
  it("ignores a second click while the first delete is still in flight", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    // Kept unresolved for the length of the test — the point is to observe
    // behavior *during* the in-flight window a real double-click would race
    // against, not after it settles.
    vi.mocked(deleteWorktree).mockReturnValue(new Promise(() => {}));
    render(<Harness />);

    const deleteButton = await screen.findByTitle("Delete");
    fireEvent.click(deleteButton);
    fireEvent.click(deleteButton);

    // A real bug: with no guard, this second click opened its own confirm()
    // dialog and, once answered, sent a second concurrent DELETE request —
    // see the PLAN.md/PROGRESS.md entry on hardRemoveWorktree closing
    // terminal sessions before the git removal was known to succeed.
    expect(confirmSpy).toHaveBeenCalledTimes(1);
    expect(deleteWorktree).toHaveBeenCalledTimes(1);
    expect(deleteButton).toBeDisabled();

    confirmSpy.mockRestore();
  });
});
