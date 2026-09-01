// Package-level home for worktree LIFECYCLE rules: which actions are
// currently allowed against a given worktree. These are domain rules —
// "what a worktree is allowed to do" — not incidental request-handling
// glue, and they're deliberately kept together in one small file rather
// than inlined as a scattered `if wt.Pinned { ... }` at whatever handler,
// sweep, or frontend component happens to need the check first. See
// docs/architecture.md's "Worktree lifecycle rules" note for the concern
// this guards against and why it matters when this codebase gets
// refactored later.
//
// The frontend mirrors these rules for UX (disabling/hiding an action
// before the request round-trip, never as a substitute for this check)
// in web/src/worktreeLifecycle.ts — kept in sync by hand, same convention
// as internal/audit/events.go <-> web/src/auditEvents.ts.
//
// Add a new lifecycle rule here, as its own named function, when one is
// needed — not as an inline conditional at the call site that happens to
// need it first.
package api

import (
	"errors"

	"worktree-studio/internal/store"
)

// ErrWorktreePinned is returned by CanArchiveWorktree for a pinned
// worktree. handleArchiveWorktree surfaces it as a 409 — the same "the
// caller needs to act on this, not a server bug" treatment
// gitops.ErrWorktreeDirty gets for a dirty worktree delete.
var ErrWorktreePinned = errors.New("worktree is pinned")

// CanArchiveWorktree reports whether wt may currently be archived. A
// pinned worktree can never be archived — that IS what pinning means —
// regardless of whether the request came from the UI, a script, or (in
// the future) some automated sweep. The 60-day retention sweep
// (archive_sweep.go) never needs to separately check this itself: it only
// ever acts on worktrees that already reached status=archived, and a
// pinned worktree can never reach that status in the first place.
func CanArchiveWorktree(wt store.Worktree) error {
	if wt.Pinned {
		return ErrWorktreePinned
	}
	return nil
}
