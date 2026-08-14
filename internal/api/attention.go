// Global (not worktree-scoped, unlike handleFilesWS/handleTerminalWS) push
// channel for "a claude session in this worktree needs your input" — see
// internal/attention for the tracker itself and internal/claudehook for the
// Notification hook that feeds it via handleClaudeHook below.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/websocket"
)

// handleAttentionWS streams every attention.Event as it happens, one JSON
// text message per event: {"type":"snapshot","pending":{"<worktreeID>":
// "<message>", ...}} sent once immediately on connect (so a browser tab
// opened after the fact still sees whatever's already pending), then
// {"type":"update","worktree_id":"...","pending":true,"message":"..."} for
// every change after that. Server -> client only, same read-loop-just-for-
// disconnect-detection pattern as handleFilesWS.
func (s *Server) handleAttentionWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.Log.Warn("attention ws upgrade failed", "err", err)
		return
	}
	defer conn.Close()

	events, unsubscribe := s.Attention.Subscribe()
	defer unsubscribe()

	snapshot, _ := json.Marshal(map[string]any{"type": "snapshot", "pending": s.Attention.Snapshot()})
	if err := conn.WriteMessage(websocket.TextMessage, snapshot); err != nil {
		return
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return
			}
			payload, _ := json.Marshal(map[string]any{
				"type":        "update",
				"worktree_id": ev.WorktreeID,
				"pending":     ev.Pending,
				"message":     ev.Message,
			})
			if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}

// handleClearAttention lets the frontend explicitly dismiss a worktree's
// pending badge — called when its detail page is opened, on the theory
// that actually looking at the worktree counts as "seen it" even though
// nothing about the underlying claude session necessarily changed. Always
// succeeds (clearing an already-clear or unknown worktree id is a no-op in
// attention.Tracker, not an error).
func (s *Server) handleClearAttention(w http.ResponseWriter, r *http.Request) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}
	s.Attention.Clear(wt.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "cleared"})
}
