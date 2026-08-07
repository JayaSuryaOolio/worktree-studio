package api

import (
	"net/http"

	"worktree-studio/internal/audit"
	"worktree-studio/internal/store"
)

// handleArchiveWorktree marks a worktree "archived" — a pure visibility
// flag, no git operation at all. The git worktree and its branch are left
// completely untouched on disk, which is the whole point: unlike delete,
// archiving something doesn't destroy the ability to reopen a terminal in
// it later (including resuming a claude session recorded against it — see
// the claude.session.create audit event). Archived worktrees are excluded
// from the normal list (handleListWorktrees) until a future settings-modal
// view exists to browse them.
func (s *Server) handleArchiveWorktree(w http.ResponseWriter, r *http.Request) {
	s.setWorktreeStatus(w, r, store.WorktreeStatusArchived, audit.EventWorktreeArchive)
}

// handleUnarchiveWorktree reverses handleArchiveWorktree.
func (s *Server) handleUnarchiveWorktree(w http.ResponseWriter, r *http.Request) {
	s.setWorktreeStatus(w, r, store.WorktreeStatusActive, audit.EventWorktreeUnarchive)
}

func (s *Server) setWorktreeStatus(w http.ResponseWriter, r *http.Request, status string, event audit.Event) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}

	if err := s.Store.SetWorktreeStatus(wt.ID, status); err != nil {
		s.Log.Error("set worktree status", "err", err, "status", status)
		writeError(w, http.StatusInternalServerError, "failed to update worktree status")
		return
	}

	s.auditLog(event, map[string]any{
		"repo_id":     wt.RepoID,
		"worktree_id": wt.ID,
		"name":        wt.Name,
		"branch":      wt.Branch,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}
