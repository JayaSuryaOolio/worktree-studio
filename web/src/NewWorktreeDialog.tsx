import { useEffect, useRef, useState, FormEvent } from "react";
import { listBranches, newNameSuggestion, TaskRef } from "./api";
import { useRepoContext } from "./RepoContext";
import TaskPicker, { PickedTask } from "./TaskPicker";
import { taskWorktreeName } from "./tasks";

interface Props {
  repoId: string;
  // Pre-picked when opened from a task (popup, board) — "another worktree
  // for this task", usually in a different repo.
  task?: PickedTask;
  onCreate: (repoId: string, name: string, sourceBranch: string, task?: TaskRef) => Promise<void>;
  onClose: () => void;
}

export default function NewWorktreeDialog({ repoId: initialRepoId, task: initialTask, onCreate, onClose }: Props) {
  const { repos } = useRepoContext();
  const [repoId, setRepoId] = useState(initialRepoId);
  const [task, setTask] = useState<PickedTask | null>(initialTask ?? null);
  const [name, setName] = useState(initialTask ? taskWorktreeName(initialTask) : "");
  // Once the name is typed by hand, picking a task no longer overwrites it.
  const nameTouched = useRef(false);
  const [loadingSuggestion, setLoadingSuggestion] = useState(true);
  const [branches, setBranches] = useState<string[] | null>(null);
  const [branchesError, setBranchesError] = useState<string | null>(null);
  const [sourceBranch, setSourceBranch] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (initialTask) {
      setLoadingSuggestion(false);
      return;
    }
    newNameSuggestion(repoId)
      .then((n) => !nameTouched.current && setName(n))
      .catch((err) => setError(err.message))
      .finally(() => setLoadingSuggestion(false));
  }, [repoId, initialTask]);

  useEffect(() => {
    setBranches(null);
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
      await onCreate(repoId, name, sourceBranch, task?.ref);
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
          {repos.length > 1 && (
            <>
              <label htmlFor="wt-repo">Repo</label>
              <select id="wt-repo" value={repoId} onChange={(e) => setRepoId(e.target.value)}>
                {repos.map((r) => (
                  <option key={r.id} value={r.id}>
                    {r.name}
                  </option>
                ))}
              </select>
            </>
          )}

          <label>Task</label>
          <TaskPicker
            value={task}
            onChange={(t) => {
              setTask(t);
              if (t && !nameTouched.current) setName(taskWorktreeName(t));
            }}
          />

          <label htmlFor="wt-name">
            Name (used as branch name and directory name)
          </label>
          <input
            id="wt-name"
            type="text"
            value={name}
            placeholder={loadingSuggestion ? "generating suggestion…" : ""}
            onChange={(e) => {
              nameTouched.current = true;
              setName(e.target.value);
            }}
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
