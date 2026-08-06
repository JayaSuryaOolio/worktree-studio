// Terminal layout persistence: GET/PUT a worktree's saved dockview
// arrangement (which panels, tiled how, which tabs). The layout itself is
// an opaque JSON document produced/consumed by dockview's own
// toJSON()/fromJSON() — this server never inspects its shape, just stores
// and returns it verbatim. See PLAN.md step 7.5 and docs/architecture.md.
package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

func (s *Server) handleGetLayout(w http.ResponseWriter, r *http.Request) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}

	layoutJSON, err := s.Store.GetWorktreeLayout(wt.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "no saved layout for this worktree yet")
			return
		}
		s.Log.Error("get worktree layout", "err", err, "worktree_id", wt.ID)
		writeError(w, http.StatusInternalServerError, "failed to get worktree layout")
		return
	}

	// layoutJSON is already a complete JSON document (dockview's own
	// serialization) — write it verbatim rather than round-tripping it
	// through writeJSON, which would re-encode it as a JSON *string*
	// (with escaped quotes) instead of returning the object itself.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(layoutJSON))
}

func (s *Server) handleSaveLayout(w http.ResponseWriter, r *http.Request) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	if !json.Valid(body) {
		writeError(w, http.StatusBadRequest, "request body is not valid JSON")
		return
	}

	if err := s.Store.SaveWorktreeLayout(wt.ID, string(body)); err != nil {
		s.Log.Error("save worktree layout", "err", err, "worktree_id", wt.ID)
		writeError(w, http.StatusInternalServerError, "failed to save worktree layout")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}
