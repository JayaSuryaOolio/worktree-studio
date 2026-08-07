// Mirrors internal/audit/events.go's Event type — the set of audit-log
// event names the backend actually emits today. Kept in sync by hand
// (there's no codegen step in this project, same as every other
// cross-language constant here, e.g. the terminal resize-message JSON
// shape) — if you add a new audit.Event on the Go side, add it here too so
// EVENT_LABELS in WorktreeAuditLog.tsx can be typed against it.
//
// AuditLogEntry.event itself stays `string`, not this union — the log
// viewer must render event types it doesn't recognize (future ones added
// on the Go side before this file catches up, or ones this file simply
// never learns about) via its raw-JSON fallback rather than refusing to
// render them.
export const AUDIT_EVENTS = [
  "repo.add",
  "worktree.create",
  "worktree.remove",
  "worktree.archive",
  "worktree.unarchive",
  "terminal.create",
  "terminal.close",
  "spotlight.start",
  "spotlight.stop",
  "claude.session.create",
] as const;

export type AuditEventType = (typeof AUDIT_EVENTS)[number];
