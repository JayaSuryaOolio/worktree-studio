// Terminal endpoints: REST CRUD for tmux-backed terminal sessions scoped to
// a worktree, plus the websocket endpoint that attaches a pty to the tmux
// session and relays it to the browser. See internal/term and PLAN.md
// section 3.
package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"worktree-studio/internal/audit"
	"worktree-studio/internal/store"
	"worktree-studio/internal/term"
)

// getRepoAndWorktree looks up a worktree scoped to a repo, writing the
// appropriate 404/500 response and returning ok=false if it can't be found.
func (s *Server) getRepoAndWorktree(w http.ResponseWriter, r *http.Request) (store.Worktree, bool) {
	repoID := chi.URLParam(r, "repoID")
	worktreeID := chi.URLParam(r, "worktreeID")

	repo, err := s.Store.GetRepo(repoID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "repo not found")
			return store.Worktree{}, false
		}
		s.Log.Error("get repo", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to look up repo")
		return store.Worktree{}, false
	}

	wt, err := s.Store.GetWorktree(worktreeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "worktree not found")
			return store.Worktree{}, false
		}
		s.Log.Error("get worktree", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to look up worktree")
		return store.Worktree{}, false
	}
	if wt.RepoID != repo.ID {
		writeError(w, http.StatusNotFound, "worktree not found")
		return store.Worktree{}, false
	}
	return wt, true
}

func (s *Server) handleListTerminals(w http.ResponseWriter, r *http.Request) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}
	sessions, err := s.Term.ListSessions(wt.ID)
	if err != nil {
		s.Log.Error("list terminal sessions", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list terminal sessions")
		return
	}
	if sessions == nil {
		sessions = []store.TerminalSession{}
	}
	writeJSON(w, http.StatusOK, sessions)
}

type createTerminalRequest struct {
	TabLabel       string `json:"tab_label"`
	InitialCommand string `json:"initial_command"`
	// ClaudeSessionID/ClaudeSessionTitle are set by the frontend when the
	// initial command is a `claude --session-id <uuid> ...` invocation it
	// generated itself (see createWorktreeWithClaudeTerminal in
	// worktreeActions.ts) — logged as their own audit event so the id (and
	// a human-readable title) survive independently of this terminal/tmux
	// session, letting a person later `claude --resume <id>` in this
	// worktree even after the tab/session that started it is long gone.
	ClaudeSessionID    string `json:"claude_session_id"`
	ClaudeSessionTitle string `json:"claude_session_title"`
}

func (s *Server) handleCreateTerminal(w http.ResponseWriter, r *http.Request) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}

	var req createTerminalRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req) // body is optional; a bad/empty body just means "use the default label, no initial command"
	}
	label := req.TabLabel
	if label == "" {
		label = "shell"
	}

	ts, err := s.Term.CreateSession(wt.ID, wt.Path, label, req.InitialCommand)
	if err != nil {
		s.Log.Error("create terminal session", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to create terminal session: "+err.Error())
		return
	}

	if req.ClaudeSessionID != "" {
		s.auditLog(audit.EventClaudeSessionCreate, map[string]any{
			"repo_id":           wt.RepoID,
			"worktree_id":       wt.ID,
			"terminal_id":       ts.ID,
			"claude_session_id": req.ClaudeSessionID,
			"title":             req.ClaudeSessionTitle,
		})
	}

	writeJSON(w, http.StatusCreated, ts)
}

func (s *Server) handleDeleteTerminal(w http.ResponseWriter, r *http.Request) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}
	terminalID := chi.URLParam(r, "terminalID")

	ts, err := s.Store.GetTerminalSession(terminalID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "terminal session not found")
			return
		}
		s.Log.Error("get terminal session", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to look up terminal session")
		return
	}
	if ts.WorktreeID != wt.ID {
		writeError(w, http.StatusNotFound, "terminal session not found")
		return
	}

	if err := s.Term.CloseSession(ts); err != nil {
		s.Log.Error("close terminal session", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to close terminal session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "closed"})
}

var upgrader = websocket.Upgrader{
	// This is a local dev tool bound to localhost by default (see PLAN.md's
	// deferred auth/remote-access hardening note) — same-origin checks
	// aren't the load-bearing security boundary here, so allow any origin
	// rather than fighting the dev-server proxy's different origin.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// handleTerminalWS upgrades to a websocket, attaches a pty to the terminal
// session's tmux session, and relays bytes both ways. Protocol: binary ws
// messages are raw bytes fed straight into the pty (keystrokes in, tmux's
// rendered output out); text ws messages are JSON control frames, of which
// the only one currently defined is {"type":"resize","cols":N,"rows":N}.
func (s *Server) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	terminalID := chi.URLParam(r, "terminalID")

	ts, err := s.Store.GetTerminalSession(terminalID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "terminal session not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to look up terminal session", http.StatusInternalServerError)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.Log.Warn("ws upgrade failed", "err", err)
		return
	}
	defer conn.Close()

	pf, cmd, err := term.Attach(ts.TmuxSessionName)
	if err != nil {
		s.Log.Error("attach pty", "err", err)
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"failed to attach terminal"}`))
		return
	}
	defer func() {
		_ = pf.Close()
		_ = cmd.Process.Kill() // kills the `tmux attach` client, NOT the tmux session it was attached to
	}()

	// pty -> websocket
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 32*1024)
		for {
			n, err := pf.Read(buf)
			if n > 0 {
				if werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					s.Log.Debug("pty read ended", "err", err)
				}
				return
			}
		}
	}()

	// websocket -> pty (+ resize control messages)
	type resizeMsg struct {
		Type string `json:"type"`
		Cols uint16 `json:"cols"`
		Rows uint16 `json:"rows"`
	}
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		switch msgType {
		case websocket.BinaryMessage:
			if _, err := pf.Write(data); err != nil {
				break
			}
		case websocket.TextMessage:
			var m resizeMsg
			if json.Unmarshal(data, &m) == nil && m.Type == "resize" && m.Cols > 0 && m.Rows > 0 {
				_ = term.Resize(pf, m.Cols, m.Rows)
			}
		}
	}

	<-done
}
