import { useState, FormEvent } from "react";
import { addRepo, Repo } from "./api";

interface Props {
  onCreated: (repo: Repo) => void;
  onClose: () => void;
}

export default function AddRepoDialog({ onCreated, onClose }: Props) {
  const [path, setPath] = useState("");
  const [name, setName] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const repo = await addRepo(name, path);
      onCreated(repo);
      onClose();
    } catch (err) {
      setError((err as Error).message);
      setSubmitting(false);
    }
  }

  return (
    <div className="dialog-backdrop" onClick={onClose}>
      <div className="dialog" onClick={(e) => e.stopPropagation()}>
        <h2 style={{ marginTop: 0 }}>Add repo</h2>
        <form onSubmit={onSubmit}>
          <label htmlFor="repo-path">Local path</label>
          <input
            id="repo-path"
            type="text"
            placeholder="/absolute/path/to/repo"
            value={path}
            onChange={(e) => setPath(e.target.value)}
            autoFocus
            required
          />
          <label htmlFor="repo-name">Display name (optional)</label>
          <input
            id="repo-name"
            type="text"
            placeholder="defaults to the directory name"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
          <div className="actions">
            <button type="button" onClick={onClose}>
              Cancel
            </button>
            <button type="submit" disabled={submitting}>
              {submitting ? "Adding…" : "Add repo"}
            </button>
          </div>
        </form>
        {error && <p className="error">{error}</p>}
      </div>
    </div>
  );
}
