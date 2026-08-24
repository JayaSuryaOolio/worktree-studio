// Spotlight endpoints: start/stop/status for mirroring a worktree's source
// into its repo's root checkout via the external `spotlight` CLI. See
// internal/spotlight and PLAN.md section 2.
//
// Two families of HTTP handler share the same underlying logic (the
// start/stop/status* helpers below): the repo/worktree-scoped ones
// (handleSpotlightStart/Stop/Status, driven by the UI, which already knows
// the worktree's id from the page it's on) and the path-based ones
// (handleSpotlightCLIStart/Stop/Status, driven by the `worktree-studio
// spotlight` CLI subcommand in cmd/worktree-studio/spotlight.go), which
// resolve the target worktree from an arbitrary filesystem path instead —
// the CLI's caller (e.g. Claude Code, running inside a terminal panel) has
// a path, not a worktree id. Routing spotlight through worktree-studio's
// own server this way, rather than having the CLI subcommand shell out to
// the external `spotlight` binary directly, keeps audit logging and the
// UI's own status view in sync with anything started this way — the same
// reason handleOpenFile (hooks.go) exists instead of a caller editing the
// browser's file tree state directly.
package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"

	"worktree-studio/internal/audit"
	"worktree-studio/internal/spotlight"
	"worktree-studio/internal/store"
)

// resolveBestEffortEqual reports whether two paths refer to the same
// location, tolerating the same /tmp-or-/var-is-a-symlink divergence that
// internal/spotlight.StatusForRoot already accounts for.
func resolveBestEffortEqual(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return a == b
	}
	return ra == rb
}

// spotlightStatusForWorktree is handleSpotlightStatus's actual logic,
// extracted so handleSpotlightCLIStatus (resolved from a path instead of a
// worktree id already known from the page the request came from) can share
// it exactly rather than reimplementing the same active/root/pid shape.
func (s *Server) spotlightStatusForWorktree(wt store.Worktree) (map[string]any, error) {
	repo, err := s.Store.GetRepo(wt.RepoID)
	if err != nil {
		return nil, err
	}

	status, err := spotlight.StatusForRoot(repo.Path)
	if err != nil {
		if errors.Is(err, spotlight.ErrNotFound) {
			return map[string]any{"available": false}, nil
		}
		return nil, err
	}

	if status == nil {
		return map[string]any{"available": true, "active": false}, nil
	}

	active := resolveBestEffortEqual(status.Worktree, wt.Path)
	return map[string]any{
		"available":            true,
		"active":               active,
		"root":                 status.Root,
		"active_worktree_path": status.Worktree,
		"pid":                  status.PID,
	}, nil
}

func (s *Server) handleSpotlightStatus(w http.ResponseWriter, r *http.Request) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}
	result, err := s.spotlightStatusForWorktree(wt)
	if err != nil {
		s.Log.Error("spotlight status", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to query spotlight status")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// startSpotlightForWorktree is handleSpotlightStart's actual logic
// (spotlight.Start plus the success audit log), extracted so
// handleSpotlightCLIStart can share it exactly. Error mapping to an HTTP
// status stays with each caller, since the CLI's own stderr message for
// the same errors reads differently than a browser-facing JSON body.
func (s *Server) startSpotlightForWorktree(wt store.Worktree, stash bool) (string, error) {
	root, err := spotlight.Start(wt.Path, stash)
	if err != nil {
		return "", err
	}
	s.auditLog(audit.EventSpotlightStart, map[string]any{
		"repo_id":     wt.RepoID,
		"worktree_id": wt.ID,
		"worktree":    wt.Path,
		"root":        root,
	})
	return root, nil
}

func (s *Server) handleSpotlightStart(w http.ResponseWriter, r *http.Request) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}

	// ?stash=true means the user already confirmed, in the UI, that they
	// want the root's uncommitted changes stashed rather than blocking on
	// them — same "refuse first, retry with an explicit flag once
	// confirmed" shape as worktree deletion's ?force=true.
	stash := r.URL.Query().Get("stash") == "true"

	root, err := s.startSpotlightForWorktree(wt, stash)
	if err != nil {
		switch {
		case errors.Is(err, spotlight.ErrNotFound):
			writeError(w, http.StatusServiceUnavailable, "spotlight CLI is not installed (see github.com/JayaSuryaOolio/spotlight)")
		case errors.Is(err, spotlight.ErrRootDirty):
			writeError(w, http.StatusConflict, "the repo's root checkout has uncommitted changes; retry with ?stash=true to stash them and start anyway, or commit/stash them yourself first")
		default:
			s.Log.Error("spotlight start", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to start spotlight: "+err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"root": root})
}

// stopSpotlightForWorktree is handleSpotlightStop's actual logic
// (spotlight.Stop plus the success audit log), extracted so
// handleSpotlightCLIStop can share it exactly.
func (s *Server) stopSpotlightForWorktree(wt store.Worktree) error {
	repo, err := s.Store.GetRepo(wt.RepoID)
	if err != nil {
		return err
	}
	if err := spotlight.Stop(repo.Path); err != nil {
		return err
	}
	s.auditLog(audit.EventSpotlightStop, map[string]any{
		"repo_id":     wt.RepoID,
		"worktree_id": wt.ID,
		"worktree":    wt.Path,
		"root":        repo.Path,
	})
	return nil
}

func (s *Server) handleSpotlightStop(w http.ResponseWriter, r *http.Request) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}

	if err := s.stopSpotlightForWorktree(wt); err != nil {
		if errors.Is(err, spotlight.ErrNotFound) {
			writeError(w, http.StatusServiceUnavailable, "spotlight CLI is not installed (see github.com/JayaSuryaOolio/spotlight)")
			return
		}
		s.Log.Error("spotlight stop", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to stop spotlight: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// resolveWorktreeByPathForCLI looks up the worktree that owns path (see
// store.FindWorktreeByPath — an exact match or any subdirectory of it),
// writing the same tolerant {"status":"no matching worktree"} 200 body
// handleOpenFile uses for the same "path isn't inside any worktree
// worktree-studio tracks" outcome, since that's a normal, expected result
// for a CLI subcommand that can be run from anywhere — not an error. ok is
// false if a response was already written (either that no-match case, or a
// real lookup failure) and the caller should return immediately.
func (s *Server) resolveWorktreeByPathForCLI(w http.ResponseWriter, path string) (store.Worktree, bool) {
	wt, err := s.Store.FindWorktreeByPath(path)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "no matching worktree"})
			return store.Worktree{}, false
		}
		s.Log.Warn("spotlight cli: resolve worktree by path", "err", err, "path", path)
		writeError(w, http.StatusInternalServerError, "failed to resolve worktree")
		return store.Worktree{}, false
	}
	return wt, true
}

type spotlightCLIRequest struct {
	Path  string `json:"path"`
	Stash bool   `json:"stash"`
}

// handleSpotlightCLIStart backs the `worktree-studio spotlight --start
// [path]` CLI subcommand (see cmd/worktree-studio/spotlight.go): unlike
// handleSpotlightStart, it's given a filesystem path instead of a
// repo/worktree id pair, since that's all a shell (or Claude, calling this
// directly instead of shelling out to the external `spotlight` binary
// itself) has to identify which worktree it means.
func (s *Server) handleSpotlightCLIStart(w http.ResponseWriter, r *http.Request) {
	var req spotlightCLIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	wt, ok := s.resolveWorktreeByPathForCLI(w, req.Path)
	if !ok {
		return
	}

	root, err := s.startSpotlightForWorktree(wt, req.Stash)
	if err != nil {
		switch {
		case errors.Is(err, spotlight.ErrNotFound):
			writeError(w, http.StatusServiceUnavailable, "spotlight CLI is not installed (see github.com/JayaSuryaOolio/spotlight)")
		case errors.Is(err, spotlight.ErrRootDirty):
			writeError(w, http.StatusConflict, "the repo's root checkout has uncommitted changes; retry with \"stash\": true to stash them and start anyway, or commit/stash them yourself first")
		default:
			s.Log.Error("spotlight cli start", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to start spotlight: "+err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"root": root})
}

// handleSpotlightCLIStop backs `worktree-studio spotlight --stop [path]` —
// see handleSpotlightCLIStart's comment for why this takes a path instead
// of a worktree id.
func (s *Server) handleSpotlightCLIStop(w http.ResponseWriter, r *http.Request) {
	var req spotlightCLIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	wt, ok := s.resolveWorktreeByPathForCLI(w, req.Path)
	if !ok {
		return
	}

	if err := s.stopSpotlightForWorktree(wt); err != nil {
		if errors.Is(err, spotlight.ErrNotFound) {
			writeError(w, http.StatusServiceUnavailable, "spotlight CLI is not installed (see github.com/JayaSuryaOolio/spotlight)")
			return
		}
		s.Log.Error("spotlight cli stop", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to stop spotlight: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// handleSpotlightCLIStatus backs `worktree-studio spotlight --status
// [path]` — see handleSpotlightCLIStart's comment for why this takes a
// path instead of a worktree id.
func (s *Server) handleSpotlightCLIStatus(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	wt, ok := s.resolveWorktreeByPathForCLI(w, path)
	if !ok {
		return
	}

	result, err := s.spotlightStatusForWorktree(wt)
	if err != nil {
		s.Log.Error("spotlight cli status", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to query spotlight status")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
