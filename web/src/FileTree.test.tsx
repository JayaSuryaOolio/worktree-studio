import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import FileTree, { filterTreeToChangedFiles } from "./FileTree";

vi.mock("./api", () => ({
  getFileTree: vi.fn(),
  getWorktreeSummary: vi.fn(),
}));

import { getFileTree, getWorktreeSummary, type FileNode, type WorktreeSummary } from "./api";

const tree: FileNode[] = [
  {
    name: "src",
    path: "src",
    type: "dir",
    children: [
      { name: "main.go", path: "src/main.go", type: "file" },
      { name: "helper.go", path: "src/helper.go", type: "file" },
    ],
  },
  { name: "README.md", path: "README.md", type: "file" },
  // No `children` field at all — matches what the backend sends for an
  // opaque dir (internal/files.collapseOpaqueDirs), where Children is
  // omitted (omitempty) rather than an empty array.
  { name: "node_modules", path: "node_modules", type: "dir" },
];

const noSummary: WorktreeSummary = {
  branch: "feature",
  ahead: 0,
  behind: 0,
  has_upstream: true,
  dirty: false,
  changed_files: [],
  pr: null,
};

beforeEach(() => {
  vi.mocked(getFileTree).mockResolvedValue(tree);
  vi.mocked(getWorktreeSummary).mockResolvedValue(noSummary);
  localStorage.clear();
});

describe("FileTree", () => {
  it("clicking a file calls onOpenFile with its path", async () => {
    const onOpenFile = vi.fn();
    const user = userEvent.setup();
    render(<FileTree repoId="r1" worktreeId="w1" onOpenFile={onOpenFile} />);

    await user.click(await screen.findByText("main.go"));
    expect(onOpenFile).toHaveBeenCalledWith("src/main.go");
  });

  it("shows node_modules but clicking it does not call onOpenFile or expand anything", async () => {
    const onOpenFile = vi.fn();
    const user = userEvent.setup();
    render(<FileTree repoId="r1" worktreeId="w1" onOpenFile={onOpenFile} />);

    const row = await screen.findByText("node_modules");
    await user.click(row);
    expect(onOpenFile).not.toHaveBeenCalled();
    // No disclosure arrow rendered for an opaque dir (see FileTreeNode).
    expect(row.closest(".file-tree-row")?.textContent).not.toMatch(/[▾▸]/);
  });

  it("collapse-all closes a folder that was opened", async () => {
    const user = userEvent.setup();
    render(<FileTree repoId="r1" worktreeId="w1" onOpenFile={vi.fn()} />);

    // openByDefault is true, so src/main.go starts visible.
    await screen.findByText("main.go");

    await user.click(screen.getByTitle("Collapse all folders"));
    await waitFor(() => expect(screen.queryByText("main.go")).not.toBeInTheDocument());
    // The collapsed folder itself is still there, just closed.
    expect(screen.getByText("src")).toBeInTheDocument();
  });

  it("the git-filter icon is disabled when there are no changed files", async () => {
    render(<FileTree repoId="r1" worktreeId="w1" onOpenFile={vi.fn()} />);
    await screen.findByText("main.go");
    expect(await screen.findByTitle("Filter to only files changed in this branch")).toBeDisabled();
  });

  it("clicking the git icon filters the tree to only changed files, preserving nesting", async () => {
    vi.mocked(getWorktreeSummary).mockResolvedValue({
      ...noSummary,
      dirty: true,
      changed_files: ["src/main.go", "README.md"],
    });
    const user = userEvent.setup();
    render(<FileTree repoId="r1" worktreeId="w1" onOpenFile={vi.fn()} />);
    await screen.findByText("helper.go");

    const gitButton = await screen.findByTitle("Filter to only files changed in this branch");
    expect(gitButton).not.toBeDisabled();
    await user.click(gitButton);

    // main.go (changed) and its parent dir survive; helper.go (unchanged)
    // and node_modules (no changed descendants) don't.
    expect(screen.getByText("src")).toBeInTheDocument();
    expect(screen.getByText("main.go")).toBeInTheDocument();
    expect(screen.getByText("README.md")).toBeInTheDocument();
    expect(screen.queryByText("helper.go")).not.toBeInTheDocument();
    expect(screen.queryByText("node_modules")).not.toBeInTheDocument();

    // Clicking again turns the filter back off.
    await user.click(screen.getByTitle(/Showing only files changed/));
    expect(await screen.findByText("helper.go")).toBeInTheDocument();
  });

  it("shows a PR badge that opens the PR URL when clicked", async () => {
    vi.mocked(getWorktreeSummary).mockResolvedValue({
      ...noSummary,
      pr: { number: 42, title: "Add the thing", state: "OPEN", url: "https://example.com/pr/42", is_draft: false },
    });
    const openSpy = vi.spyOn(window, "open").mockImplementation(() => null);
    const user = userEvent.setup();
    render(<FileTree repoId="r1" worktreeId="w1" onOpenFile={vi.fn()} />);

    const badge = await screen.findByText("#42 ↗");
    await user.click(badge);
    expect(openSpy).toHaveBeenCalledWith("https://example.com/pr/42", "_blank", "noopener,noreferrer");
    openSpy.mockRestore();
  });
});

describe("filterTreeToChangedFiles", () => {
  it("keeps only matching files and their ancestor directories", () => {
    const filtered = filterTreeToChangedFiles(tree, new Set(["src/main.go"]));
    expect(filtered).toEqual([
      { name: "src", path: "src", type: "dir", children: [{ name: "main.go", path: "src/main.go", type: "file" }] },
    ]);
  });

  it("returns an empty array when nothing matches", () => {
    expect(filterTreeToChangedFiles(tree, new Set())).toEqual([]);
  });

  it("drops a directory entirely when none of its descendants match", () => {
    const filtered = filterTreeToChangedFiles(tree, new Set(["README.md"]));
    expect(filtered).toEqual([{ name: "README.md", path: "README.md", type: "file" }]);
  });
});
