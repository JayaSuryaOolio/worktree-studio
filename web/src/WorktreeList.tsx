import { Link } from "react-router-dom";
import { Worktree } from "./api";

interface Props {
  worktrees: Worktree[];
  onDelete: (wt: Worktree) => void;
}

export default function WorktreeList({ worktrees, onDelete }: Props) {
  if (worktrees.length === 0) {
    return <p>No worktrees yet for this repo.</p>;
  }

  return (
    <table>
      <thead>
        <tr>
          <th>Branch</th>
          <th>Path</th>
          <th>Created</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        {worktrees.map((wt) => (
          <tr key={wt.id}>
            <td>{wt.branch}</td>
            <td>
              <code>{wt.path}</code>
            </td>
            <td>{new Date(wt.created_at).toLocaleString()}</td>
            <td>
              <Link to={`/repo/${wt.repo_id}/worktree/${wt.id}`}>Open</Link>{" "}
              <button className="danger" onClick={() => onDelete(wt)}>
                Delete
              </button>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
