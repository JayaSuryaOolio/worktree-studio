// Frontend mirror of internal/api/worktree_rules.go's worktree lifecycle
// rules — kept in sync by hand, same convention as auditEvents.ts <->
// internal/audit/events.go. See that Go file's own doc comment (and
// docs/architecture.md's "Worktree lifecycle rules" note) for why these
// live in one small, clearly-named place instead of as a scattered
// `wt.pinned ? ... : ...` at whatever component happens to need the check
// first — add a new rule here, as its own named function, when one is
// needed.
//
// This side exists purely for UX (disabling/hiding an action, wording a
// tooltip, before the request round-trip) — the backend enforces the real
// rule regardless of what this returns, so a stale/bypassed frontend
// check can never actually let a disallowed action through.
import { Worktree } from "./api";

/** Mirrors CanArchiveWorktree: a pinned worktree can never be archived. */
export function canArchiveWorktree(wt: Pick<Worktree, "pinned">): boolean {
  return !wt.pinned;
}
