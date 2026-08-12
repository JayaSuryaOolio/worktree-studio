import { useEffect, useState, FormEvent } from "react";
import { listBranches, newNameSuggestion } from "./api";

interface Props {
  repoId: string;
  onCreate: (name: string, sourceBranch: string) => Promise<void>;
  onClose: () => void;
}

export default function NewWorktreeDialog({ repoId, onCreate, onClose }: Props) {
  const [name, setName] = useState("");
  const [loadingSuggestion, setLoadingSuggestion] = useState(true);
  const [branches, setBranches] = useState<string[] | null>(null);
  const [branchesError, setBranchesError] = useState<string | null>(null);
  const [sourceBranch, setSourceBranch] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    newNameSuggestion(repoId)
      .then(setName)
      .catch((err) => setError(err.message))
      .finally(() => setLoadingSuggestion(false));
  }, [repoId]);

  useEffect(() => {
    listBranches(repoId)
      .then(({ branches, default: def }) => {
        // The resolved default might not itself be one of the listed
        // branches (e.g. DetectDefaultBranch strips a "refs/remotes/
        // origin/HEAD" pointer down to a bare name with no matching local
        // ref of its own) — prepend it rather than leave the dropdown's
        // pre-selection not actually match any <option>.
        setBranches(def && !branches.includes(def) ? [def, ...branches] : branches);
        setSourceBranch(def);
      })
      .catch((err) => setBranchesError((err as Error).message));
  }, [repoId]);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await onCreate(name, sourceBranch);
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

          <label htmlFor="wt-source-branch">Create from</label>
          {branchesError ? (
            <p className="error">{branchesError}</p>
          ) : (
            <select
              id="wt-source-branch"
              value={sourceBranch}
              onChange={(e) => setSourceBranch(e.target.value)}
              disabled={branches === null}
            >
              {branches === null ? (
                <option value="">Loading branches…</option>
              ) : (
                branches.map((b) => (
                  <option key={b} value={b}>
                    {b}
                  </option>
                ))
              )}
            </select>
          )}

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
