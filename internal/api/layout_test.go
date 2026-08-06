package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"worktree-studio/internal/store"
)

func TestLayoutNotFoundThenSaveThenGet(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	// No layout saved yet.
	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/layout", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET layout before any save: status = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()

	layout := map[string]any{"grid": map[string]any{"root": map[string]any{"type": "leaf", "data": map[string]any{"views": []string{"t1"}}}}}
	resp = doJSON(t, http.MethodPut, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/layout", layout)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT layout: status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/layout", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET layout after save: status = %d, want 200", resp.StatusCode)
	}
	var got map[string]any
	decodeInto(t, resp, &got)
	gridBytes, _ := json.Marshal(got["grid"])
	wantBytes, _ := json.Marshal(layout["grid"])
	if string(gridBytes) != string(wantBytes) {
		t.Errorf("GET layout returned %s, want %s", gridBytes, wantBytes)
	}

	// Saving again must overwrite (upsert), not error or duplicate.
	layout2 := map[string]any{"grid": map[string]any{"root": map[string]any{"type": "leaf", "data": map[string]any{"views": []string{"t2"}}}}}
	resp = doJSON(t, http.MethodPut, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/layout", layout2)
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/layout", nil)
	decodeInto(t, resp, &got)
	gridBytes, _ = json.Marshal(got["grid"])
	wantBytes, _ = json.Marshal(layout2["grid"])
	if string(gridBytes) != string(wantBytes) {
		t.Errorf("GET layout after update returned %s, want %s", gridBytes, wantBytes)
	}
}

func TestLayoutRejectsInvalidJSON(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)
	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/layout", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT layout: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT invalid-JSON layout: status = %d, want 400", resp.StatusCode)
	}
}

func TestLayoutNotFoundForMissingWorktree(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/does-not-exist/layout", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET layout for missing worktree: status = %d, want 404", resp.StatusCode)
	}
}
