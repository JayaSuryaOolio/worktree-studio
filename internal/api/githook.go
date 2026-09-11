// Repo-scoped endpoints for the branch-tracking git hook (see
// internal/githook): installing/uninstalling the shared post-checkout hook
// for a registered repo, and receiving that hook's fire-and-forget POST
// when someone actually checks a branch out inside one of its worktrees.
package api

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"worktree-studio/internal/audit"
	"worktree-studio/internal/githook"
)

func (s *Server) handleGitHookStatus(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	repo, err := s.Store.GetRepo(repoID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "repo not found")
			return
		}
		s.Log.Error("get repo", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to look up repo")
		return
	}

	installed, err := githook.IsInstalled(repo.Path)
	if err != nil {
		s.Log.Error("check git hook status", "err", err, "repo_id", repoID)
		writeError(w, http.StatusInternalServerError, "failed to read post-checkout hook: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"installed": installed})
}

// handleGitHookInstall installs the shared post-checkout hook for repo's
// common .git/hooks directory — see githook.Install. Only ever reached via
// an explicit user click in the repo settings UI, same posture as
// internal/claudehook's install endpoints: editing something outside
// worktree-studio's own directory is not something to do without asking.
func (s *Server) handleGitHookInstall(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	repo, err := s.Store.GetRepo(repoID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "repo not found")
			return
		}
		s.Log.Error("get repo", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to look up repo")
		return
	}

	if err := githook.Install(repo.Path, s.SelfBaseURL); err != nil {
		s.Log.Error("install git hook", "err", err, "repo_id", repoID)
		writeError(w, http.StatusInternalServerError, "failed to install post-checkout hook: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "installed"})
}

func (s *Server) handleGitHookUninstall(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	repo, err := s.Store.GetRepo(repoID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "repo not found")
			return
		}
		s.Log.Error("get repo", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to look up repo")
		return
	}

	if err := githook.Uninstall(repo.Path); err != nil {
		s.Log.Error("uninstall git hook", "err", err, "repo_id", repoID)
		writeError(w, http.StatusInternalServerError, "failed to uninstall post-checkout hook: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "uninstalled"})
}

// handleGitHookPostCheckout receives the installed post-checkout hook's
// fire-and-forget POST (form-encoded, not JSON — see githook.block) and
// logs a worktree.branch_change event if its cwd resolves to a tracked
// worktree.
//
// Same tolerant, always-200 posture as handleClaudeHook and for the same
// reason: this is called synchronously by a git hook with its own short
// curl timeout, immediately after a real `git checkout` — a missed or slow
// response here must never be visible as hook failure, so a cwd that
// doesn't match any tracked worktree (this hook is shared repo-wide, so it
// also fires for checkouts in the *main* checkout, not just worktrees) is a
// silent no-op, not an error.
func (s *Server) handleGitHookPostCheckout(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}
	cwd := r.FormValue("cwd")
	branch := r.FormValue("branch")
	if cwd == "" || branch == "" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	wt, err := s.Store.FindWorktreeByPath(cwd)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			s.Log.Warn("git hook: resolve worktree by cwd", "err", err, "cwd", cwd)
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "no matching worktree"})
		return
	}

	s.auditLog(audit.EventWorktreeBranchChange, map[string]any{
		"repo_id":     wt.RepoID,
		"worktree_id": wt.ID,
		"branch":      branch,
		"prev_head":   r.FormValue("prev_head"),
		"new_head":    r.FormValue("new_head"),
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged"})
}
