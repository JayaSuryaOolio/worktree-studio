// Spotlight endpoints: start/stop/status for mirroring a worktree's source
// into its repo's root checkout via the external `spotlight` CLI. See
// internal/spotlight and PLAN.md section 2.
package api

import (
	"errors"
	"net/http"
	"path/filepath"

	"worktree-studio/internal/audit"
	"worktree-studio/internal/spotlight"
)

// resolveBestEffortEqual reports whether two paths refer to the same
// location, tolerating the same /tmp-or-/var-is-a-symlink divergence that
// internal/spotlight.StatusForRoot already accounts for.
func resolveBestEffortEqual(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return a == b
	}
	return ra == rb
}

func (s *Server) handleSpotlightStatus(w http.ResponseWriter, r *http.Request) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}
	repo, err := s.Store.GetRepo(wt.RepoID)
	if err != nil {
		s.Log.Error("get repo for spotlight status", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to look up repo")
		return
	}

	status, err := spotlight.StatusForRoot(repo.Path)
	if err != nil {
		if errors.Is(err, spotlight.ErrNotFound) {
			writeJSON(w, http.StatusOK, map[string]any{"available": false})
			return
		}
		s.Log.Error("spotlight status", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to query spotlight status")
		return
	}

	if status == nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": true, "active": false})
		return
	}

	active := resolveBestEffortEqual(status.Worktree, wt.Path)
	writeJSON(w, http.StatusOK, map[string]any{
		"available":            true,
		"active":               active,
		"root":                 status.Root,
		"active_worktree_path": status.Worktree,
		"pid":                  status.PID,
	})
}

func (s *Server) handleSpotlightStart(w http.ResponseWriter, r *http.Request) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}

	root, err := spotlight.Start(wt.Path)
	if err != nil {
		switch {
		case errors.Is(err, spotlight.ErrNotFound):
			writeError(w, http.StatusServiceUnavailable, "spotlight CLI is not installed (see github.com/JayaSuryaOolio/spotlight)")
		case errors.Is(err, spotlight.ErrRootDirty):
			writeError(w, http.StatusConflict, "the repo's root checkout has uncommitted changes; commit or stash them before starting spotlight")
		default:
			s.Log.Error("spotlight start", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to start spotlight: "+err.Error())
		}
		return
	}

	s.auditLog(audit.EventSpotlightStart, map[string]any{
		"repo_id":     wt.RepoID,
		"worktree_id": wt.ID,
		"worktree":    wt.Path,
		"root":        root,
	})
	writeJSON(w, http.StatusOK, map[string]string{"root": root})
}

func (s *Server) handleSpotlightStop(w http.ResponseWriter, r *http.Request) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}
	repo, err := s.Store.GetRepo(wt.RepoID)
	if err != nil {
		s.Log.Error("get repo for spotlight stop", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to look up repo")
		return
	}

	if err := spotlight.Stop(repo.Path); err != nil {
		if errors.Is(err, spotlight.ErrNotFound) {
			writeError(w, http.StatusServiceUnavailable, "spotlight CLI is not installed (see github.com/JayaSuryaOolio/spotlight)")
			return
		}
		s.Log.Error("spotlight stop", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to stop spotlight: "+err.Error())
		return
	}

	s.auditLog(audit.EventSpotlightStop, map[string]any{
		"repo_id":     wt.RepoID,
		"worktree_id": wt.ID,
		"worktree":    wt.Path,
		"root":        repo.Path,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}
