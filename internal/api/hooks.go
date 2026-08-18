// Global (not repo/worktree-scoped) endpoints for Claude Code hook
// integration. See internal/claudehook and PLAN.md's "Claude Code hooks"
// section for the full design.
package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"

	"github.com/go-chi/chi/v5"

	"worktree-studio/internal/audit"
	"worktree-studio/internal/claudehook"
	"worktree-studio/internal/files"
	"worktree-studio/internal/openfile"
)

// handleClaudeHook receives a Claude Code hook's stdin JSON, forwarded
// verbatim as the POST body by the one installed hook script shared by
// both events this server registers (see internal/claudehook/install.go's
// hookEventNames): SessionStart, handled below the same way it always was
// (logs claude.session.create with source="hook"), and Notification —
// fired when Claude is waiting on a permission prompt, waiting for user
// input, or reporting something finished — which instead marks the
// resolved worktree "pending" in s.Attention so the sidebar can badge it
// (see internal/attention and docs/architecture.md's "Claude Code hooks"
// section). A Notification whose message is just background-progress
// chatter (see claudehook.IsBlockingNotification) is deliberately not
// badged — per direct feedback, seeing "waiting for background agents…"
// pop up as if it needed you was noise, not signal.
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
		if !claudehook.IsBlockingNotification(payload.Message) {
			// A status update about ongoing background work (e.g. still
			// waiting on background agents), not something that actually
			// needs the user — see claudehook.IsBlockingNotification's own
			// comment. Deliberately not badged/notified.
			writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
			return
		}
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

type openFileRequest struct {
	Cwd  string `json:"cwd"`
	Path string `json:"path"`
}

// handleOpenFile receives the `worktree-studio open-file <path>` CLI
// subcommand's POST (see cmd/worktree-studio/openfile.go), resolves which
// tracked worktree the calling shell's cwd belongs to, and publishes an
// openfile.Event so any browser tab with that worktree open can open the
// file in its editor (see internal/openfile and web/src/useOpenFileStream.ts).
//
// Unlike handleClaudeHook, this is called interactively by a human waiting
// on the CLI's own exit code, not fire-and-forget by a hook script with its
// own timeout — so failures here are real errors (4xx/5xx), not silent 200
// no-ops, except for "cwd isn't inside any tracked worktree" which is a
// completely normal outcome (the CLI can be run from anywhere) reported as
// its own distinct status rather than an error.
func (s *Server) handleOpenFile(w http.ResponseWriter, r *http.Request) {
	var req openFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Cwd == "" || req.Path == "" {
		writeError(w, http.StatusBadRequest, "cwd and path are required")
		return
	}

	wt, err := s.Store.FindWorktreeByPath(req.Cwd)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "no matching worktree"})
			return
		}
		s.Log.Warn("open-file: resolve worktree by cwd", "err", err, "cwd", req.Cwd)
		writeError(w, http.StatusInternalServerError, "failed to resolve worktree")
		return
	}

	// req.Path may be relative to req.Cwd (the common case: `worktree-studio
	// open-file some/file.go` run from inside a subdirectory) or absolute;
	// either way it's first made absolute, then re-expressed relative to the
	// worktree root before going through files.ResolvePath's own escape
	// check — that function only accepts an already-relative path.
	absPath := req.Path
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(req.Cwd, req.Path)
	}
	relPath, err := filepath.Rel(wt.Path, absPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to resolve path")
		return
	}
	if _, err := files.ResolvePath(wt.Path, relPath); err != nil {
		writeError(w, http.StatusBadRequest, "path is outside the worktree")
		return
	}

	s.OpenFile.Publish(openfile.Event{WorktreeID: wt.ID, Path: filepath.ToSlash(relPath)})
	writeJSON(w, http.StatusOK, map[string]string{"status": "opened"})
}
