import { useState, FormEvent } from "react";

interface Props {
  repoName: string;
  onAttach: (path: string, name: string) => Promise<void>;
  onClose: () => void;
}

// Registers an already-on-disk git worktree (created by hand, or by some
// other tool) rather than creating a new one — the counterpart to
// NewWorktreeDialog. No git mutation happens here at all; the backend just
// checks the path is actually one of this repo's real worktrees.
export default function AttachWorktreeDialog({ repoName, onAttach, onClose }: Props) {
  const [path, setPath] = useState("");
  const [name, setName] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await onAttach(path, name);
      onClose();
    } catch (err) {
      setError((err as Error).message);
      setSubmitting(false);
    }
  }

  return (
    <div className="dialog-backdrop" onClick={onClose}>
      <div className="dialog" onClick={(e) => e.stopPropagation()}>
        <h2 style={{ marginTop: 0 }}>Attach existing worktree in {repoName}</h2>
        <form onSubmit={onSubmit}>
          <label htmlFor="attach-wt-path">Worktree path</label>
          <input
            id="attach-wt-path"
            type="text"
            placeholder="/absolute/path/to/existing/worktree"
            value={path}
            onChange={(e) => setPath(e.target.value)}
            autoFocus
            required
          />
          <label htmlFor="attach-wt-name">Display name (optional)</label>
          <input
            id="attach-wt-name"
            type="text"
            placeholder="defaults to ext_<directory name>"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
          <div className="actions">
            <button type="button" onClick={onClose}>
              Cancel
            </button>
            <button type="submit" disabled={submitting}>
              {submitting ? "Attaching…" : "Attach"}
            </button>
          </div>
        </form>
        {error && <p className="error">{error}</p>}
      </div>
    </div>
  );
}
