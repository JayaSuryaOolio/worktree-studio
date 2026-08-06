import { useEffect, useState, FormEvent } from "react";
import { newNameSuggestion } from "./api";

interface Props {
  repoId: string;
  onCreate: (name: string) => Promise<void>;
  onClose: () => void;
}

export default function NewWorktreeDialog({ repoId, onCreate, onClose }: Props) {
  const [name, setName] = useState("");
  const [loadingSuggestion, setLoadingSuggestion] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    newNameSuggestion(repoId)
      .then(setName)
      .catch((err) => setError(err.message))
      .finally(() => setLoadingSuggestion(false));
  }, [repoId]);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await onCreate(name);
      onClose();
    } catch (err) {
      setError((err as Error).message);
      setSubmitting(false);
    }
  }

  return (
    <div className="dialog-backdrop" onClick={onClose}>
      <div className="dialog" onClick={(e) => e.stopPropagation()}>
        <h2 style={{ marginTop: 0 }}>New worktree</h2>
        <form onSubmit={onSubmit}>
          <label htmlFor="wt-name">
            Name (used as branch name and directory name)
          </label>
          <input
            id="wt-name"
            type="text"
            value={name}
            placeholder={loadingSuggestion ? "generating suggestion…" : ""}
            onChange={(e) => setName(e.target.value)}
            autoFocus
            required
          />
          <div className="actions">
            <button type="button" onClick={onClose}>
              Cancel
            </button>
            <button type="submit" disabled={submitting || loadingSuggestion}>
              {submitting ? "Creating…" : "Create"}
            </button>
          </div>
        </form>
        {error && <p className="error">{error}</p>}
      </div>
    </div>
  );
}
