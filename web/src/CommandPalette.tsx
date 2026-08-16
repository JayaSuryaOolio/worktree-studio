import { Command } from "cmdk";
import { useEffect, useState } from "react";
import { useMatch, useNavigate } from "react-router-dom";
import { getFileTree, type FileNode } from "./api";
import { openFileInActiveWorktree } from "./activeWorktreeFileOpener";
import { useRepoContext } from "./RepoContext";

interface Props {
  onAddRepo: () => void;
  onNewWorktree: (repoId: string) => void;
  onAttachWorktree: (repoId: string) => void;
}

// Flattens a file tree into just the file paths (skipping directories
// entirely) — a flat list is all Quick-Open-style search needs, and cmdk
// does its own fuzzy matching against each item's text, so there's no
// tree structure to preserve here.
function flattenFilePaths(nodes: FileNode[]): string[] {
  const out: string[] = [];
  for (const n of nodes) {
    if (n.type === "file") {
      out.push(n.path);
    } else if (n.children) {
      out.push(...flattenFilePaths(n.children));
    }
  }
  return out;
}

// A Cmd/Ctrl+K command palette — one of this app's three sanctioned
// interaction surfaces alongside modals and dropdowns (see PLAN.md step
// 7: this is an SPA, new capability is not supposed to mean new pages).
// Scoped deliberately narrow: jump to a repo or worktree, trigger the
// existing add-repo/new-worktree modals, or (when currently viewing a
// worktree) fuzzy-open one of its files — not arbitrary command execution.
export default function CommandPalette({ onAddRepo, onNewWorktree, onAttachWorktree }: Props) {
  const [open, setOpen] = useState(false);
  const { repos, worktreesByRepo } = useRepoContext();
  const navigate = useNavigate();

  // File search is scoped to whichever worktree the user is currently
  // looking at ("current directory", per the direct request this was
  // built for) — not a cross-worktree search. That's also what makes
  // opening a selected file simple: activeWorktreeFileOpener always
  // targets whatever WorktreeDetail is mounted right now, which this
  // match guarantees is the same worktree the results came from.
  const worktreeMatch = useMatch("/repo/:repoId/worktree/:worktreeId");
  const currentRepoId = worktreeMatch?.params.repoId ?? null;
  const currentWorktreeId = worktreeMatch?.params.worktreeId ?? null;
  const [filePaths, setFilePaths] = useState<string[]>([]);

  useEffect(() => {
    if (!open || !currentRepoId || !currentWorktreeId) {
      setFilePaths([]);
      return;
    }
    getFileTree(currentRepoId, currentWorktreeId)
      .then((tree) => setFilePaths(flattenFilePaths(tree)))
      .catch(() => setFilePaths([]));
    // Refetched fresh each time the palette opens (cheap — one request) rather
    // than kept live, so a file added since the palette was last opened
    // still shows up without needing a background poll/watch just for this.
  }, [open, currentRepoId, currentWorktreeId]);

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (!(e.metaKey || e.ctrlKey)) return;
      const key = e.key.toLowerCase();
      // Cmd/Ctrl+P is remapped to this palette too — this is a local tool
      // for jumping around repos/worktrees/files, not a document anyone
      // needs the browser's own print dialog for, and "P" already reads as
      // "quick open" muscle memory from every editor this app is modeled
      // on (VS Code, JetBrains, etc.).
      if (key === "k" || key === "p") {
        e.preventDefault();
        setOpen((o) => !o);
      }
    }
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, []);

  function runAndClose(fn: () => void) {
    setOpen(false);
    fn();
  }

  return (
    <Command.Dialog
      open={open}
      onOpenChange={setOpen}
      label="Command palette"
      className="command-palette"
      overlayClassName="command-palette-overlay"
      contentClassName="command-palette-content"
    >
      <Command.Input placeholder="Jump to a repo/worktree, open a file, or run a command…" />
      <Command.List>
        <Command.Empty>No matches.</Command.Empty>

        <Command.Group heading="Actions">
          <Command.Item onSelect={() => runAndClose(onAddRepo)}>+ Add repo</Command.Item>
        </Command.Group>

        {filePaths.length > 0 && (
          <Command.Group heading="Files in this worktree">
            {filePaths.map((path) => (
              <Command.Item key={path} value={path} onSelect={() => runAndClose(() => openFileInActiveWorktree(path))}>
                {path}
              </Command.Item>
            ))}
          </Command.Group>
        )}

        {repos.map((r) => (
          <Command.Group key={r.id} heading={r.name}>
            <Command.Item onSelect={() => runAndClose(() => navigate(`/repo/${r.id}`))}>
              Open {r.name}
            </Command.Item>
            <Command.Item onSelect={() => runAndClose(() => onNewWorktree(r.id))}>
              + New worktree in {r.name}
            </Command.Item>
            <Command.Item onSelect={() => runAndClose(() => onAttachWorktree(r.id))}>
              📂 Attach existing worktree in {r.name}
            </Command.Item>
            {(worktreesByRepo[r.id] ?? []).map((wt) => (
              <Command.Item
                key={wt.id}
                value={`${r.name} ${wt.branch}`}
                onSelect={() => runAndClose(() => navigate(`/repo/${r.id}/worktree/${wt.id}`))}
              >
                {wt.branch}
              </Command.Item>
            ))}
          </Command.Group>
        ))}
      </Command.List>
    </Command.Dialog>
  );
}
