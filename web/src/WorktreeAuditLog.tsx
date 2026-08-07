import { useEffect, useState } from "react";
import { AuditLogEntry, getWorktreeAuditLog } from "./api";

interface Props {
  repoId: string;
  worktreeId: string;
  title: string;
  onClose: () => void;
}

// Human-friendly label + icon per known event type. Anything not listed
// here (including event types added by future steps, e.g. a diff
// "send to agent" action) still renders fine via the fallback below —
// this map is a presentation nicety, not a whitelist.
const EVENT_LABELS: Record<string, { icon: string; label: string }> = {
  "worktree.create": { icon: "🌱", label: "Worktree created" },
  "worktree.remove": { icon: "🗑️", label: "Worktree removed" },
  "terminal.create": { icon: "🖥️", label: "Terminal opened" },
  "terminal.close": { icon: "⏹️", label: "Terminal closed" },
  "spotlight.start": { icon: "🔦", label: "Spotlight started" },
  "spotlight.stop": { icon: "🔦", label: "Spotlight stopped" },
};

// A per-worktree checkpoint summary that's worth a glance at without
// opening the raw-JSON details below (branch/path for creation, tab
// label for terminals, etc.) — keeps the common case scannable.
function summarize(entry: AuditLogEntry): string | null {
  switch (entry.event) {
    case "worktree.create":
    case "worktree.remove":
      return typeof entry.branch === "string" ? `branch ${entry.branch}` : null;
    case "terminal.create":
    case "terminal.close":
      return typeof entry.tab_label === "string" ? entry.tab_label : null;
    default:
      return null;
  }
}

export default function WorktreeAuditLog({ repoId, worktreeId, title, onClose }: Props) {
  const [entries, setEntries] = useState<AuditLogEntry[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    getWorktreeAuditLog(repoId, worktreeId)
      .then((es) => {
        if (!cancelled) setEntries(es);
      })
      .catch((err) => {
        if (!cancelled) setError((err as Error).message);
      });
    return () => {
      cancelled = true;
    };
  }, [repoId, worktreeId]);

  return (
    <div className="dialog-backdrop" onClick={onClose}>
      <div className="dialog audit-log-dialog" onClick={(e) => e.stopPropagation()}>
        <h2 style={{ marginTop: 0 }}>Log — {title}</h2>
        {error && <p className="error">{error}</p>}
        {!error && entries === null && <p className="muted">Loading…</p>}
        {!error && entries !== null && entries.length === 0 && (
          <p className="muted">No events recorded yet for this worktree.</p>
        )}
        {!error && entries !== null && entries.length > 0 && (
          <ul className="audit-log-list">
            {entries.map((e, i) => {
              const meta = EVENT_LABELS[e.event] ?? { icon: "•", label: e.event };
              const summary = summarize(e);
              return (
                <li key={i} className="audit-log-entry">
                  <span className="audit-log-icon" aria-hidden="true">
                    {meta.icon}
                  </span>
                  <span className="audit-log-body">
                    <span className="audit-log-label">{meta.label}</span>
                    {summary && <span className="audit-log-summary"> — {summary}</span>}
                    <span className="audit-log-time muted">
                      {new Date(e.ts).toLocaleString()}
                    </span>
                  </span>
                  <details className="audit-log-raw">
                    <summary>raw</summary>
                    <pre>{JSON.stringify(e, null, 2)}</pre>
                  </details>
                </li>
              );
            })}
          </ul>
        )}
        <div className="actions">
          <button type="button" onClick={onClose}>
            Close
          </button>
        </div>
      </div>
    </div>
  );
}
