import { render, screen, waitFor } from "@testing-library/react";
import { within } from "@testing-library/dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import WorktreeAuditLog from "./WorktreeAuditLog";

vi.mock("./api", () => ({
  getWorktreeAuditLog: vi.fn(),
  getClaudeSessionTitle: vi.fn(),
}));

import { getClaudeSessionTitle, getWorktreeAuditLog } from "./api";

beforeEach(() => {
  // Default: no local transcript found for any session id, so summarize()
  // falls back to the entry's own stored `title` field/bare id — tests
  // that care about the live-fetched title override this per-test.
  vi.mocked(getClaudeSessionTitle).mockResolvedValue(null);
});

describe("WorktreeAuditLog", () => {
  it("renders entries newest-first with friendly labels and a summary", async () => {
    vi.mocked(getWorktreeAuditLog).mockResolvedValue([
      {
        ts: "2026-01-02T00:00:00Z",
        event: "terminal.create",
        worktree_id: "w1",
        tab_label: "Terminal 1",
      },
      {
        ts: "2026-01-01T00:00:00Z",
        event: "worktree.create",
        worktree_id: "w1",
        branch: "feature",
      },
    ]);

    render(<WorktreeAuditLog repoId="r1" worktreeId="w1" title="feature" onClose={() => {}} />);

    const items = await screen.findAllByRole("listitem");
    expect(items).toHaveLength(2);
    expect(within(items[0]).getByText("Terminal opened")).toBeInTheDocument();
    expect(items[0].querySelector(".audit-log-summary")?.textContent).toMatch(/Terminal 1/);
    expect(within(items[1]).getByText("Worktree created")).toBeInTheDocument();
    expect(items[1].querySelector(".audit-log-summary")?.textContent).toMatch(/branch feature/);
    expect(getWorktreeAuditLog).toHaveBeenCalledWith("r1", "w1");
  });

  it("renders claude.session.create and archive/unarchive events with friendly labels", async () => {
    vi.mocked(getWorktreeAuditLog).mockResolvedValue([
      {
        ts: "2026-01-03T00:00:00Z",
        event: "worktree.archive",
        worktree_id: "w1",
        branch: "feature",
      },
      {
        ts: "2026-01-02T00:00:00Z",
        event: "claude.session.create",
        worktree_id: "w1",
        claude_session_id: "abc-123",
        title: "feature",
      },
    ]);

    render(<WorktreeAuditLog repoId="r1" worktreeId="w1" title="feature" onClose={() => {}} />);

    const items = await screen.findAllByRole("listitem");
    expect(items).toHaveLength(2);
    expect(within(items[0]).getByText("Worktree archived")).toBeInTheDocument();
    expect(within(items[1]).getByText("Claude session started")).toBeInTheDocument();
    expect(items[1].querySelector(".audit-log-summary")?.textContent).toMatch(/abc-123/);
  });

  it("prefers a live-fetched transcript title over the stored title once it resolves", async () => {
    vi.mocked(getWorktreeAuditLog).mockResolvedValue([
      {
        ts: "2026-01-02T00:00:00Z",
        event: "claude.session.create",
        worktree_id: "w1",
        claude_session_id: "abc-123",
        title: "feature",
      },
    ]);
    vi.mocked(getClaudeSessionTitle).mockResolvedValue("fix the login bug please");

    render(<WorktreeAuditLog repoId="r1" worktreeId="w1" title="feature" onClose={() => {}} />);

    const item = (await screen.findAllByRole("listitem"))[0];
    await screen.findByText(/fix the login bug please/);
    expect(item.querySelector(".audit-log-summary")?.textContent).toBe(
      " — fix the login bug please (abc-123)"
    );
    expect(getClaudeSessionTitle).toHaveBeenCalledWith("abc-123");
  });

  it("shows the full injected context text for claude.session.context, not just raw JSON", async () => {
    const context =
      "Ooga. Claude wake up in cave (folder): /tmp/wt\nOoo, worktree-studio cave! Branch-mark say: feature";
    vi.mocked(getWorktreeAuditLog).mockResolvedValue([
      {
        ts: "2026-01-02T00:00:00Z",
        event: "claude.session.context",
        worktree_id: "w1",
        context,
      },
    ]);

    render(<WorktreeAuditLog repoId="r1" worktreeId="w1" title="feature" onClose={() => {}} />);

    const item = (await screen.findAllByRole("listitem"))[0];
    expect(within(item).getByText("Context injected into Claude")).toBeInTheDocument();
    const contextBlock = item.querySelector(".audit-log-context");
    expect(contextBlock).not.toBeNull();
    expect(contextBlock?.textContent).toBe(context);
  });

  it("shows an empty state when there are no events", async () => {
    vi.mocked(getWorktreeAuditLog).mockResolvedValue([]);
    render(<WorktreeAuditLog repoId="r1" worktreeId="w1" title="feature" onClose={() => {}} />);
    expect(await screen.findByText(/no events recorded yet/i)).toBeInTheDocument();
  });

  it("shows an error message if the fetch fails", async () => {
    vi.mocked(getWorktreeAuditLog).mockRejectedValue(new Error("boom"));
    render(<WorktreeAuditLog repoId="r1" worktreeId="w1" title="feature" onClose={() => {}} />);
    await waitFor(() => expect(screen.getByText("boom")).toBeInTheDocument());
  });
});
