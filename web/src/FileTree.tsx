import { useEffect, useMemo, useRef, useState } from "react";
import { Tree, type NodeRendererProps, type TreeApi } from "react-arborist";
import { getFileTree, type FileNode } from "./api";
import FileTypeIcon from "./icons/FileTypeIcon";
import { CollapseAllIcon } from "./icons/FileTreeIcons";
import { useWorktreeSummary } from "./useWorktreeSummary";
import { registerActiveFileTreeActions } from "./activeFileTreeActions";

interface FileTreeProps {
  repoId: string;
  worktreeId: string;
  onOpenFile: (path: string) => void;
  /** The currently-active editor panel's file path, if any — highlighted
   * and scrolled into view in the tree. See WorktreeDetail.tsx's dockview
   * onDidActivePanelChange wiring. */
  activePath?: string | null;
  /** The worktree's full filesystem path — its basename is shown in the
   * header (the git-filter/PR-badge controls that used to live here moved
   * to the sidebar's expandable worktree card, see activeFileTreeActions.ts;
   * collapse-all stays here since it only makes sense right next to the
   * tree it acts on), and the full path is what the header's copy button
   * copies. */
  folderPath?: string;
}

// Prunes tree down to only the files in changedFiles (matched by their
// full path, the same repo-relative forward-slash form both
// internal/files.go and `git status` use), keeping every ancestor
// directory that has at least one matching descendant — a flat list of
// changed files wouldn't show where they actually live. Exported for
// direct testing; used by the header's git-icon toggle below.
export function filterTreeToChangedFiles(nodes: FileNode[], changedFiles: ReadonlySet<string>): FileNode[] {
  const out: FileNode[] = [];
  for (const node of nodes) {
    if (node.type === "file") {
      if (changedFiles.has(node.path)) out.push(node);
      continue;
    }
    const filteredChildren = node.children ? filterTreeToChangedFiles(node.children, changedFiles) : [];
    if (filteredChildren.length > 0) {
      out.push({ ...node, children: filteredChildren });
    }
  }
  return out;
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
export default function FileTree({ repoId, worktreeId, onOpenFile, activePath, folderPath }: FileTreeProps) {
  const [tree, setTree] = useState<FileNode[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [filterToChanged, setFilterToChanged] = useState(false);
  const [pathCopied, setPathCopied] = useState(false);
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number; path: string } | null>(null);
  const treeApiRef = useRef<TreeApi<FileNode> | undefined>(undefined);
  const [containerRef, size] = useElementSize<HTMLDivElement>();
  const folderName = folderPath?.split("/").pop();

  // Closes on an outside click, Escape, or the tree scrolling underneath
  // it — the same dismissal set a native context menu gets for free, which
  // this one needs to replicate since main.tsx suppresses the native one
  // everywhere except real text-editing surfaces (see contextMenuPolicy.ts).
  useEffect(() => {
    if (!contextMenu) return;
    function close() {
      setContextMenu(null);
    }
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") close();
    }
    document.addEventListener("mousedown", close);
    document.addEventListener("keydown", onKeyDown);
    document.addEventListener("scroll", close, true);
    return () => {
      document.removeEventListener("mousedown", close);
      document.removeEventListener("keydown", onKeyDown);
      document.removeEventListener("scroll", close, true);
    };
  }, [contextMenu]);

  function copyPathToClipboard(path: string) {
    navigator.clipboard.writeText(path);
    setContextMenu(null);
  }

  function copyAbsolutePathToClipboard(path: string) {
    if (folderPath) navigator.clipboard.writeText(`${folderPath}/${path}`);
    setContextMenu(null);
  }

  function copyFolderPath() {
    if (!folderPath) return;
    navigator.clipboard.writeText(folderPath).then(() => {
      setPathCopied(true);
      window.setTimeout(() => setPathCopied(false), 1500);
    });
  }

  // Always enabled (unlike WorktreeHoverPopover's hover-gated fetch) —
  // the header's git icon/PR badge should be ready the moment the panel
  // is visible, not wait for a first hover.
  const { summary } = useWorktreeSummary(repoId, worktreeId, true);

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

  const changedFilesSet = useMemo(() => new Set(summary?.changed_files ?? []), [summary]);
  const displayedTree = useMemo(() => {
    if (!tree || !filterToChanged) return tree;
    return filterTreeToChangedFiles(tree, changedFilesSet);
  }, [tree, filterToChanged, changedFilesSet]);

  // Registers the git-filter control (rendered in the sidebar's expandable
  // worktree card) against this specific FileTree instance — only while
  // it's actually mounted (i.e. only while the files panel is open), same
  // no-deps re-register-every-render idiom as
  // activeWorktreeFileOpener.ts/activeWorktreeActions.ts.
  useEffect(() => {
    registerActiveFileTreeActions({
      worktreeId,
      filterToChanged,
      changedFilesAvailable: changedFilesSet.size > 0,
      toggleChangedFilesFilter: () => setFilterToChanged((v) => !v),
    });
    return () => registerActiveFileTreeActions(null);
  });

  return (
    <div className="file-tree-panel">
      <div className="file-tree-heading">
        {/* A label, not the folder name. A worktree's directory is named
            after its branch, so this used to print the exact string the
            worktree header beside it was already showing — two headers,
            one piece of information. The full path is still on the title
            and behind the copy button to the right. */}
        <span className="file-tree-heading-label" title={folderPath || folderName}>
          Files
        </span>
        <button
          type="button"
          className="file-tree-copy-path"
          title="Copy full folder path"
          disabled={!folderPath}
          onClick={copyFolderPath}
        >
          {pathCopied ? "Copied" : "⧉"}
        </button>
        <button
          type="button"
          className="file-tree-collapse-all"
          title="Collapse all folders"
          onClick={() => treeApiRef.current?.closeAll()}
        >
          <CollapseAllIcon size={14} />
        </button>
      </div>
      <div className="file-tree-body" ref={containerRef}>
        {error && <div className="file-tree-error">{error}</div>}
        {!error && !tree && <div className="file-tree-loading">Loading files…</div>}
        {!error && displayedTree && displayedTree.length === 0 && (
          <div className="file-tree-empty">{filterToChanged ? "No changed files" : "No files"}</div>
        )}
        {!error && displayedTree && displayedTree.length > 0 && size.width > 0 && (
          <Tree<FileNode>
            ref={treeApiRef}
            data={displayedTree}
            idAccessor="path"
            childrenAccessor="children"
            // Collapsed, not react-arborist's expanded-everything default:
            // a real repo opens as hundreds of rows of directories you
            // didn't ask for, and the top-level folders — the thing you
            // actually navigate by — are pushed off the bottom of the
            // panel. The active file's ancestors are still opened for you
            // (see the activePath effect above), and collapse-all in the
            // heading gets you back here.
            openByDefault={false}
            disableDrag
            disableDrop
            disableEdit
            width={size.width}
            height={size.height}
            indent={14}
            rowHeight={22}
            selection={activePath ?? undefined}
          >
            {(props) => (
              <FileTreeNode
                {...props}
                onOpenFile={onOpenFile}
                onContextMenu={(path, x, y) => setContextMenu({ path, x, y })}
                contextMenuActive={contextMenu?.path === props.node.data.path}
              />
            )}
          </Tree>
        )}
        {contextMenu && (
          <div
            className="file-tree-context-menu"
            style={{ left: contextMenu.x, top: contextMenu.y }}
            // The outside-mousedown listener above would otherwise close
            // this before the click on its own item ever registers.
            onMouseDown={(e) => e.stopPropagation()}
          >
            <button
              type="button"
              className="file-tree-context-menu-item"
              onClick={() => copyPathToClipboard(contextMenu.path)}
            >
              Copy path
            </button>
            {folderPath && (
              <button
                type="button"
                className="file-tree-context-menu-item"
                onClick={() => copyAbsolutePathToClipboard(contextMenu.path)}
              >
                Copy absolute path
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

function FileTreeNode({
  node,
  style,
  onOpenFile,
  onContextMenu,
  contextMenuActive,
}: NodeRendererProps<FileNode> & {
  onOpenFile: (path: string) => void;
  onContextMenu: (path: string, x: number, y: number) => void;
  /** True while this row's own right-click menu is open. Right-clicking
   * puts the context menu right on top of the cursor, which knocks out the
   * row's own CSS :hover the instant the menu appears — without this, the
   * row the menu belongs to would look no different from any other,
   * unlike a native menu's usual "this is what I'm acting on" affordance. */
  contextMenuActive: boolean;
}) {
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
      }${contextMenuActive ? " file-tree-row-context-active" : ""}`}
      onClick={handleClick}
      onContextMenu={(e) => {
        e.preventDefault();
        onContextMenu(node.data.path, e.clientX, e.clientY);
      }}
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
