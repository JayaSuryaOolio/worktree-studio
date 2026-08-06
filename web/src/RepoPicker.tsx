import { useState, FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { addRepo } from "./api";
import { useRepoContext } from "./RepoContext";

// The registered-repos list itself now lives in the sidebar (always
// visible, auto-loaded via RepoContext) — this page is just the "add a
// repo" form plus a welcome/empty state. Navigates straight to the new
// repo's workspace on success, since there's no separate list here to
// click through anymore.
export default function RepoPicker() {
  const { repos, refreshRepos } = useRepoContext();
  const navigate = useNavigate();
  const [path, setPath] = useState("");
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      const repo = await addRepo(name, path);
      setPath("");
      setName("");
      refreshRepos();
      navigate(`/repo/${repo.id}`);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="container">
      <h1>worktree-studio</h1>
      {repos.length === 0 ? (
        <p>Register a repo by its local path to get started.</p>
      ) : (
        <p>Pick a repo from the sidebar, or register another one below.</p>
      )}

      <h2>Add a repo</h2>
      <form onSubmit={onSubmit}>
        <input
          type="text"
          placeholder="/absolute/path/to/repo"
          value={path}
          onChange={(e) => setPath(e.target.value)}
          required
        />
        <input
          type="text"
          placeholder="display name (optional)"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <button type="submit" disabled={submitting}>
          {submitting ? "Adding…" : "Add repo"}
        </button>
      </form>
      {error && <p className="error">{error}</p>}
    </div>
  );
}
