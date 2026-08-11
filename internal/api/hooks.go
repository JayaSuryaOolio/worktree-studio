// Global (not repo/worktree-scoped) endpoints for Claude Code hook
// integration. See internal/claudehook and PLAN.md's "Claude Code hooks"
// section for the full design.
package api

import (
	"database/sql"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"worktree-studio/internal/audit"
	"worktree-studio/internal/claudehook"
	"worktree-studio/internal/store"
)

// handleClaudeHook receives a Claude Code SessionStart hook's stdin JSON,
// forwarded verbatim as the POST body by the installed hook script (see
// internal/api/settings.go's hook install action for the script content).
// Resolves the hook's cwd to one of this server's tracked worktrees and
// logs claude.session.create with source="hook" if found.
//
// A cwd that doesn't match any tracked worktree is the expected common
// case (this hook is installed globally, so it fires for every claude
// session on the machine, not just ones inside worktree-studio-managed
// worktrees) — a deliberate, silent 200 no-op, not an error. Likewise a
// malformed body or a store error is logged but still answered 200: this
// endpoint is called synchronously by a hook with its own timeout, and a
// missed audit-log entry is far less disruptive than blocking or failing
// someone's claude session startup because worktree-studio hiccupped.
func (s *Server) handleClaudeHook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.Log.Warn("claude hook: read body", "err", err)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	payload, err := claudehook.ParsePayload(body)
	if err != nil {
		s.Log.Warn("claude hook: parse payload", "err", err)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}
	if payload.SessionID == "" || payload.Cwd == "" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	wt, err := s.Store.FindWorktreeByPath(payload.Cwd)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			s.Log.Warn("claude hook: resolve worktree by cwd", "err", err, "cwd", payload.Cwd)
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "no matching worktree"})
		return
	}

	s.auditLog(audit.EventClaudeSessionCreate, map[string]any{
		"repo_id":           wt.RepoID,
		"worktree_id":       wt.ID,
		"claude_session_id": payload.SessionID,
		"source":            "hook",
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged"})
}

// handleClaudeSessionTitle looks up a human-readable title for a claude
// session id by reading its own local transcript (see
// claudehook.SessionTitle) — used by the audit log viewer to show more
// than a bare session id for claude.session.create entries, regardless of
// whether that entry came from the hook (no title logged at all) or the
// older launch-time path (which does log a title, but the transcript is
// the more accurate source once the session has actually said something).
func (s *Server) handleClaudeSessionTitle(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	title, err := claudehook.SessionTitle(sessionID)
	if err != nil {
		if errors.Is(err, claudehook.ErrTranscriptNotFound) {
			writeError(w, http.StatusNotFound, "no transcript found for this session id")
			return
		}
		s.Log.Warn("claude session title lookup", "err", err, "session_id", sessionID)
		writeError(w, http.StatusInternalServerError, "failed to look up session title")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"title": title})
}

// handleListAllWorktrees returns every worktree across every registered
// repo (any status), for the settings modal's cross-repo "Worktrees" tab.
// Excludes the synthetic root worktrees (see EnsureRootWorktree) — same
// reasoning as handleListWorktrees.
func (s *Server) handleListAllWorktrees(w http.ResponseWriter, r *http.Request) {
	worktrees, err := s.Store.ListAllWorktreesWithRepo()
	if err != nil {
		s.Log.Error("list all worktrees", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list worktrees")
		return
	}

	out := make([]store.WorktreeWithRepo, 0, len(worktrees))
	for _, wt := range worktrees {
		if wt.Source == store.WorktreeSourceRoot {
			continue
		}
		out = append(out, wt)
	}
	writeJSON(w, http.StatusOK, out)
}
