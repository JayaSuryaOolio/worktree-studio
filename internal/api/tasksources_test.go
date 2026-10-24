package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"worktree-studio/internal/store"
)

// fakeAdapter answers `list` with one task and `get` with a renamed one.
func fakeAdapter(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fake")
	script := `#!/bin/sh
case "$1" in
list) echo '[{"key":"FK-1","title":"Listed","url":"https://fake/FK-1"}]' ;;
get)  echo '[{"key":"FK-1","title":"Renamed upstream","url":"https://fake/FK-1"}]' ;;
esac
`
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTaskSourceSearchPickAndRefresh(t *testing.T) {
	base, repo, srv := setupTaskRepo(t)

	resp := doJSON(t, http.MethodPost, base+"/api/task-sources", map[string]string{"id": "local", "command": fakeAdapter(t)})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf(`id "local": status = %d, want 400`, resp.StatusCode)
	}
	resp = doJSON(t, http.MethodPost, base+"/api/task-sources", map[string]string{"id": "fake", "command": "relative/path"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("relative command: status = %d, want 400", resp.StatusCode)
	}
	resp = doJSON(t, http.MethodPost, base+"/api/task-sources", map[string]string{"id": "fake", "command": fakeAdapter(t)})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("add source: status = %d", resp.StatusCode)
	}

	resp = doJSON(t, http.MethodPost, base+"/api/task-sources/fake/test", nil)
	var test struct{ Count int }
	decodeInto(t, resp, &test)
	if test.Count != 1 {
		t.Fatalf("test count = %d", test.Count)
	}

	resp = doJSON(t, http.MethodGet, base+"/api/tasks/search?q=x", nil)
	var found []store.Task
	decodeInto(t, resp, &found)
	if len(found) != 1 || found[0].Source != "fake" || found[0].Key != "FK-1" {
		t.Fatalf("search = %+v", found)
	}

	// Picking the result sends its title along with the ref.
	resp = doJSON(t, http.MethodPost, base+"/api/repos/"+repo.ID+"/worktrees/", map[string]any{
		"name": "fk", "task": map[string]string{"source": "fake", "key": "FK-1", "title": "Listed", "url": "https://fake/FK-1"},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: status = %d", resp.StatusCode)
	}
	if task, _ := srv.Store.GetTask("fake", "FK-1"); task.Title != "Listed" {
		t.Fatalf("picked title = %q", task.Title)
	}

	resp = doJSON(t, http.MethodPost, base+"/api/tasks/refresh?force=1", nil)
	var ref struct{ Refreshed int }
	decodeInto(t, resp, &ref)
	if task, _ := srv.Store.GetTask("fake", "FK-1"); ref.Refreshed != 1 || task.Title != "Renamed upstream" {
		t.Fatalf("after refresh: refreshed=%d title=%q", ref.Refreshed, task.Title)
	}
}
