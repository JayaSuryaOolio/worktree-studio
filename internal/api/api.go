// Package api provides the HTTP handlers for worktree-studio: registering
// repos and creating/removing worktrees under them. Every mutating handler
// logs an event through internal/audit.
package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	// LogFilePath is where Log's own output is mirrored on disk (see
	// main.go's logFilePath), or "" if that file couldn't be opened (Log
	// output still goes to stdout in that case, just not durably). Read by
	// handleGetLogs for the main settings modal's Logs tab.
	LogFilePath string
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

		r.Put("/{repoID}/settings", s.handleUpdateRepoSettings)
		r.Get("/{repoID}/branches", s.handleListBranches)

		r.Route("/{repoID}/worktrees", func(r chi.Router) {
			r.Get("/", s.handleListWorktrees)
			r.Post("/", s.handleCreateWorktree)
			r.Post("/import", s.handleImportWorktree)
			r.Get("/new-name-suggestion", s.handleNewNameSuggestion)
			r.Get("/external", s.handleListExternalWorktrees)
			r.Get("/archived", s.handleListArchivedWorktrees)

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
					r.Get("/{terminalID}/cwd", s.handleGetTerminalCwd)
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
	r.Get("/api/repos/{repoID}/terminals/all", s.handleListTerminalsForRepo)

	r.Route("/api/settings", func(r chi.Router) {
		r.Get("/dependencies", s.handleGetDependencyStatus)
		r.Post("/dependencies/claude-hook/install", s.handleInstallClaudeHook)
		r.Post("/dependencies/claude-hook/uninstall", s.handleUninstallClaudeHook)
		r.Post("/dependencies/skill/install", s.handleInstallSkill)
		r.Get("/logs", s.handleGetLogs)
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

	s.EnsureRootWorktree(repo)

	writeJSON(w, http.StatusCreated, repo)
}

// EnsureRootWorktree makes sure a synthetic worktree row exists for repo's
// own root checkout (store.WorktreeSourceRoot, id store.RootWorktreeID),
// inserting one if missing. Failures are logged but not surfaced — the repo
// itself is already usable without it, this only unlocks the "open the repo
// root like a worktree" sidebar shortcut, and every caller (repo add, and
// the startup backfill in main.go for repos registered before this existed)
// treats it as best-effort.
func (s *Server) EnsureRootWorktree(repo store.Repo) {
	id := store.RootWorktreeID(repo.ID)
	if _, err := s.Store.GetWorktree(id); err == nil {
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		s.Log.Error("check root worktree", "err", err, "repo_id", repo.ID)
		return
	}

	branch := ""
	if status, err := gitops.Status(repo.Path); err == nil {
		branch = status.Branch
	}

	wt := store.Worktree{
		ID:     id,
		RepoID: repo.ID,
		Name:   "root",
		Branch: branch,
		Path:   repo.Path,
		Status: store.WorktreeStatusActive,
		Source: store.WorktreeSourceRoot,
	}
	if err := s.Store.AddWorktree(wt); err != nil {
		s.Log.Error("add root worktree", "err", err, "repo_id", repo.ID)
	}
}

type updateRepoSettingsRequest struct {
	BaseBranch string `json:"base_branch"`
}

// handleUpdateRepoSettings sets repo.BaseBranch, the explicit override for
// which branch new worktrees are created from (see handleCreateWorktree and
// gitops.DetectDefaultBranch). Posting "" reverts to auto-detection —
// deliberately not validated against the repo's actual local/remote
// branches here: a typo or a branch that doesn't exist yet just surfaces as
// git's own "invalid reference" error on the next worktree-create attempt,
// which is a clear enough signal without duplicating git's own branch
// resolution logic here.
func (s *Server) handleUpdateRepoSettings(w http.ResponseWriter, r *http.Request) {
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

	var req updateRepoSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := s.Store.UpdateRepoBaseBranch(repoID, req.BaseBranch); err != nil {
		s.Log.Error("update repo base branch", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to update repo settings")
		return
	}

	s.auditLog(audit.EventRepoUpdateBaseBranch, map[string]any{
		"repo_id":     repoID,
		"base_branch": req.BaseBranch,
	})

	repo, err := s.Store.GetRepo(repoID)
	if err != nil {
		s.Log.Error("get repo after update", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to look up updated repo")
		return
	}
	writeJSON(w, http.StatusOK, repo)
}

// listBranchesResponse is handleListBranches's response shape: every
// branch (local + remote-tracking) plus which one would currently be used
// as a new worktree's start point if nothing else were specified — the
// new-worktree dialog's branch dropdown pre-selects Default, but lets a
// person deliberately pick any other entry in Branches (e.g. a fresher
// "origin/<branch>" ref if nobody's fetched the local branch of the same
// name recently).
type listBranchesResponse struct {
	Branches []string `json:"branches"`
	Default  string   `json:"default"`
}

func (s *Server) handleListBranches(w http.ResponseWriter, r *http.Request) {
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

	branches, err := gitops.ListBranches(repo.Path)
	if err != nil {
		s.Log.Error("list branches", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list branches: "+err.Error())
		return
	}

	def := repo.BaseBranch
	if def == "" {
		def = gitops.DetectDefaultBranch(repo.Path)
	}

	writeJSON(w, http.StatusOK, listBranchesResponse{Branches: branches, Default: def})
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

	// The synthetic root worktree (see EnsureRootWorktree) isn't a real git
	// worktree, so it's excluded from every normal worktree listing —
	// reached instead through the sidebar's repo-name link straight to
	// store.RootWorktreeID(repoID).
	out := make([]store.Worktree, 0, len(worktrees))
	for _, wt := range worktrees {
		if wt.Source == store.WorktreeSourceRoot {
			continue
		}
		out = append(out, wt)
	}
	writeJSON(w, http.StatusOK, out)
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
	// SourceBranch optionally overrides which branch/ref this worktree is
	// created from — e.g. picked from the new-worktree dialog's branch
	// dropdown (see handleListBranches), which can point at a remote-
	// tracking ref like "origin/main" rather than the local branch of the
	// same name. "" falls back to repo.BaseBranch, then auto-detection,
	// same as before this field existed.
	SourceBranch string `json:"source_branch"`
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
	// Resolution order: an explicit per-request SourceBranch (the
	// new-worktree dialog's branch dropdown) beats repo.BaseBranch (an
	// explicit per-repo override, see handleUpdateRepoSettings) beats
	// auto-detection, and if that also comes up empty, AddWorktree falls
	// back to its own implicit-HEAD default. Without at least the
	// repo/auto-detect fallback, every worktree would silently branch off
	// whatever the main checkout's HEAD happened to be at creation time,
	// which is not reliably "the" base branch (e.g. if the main checkout
	// itself was left on a feature branch).
	baseBranch := req.SourceBranch
	if baseBranch == "" {
		baseBranch = repo.BaseBranch
	}
	if baseBranch == "" {
		baseBranch = gitops.DetectDefaultBranch(repo.Path)
	}
	if err := gitops.AddWorktree(repo.Path, worktreePath, branch, baseBranch); err != nil {
		s.Log.Error("git worktree add", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to create git worktree: "+err.Error())
		return
	}

	wt := store.Worktree{
		ID:           newID(),
		RepoID:       repo.ID,
		Name:         slug,
		Branch:       branch,
		Path:         worktreePath,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		Status:       store.WorktreeStatusActive,
		Source:       store.WorktreeSourceCreated,
		SourceBranch: baseBranch,
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
		"base_branch": baseBranch,
	})

	writeJSON(w, http.StatusCreated, wt)
}

// externalWorktreeEntry is a worktree that `git worktree list` reports for a
// repo but that isn't tracked in worktree-studio's own DB yet — a candidate
// for the settings page's "attach" flow.
type externalWorktreeEntry struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
}

// handleListExternalWorktrees returns every worktree `git worktree list`
// reports for this repo that isn't already tracked in the DB (matched by
// path) — the settings page's "the rest" datagrid, populated live from git
// rather than the DB since these are, by definition, worktrees
// worktree-studio has no record of.
func (s *Server) handleListExternalWorktrees(w http.ResponseWriter, r *http.Request) {
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

	entries, err := gitops.ListWorktrees(repo.Path)
	if err != nil {
		s.Log.Error("git worktree list", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list git worktrees: "+err.Error())
		return
	}

	out := []externalWorktreeEntry{}
	for _, e := range entries {
		// repo.Path itself always shows up in `git worktree list` (the
		// primary checkout) — never a candidate to import as a worktree.
		if resolveBestEffortEqual(e.Path, repo.Path) {
			continue
		}
		// WorktreePathExists (not scoped to this repo) matches
		// handleImportWorktree's own duplicate check, so a worktree that
		// disappears from this list because it just got attached can't
		// reappear via some other repo registration sharing the path.
		exists, err := s.Store.WorktreePathExists(e.Path)
		if err != nil {
			s.Log.Error("check worktree exists", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to check known worktrees")
			return
		}
		if exists {
			continue
		}
		out = append(out, externalWorktreeEntry{Path: e.Path, Branch: strings.TrimPrefix(e.Branch, "refs/heads/")})
	}
	writeJSON(w, http.StatusOK, out)
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
	if err := s.hardRemoveWorktree(repo, wt, force); err != nil {
		if errors.Is(err, gitops.ErrWorktreeDirty) {
			// Not a server error: the user needs to make a call (retry with
			// force=true to discard changes, or go clean things up first),
			// so surface it as a 409 rather than a 500.
			writeError(w, http.StatusConflict, "worktree has uncommitted changes or untracked files; retry with ?force=true to remove it anyway and discard them")
			return
		}
		s.Log.Error("remove worktree", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to remove worktree: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// hardRemoveWorktree does the actual destructive teardown shared by
// handleDeleteWorktree (a user-initiated delete) and
// SweepExpiredArchivedWorktrees (archive_sweep.go, the 60-day retention
// cleanup for archived worktrees): close any terminal sessions (otherwise
// the tmux session — a real OS process — becomes a permanent orphan once
// its DB row disappears via ON DELETE CASCADE, found the hard way while
// testing step 7.4), remove the git worktree checkout, and delete the DB
// row outright. Does NOT delete the worktree's branch, same as this flow
// has always done (see gitops.RemoveWorktree's own doc comment for why
// that's a separate call).
func (s *Server) hardRemoveWorktree(repo store.Repo, wt store.Worktree, force bool) error {
	sessions, err := s.Store.ListTerminalSessions(wt.ID)
	if err != nil {
		return fmt.Errorf("list terminal sessions: %w", err)
	}
	for _, ts := range sessions {
		if err := s.Term.CloseSession(ts); err != nil {
			return fmt.Errorf("close terminal session %s: %w", ts.ID, err)
		}
	}

	if err := gitops.RemoveWorktree(repo.Path, wt.Path, force); err != nil {
		return err // may wrap gitops.ErrWorktreeDirty — callers check with errors.Is
	}

	if err := s.Store.RemoveWorktree(wt.ID); err != nil {
		return fmt.Errorf("delete worktree record: %w", err)
	}

	s.auditLog(audit.EventWorktreeRemove, map[string]any{
		"repo_id":     repo.ID,
		"worktree_id": wt.ID,
		"name":        wt.Name,
		"branch":      wt.Branch,
		"path":        wt.Path,
	})
	return nil
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
