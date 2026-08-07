import { useEffect, useState } from "react";
import { AuditLogEntry, getClaudeSessionTitle, getWorktreeAuditLog } from "./api";
import { AuditEventType } from "./auditEvents";

interface Props {
  repoId: string;
  worktreeId: string;
  title: string;
  onClose: () => void;
}

// Human-friendly label + icon per known event type. Keyed by AuditEventType
// (Record<AuditEventType, ...> below) so adding a new audit.Event on the Go
// side without updating auditEvents.ts/this map is a compile error here,
// not a silently-unstyled row — but the lookup at render time still falls
// back gracefully (see EVENT_LABELS[e.event] below) for any event this
// frontend build genuinely doesn't know about yet (an older frontend build
// talking to a newer backend, e.g. mid-deploy).
const EVENT_LABELS: Record<AuditEventType, { icon: string; label: string }> = {
  // Never actually shown here in practice (repo.add carries no
  // worktree_id, so it can't match this view's filter) — included only so
  // Record<AuditEventType, ...> stays exhaustive against auditEvents.ts.
  "repo.add": { icon: "📁", label: "Repo registered" },
  "worktree.create": { icon: "🌱", label: "Worktree created" },
  "worktree.remove": { icon: "🗑️", label: "Worktree removed" },
  "worktree.archive": { icon: "📦", label: "Worktree archived" },
  "worktree.unarchive": { icon: "📤", label: "Worktree unarchived" },
  "terminal.create": { icon: "🖥️", label: "Terminal opened" },
  "terminal.close": { icon: "⏹️", label: "Terminal closed" },
  "spotlight.start": { icon: "🔦", label: "Spotlight started" },
  "spotlight.stop": { icon: "🔦", label: "Spotlight stopped" },
  "claude.session.create": { icon: "🤖", label: "Claude session started" },
};

// A per-worktree checkpoint summary that's worth a glance at without
// opening the raw-JSON details below (branch/path for creation, tab
// label for terminals, etc.) — keeps the common case scannable.
//
// realTitles maps claude_session_id -> a title fetched live from that
// session's own local transcript (see fetchClaudeTitles below) — when
// available, it's a much better label than the one this app itself
// assigned at launch time (which the hook-driven path doesn't even set —
// see internal/claudehook), since it's the session's actual first message
// rather than just the worktree's name repeated back.
function summarize(entry: AuditLogEntry, realTitles: Record<string, string | null>): string | null {
  switch (entry.event) {
    case "worktree.create":
    case "worktree.remove":
      return typeof entry.branch === "string" ? `branch ${entry.branch}` : null;
    case "terminal.create":
    case "terminal.close":
      return typeof entry.tab_label === "string" ? entry.tab_label : null;
    case "claude.session.create": {
      const id = entry.claude_session_id;
      if (typeof id !== "string") return null;
      const real = realTitles[id];
      const label = real ?? (typeof entry.title === "string" ? entry.title : undefined);
      return label ? `${label} (${id})` : id;
    }
    default:
      return null;
  }
}

export default function WorktreeAuditLog({ repoId, worktreeId, title, onClose }: Props) {
  const [entries, setEntries] = useState<AuditLogEntry[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [realTitles, setRealTitles] = useState<Record<string, string | null>>({});

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

  useEffect(() => {
    if (!entries) return;
    let cancelled = false;
    const ids = entries
      .filter((e) => e.event === "claude.session.create")
      .map((e) => e.claude_session_id)
      .filter((id): id is string => typeof id === "string");
    // Best-effort, one fetch per session id, silently ignoring failures —
    // a missing/unreadable transcript just means this entry falls back to
    // its stored `title` field (or the bare id), not an error worth
    // surfacing in a log viewer.
    for (const id of new Set(ids)) {
      getClaudeSessionTitle(id)
        .then((t) => {
          if (!cancelled) setRealTitles((prev) => ({ ...prev, [id]: t }));
        })
        .catch(() => {
          /* fall back to the stored title/bare id — see summarize() */
        });
    }
    return () => {
      cancelled = true;
    };
  }, [entries]);

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
              // e.event is `string` (see AuditLogEntry) since the log must
              // render event types this frontend build doesn't know about
              // yet — the cast is safe because of the `?? fallback` right
              // after it, which is what actually handles that case.
              const meta = (EVENT_LABELS as Record<string, { icon: string; label: string }>)[e.event] ?? {
                icon: "•",
                label: e.event,
              };
              const summary = summarize(e, realTitles);
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
