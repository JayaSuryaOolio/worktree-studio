import { useEffect, useState, FormEvent } from "react";
import { Link } from "react-router-dom";
import { addRepo, listRepos, Repo } from "./api";

export default function RepoPicker() {
  const [repos, setRepos] = useState<Repo[]>([]);
  const [path, setPath] = useState("");
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  function refresh() {
    listRepos()
      .then(setRepos)
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }

  useEffect(refresh, []);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await addRepo(name, path);
      setPath("");
      setName("");
      refresh();
    } catch (err) {
      setError((err as Error).message);
    }
  }

  return (
    <div className="container">
      <h1>worktree-studio</h1>
      <p>Register a repo by its local path, then manage its worktrees.</p>

      <h2>Registered repos</h2>
      {loading ? (
        <p>Loading…</p>
      ) : repos.length === 0 ? (
        <p>No repos registered yet.</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Path</th>
            </tr>
          </thead>
          <tbody>
            {repos.map((r) => (
              <tr key={r.id}>
                <td>
                  <Link className="repo-link" to={`/repo/${r.id}`}>
                    {r.name}
                  </Link>
                </td>
                <td>
                  <code>{r.path}</code>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
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
        <button type="submit">Add repo</button>
      </form>
      {error && <p className="error">{error}</p>}
    </div>
  );
}
