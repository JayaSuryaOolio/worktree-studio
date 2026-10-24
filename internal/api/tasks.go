package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"

	"worktree-studio/internal/audit"
	"worktree-studio/internal/store"
	"worktree-studio/internal/tasksrc"
)

// Tasks are a label on worktrees (see docs/tasks-plan.md and
// internal/store/tasks.go). These handlers set that label, edit the task
// behind it, and manage the user-defined status list.

// taskRef names the task a worktree should carry: an existing (Source, Key),
// or NewTitle to create a local task. All empty clears the label.
type taskRef struct {
	Source   string `json:"source"`
	Key      string `json:"key"`
	URL      string `json:"url"`
	NewTitle string `json:"new_title"`
	// Title is what a source adapter said when the task was picked; it's
	// stored only if the task has no title yet.
	Title string `json:"title"`
}

// errInvalidTask marks taskRef problems the caller should see as a 400.
var errInvalidTask = errors.New("invalid task")

func (ref *taskRef) normalize() {
	ref.Source = strings.TrimSpace(ref.Source)
	ref.Key = strings.TrimSpace(ref.Key)
	ref.URL = strings.TrimSpace(ref.URL)
	ref.NewTitle = strings.TrimSpace(ref.NewTitle)
	if ref.Key != "" && ref.Source == "" {
		ref.Source = store.TaskSourceLocal
	}
}

func (ref taskRef) empty() bool { return ref.Key == "" && ref.NewTitle == "" }

// validateTaskRef checks ref without changing anything, so createWorktree
// can refuse a bad task before running `git worktree add`.
func (s *Server) validateTaskRef(ref taskRef) error {
	if ref.NewTitle != "" || ref.Key == "" {
		return nil
	}
	if ref.Source == store.TaskSourceLocal {
		if _, err := s.Store.GetTask(ref.Source, ref.Key); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: no local task #%s", errInvalidTask, ref.Key)
			}
			return err
		}
	}
	return nil
}

// assignTask labels wt with ref (creating the task row on first use), or
// clears its label when ref is empty, then settles the status of both the
// old and the new task.
func (s *Server) assignTask(wt store.Worktree, ref taskRef) (store.Worktree, error) {
	ref.normalize()
	if wt.Source == store.WorktreeSourceRoot && !ref.empty() {
		return wt, fmt.Errorf("%w: the repo's root checkout can't carry a task", errInvalidTask)
	}
	if err := s.validateTaskRef(ref); err != nil {
		return wt, err
	}

	var source, key string
	switch {
	case ref.NewTitle != "":
		t, err := s.Store.CreateLocalTask(ref.NewTitle, ref.URL)
		if err != nil {
			return wt, fmt.Errorf("create task: %w", err)
		}
		source, key = t.Source, t.Key
	case ref.Key != "":
		t, err := s.Store.EnsureTask(ref.Source, ref.Key, ref.URL)
		if err != nil {
			return wt, fmt.Errorf("save task: %w", err)
		}
		if t.Title == "" && strings.TrimSpace(ref.Title) != "" {
			if err := s.Store.RefreshTask(t.Source, t.Key, strings.TrimSpace(ref.Title), ""); err != nil {
				return wt, fmt.Errorf("save task title: %w", err)
			}
		}
		source, key = ref.Source, ref.Key
	}

	oldSource, oldKey := wt.TaskSource, wt.TaskKey
	if err := s.Store.SetWorktreeTask(wt.ID, source, key); err != nil {
		return wt, fmt.Errorf("label worktree: %w", err)
	}
	wt.TaskSource, wt.TaskKey = source, key

	if oldSource != source || oldKey != key {
		s.settleTask(oldSource, oldKey)
		s.auditLog(audit.EventWorktreeTaskSet, map[string]any{
			"repo_id":     wt.RepoID,
			"worktree_id": wt.ID,
			"name":        wt.Name,
			"task_source": source,
			"task_key":    key,
		})
	}
	s.settleTask(source, key)
	return wt, nil
}

// settleTask runs store.SettleTaskStatus, logging rather than failing: the
// worktree change that triggered it has already happened.
func (s *Server) settleTask(source, key string) {
	if err := s.Store.SettleTaskStatus(source, key); err != nil {
		s.Log.Warn("settle task status", "err", err, "source", source, "key", key)
	}
}

func (s *Server) writeTaskError(w http.ResponseWriter, err error) {
	if errors.Is(err, errInvalidTask) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.Log.Error("assign task", "err", err)
	writeError(w, http.StatusInternalServerError, "failed to set task")
}

// handleSetWorktreeTask: PUT /api/repos/{repoID}/worktrees/{worktreeID}/task
func (s *Server) handleSetWorktreeTask(w http.ResponseWriter, r *http.Request) {
	wt, ok := s.getRepoAndWorktree(w, r)
	if !ok {
		return
	}
	var ref taskRef
	if err := json.NewDecoder(r.Body).Decode(&ref); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	wt, err := s.assignTask(wt, ref)
	if err != nil {
		s.writeTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, wt)
}

type setWorktreeTaskCLIRequest struct {
	Path string `json:"path"`
	taskRef
}

// handleSetWorktreeTaskCLI backs `worktree-studio set-task`: the same as
// handleSetWorktreeTask, with the worktree resolved from a filesystem path.
func (s *Server) handleSetWorktreeTaskCLI(w http.ResponseWriter, r *http.Request) {
	var req setWorktreeTaskCLIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	wt, ok := s.resolveWorktreeByPathForCLI(w, req.Path)
	if !ok {
		return
	}
	wt, err := s.assignTask(wt, req.taskRef)
	if err != nil {
		s.writeTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, wt)
}

// taskWithWorktrees is a task plus every worktree carrying it, archived ones
// included (the board shows them dimmed).
type taskWithWorktrees struct {
	store.Task
	Worktrees []store.Worktree `json:"worktrees"`
}

// handleListTasks: GET /api/tasks. Every task row exists because some
// worktree was labelled with it, so this is the whole board.
func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.Store.ListTasks()
	if err != nil {
		s.Log.Error("list tasks", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list tasks")
		return
	}
	wts, err := s.Store.ListTaskWorktrees()
	if err != nil {
		s.Log.Error("list task worktrees", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list tasks")
		return
	}
	byTask := map[[2]string][]store.Worktree{}
	for _, wt := range wts {
		k := [2]string{wt.TaskSource, wt.TaskKey}
		byTask[k] = append(byTask[k], wt)
	}
	out := make([]taskWithWorktrees, 0, len(tasks))
	for _, t := range tasks {
		ws := byTask[[2]string{t.Source, t.Key}]
		if ws == nil {
			ws = []store.Worktree{}
		}
		out = append(out, taskWithWorktrees{Task: t, Worktrees: ws})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSearchTasks: GET /api/tasks/search?q= — the new-worktree picker.
// Tasks worktrees already carry come first (another worktree on a running
// task is the common case), then each configured source's `list --query`.
// A failing source is logged and skipped; Settings → Tasks → Test shows why.
func (s *Server) handleSearchTasks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	tasks, err := s.Store.SearchTasks(q, 20)
	if err != nil {
		s.Log.Error("search tasks", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to search tasks")
		return
	}
	srcs, err := s.Store.ListTaskSources()
	if err != nil {
		s.Log.Error("list task sources", "err", err)
	}

	found := make([][]tasksrc.Item, len(srcs))
	var wg sync.WaitGroup
	for i, src := range srcs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			items, err := tasksrc.List(src.Command, q)
			if err != nil {
				s.Log.Warn("task source search", "source", src.ID, "err", err)
			}
			found[i] = items
		}()
	}
	wg.Wait()

	seen := map[[2]string]bool{}
	for _, t := range tasks {
		seen[[2]string{t.Source, t.Key}] = true
	}
	for i, items := range found {
		for _, it := range items {
			if k := [2]string{srcs[i].ID, it.Key}; !seen[k] {
				seen[k] = true
				tasks = append(tasks, store.Task{Source: srcs[i].ID, Key: it.Key, Title: it.Title, URL: it.URL})
			}
		}
	}
	writeJSON(w, http.StatusOK, tasks)
}

// handleUpdateTask: PATCH /api/tasks/{source}/{key} with {title?, url?, status_id?}.
func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	var p store.TaskPatch
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if p.StatusID != nil {
		if ok, err := s.taskStatusExists(*p.StatusID); err != nil || !ok {
			writeError(w, http.StatusBadRequest, "unknown status")
			return
		}
	}
	t, err := s.Store.UpdateTask(chi.URLParam(r, "source"), chi.URLParam(r, "key"), p)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		s.Log.Error("update task", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to update task")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) taskStatusExists(id int64) (bool, error) {
	sts, err := s.Store.ListTaskStatuses()
	if err != nil {
		return false, err
	}
	for _, st := range sts {
		if st.ID == id {
			return true, nil
		}
	}
	return false, nil
}

// --- Task statuses (Settings → Tasks) ---

func (s *Server) handleListTaskStatuses(w http.ResponseWriter, r *http.Request) {
	sts, err := s.Store.ListTaskStatuses()
	if err != nil {
		s.Log.Error("list task statuses", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list statuses")
		return
	}
	writeJSON(w, http.StatusOK, sts)
}

type taskStatusRequest struct {
	Name string `json:"name"`
}

func decodeStatusName(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req taskStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return "", false
	}
	return strings.TrimSpace(req.Name), true
}

func (s *Server) handleAddTaskStatus(w http.ResponseWriter, r *http.Request) {
	name, ok := decodeStatusName(w, r)
	if !ok {
		return
	}
	st, err := s.Store.AddTaskStatus(name)
	if err != nil {
		writeError(w, http.StatusConflict, "a status with that name already exists")
		return
	}
	writeJSON(w, http.StatusCreated, st)
}

func statusIDParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "statusID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid status id")
		return 0, false
	}
	return id, true
}

func (s *Server) handleRenameTaskStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := statusIDParam(w, r)
	if !ok {
		return
	}
	name, ok := decodeStatusName(w, r)
	if !ok {
		return
	}
	if err := s.Store.RenameTaskStatus(id, name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "status not found")
			return
		}
		writeError(w, http.StatusConflict, "a status with that name already exists")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReorderTaskStatuses(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.Store.ReorderTaskStatuses(req.IDs); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "status not found")
			return
		}
		s.Log.Error("reorder task statuses", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to reorder statuses")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleDeleteTaskStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := statusIDParam(w, r)
	if !ok {
		return
	}
	switch err := s.Store.DeleteTaskStatus(id); {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case errors.Is(err, store.ErrTaskStatusInUse), errors.Is(err, store.ErrLastTaskStatus):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, sql.ErrNoRows):
		writeError(w, http.StatusNotFound, "status not found")
	default:
		s.Log.Error("delete task status", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to delete status")
	}
}
