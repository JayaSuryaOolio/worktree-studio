// Global (not worktree-scoped) push channel for "open this file in the
// editor" — see internal/openfile for the tracker itself and hooks.go's
// handleOpenFile for the `worktree-studio open-file <path>` CLI subcommand
// that feeds it. A browser tab receiving an event for a worktree it isn't
// currently showing is expected to navigate there before opening the file
// (see web/src/RepoContext.tsx) rather than this being worktree-scoped like
// handleFilesWS — there's no dockview/editor instance to push into for a
// worktree that isn't the open tab yet.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/websocket"
)

// handleOpenFileWS streams every openfile.Event as it happens, one JSON
// text message per event: {"type":"open-file","worktree_id":"...","path":
// "..."}. Unlike handleAttentionWS there is nothing to send on connect —
// "open this file" is a one-shot instruction, not state to replay (see
// internal/openfile's doc comment). Server -> client only, same
// read-loop-just-for-disconnect-detection pattern as handleAttentionWS.
func (s *Server) handleOpenFileWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.Log.Warn("open-file ws upgrade failed", "err", err)
		return
	}
	defer conn.Close()

	events, unsubscribe := s.OpenFile.Subscribe()
	defer unsubscribe()

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
				"type":        "open-file",
				"worktree_id": ev.WorktreeID,
				"path":        ev.Path,
			})
			if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}
