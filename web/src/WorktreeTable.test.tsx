import { render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { Worktree } from "./api";
import WorktreeTable from "./WorktreeTable";

function wt(over: Partial<Worktree> & { branch: string }): Worktree {
  return {
    id: over.branch,
    repo_id: "r1",
    name: over.branch,
    path: `/Users/j/.worktree-studio/worktrees/9af16fe/${over.branch}`,
    source_branch: "origin/master",
    created_at: "2026-08-20T12:00:00Z",
    status: "active",
    ...over,
  } as Worktree;
}

function renderTable(worktrees: Worktree[]) {
  return render(
    <MemoryRouter>
      <WorktreeTable worktrees={worktrees} loading={false} emptyText="none" />
    </MemoryRouter>
  );
}

const nine = ["a-one", "b-two", "c-three", "d-four", "e-five", "f-six", "g-seven", "h-eight", "i-nine"];

describe("WorktreeTable", () => {
  it("drops a column entirely when every row agrees, and says so once", () => {
    renderTable(nine.map((b) => wt({ branch: b })));

    expect(screen.getByText(/All created from/)).toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: "Created from" })).not.toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: "Status" })).not.toBeInTheDocument();
    // Said once in the caption, not nine times in a column.
    expect(screen.getAllByText("origin/master")).toHaveLength(1);
  });

  // The bug this replaced commonValue over: two attached worktrees made
  // the other nine keep printing "origin/master" in full.
  it("keeps the column for the outliers only, when most rows agree", () => {
    const rows = [
      ...nine.map((b) => wt({ branch: b })),
      wt({ branch: "attached-one", source_branch: "", path: "/elsewhere/pos/nassau" }),
      wt({ branch: "attached-two", source_branch: "release/7-35", path: "/elsewhere/pos/columbus" }),
    ];
    renderTable(rows);

    expect(screen.getByText(/Mostly created from/)).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Created from" })).toBeInTheDocument();

    // The nine matching rows say nothing in that column; only the one
    // genuine exception does.
    expect(screen.getAllByText("origin/master")).toHaveLength(1); // the caption
    expect(screen.getByText("release/7-35")).toBeInTheDocument();

    const matching = screen.getByRole("link", { name: "a-one" }).closest("tr")!;
    expect(within(matching).queryByText("origin/master")).not.toBeInTheDocument();
  });

  it("shows only the leaf for a row under the hoisted root, and the elided path otherwise", () => {
    const rows = [
      ...nine.map((b) => wt({ branch: b })),
      wt({ branch: "outside", path: "/somewhere/else/pos/nassau" }),
    ];
    renderTable(rows);

    // The cell also holds the copy button, so assert on the text span
    // rather than the whole cell.
    const pathText = (row: HTMLElement) => row.querySelector(".path-cell-text")!.textContent;

    const inside = screen.getByRole("link", { name: "a-one" }).closest("tr")!;
    expect(within(inside).getByTitle("/Users/j/.worktree-studio/worktrees/9af16fe/a-one")).toBeInTheDocument();
    expect(pathText(inside)).toBe("a-one");

    const outside = screen.getByRole("link", { name: "outside" }).closest("tr")!;
    expect(within(outside).getByTitle("/somewhere/else/pos/nassau")).toBeInTheDocument();
    expect(pathText(outside)).toBe("…/pos/nassau");
  });

  it("keeps a column that is genuinely carrying per-row information", () => {
    const rows = [
      wt({ branch: "a", source_branch: "origin/master" }),
      wt({ branch: "b", source_branch: "origin/main" }),
      wt({ branch: "c", source_branch: "release/7-35" }),
      wt({ branch: "d", source_branch: "develop" }),
    ];
    renderTable(rows);

    expect(screen.getByRole("columnheader", { name: "Created from" })).toBeInTheDocument();
    expect(screen.getByText("origin/main")).toBeInTheDocument();
    expect(screen.getByText("develop")).toBeInTheDocument();
  });

  it("renders a relative age, with the exact timestamp one hover away", () => {
    renderTable([wt({ branch: "a", created_at: "2020-01-01T00:00:00Z" })]);
    const cell = screen.getByText(/ago$/);
    expect(cell).toHaveAttribute("title", new Date("2020-01-01T00:00:00Z").toLocaleString());
  });

  it("still renders the empty and loading states", () => {
    const { rerender } = render(
      <MemoryRouter>
        <WorktreeTable worktrees={[]} loading={false} emptyText="No local worktrees yet." />
      </MemoryRouter>
    );
    expect(screen.getByText("No local worktrees yet.")).toBeInTheDocument();

    rerender(
      <MemoryRouter>
        <WorktreeTable worktrees={[]} loading emptyText="No local worktrees yet." />
      </MemoryRouter>
    );
    expect(screen.getByText("Loading…")).toBeInTheDocument();
  });
});
