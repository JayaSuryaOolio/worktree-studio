package api

import (
	"net/http"
)

// handlePinWorktree marks a worktree pinned: exempt from ever being
// archived (CanArchiveWorktree, worktree_rules.go) and sorted ahead of
// every unpinned worktree in its repo (store.ListWorktrees's ORDER BY).
// Purely a flag, same posture as archive/unarchive — no git operation.
//
// Deliberately not audit-logged: pin/unpin was audit-logged briefly, but
// per direct feedback it's noise, not a checkpoint worth a permanent
// record — unlike archive/create/remove, toggling it destroys nothing and
// carries no useful "what happened to this piece of work" signal on its
// own, especially skimming a log full of other, more meaningful entries.
func (s *Server) handlePinWorktree(w http.ResponseWriter, r *http.Request) {
	s.setWorktreePinned(w, r, true)
}

// handleUnpinWorktree reverses handlePinWorktree.
func (s *Server) handleUnpinWorktree(w http.ResponseWriter, r *http.Request) {
	s.setWorktreePinned(w, r, false)
}

func (s *Server) setWorktreePinned(w http.ResponseWriter, r *http.Request, pinned bool) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}

	if err := s.Store.SetWorktreePinned(wt.ID, pinned); err != nil {
		s.Log.Error("set worktree pinned", "err", err, "pinned", pinned)
		writeError(w, http.StatusInternalServerError, "failed to update worktree pin state")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"pinned": pinned})
}
