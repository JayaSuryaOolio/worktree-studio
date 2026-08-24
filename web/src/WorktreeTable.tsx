import { useState } from "react";
import { Link } from "react-router-dom";
import { Worktree } from "./api";
import { splitBranchLabel } from "./branchLabel";
import { dominantValue, leafName, parentDir, relativeTime, shortenPath } from "./format";

// One line per worktree. This table used to take three wrapped lines per
// row for five columns, of which three said the same thing on every row:
// the full path (~62 identical leading characters, wrapping), the source
// branch ("origin/master" everywhere), and a "Status: active" badge on
// 100% of rows. See docs/design-system.md — "show the difference, not the
// data", and "if it renders on every row, it isn't a signal".
//
// Nothing is lost. What MOST rows share is hoisted into one caption above
// the table and only the rows that differ say anything, which is what
// makes those rows findable; the path is elided with the full value on
// hover and behind a copy action; and the timestamp is relative, with the
// exact one in its title.
//
// dominantValue, not commonValue: a repo with eleven worktrees, nine from
// origin/master and two attached from elsewhere, kept printing
// "origin/master" nine times because two rows disagreed. One outlier
// shouldn't defeat the whole point.
export default function WorktreeTable({
  worktrees,
  loading,
  emptyText,
}: {
  worktrees: Worktree[];
  loading: boolean;
  emptyText: string;
}) {
  if (loading) return <p className="muted">Loading…</p>;
  if (worktrees.length === 0) return <p className="muted">{emptyText}</p>;

  const source = dominantValue(worktrees, (w) => w.source_branch);
  const status = dominantValue(worktrees, (w) => w.status);
  const root = dominantValue(worktrees, (w) => parentDir(w.path));

  // A column earns its place only if some row still has something to say
  // in it. With no exceptions, every cell would be blank.
  const showSource = source === null || source.exceptions > 0;
  const showStatus = status === null || status.exceptions > 0;

  return (
    <>
      {(source || root) && (
        <p className="table-caption">
          {source && (
            <>
              {source.exceptions > 0 ? "Mostly created from " : "All created from "}
              <span className="mono">{source.value}</span>
            </>
          )}
          {source && root && <> · </>}
          {root && (
            <>
              in <span className="mono">{root.value}/</span>
            </>
          )}
          {(source?.exceptions || root?.exceptions) ? <> — exceptions shown below</> : null}
        </p>
      )}
      <table>
        <thead>
          <tr>
            <th>Branch</th>
            {showSource && <th>Created from</th>}
            <th>Path</th>
            {showStatus && <th>Status</th>}
            <th className="col-right">Created</th>
          </tr>
        </thead>
        <tbody>
          {worktrees.map((wt) => (
            <tr key={wt.id}>
              <td>
                <Link className="repo-link mono" to={`/repo/${wt.repo_id}/worktree/${wt.id}`}>
                  {wt.branch}
                </Link>
              </td>
              {showSource && (
                <td className="mono">
                  {/* Blank where it matches what the caption already said. */}
                  {source && wt.source_branch === source.value ? null : (
                    wt.source_branch || <span className="muted">—</span>
                  )}
                </td>
              )}
              <td>
                <PathCell path={wt.path} underRoot={root !== null && parentDir(wt.path) === root.value} />
              </td>
              {showStatus && (
                <td>
                  {status && wt.status === status.value ? null : (
                    <span className={`badge badge-status-${wt.status}`}>{wt.status}</span>
                  )}
                </td>
              )}
              <td className="col-right mono" title={new Date(wt.created_at).toLocaleString()}>
                {relativeTime(wt.created_at)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  );
}

// The full path stays available — on hover, and as a copy — it just stops
// taking the widest column in the table to say the same thing twenty
// times. A row under the hoisted root shows only its leaf, since the
// caption above already said where it lives.
//
// The leaf itself middle-truncates, for the same reason branch names do:
// a worktree directory is named after its branch, so the tail is what
// tells two of them apart (see branchLabel.ts).
export function PathCell({ path, underRoot }: { path: string; underRoot: boolean }) {
  const [copied, setCopied] = useState(false);
  const shown = underRoot ? leafName(path) : shortenPath(path);
  const { head, tail } = splitBranchLabel(shown);

  async function copy() {
    try {
      await navigator.clipboard.writeText(path);
      setCopied(true);
      setTimeout(() => setCopied(false), 1200);
    } catch {
      // Clipboard denied (no permission, insecure context) — the full
      // path is still on the title attribute, so this isn't a dead end.
    }
  }

  return (
    <span className="path-cell" title={path}>
      <span className="path-cell-text mono">
        <span className="path-cell-head">{head}</span>
        {tail !== "" && <span className="path-cell-tail">{tail}</span>}
      </span>
      <button type="button" className="path-cell-copy" title="Copy full path" onClick={copy}>
        {copied ? "Copied" : "⧉"}
      </button>
    </span>
  );
}
