package api

import (
	"time"
)

// ArchivedWorktreeRetention is how long an archived worktree's git
// checkout and DB record are kept before SweepExpiredArchivedWorktrees
// removes them outright — a hard delete, not another soft-delete flag.
// Archiving itself (see store.WorktreeStatusArchived's doc comment)
// deliberately leaves everything on disk so a worktree can be reopened
// later; this is what actually reclaims that space once it's sat
// archived, untouched, long enough that it's clearly not coming back.
const ArchivedWorktreeRetention = 60 * 24 * time.Hour

// SweepExpiredArchivedWorktrees hard-removes (git worktree + DB row) every
// worktree that's been archived for longer than ArchivedWorktreeRetention.
// Meant to run once at server startup and then periodically for as long as
// the server keeps running (see main.go) — running it at startup as well
// means a worktree that crossed the threshold while the server was down
// still gets cleaned up promptly next time it starts, rather than waiting
// out a full period. Best-effort per worktree: a failure removing one
// (e.g. its repo was since unregistered, or the path is already gone) is
// logged and doesn't stop the sweep from continuing to the rest.
func (s *Server) SweepExpiredArchivedWorktrees() {
	cutoff := time.Now().UTC().Add(-ArchivedWorktreeRetention).Format(time.RFC3339)
	expired, err := s.Store.ListArchivedWorktreesOlderThan(cutoff)
	if err != nil {
		s.Log.Error("list expired archived worktrees", "err", err)
		return
	}

	for _, wt := range expired {
		repo, err := s.Store.GetRepo(wt.RepoID)
		if err != nil {
			s.Log.Error("sweep: look up repo for expired archived worktree", "err", err, "worktree_id", wt.ID)
			continue
		}
		// Always force: by this point the worktree has been archived (and
		// therefore untouched) for the full retention period, so there's
		// no one left to ask "keep these uncommitted changes?" — unlike
		// the user-initiated delete endpoint, which surfaces that as a 409
		// for a person to decide.
		if err := s.hardRemoveWorktree(repo, wt, true); err != nil {
			s.Log.Error("sweep: remove expired archived worktree", "err", err, "worktree_id", wt.ID, "path", wt.Path)
			continue
		}
		s.Log.Info("swept expired archived worktree", "worktree_id", wt.ID, "path", wt.Path, "archived_at", wt.ArchivedAt)
	}
}
