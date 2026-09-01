import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import FileTree, { filterTreeToChangedFiles } from "./FileTree";
import { getActiveFileTreeActions } from "./activeFileTreeActions";

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

// Folders start collapsed (openByDefault={false} — see FileTree.tsx), so
// anything nested has to be opened first, exactly as a real user would.
async function expandSrc(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByText("src"));
  return screen.findByText("main.go");
}

describe("FileTree", () => {
  it("opens with every folder collapsed", async () => {
    render(<FileTree repoId="r1" worktreeId="w1" onOpenFile={vi.fn()} />);

    // Top-level entries are there; nothing inside them is.
    expect(await screen.findByText("src")).toBeInTheDocument();
    expect(screen.getByText("README.md")).toBeInTheDocument();
    expect(screen.queryByText("main.go")).not.toBeInTheDocument();
  });

  it("clicking a file calls onOpenFile with its path", async () => {
    const onOpenFile = vi.fn();
    const user = userEvent.setup();
    render(<FileTree repoId="r1" worktreeId="w1" onOpenFile={onOpenFile} />);

    await user.click(await expandSrc(user));
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

  // The header used to print the folder's basename, which is a worktree
  // directory named after its branch — the exact string the worktree
  // header immediately to its right was already showing. Two headers, one
  // piece of information. It's a region label now, with the full path
  // still on the title and behind the copy button.
  it("labels the panel rather than repeating the branch name", async () => {
    render(
      <FileTree repoId="r1" worktreeId="w1" onOpenFile={vi.fn()} folderPath="/tmp/adelaide-wt/my-worktree" />
    );
    const label = await screen.findByText("Files");
    expect(label).toHaveAttribute("title", "/tmp/adelaide-wt/my-worktree");
    expect(screen.queryByText("my-worktree")).not.toBeInTheDocument();
  });

  it("copies the full folder path when the copy button is clicked", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();
    render(
      <FileTree repoId="r1" worktreeId="w1" onOpenFile={vi.fn()} folderPath="/tmp/adelaide-wt/my-worktree" />
    );
    // Set up after userEvent.setup() — it installs its own navigator.clipboard
    // stub for copy/paste emulation, which would otherwise clobber this one.
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });

    await user.click(await screen.findByTitle("Copy full folder path"));
    expect(writeText).toHaveBeenCalledWith("/tmp/adelaide-wt/my-worktree");
  });

  it("right-clicking a file shows a context menu that copies its path", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();
    render(<FileTree repoId="r1" worktreeId="w1" onOpenFile={vi.fn()} />);
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });

    const row = await expandSrc(user);
    await user.pointer({ keys: "[MouseRight]", target: row });

    const item = await screen.findByText("Copy path");
    await user.click(item);

    expect(writeText).toHaveBeenCalledWith("src/main.go");
    expect(screen.queryByText("Copy path")).not.toBeInTheDocument();
  });

  it("offers and copies the absolute path when a folderPath is known", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();
    render(
      <FileTree repoId="r1" worktreeId="w1" onOpenFile={vi.fn()} folderPath="/tmp/adelaide-wt/my-worktree" />
    );
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });

    const row = await expandSrc(user);
    await user.pointer({ keys: "[MouseRight]", target: row });

    await user.click(await screen.findByText("Copy absolute path"));

    expect(writeText).toHaveBeenCalledWith("/tmp/adelaide-wt/my-worktree/src/main.go");
  });

  it("does not offer 'Copy absolute path' without a known folderPath", async () => {
    const user = userEvent.setup();
    render(<FileTree repoId="r1" worktreeId="w1" onOpenFile={vi.fn()} />);

    const row = await expandSrc(user);
    await user.pointer({ keys: "[MouseRight]", target: row });

    await screen.findByText("Copy path");
    expect(screen.queryByText("Copy absolute path")).not.toBeInTheDocument();
  });

  it("keeps the right-clicked row visually active while its menu is open", async () => {
    const user = userEvent.setup();
    render(<FileTree repoId="r1" worktreeId="w1" onOpenFile={vi.fn()} />);

    const row = await expandSrc(user);
    expect(row.closest(".file-tree-row")?.className).not.toContain("file-tree-row-context-active");

    await user.pointer({ keys: "[MouseRight]", target: row });
    await screen.findByText("Copy path");
    expect(screen.getByText("main.go").closest(".file-tree-row")?.className).toContain(
      "file-tree-row-context-active"
    );

    await user.click(document.body);
    await waitFor(() => expect(screen.queryByText("Copy path")).not.toBeInTheDocument());
    expect(screen.getByText("main.go").closest(".file-tree-row")?.className).not.toContain(
      "file-tree-row-context-active"
    );
  });

  it("closes the context menu on an outside click without copying", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();
    render(<FileTree repoId="r1" worktreeId="w1" onOpenFile={vi.fn()} />);
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });

    const row = await expandSrc(user);
    await user.pointer({ keys: "[MouseRight]", target: row });
    await screen.findByText("Copy path");

    await user.click(document.body);

    expect(screen.queryByText("Copy path")).not.toBeInTheDocument();
    expect(writeText).not.toHaveBeenCalled();
  });

  it("collapse-all closes a folder that was opened", async () => {
    const user = userEvent.setup();
    render(<FileTree repoId="r1" worktreeId="w1" onOpenFile={vi.fn()} />);

    await expandSrc(user);

    await user.click(screen.getByTitle("Collapse all folders"));
    await waitFor(() => expect(screen.queryByText("main.go")).not.toBeInTheDocument());
    // The collapsed folder itself is still there, just closed.
    expect(screen.getByText("src")).toBeInTheDocument();
  });

  it("registers changedFilesAvailable as false when there are no changed files", async () => {
    render(<FileTree repoId="r1" worktreeId="w1" onOpenFile={vi.fn()} />);
    await screen.findByText("src");
    await waitFor(() => expect(getActiveFileTreeActions()?.changedFilesAvailable).toBe(false));
  });

  it("toggleChangedFilesFilter filters the tree to only changed files, preserving nesting", async () => {
    vi.mocked(getWorktreeSummary).mockResolvedValue({
      ...noSummary,
      dirty: true,
      changed_files: ["src/main.go", "README.md"],
    });
    const user = userEvent.setup();
    render(<FileTree repoId="r1" worktreeId="w1" onOpenFile={vi.fn()} />);
    await expandSrc(user);
    expect(screen.getByText("helper.go")).toBeInTheDocument();

    await waitFor(() => expect(getActiveFileTreeActions()?.changedFilesAvailable).toBe(true));
    act(() => {
      getActiveFileTreeActions()?.toggleChangedFilesFilter();
    });

    // main.go (changed) and its parent dir survive; helper.go (unchanged)
    // and node_modules (no changed descendants) don't.
    await waitFor(() => expect(screen.queryByText("helper.go")).not.toBeInTheDocument());
    expect(screen.getByText("src")).toBeInTheDocument();
    expect(screen.getByText("main.go")).toBeInTheDocument();
    expect(screen.getByText("README.md")).toBeInTheDocument();
    expect(screen.queryByText("node_modules")).not.toBeInTheDocument();

    // Toggling again turns the filter back off.
    act(() => {
      getActiveFileTreeActions()?.toggleChangedFilesFilter();
    });
    expect(await screen.findByText("helper.go")).toBeInTheDocument();
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
