import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import FileTree from "./FileTree";

vi.mock("./api", () => ({
  getFileTree: vi.fn(),
}));

import { getFileTree, type FileNode } from "./api";

const tree: FileNode[] = [
  {
    name: "src",
    path: "src",
    type: "dir",
    children: [{ name: "main.go", path: "src/main.go", type: "file" }],
  },
  { name: "README.md", path: "README.md", type: "file" },
  // No `children` field at all — matches what the backend sends for an
  // opaque dir (internal/files.collapseOpaqueDirs), where Children is
  // omitted (omitempty) rather than an empty array.
  { name: "node_modules", path: "node_modules", type: "dir" },
];

beforeEach(() => {
  vi.mocked(getFileTree).mockResolvedValue(tree);
});

describe("FileTree", () => {
  it("shows the folder name heading", async () => {
    render(<FileTree repoId="r1" worktreeId="w1" folderName="my-worktree" onOpenFile={vi.fn()} />);
    expect(await screen.findByText(/my-worktree/)).toBeInTheDocument();
  });

  it("clicking a file calls onOpenFile with its path", async () => {
    const onOpenFile = vi.fn();
    const user = userEvent.setup();
    render(<FileTree repoId="r1" worktreeId="w1" folderName="wt" onOpenFile={onOpenFile} />);

    await user.click(await screen.findByText("main.go"));
    expect(onOpenFile).toHaveBeenCalledWith("src/main.go");
  });

  it("shows node_modules but clicking it does not call onOpenFile or expand anything", async () => {
    const onOpenFile = vi.fn();
    const user = userEvent.setup();
    render(<FileTree repoId="r1" worktreeId="w1" folderName="wt" onOpenFile={onOpenFile} />);

    const row = await screen.findByText("node_modules");
    await user.click(row);
    expect(onOpenFile).not.toHaveBeenCalled();
    // No disclosure arrow rendered for an opaque dir (see FileTreeNode).
    expect(row.closest(".file-tree-row")?.textContent).not.toMatch(/[▾▸]/);
  });

  it("collapse-all closes a folder that was opened", async () => {
    const user = userEvent.setup();
    render(<FileTree repoId="r1" worktreeId="w1" folderName="wt" onOpenFile={vi.fn()} />);

    // openByDefault is true, so src/main.go starts visible.
    await screen.findByText("main.go");

    await user.click(screen.getByTitle("Collapse all folders"));
    await waitFor(() => expect(screen.queryByText("main.go")).not.toBeInTheDocument());
    // The collapsed folder itself is still there, just closed.
    expect(screen.getByText("src")).toBeInTheDocument();
  });
});
