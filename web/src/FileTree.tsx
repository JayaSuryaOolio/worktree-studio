import { useEffect, useRef, useState } from "react";
import { Tree, type NodeRendererProps, type TreeApi } from "react-arborist";
import { getFileTree, type FileNode } from "./api";
import FileTypeIcon from "./icons/FileTypeIcon";

interface FileTreeProps {
  repoId: string;
  worktreeId: string;
  folderName: string;
  onOpenFile: (path: string) => void;
  /** The currently-active editor panel's file path, if any — highlighted
   * and scrolled into view in the tree. See WorktreeDetail.tsx's dockview
   * onDidActivePanelChange wiring. */
  activePath?: string | null;
}

// Measures its own content box via ResizeObserver — react-arborist (built
// on react-window) needs explicit pixel width/height, it can't just fill a
// flex parent via CSS the way the old hand-rolled tree could.
function useElementSize<T extends HTMLElement>() {
  const ref = useRef<T | null>(null);
  // Starts at a reasonable non-zero fallback (matching the sidebar's own
  // CSS width) rather than {0, 0} — a real browser's ResizeObserver fires
  // with the true size almost immediately, but jsdom's test polyfill is a
  // no-op that never fires at all, and gating the tree's first render on
  // an observer that will never report anything would mean it never
  // renders in tests.
  const [size, setSize] = useState({ width: 240, height: 400 });

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const observer = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (!entry) return;
      setSize({ width: entry.contentRect.width, height: entry.contentRect.height });
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  return [ref, size] as const;
}

// A persistent navigation sidebar, not a dockview panel — unlike terminals
// and editors, this isn't something that makes sense to split/tab/close,
// so it lives outside the dockview grid entirely (see the flex layout in
// WorktreeDetail.tsx's .worktree-body). Built on react-arborist rather
// than a hand-rolled tree for a closer-to-VS-Code feel (keyboard nav,
// virtualization) with far less of our own code to maintain.
export default function FileTree({ repoId, worktreeId, folderName, onOpenFile, activePath }: FileTreeProps) {
  const [tree, setTree] = useState<FileNode[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const treeApiRef = useRef<TreeApi<FileNode> | undefined>(undefined);
  const [containerRef, size] = useElementSize<HTMLDivElement>();

  useEffect(() => {
    getFileTree(repoId, worktreeId)
      .then(setTree)
      .catch((err) => setError((err as Error).message));
  }, [repoId, worktreeId]);

  // Keep the tree's own selection/scroll in sync with whichever file is
  // the active editor panel — makes the sidebar answer "where am I" the
  // same way a file tree opening to the active file in VS Code does.
  useEffect(() => {
    if (!activePath || !treeApiRef.current) return;
    const api = treeApiRef.current;
    const node = api.get(activePath);
    if (!node) return; // tree hasn't loaded this path yet, or it's since been removed
    node.openParents();
    api.select(activePath);
    api.scrollTo(activePath);
  }, [activePath, tree]);

  return (
    <div className="file-tree-panel">
      <div className="file-tree-heading">
        <span className="file-tree-heading-name" title={folderName}>
          {folderName}
        </span>
        <button
          type="button"
          className="file-tree-collapse-all"
          title="Collapse all folders"
          onClick={() => treeApiRef.current?.closeAll()}
        >
          ⊟
        </button>
      </div>
      <div className="file-tree-body" ref={containerRef}>
        {error && <div className="file-tree-error">{error}</div>}
        {!error && !tree && <div className="file-tree-loading">Loading files…</div>}
        {!error && tree && tree.length === 0 && <div className="file-tree-empty">No files</div>}
        {!error && tree && tree.length > 0 && size.width > 0 && (
          <Tree<FileNode>
            ref={treeApiRef}
            data={tree}
            idAccessor="path"
            childrenAccessor="children"
            openByDefault
            disableDrag
            disableDrop
            disableEdit
            width={size.width}
            height={size.height}
            indent={14}
            rowHeight={22}
            selection={activePath ?? undefined}
          >
            {(props) => <FileTreeNode {...props} onOpenFile={onOpenFile} />}
          </Tree>
        )}
      </div>
    </div>
  );
}

function FileTreeNode({
  node,
  style,
  onOpenFile,
}: NodeRendererProps<FileNode> & { onOpenFile: (path: string) => void }) {
  const isDir = node.data.type === "dir";
  // A "dir" with no children data is an opaque directory (node_modules —
  // see internal/files.collapseOpaqueDirs): shown so its presence isn't a
  // surprise, but there's nothing under it to expand into.
  const isOpaque = isDir && node.isLeaf;

  function handleClick() {
    if (isOpaque) return;
    if (isDir) {
      node.toggle();
      return;
    }
    onOpenFile(node.data.path);
  }

  return (
    <div
      style={style}
      className={`file-tree-row${node.isSelected ? " file-tree-row-selected" : ""}${
        isOpaque ? " file-tree-row-opaque" : ""
      }`}
      onClick={handleClick}
      title={isOpaque ? `${node.data.name} — files hidden, not browsable here` : node.data.name}
    >
      {isDir ? (
        // Opaque dirs (node_modules, build — see internal/files.collapseOpaqueDirs)
        // still get the plain closed-folder icon, not a blank slot — the
        // point is the folder's presence should never be a surprise.
        <span className="file-tree-disclosure">{!isOpaque && node.isOpen ? "📂" : "📁"}</span>
      ) : (
        <span className="file-tree-disclosure file-tree-file-icon">
          <FileTypeIcon name={node.data.name} size={14} />
        </span>
      )}
      <span>&nbsp;</span>
      <span className="file-tree-label">{node.data.name}</span>
      {isOpaque && <span className="file-tree-opaque-note">Files hidden</span>}
    </div>
  );
}
