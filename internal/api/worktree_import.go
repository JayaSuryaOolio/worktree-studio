// Importing an existing, manually-created git worktree into
// worktree-studio's registry — the counterpart to handleCreateWorktree,
// which always runs `git worktree add` itself. This path does no git
// mutation at all: it only registers a `worktrees` row for a worktree
// that's already sitting on disk (e.g. created by hand with `git worktree
// add`, or by some other tool), so it shows up in the UI the same as one
// worktree-studio created itself.
package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"worktree-studio/internal/audit"
	"worktree-studio/internal/gitops"
	"worktree-studio/internal/store"
)

type importWorktreeRequest struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

func (s *Server) handleImportWorktree(w http.ResponseWriter, r *http.Request) {
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

	var req importWorktreeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	path := filepath.Clean(req.Path)
	if path == "" || path == "." {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if !filepath.IsAbs(path) {
		writeError(w, http.StatusBadRequest, "path must be absolute")
		return
	}

	entries, err := gitops.ListWorktrees(repo.Path)
	if err != nil {
		s.Log.Error("list worktrees for import", "err", err, "repo_id", repo.ID)
		writeError(w, http.StatusInternalServerError, "failed to inspect this repo's git worktrees")
		return
	}

	entry, ok := findWorktreeEntryByPath(entries, path)
	if !ok {
		writeError(w, http.StatusBadRequest,
			"path is not a git worktree of this repo — it must already appear in `git worktree list` run from "+repo.Path)
		return
	}

	branch := strings.TrimPrefix(entry.Branch, "refs/heads/")
	if branch == "" {
		// A detached-HEAD worktree has no "branch" line in `git worktree
		// list --porcelain` output at all. Most of this app's UI (branch
		// display, the audit log, future push/PR workflows) assumes a real
		// branch — rather than half-supporting a branchless worktree with
		// a fake label, ask the user to check one out first.
		writeError(w, http.StatusBadRequest,
			"this worktree is in detached HEAD state (no branch checked out) — check out a branch there first, then import it")
		return
	}

	exists, err := s.Store.WorktreePathExists(entry.Path)
	if err != nil {
		s.Log.Error("check worktree exists", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to check existing worktrees")
		return
	}
	if exists {
		writeError(w, http.StatusConflict, "a worktree at this path is already registered")
		return
	}

	name := req.Name
	if name == "" {
		// Prefixed so an imported worktree is visually distinguishable in
		// the list from one worktree-studio created itself — those are
		// always a bare adjective-noun slug, never "ext_"-prefixed.
		name = "ext_" + filepath.Base(entry.Path)
	}

	wt := store.Worktree{
		ID:        newID(),
		RepoID:    repo.ID,
		Name:      name,
		Branch:    branch,
		Path:      entry.Path,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Status:    store.WorktreeStatusActive,
	}
	if err := s.Store.AddWorktree(wt); err != nil {
		s.Log.Error("save imported worktree", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to save worktree record")
		return
	}

	s.auditLog(audit.EventWorktreeImport, map[string]any{
		"repo_id":     repo.ID,
		"worktree_id": wt.ID,
		"name":        wt.Name,
		"branch":      wt.Branch,
		"path":        wt.Path,
	})

	writeJSON(w, http.StatusCreated, wt)
}

// findWorktreeEntryByPath finds the entry (if any) whose path refers to the
// same directory as path — a plain string compare would false-negative on
// macOS, where a path under /tmp or /var (as a user might type, or as many
// tools report) and git's own canonicalized /private/tmp or /private/var
// form are the same directory but different strings. Reuses
// resolveBestEffortEqual (spotlight.go), the same fix already made for the
// identical class of bug found in internal/spotlight's own test suite.
func findWorktreeEntryByPath(entries []gitops.WorktreeEntry, path string) (gitops.WorktreeEntry, bool) {
	for _, e := range entries {
		if resolveBestEffortEqual(e.Path, path) {
			return e, true
		}
	}
	return gitops.WorktreeEntry{}, false
}
