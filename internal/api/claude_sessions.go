package api

import (
	"net/http"

	"worktree-studio/internal/claudehook"
)

// handleListWorktreeClaudeSessions returns every past claude session found
// for a worktree's own path under ~/.claude/projects/ (see
// claudehook.ListSessionsForCwd) — the dockview watermark's "past
// sessions" welcome screen, letting someone pick up an old conversation
// instead of only ever starting fresh. Independent of the audit log/DB:
// this reflects whatever Claude Code itself still has on disk, so it also
// surfaces sessions started by hand outside worktree-studio entirely.
func (s *Server) handleListWorktreeClaudeSessions(w http.ResponseWriter, r *http.Request) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}

	sessions, err := claudehook.ListSessionsForCwd(wt.Path)
	if err != nil {
		s.Log.Error("list claude sessions", "err", err, "worktree_id", wt.ID)
		writeError(w, http.StatusInternalServerError, "failed to list claude sessions: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}
