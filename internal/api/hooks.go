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
)

// handleClaudeHook receives a Claude Code hook's stdin JSON, forwarded
// verbatim as the POST body by the one installed hook script shared by
// both events this server registers (see internal/claudehook/install.go's
// hookEventNames): SessionStart, handled below the same way it always was
// (logs claude.session.create with source="hook"), and Notification —
// fired when Claude is waiting on a permission prompt or user input —
// which instead marks the resolved worktree "pending" in s.Attention so
// the sidebar can badge it (see internal/attention and
// docs/architecture.md's "Claude Code hooks" section).
//
// A cwd that doesn't match any tracked worktree is the expected common
// case (this hook is installed globally, so it fires for every claude
// session on the machine, not just ones inside worktree-studio-managed
// worktrees) — a deliberate, silent 200 no-op, not an error. Likewise a
// malformed body or a store error is logged but still answered 200: this
// endpoint is called synchronously by a hook with its own timeout, and a
// missed event is far less disruptive than blocking or failing someone's
// claude session because worktree-studio hiccupped.
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
	if payload.Cwd == "" {
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

	if payload.HookEventName == "Notification" {
		s.Attention.SetPending(wt.ID, payload.Message)
		writeJSON(w, http.StatusOK, map[string]string{"status": "pending"})
		return
	}

	if payload.SessionID == "" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
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

// handleClaudeHookContext receives the session-context hook script's
// best-effort POST (see claudehook.contextScriptContent) of the exact text
// it printed to Claude's context, and logs it as claude.session.context if
// the cwd resolves to a tracked worktree — this is what lets the "Log"
// view in the UI show what got injected, not just that the hook ran.
//
// Same tolerant, always-200 posture as handleClaudeHook and for the same
// reason: this is called fire-and-forget by a shell script with its own
// short timeout at session start, and a missed log entry is far less
// disruptive than making that script's POST look like a failure.
func (s *Server) handleClaudeHookContext(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.Log.Warn("claude hook context: read body", "err", err)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	payload, err := claudehook.ParseContextPayload(body)
	if err != nil {
		s.Log.Warn("claude hook context: parse payload", "err", err)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}
	if payload.Cwd == "" || payload.Context == "" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	wt, err := s.Store.FindWorktreeByPath(payload.Cwd)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			s.Log.Warn("claude hook context: resolve worktree by cwd", "err", err, "cwd", payload.Cwd)
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "no matching worktree"})
		return
	}

	s.auditLog(audit.EventClaudeSessionContext, map[string]any{
		"repo_id":     wt.RepoID,
		"worktree_id": wt.ID,
		"context":     payload.Context,
		"source":      "hook",
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
