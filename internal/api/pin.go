package api

import (
	"net/http"

	"worktree-studio/internal/audit"
)

// handlePinWorktree marks a worktree pinned: exempt from ever being
// archived (CanArchiveWorktree, worktree_rules.go) and sorted ahead of
// every unpinned worktree in its repo (store.ListWorktrees's ORDER BY).
// Purely a flag, same posture as archive/unarchive — no git operation.
func (s *Server) handlePinWorktree(w http.ResponseWriter, r *http.Request) {
	s.setWorktreePinned(w, r, true, audit.EventWorktreePin)
}

// handleUnpinWorktree reverses handlePinWorktree.
func (s *Server) handleUnpinWorktree(w http.ResponseWriter, r *http.Request) {
	s.setWorktreePinned(w, r, false, audit.EventWorktreeUnpin)
}

func (s *Server) setWorktreePinned(w http.ResponseWriter, r *http.Request, pinned bool, event audit.Event) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}

	if err := s.Store.SetWorktreePinned(wt.ID, pinned); err != nil {
		s.Log.Error("set worktree pinned", "err", err, "pinned", pinned)
		writeError(w, http.StatusInternalServerError, "failed to update worktree pin state")
		return
	}

	s.auditLog(event, map[string]any{
		"repo_id":     wt.RepoID,
		"worktree_id": wt.ID,
		"name":        wt.Name,
		"branch":      wt.Branch,
	})

	writeJSON(w, http.StatusOK, map[string]bool{"pinned": pinned})
}
