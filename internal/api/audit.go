package api

import (
	"database/sql"
	"errors"
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"
)

// handleWorktreeAuditLog returns every audit-log entry whose "worktree_id"
// field matches this worktree, newest first. The audit log itself is a
// single global JSONL file shared by every repo/worktree (see
// internal/audit) — filtering per-worktree happens here, at read time,
// rather than by maintaining per-worktree log files, since the whole file
// is small enough for a local tool to just scan on each request.
func (s *Server) handleWorktreeAuditLog(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	worktreeID := chi.URLParam(r, "worktreeID")

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

	// Deliberately does NOT require the worktree to still exist in the
	// store: the whole point of a per-worktree log is that it outlives the
	// worktree itself (deleted worktrees, killed terminal sessions) — the
	// work's history shouldn't disappear just because the live row did.
	// Only the repo needs to exist, so a totally bogus worktree id under a
	// real repo returns an (empty) list rather than a 404.

	if s.Audit == nil {
		writeJSON(w, http.StatusOK, []map[string]any{})
		return
	}

	all, err := s.Audit.ReadAll()
	if err != nil {
		s.Log.Error("read audit log", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to read audit log")
		return
	}

	entries := make([]map[string]any, 0, len(all))
	for _, e := range all {
		wtID, _ := e["worktree_id"].(string)
		repoID, _ := e["repo_id"].(string)
		if wtID == worktreeID && repoID == repo.ID {
			entries = append(entries, e)
		}
	}

	// Newest first: the log file is append-only chronological, but a reader
	// opening this view wants to see what just happened at the top.
	sort.SliceStable(entries, func(i, j int) bool {
		ti, _ := entries[i]["ts"].(string)
		tj, _ := entries[j]["ts"].(string)
		return ti > tj
	})

	writeJSON(w, http.StatusOK, entries)
}
