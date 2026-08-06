import { Command } from "cmdk";
import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useRepoContext } from "./RepoContext";

interface Props {
  onAddRepo: () => void;
  onNewWorktree: (repoId: string) => void;
}

// A Cmd/Ctrl+K command palette — one of this app's three sanctioned
// interaction surfaces alongside modals and dropdowns (see PLAN.md step
// 7: this is an SPA, new capability is not supposed to mean new pages).
// Scoped deliberately narrow for now: jump to a repo or worktree, or
// trigger the existing add-repo/new-worktree modals — not arbitrary
// command execution.
export default function CommandPalette({ onAddRepo, onNewWorktree }: Props) {
  const [open, setOpen] = useState(false);
  const { repos, worktreesByRepo } = useRepoContext();
  const navigate = useNavigate();

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
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
      <Command.Input placeholder="Jump to a repo or worktree, or run a command…" />
      <Command.List>
        <Command.Empty>No matches.</Command.Empty>

        <Command.Group heading="Actions">
          <Command.Item onSelect={() => runAndClose(onAddRepo)}>+ Add repo</Command.Item>
        </Command.Group>

        {repos.map((r) => (
          <Command.Group key={r.id} heading={r.name}>
            <Command.Item onSelect={() => runAndClose(() => navigate(`/repo/${r.id}`))}>
              Open {r.name}
            </Command.Item>
            <Command.Item onSelect={() => runAndClose(() => onNewWorktree(r.id))}>
              + New worktree in {r.name}
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
