package api

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

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
//
// Its ptyd terminals' shells are ended (they'd otherwise sit in the daemon
// for as long as it's archived) but their tabs and scrollback are kept, and
// unarchiving brings them back (term.Manager.SuspendWorktree).
func (s *Server) handleArchiveWorktree(w http.ResponseWriter, r *http.Request) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}
	if err := CanArchiveWorktree(wt); err != nil {
		writeError(w, http.StatusConflict, "pinned worktrees can't be archived — unpin it first")
		return
	}
	s.RecordClaudeSessions() // so the restored tabs resume their claude
	if err := s.Term.SuspendWorktree(wt.ID); err != nil {
		s.Log.Warn("end archived worktree's ptyd terminals", "err", err, "worktree_id", wt.ID)
	}
	s.setWorktreeStatus(w, wt, store.WorktreeStatusArchived, audit.EventWorktreeArchive)
}

// handleUnarchiveWorktree reverses handleArchiveWorktree, restoring its ptyd
// terminals. No lifecycle rule gates this direction — unpinning isn't
// required, and a pinned worktree can never have reached "archived" in the
// first place anyway.
func (s *Server) handleUnarchiveWorktree(w http.ResponseWriter, r *http.Request) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}
	if _, err := s.Term.RestoreWorktree(wt.ID); err != nil {
		s.Log.Warn("restore unarchived worktree's ptyd terminals", "err", err, "worktree_id", wt.ID)
	}
	s.setWorktreeStatus(w, wt, store.WorktreeStatusActive, audit.EventWorktreeUnarchive)
}

// handleListArchivedWorktrees returns every archived worktree for a repo —
// the settings page's "Archived worktrees" section, where an archived
// worktree can be unarchived (handleUnarchiveWorktree) before
// SweepExpiredArchivedWorktrees hard-removes it (git worktree + DB row)
// once it's been archived for ArchivedWorktreeRetention.
func (s *Server) handleListArchivedWorktrees(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")

	if _, err := s.Store.GetRepo(repoID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "repo not found")
			return
		}
		s.Log.Error("get repo", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to look up repo")
		return
	}

	worktrees, err := s.Store.ListWorktrees(repoID, store.WorktreeStatusArchived)
	if err != nil {
		s.Log.Error("list archived worktrees", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list archived worktrees")
		return
	}
	if worktrees == nil {
		worktrees = []store.Worktree{}
	}
	writeJSON(w, http.StatusOK, worktrees)
}

func (s *Server) setWorktreeStatus(w http.ResponseWriter, wt store.Worktree, status string, event audit.Event) {
	if err := s.Store.SetWorktreeStatus(wt.ID, status); err != nil {
		s.Log.Error("set worktree status", "err", err, "status", status)
		writeError(w, http.StatusInternalServerError, "failed to update worktree status")
		return
	}
	s.settleTask(wt.TaskSource, wt.TaskKey)

	s.auditLog(event, map[string]any{
		"repo_id":     wt.RepoID,
		"worktree_id": wt.ID,
		"name":        wt.Name,
		"branch":      wt.Branch,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}
