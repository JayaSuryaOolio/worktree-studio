package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"worktree-studio/internal/audit"
	"worktree-studio/internal/store"
	"worktree-studio/internal/tasksrc"
)

// Task sources: user-configured adapter commands (internal/tasksrc) that
// feed the picker search and keep external task titles fresh.

var taskSourceID = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func (s *Server) handleListTaskSources(w http.ResponseWriter, r *http.Request) {
	srcs, err := s.Store.ListTaskSources()
	if err != nil {
		s.Log.Error("list task sources", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list sources")
		return
	}
	writeJSON(w, http.StatusOK, srcs)
}

// handleAddTaskSource: POST /api/task-sources {id, name, command}. The
// command must be an absolute path to an executable — it's run as the user,
// so it only ever comes from this explicit settings action.
func (s *Server) handleAddTaskSource(w http.ResponseWriter, r *http.Request) {
	var src store.TaskSource
	if err := json.NewDecoder(r.Body).Decode(&src); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	src.ID, src.Name, src.Command = strings.TrimSpace(src.ID), strings.TrimSpace(src.Name), strings.TrimSpace(src.Command)
	if !taskSourceID.MatchString(src.ID) || src.ID == store.TaskSourceLocal {
		writeError(w, http.StatusBadRequest, `id must be lowercase letters, digits and dashes, and not "local"`)
		return
	}
	if !filepath.IsAbs(src.Command) {
		writeError(w, http.StatusBadRequest, "command must be an absolute path")
		return
	}
	if _, err := exec.LookPath(src.Command); err != nil {
		writeError(w, http.StatusBadRequest, "command is not an executable file: "+err.Error())
		return
	}
	if src.Name == "" {
		src.Name = src.ID
	}
	if err := s.Store.AddTaskSource(src); err != nil {
		writeError(w, http.StatusConflict, "a source with that id already exists")
		return
	}
	s.auditLog(audit.EventTaskSourceAdd, map[string]any{"id": src.ID, "command": src.Command})
	writeJSON(w, http.StatusCreated, src)
}

func (s *Server) handleDeleteTaskSource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sourceID")
	if err := s.Store.DeleteTaskSource(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "source not found")
			return
		}
		s.Log.Error("delete task source", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to delete source")
		return
	}
	s.auditLog(audit.EventTaskSourceRemove, map[string]any{"id": id})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) taskSource(id string) (store.TaskSource, bool) {
	srcs, err := s.Store.ListTaskSources()
	if err != nil {
		return store.TaskSource{}, false
	}
	for _, src := range srcs {
		if src.ID == id {
			return src, true
		}
	}
	return store.TaskSource{}, false
}

// handleTestTaskSource: POST /api/task-sources/{id}/test — runs `list`
// once and returns what came back, or the adapter's error as a 502.
func (s *Server) handleTestTaskSource(w http.ResponseWriter, r *http.Request) {
	src, ok := s.taskSource(chi.URLParam(r, "sourceID"))
	if !ok {
		writeError(w, http.StatusNotFound, "source not found")
		return
	}
	items, err := tasksrc.List(src.Command, "")
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(items), "sample": items[:min(len(items), 5)]})
}

// taskRefreshTTL spaces out title refreshes: the UI asks on every load, the
// adapters only run when the last refresh is older than this.
const taskRefreshTTL = 5 * time.Minute

var taskRefresh struct {
	sync.Mutex
	last time.Time
}

// handleRefreshTasks: POST /api/tasks/refresh[?force=1] — asks each source
// (`get`, batched) about the external tasks active worktrees carry and
// stores their titles (and url, if the task has none). Returns
// {refreshed, errors: {source: message}}; a failing source keeps its
// cached titles.
func (s *Server) handleRefreshTasks(w http.ResponseWriter, r *http.Request) {
	taskRefresh.Lock()
	defer taskRefresh.Unlock()
	errs := map[string]string{}
	if r.URL.Query().Get("force") == "" && time.Since(taskRefresh.last) < taskRefreshTTL {
		writeJSON(w, http.StatusOK, map[string]any{"refreshed": 0, "errors": errs})
		return
	}
	taskRefresh.last = time.Now()

	srcs, err := s.Store.ListTaskSources()
	if err != nil {
		s.Log.Error("list task sources", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to refresh tasks")
		return
	}
	n := 0
	for _, src := range srcs {
		keys, err := s.Store.ActiveTaskKeys(src.ID)
		if err != nil {
			errs[src.ID] = err.Error()
			continue
		}
		items, err := tasksrc.Get(src.Command, keys)
		if err != nil {
			s.Log.Warn("task source refresh", "source", src.ID, "err", err)
			errs[src.ID] = err.Error()
			continue
		}
		for _, it := range items {
			if err := s.Store.RefreshTask(src.ID, it.Key, it.Title, it.URL); err != nil {
				errs[src.ID] = err.Error()
				continue
			}
			n++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"refreshed": n, "errors": errs})
}
