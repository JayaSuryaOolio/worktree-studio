// Worktree git-status endpoint: dirty/ahead/behind, for the monitoring
// dashboard badges in WorktreeList.tsx. See PLAN.md section on the
// monitoring-dashboard step and docs/architecture.md for the REST-polling
// simplification vs. the originally-sketched ws-push design.
package api

import (
	"net/http"

	"worktree-studio/internal/gitops"
)

func (s *Server) handleWorktreeStatus(w http.ResponseWriter, r *http.Request) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}

	status, err := gitops.Status(wt.Path)
	if err != nil {
		s.Log.Error("git status", "err", err, "worktree_id", wt.ID)
		writeError(w, http.StatusInternalServerError, "failed to get worktree status: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"branch":       status.Branch,
		"dirty":        status.Dirty,
		"has_upstream": status.HasUpstream,
		"ahead":        status.Ahead,
		"behind":       status.Behind,
	})
}
