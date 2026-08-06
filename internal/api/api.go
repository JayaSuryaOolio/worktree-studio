// Package api provides the HTTP handlers for worktree-studio: registering
// repos and creating/removing worktrees under them. Every mutating handler
// logs an event through internal/audit.
package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"

	"worktree-studio/internal/audit"
	"worktree-studio/internal/gitops"
	"worktree-studio/internal/store"
	"worktree-studio/internal/term"
)

// Server holds the dependencies HTTP handlers need.
type Server struct {
	Store        *store.Store
	Audit        *audit.Logger
	Term         *term.Manager
	WorktreeRoot string // base dir for created worktrees, e.g. ~/.worktree-studio/worktrees
	Log          *slog.Logger
}

// Routes mounts all API routes onto r.
func (s *Server) Routes(r chi.Router) {
	r.Route("/api/repos", func(r chi.Router) {
		r.Get("/", s.handleListRepos)
		r.Post("/", s.handleAddRepo)

		r.Route("/{repoID}/worktrees", func(r chi.Router) {
			r.Get("/", s.handleListWorktrees)
			r.Post("/", s.handleCreateWorktree)
			r.Get("/new-name-suggestion", s.handleNewNameSuggestion)

			r.Route("/{worktreeID}", func(r chi.Router) {
				r.Delete("/", s.handleDeleteWorktree)

				r.Route("/terminals", func(r chi.Router) {
					r.Get("/", s.handleListTerminals)
					r.Post("/", s.handleCreateTerminal)
					r.Delete("/{terminalID}", s.handleDeleteTerminal)
				})
			})
		})
	})

	r.Get("/ws/terminals/{terminalID}", s.handleTerminalWS)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// --- Repos ---

func (s *Server) handleListRepos(w http.ResponseWriter, r *http.Request) {
	repos, err := s.Store.ListRepos()
	if err != nil {
		s.Log.Error("list repos", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list repos")
		return
	}
	if repos == nil {
		repos = []store.Repo{}
	}
	writeJSON(w, http.StatusOK, repos)
}

type addRepoRequest struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func (s *Server) handleAddRepo(w http.ResponseWriter, r *http.Request) {
	var req addRepoRequest
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

	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		writeError(w, http.StatusBadRequest, "path does not exist or is not a directory")
		return
	}

	if !gitops.IsGitRepo(path) {
		writeError(w, http.StatusBadRequest, "path is not a git repository")
		return
	}

	exists, err := s.Store.RepoPathExists(path)
	if err != nil {
		s.Log.Error("check repo exists", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to check existing repos")
		return
	}
	if exists {
		writeError(w, http.StatusConflict, "repo already registered")
		return
	}

	name := req.Name
	if name == "" {
		name = filepath.Base(path)
	}

	repo := store.Repo{ID: newID(), Name: name, Path: path}
	if err := s.Store.AddRepo(repo); err != nil {
		s.Log.Error("add repo", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to save repo")
		return
	}

	s.auditLog("repo.add", map[string]any{
		"repo_id": repo.ID,
		"name":    repo.Name,
		"path":    repo.Path,
	})

	writeJSON(w, http.StatusCreated, repo)
}

// --- Worktrees ---

func (s *Server) handleListWorktrees(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")

	if _, err := s.Store.GetRepo(repoID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "repo not found")
			return
		}
		s.Log.Error("get repo", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to look up repo")
		return
	}

	worktrees, err := s.Store.ListWorktrees(repoID)
	if err != nil {
		s.Log.Error("list worktrees", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list worktrees")
		return
	}
	if worktrees == nil {
		worktrees = []store.Worktree{}
	}
	writeJSON(w, http.StatusOK, worktrees)
}

func (s *Server) handleNewNameSuggestion(w http.ResponseWriter, r *http.Request) {
	name, err := randomName()
	if err != nil {
		s.Log.Error("generate name suggestion", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to generate name")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": name})
}

type createWorktreeRequest struct {
	Name string `json:"name"`
}

func (s *Server) handleCreateWorktree(w http.ResponseWriter, r *http.Request) {
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

	var req createWorktreeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	slug := slugify(req.Name)
	worktreePath := filepath.Join(s.WorktreeRoot, repo.ID, slug)

	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		s.Log.Error("create worktree parent dir", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to prepare worktree directory")
		return
	}

	branch := slug
	if err := gitops.AddWorktree(repo.Path, worktreePath, branch); err != nil {
		s.Log.Error("git worktree add", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to create git worktree: "+err.Error())
		return
	}

	wt := store.Worktree{
		ID:        newID(),
		RepoID:    repo.ID,
		Name:      slug,
		Branch:    branch,
		Path:      worktreePath,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.Store.AddWorktree(wt); err != nil {
		s.Log.Error("save worktree", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to save worktree record")
		return
	}

	s.auditLog("worktree.create", map[string]any{
		"repo_id":     repo.ID,
		"worktree_id": wt.ID,
		"name":        wt.Name,
		"branch":      wt.Branch,
		"path":        wt.Path,
	})

	writeJSON(w, http.StatusCreated, wt)
}

func (s *Server) handleDeleteWorktree(w http.ResponseWriter, r *http.Request) {
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

	wt, err := s.Store.GetWorktree(worktreeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "worktree not found")
			return
		}
		s.Log.Error("get worktree", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to look up worktree")
		return
	}
	if wt.RepoID != repo.ID {
		writeError(w, http.StatusNotFound, "worktree not found")
		return
	}

	force := r.URL.Query().Get("force") == "true"
	if err := gitops.RemoveWorktree(repo.Path, wt.Path, force); err != nil {
		if errors.Is(err, gitops.ErrWorktreeDirty) {
			// Not a server error: the user needs to make a call (retry with
			// force=true to discard changes, or go clean things up first),
			// so surface it as a 409 rather than a 500.
			writeError(w, http.StatusConflict, "worktree has uncommitted changes or untracked files; retry with ?force=true to remove it anyway and discard them")
			return
		}
		s.Log.Error("git worktree remove", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to remove git worktree: "+err.Error())
		return
	}

	if err := s.Store.RemoveWorktree(wt.ID); err != nil {
		s.Log.Error("delete worktree record", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to delete worktree record")
		return
	}

	s.auditLog("worktree.remove", map[string]any{
		"repo_id":     repo.ID,
		"worktree_id": wt.ID,
		"name":        wt.Name,
		"branch":      wt.Branch,
		"path":        wt.Path,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// auditLog logs an audit event, warning to the server log (but not failing
// the request) if the audit write itself fails.
func (s *Server) auditLog(event string, fields map[string]any) {
	if s.Audit == nil {
		return
	}
	if err := s.Audit.Log(event, fields); err != nil {
		s.Log.Warn("audit log write failed", "event", event, "err", err)
	}
}
