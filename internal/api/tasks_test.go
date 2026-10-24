package api

import (
	"net/http"
	"strconv"
	"testing"

	"worktree-studio/internal/store"
)

func setupTaskRepo(t *testing.T) (string, store.Repo, *Server) {
	t.Helper()
	requireGit(t)
	ts, srv := newTestServer(t)
	repoPath := newTestGitRepo(t)
	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)
	return ts.URL, repo, srv
}

// TestCreateWorktreesSharingATask covers the core flow: start one worktree
// with a new local task, start a second one on the same task, and both show
// up under that task in GET /api/tasks.
func TestCreateWorktreesSharingATask(t *testing.T) {
	base, repo, _ := setupTaskRepo(t)

	resp := doJSON(t, http.MethodPost, base+"/api/repos/"+repo.ID+"/worktrees/", map[string]any{
		"name": "api", "task": map[string]string{"new_title": "Invoice export", "url": "https://x/1"},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create with new task: status = %d", resp.StatusCode)
	}
	var first store.Worktree
	decodeInto(t, resp, &first)
	if first.TaskSource != store.TaskSourceLocal || first.TaskKey != "1" {
		t.Fatalf("first worktree task = %s:%s, want local:1", first.TaskSource, first.TaskKey)
	}

	resp = doJSON(t, http.MethodPost, base+"/api/repos/"+repo.ID+"/worktrees/", map[string]any{
		"name": "web", "task": map[string]string{"source": "local", "key": "1"},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create on existing task: status = %d", resp.StatusCode)
	}

	resp = doJSON(t, http.MethodGet, base+"/api/tasks", nil)
	var tasks []taskWithWorktrees
	decodeInto(t, resp, &tasks)
	if len(tasks) != 1 || len(tasks[0].Worktrees) != 2 || tasks[0].Title != "Invoice export" || tasks[0].URL != "https://x/1" {
		t.Fatalf("GET /api/tasks = %+v", tasks)
	}
}

// TestCreateWorktreeUnknownLocalTaskIsRefusedBeforeGit makes sure a bad task
// is caught before `git worktree add`, so nothing is left behind.
func TestCreateWorktreeUnknownLocalTaskIsRefusedBeforeGit(t *testing.T) {
	base, repo, srv := setupTaskRepo(t)

	resp := doJSON(t, http.MethodPost, base+"/api/repos/"+repo.ID+"/worktrees/", map[string]any{
		"name": "nope", "task": map[string]string{"source": "local", "key": "42"},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	wts, _ := srv.Store.ListWorktrees(repo.ID, store.WorktreeStatusActive)
	for _, wt := range wts {
		if wt.Name == "nope" {
			t.Fatal("worktree was created despite the invalid task")
		}
	}
}

// TestArchivingLastWorktreeMovesTaskToLastStatus is "archived means done",
// through the real archive endpoint, plus the reverse on unarchive.
func TestArchivingLastWorktreeMovesTaskToLastStatus(t *testing.T) {
	base, repo, srv := setupTaskRepo(t)

	resp := doJSON(t, http.MethodPost, base+"/api/repos/"+repo.ID+"/worktrees/", map[string]any{
		"name": "only", "task": map[string]string{"source": "jira", "key": "PROJ-7"},
	})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	sts, _ := srv.Store.ListTaskStatuses()
	first, last := sts[0].ID, sts[len(sts)-1].ID
	statusOf := func() int64 {
		task, err := srv.Store.GetTask("jira", "PROJ-7")
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		return task.StatusID
	}
	if got := statusOf(); got != first {
		t.Fatalf("new task status = %d, want first %d", got, first)
	}

	doJSON(t, http.MethodPost, base+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/archive", nil)
	if got := statusOf(); got != last {
		t.Fatalf("after archive status = %d, want last %d", got, last)
	}
	doJSON(t, http.MethodPost, base+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/unarchive", nil)
	if got := statusOf(); got != first {
		t.Fatalf("after unarchive status = %d, want first %d", got, first)
	}
}

// TestSetTaskCLIByPathAndClear covers the path-resolved endpoint behind
// `worktree-studio set-task`, then clearing via the per-worktree PUT.
func TestSetTaskCLIByPathAndClear(t *testing.T) {
	base, repo, srv := setupTaskRepo(t)

	resp := doJSON(t, http.MethodPost, base+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "adhoc"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	resp = doJSON(t, http.MethodPost, base+"/api/worktree-task", map[string]string{"path": wt.Path, "source": "jira", "key": "PROJ-9"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set-task cli: status = %d", resp.StatusCode)
	}
	got, _ := srv.Store.GetWorktree(wt.ID)
	if got.TaskSource != "jira" || got.TaskKey != "PROJ-9" {
		t.Fatalf("after set-task: %s:%s", got.TaskSource, got.TaskKey)
	}

	resp = doJSON(t, http.MethodPut, base+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/task", map[string]string{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clear task: status = %d", resp.StatusCode)
	}
	got, _ = srv.Store.GetWorktree(wt.ID)
	if got.TaskKey != "" {
		t.Fatalf("after clear: task_key = %q", got.TaskKey)
	}
	// PROJ-9 lost its only worktree, so it settles at the last status.
	sts, _ := srv.Store.ListTaskStatuses()
	task, _ := srv.Store.GetTask("jira", "PROJ-9")
	if task.StatusID != sts[len(sts)-1].ID {
		t.Fatalf("cleared task status = %d, want last", task.StatusID)
	}
}

func TestUpdateTaskAndStatuses(t *testing.T) {
	base, _, srv := setupTaskRepo(t)
	task, _ := srv.Store.CreateLocalTask("t", "")

	resp := doJSON(t, http.MethodPost, base+"/api/task-statuses", map[string]string{"name": "QA"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("add status: %d", resp.StatusCode)
	}
	var qa store.TaskStatus
	decodeInto(t, resp, &qa)

	resp = doJSON(t, http.MethodPatch, base+"/api/tasks/local/"+task.Key, map[string]any{"status_id": qa.ID, "title": "renamed"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch task: %d", resp.StatusCode)
	}
	resp = doJSON(t, http.MethodPatch, base+"/api/tasks/local/"+task.Key, map[string]any{"status_id": 9999})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("patch unknown status: %d, want 400", resp.StatusCode)
	}
	resp = doJSON(t, http.MethodDelete, base+"/api/task-statuses/"+strconv.FormatInt(qa.ID, 10), nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("delete in-use status: %d, want 409", resp.StatusCode)
	}
}
