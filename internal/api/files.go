// File tree, read, and write endpoints behind the in-browser editor. See
// internal/files and docs/editor-plan.md. Deliberately REST, not ws — same
// call already made for spotlight/status/layout (PLAN.md step 4's
// "design simplification" note): read/write is a request/response
// operation, not a stream. The one genuinely event-driven piece
// (fsnotify-driven external-change push) gets its own ws endpoint
// separately.
package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"worktree-studio/internal/audit"
	"worktree-studio/internal/files"
)

func (s *Server) handleFileTree(w http.ResponseWriter, r *http.Request) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}

	tree, err := files.ListTree(wt.Path)
	if err != nil {
		s.Log.Error("list file tree", "err", err, "worktree_id", wt.ID)
		writeError(w, http.StatusInternalServerError, "failed to list files")
		return
	}
	if tree == nil {
		tree = []files.FileNode{}
	}
	writeJSON(w, http.StatusOK, tree)
}

func (s *Server) handleGetFileContent(w http.ResponseWriter, r *http.Request) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "path query parameter is required")
		return
	}

	content, err := files.ReadFile(wt.Path, path)
	if err != nil {
		writeFileError(w, err)
		return
	}
	if !utf8.Valid(content) {
		writeError(w, http.StatusUnprocessableEntity, "file is not text; open it in VS Code instead")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"path": path, "content": string(content)})
}

type putFileContentRequest struct {
	Content string `json:"content"`
}

func (s *Server) handlePutFileContent(w http.ResponseWriter, r *http.Request) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "path query parameter is required")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	var req putFileContentRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Mark BEFORE writing, not after: the OS can emit (and the watcher can
	// process) the fsnotify event for this write as soon as the write
	// syscall completes, racing this handler's very next line — marking
	// first guarantees the suppression window is already active by the
	// time that event is possible. Without this, every save would
	// round-trip back as a false "changed on disk externally" push to
	// this same worktree's ws subscribers, including the tab that just
	// made the save. See docs/editor-plan.md pitfall #6. (Marking
	// pre-emptively means a write that then fails below still consumes a
	// small slice of suppression window on this path — harmless: at worst
	// one real external change to it within ~2s of the failed attempt
	// goes unreported, far better than the guaranteed race the other
	// order had.)
	if s.Files != nil {
		s.Files.MarkOwnWrite(wt.ID, path)
	}
	if err := files.WriteFile(wt.Path, path, []byte(req.Content)); err != nil {
		writeFileError(w, err)
		return
	}

	s.auditLog(audit.EventFileWrite, map[string]any{
		"repo_id":     wt.RepoID,
		"worktree_id": wt.ID,
		"path":        path,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// handleOpenInVSCode is the "complex editing" escape hatch: it shells out
// to `code <worktree-path>`, the same "best available existing tool"
// philosophy as tmux/spotlight elsewhere in this project, rather than
// trying to grow the in-browser editor into a full IDE. `code` itself
// requires a one-time manual install step (VS Code's own "Shell Command:
// Install 'code' command in PATH"), so this can fail even when VS Code
// itself is installed — see checkOnPath("code", ...) in settings.go,
// which the frontend checks before showing this button at all. A failure
// here (e.g. uninstalled between that check and this click) is still
// surfaced as a real error, not a silent no-op.
func (s *Server) handleOpenInVSCode(w http.ResponseWriter, r *http.Request) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}

	if err := exec.Command("code", wt.Path).Run(); err != nil {
		s.Log.Error("open in vscode", "err", err, "worktree_id", wt.ID)
		writeError(w, http.StatusInternalServerError, "failed to open VS Code: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "opened"})
}

// handleFilesWS pushes fsnotify-driven external-change events for a
// worktree's files — the one genuinely event-driven piece of the file
// editor (everything else is REST; see this file's top-of-file comment).
// Protocol: server -> client only, one JSON text message per changed path:
// {"type":"changed","path":"src/main.go"}. No client -> server messages are
// defined; the read loop below exists purely to detect the client
// disconnecting so the shared Watcher subscription can be released.
func (s *Server) handleFilesWS(w http.ResponseWriter, r *http.Request) {
	worktreeID := chi.URLParam(r, "worktreeID")

	wt, err := s.Store.GetWorktree(worktreeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "worktree not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to look up worktree", http.StatusInternalServerError)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.Log.Warn("files ws upgrade failed", "err", err)
		return
	}
	defer conn.Close()

	watcher, err := s.Files.Subscribe(wt.ID, wt.Path)
	if err != nil {
		s.Log.Error("subscribe file watcher", "err", err, "worktree_id", wt.ID)
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"failed to watch this worktree's files"}`))
		return
	}
	defer s.Files.Unsubscribe(wt.ID)

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
		case ev, ok := <-watcher.Events():
			if !ok {
				return
			}
			payload, _ := json.Marshal(map[string]string{"type": "changed", "path": ev.Path})
			if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}

// writeFileError maps internal/files errors to the right HTTP status —
// path-traversal attempts and "file doesn't exist" are both client errors,
// not server errors, and shouldn't be logged as if this server malfunctioned.
func writeFileError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, files.ErrPathEscapesWorktree):
		writeError(w, http.StatusBadRequest, "path escapes the worktree root")
	case errors.Is(err, files.ErrFileTooLarge):
		writeError(w, http.StatusUnprocessableEntity, "file too large to open in the editor")
	case os.IsNotExist(err):
		writeError(w, http.StatusNotFound, "file not found")
	default:
		writeError(w, http.StatusInternalServerError, "failed to access file: "+err.Error())
	}
}
