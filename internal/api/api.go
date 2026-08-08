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
	"worktree-studio/internal/files"
	"worktree-studio/internal/gitops"
	"worktree-studio/internal/store"
	"worktree-studio/internal/term"
)

// Server holds the dependencies HTTP handlers need.
type Server struct {
	Store        *store.Store
	Audit        *audit.Logger
	Term         *term.Manager
	Files        *files.Manager
	WorktreeRoot string // base dir for created worktrees, e.g. ~/.worktree-studio/worktrees
	Log          *slog.Logger
	// SelfBaseURL is this server's own reachable base URL (e.g.
	// "http://localhost:8787"), embedded into the installed claude hook
	// script so it knows where to POST — see internal/claudehook/install.go.
	SelfBaseURL string
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
				r.Get("/status", s.handleWorktreeStatus)
				r.Get("/audit-log", s.handleWorktreeAuditLog)
				r.Post("/archive", s.handleArchiveWorktree)
				r.Post("/unarchive", s.handleUnarchiveWorktree)

				r.Route("/terminals", func(r chi.Router) {
					r.Get("/", s.handleListTerminals)
					r.Post("/", s.handleCreateTerminal)
					r.Delete("/{terminalID}", s.handleDeleteTerminal)
				})

				r.Route("/spotlight", func(r chi.Router) {
					r.Get("/", s.handleSpotlightStatus)
					r.Post("/start", s.handleSpotlightStart)
					r.Post("/stop", s.handleSpotlightStop)
				})

				r.Get("/layout", s.handleGetLayout)
				r.Put("/layout", s.handleSaveLayout)

				r.Route("/files", func(r chi.Router) {
					r.Get("/tree", s.handleFileTree)
					r.Get("/content", s.handleGetFileContent)
					r.Put("/content", s.handlePutFileContent)
				})

				r.Post("/open-in-vscode", s.handleOpenInVSCode)
			})
		})
	})

	r.Get("/ws/terminals/{terminalID}", s.handleTerminalWS)
	r.Get("/ws/files/{worktreeID}", s.handleFilesWS)

	r.Post("/api/claude-hook", s.handleClaudeHook)
	r.Get("/api/claude-sessions/{sessionID}/title", s.handleClaudeSessionTitle)
	r.Get("/api/worktrees/all", s.handleListAllWorktrees)

	r.Route("/api/settings", func(r chi.Router) {
		r.Get("/dependencies", s.handleGetDependencyStatus)
		r.Post("/dependencies/claude-hook/install", s.handleInstallClaudeHook)
		r.Post("/dependencies/claude-hook/uninstall", s.handleUninstallClaudeHook)
		r.Post("/dependencies/skill/install", s.handleInstallSkill)
	})
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

	s.auditLog(audit.EventRepoAdd, map[string]any{
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

	// Only "active" worktrees show up in the normal list — archived ones
	// are deliberately hidden here (no archived-view UI exists yet; that's
	// the planned settings-modal datagrid, filterable by repo/status).
	worktrees, err := s.Store.ListWorktrees(repoID, store.WorktreeStatusActive)
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
		Status:    store.WorktreeStatusActive,
	}
	if err := s.Store.AddWorktree(wt); err != nil {
		s.Log.Error("save worktree; rolling back the git worktree/branch just created", "err", err)
		// Without this rollback, a transient store failure (disk full, a
		// locked DB, or — the way this was actually first found — the
		// server's data directory having been deleted out from under a
		// still-running process) leaves an orphaned git worktree and
		// branch that worktree-studio itself has no record of. Every
		// subsequent retry with the same name then fails at the git layer
		// ("a branch named ... already exists") with no way to recover
		// short of manually running `git worktree remove`/`git branch -D`.
		// Undoing `git worktree add -b <branch>` takes two calls: removing
		// the worktree checkout does NOT delete the branch it created
		// (that's just how git worktree remove works) — skipping the
		// branch delete would leave "a branch named ... already exists"
		// blocking every retry with the same name, which is exactly the
		// dangling-state problem this rollback exists to prevent.
		rmErr := gitops.RemoveWorktree(repo.Path, worktreePath, true)
		brErr := gitops.DeleteBranch(repo.Path, branch)
		if rmErr != nil || brErr != nil {
			s.Log.Error("rollback of orphaned git worktree/branch failed", "worktree_err", rmErr, "branch_err", brErr, "path", worktreePath, "branch", branch)
			writeError(w, http.StatusInternalServerError,
				"failed to save worktree record, AND failed to fully roll back the git worktree/branch it had already created — "+
					"manual cleanup needed: git worktree remove --force "+worktreePath+" && git -C "+repo.Path+" branch -D "+branch)
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to save worktree record (the git worktree was rolled back, safe to retry): "+err.Error())
		return
	}

	s.auditLog(audit.EventWorktreeCreate, map[string]any{
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

	// Close any terminal sessions for this worktree before removing it —
	// otherwise the tmux session (a real OS process) becomes a permanent
	// orphan: the terminal_sessions DB row disappears via this worktree's
	// ON DELETE CASCADE below regardless, but nothing besides this call
	// ever kills the actual tmux session behind it. Found by hand while
	// testing step 7.4 (a stray tmux session survived a worktree delete
	// with no trace in the DB pointing back to it).
	sessions, err := s.Store.ListTerminalSessions(wt.ID)
	if err != nil {
		s.Log.Error("list terminal sessions before worktree delete", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to look up terminal sessions")
		return
	}
	for _, ts := range sessions {
		if err := s.Term.CloseSession(ts); err != nil {
			s.Log.Error("close terminal session before worktree delete", "err", err, "terminal_id", ts.ID)
			writeError(w, http.StatusInternalServerError, "failed to close terminal session "+ts.ID+" before deleting the worktree: "+err.Error())
			return
		}
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

	s.auditLog(audit.EventWorktreeRemove, map[string]any{
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
func (s *Server) auditLog(event audit.Event, fields map[string]any) {
	if s.Audit == nil {
		return
	}
	if err := s.Audit.Log(event, fields); err != nil {
		s.Log.Warn("audit log write failed", "event", event, "err", err)
	}
}
