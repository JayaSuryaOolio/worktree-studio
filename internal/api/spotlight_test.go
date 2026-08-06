package api

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"worktree-studio/internal/spotlight"
	"worktree-studio/internal/store"
)

func requireSpotlightCLI(t *testing.T) {
	t.Helper()
	if _, err := spotlight.BinaryPath(); err != nil {
		t.Skip("spotlight CLI not available")
	}
	if _, err := exec.LookPath("fswatch"); err != nil {
		t.Skip("fswatch not found on PATH (required by the spotlight CLI itself)")
	}
}

func TestSpotlightStartStatusStop(t *testing.T) {
	requireGit(t)
	requireSpotlightCLI(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create worktree: status = %d, want 201", resp.StatusCode)
	}
	var wt store.Worktree
	decodeInto(t, resp, &wt)
	t.Cleanup(func() { _ = spotlight.Stop(repoPath) })

	// Status before starting: available, not active.
	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/spotlight/", nil)
	var status map[string]any
	decodeInto(t, resp, &status)
	if status["active"] != false {
		t.Fatalf("expected inactive before start, got %+v", status)
	}

	// Put something in the worktree so we can prove it actually mirrored.
	if err := os.WriteFile(filepath.Join(wt.Path, "hello.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/spotlight/start", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("spotlight start: status = %d, want 200", resp.StatusCode)
	}
	var startResp map[string]string
	decodeInto(t, resp, &startResp)
	if startResp["root"] == "" {
		t.Fatalf("spotlight start returned no root: %+v", startResp)
	}

	if data, err := os.ReadFile(filepath.Join(repoPath, "hello.txt")); err != nil || string(data) != "hi\n" {
		t.Fatalf("expected hello.txt mirrored into repo root, err=%v data=%q", err, data)
	}

	// Status after starting: active for this worktree.
	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/spotlight/", nil)
	decodeInto(t, resp, &status)
	if status["active"] != true {
		t.Fatalf("expected active after start, got %+v", status)
	}

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/spotlight/stop", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("spotlight stop: status = %d, want 200", resp.StatusCode)
	}

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/spotlight/", nil)
	decodeInto(t, resp, &status)
	if status["active"] != false {
		t.Fatalf("expected inactive after stop, got %+v", status)
	}
}

func TestSpotlightStartOnDirtyRootReturnsConflict(t *testing.T) {
	requireGit(t)
	requireSpotlightCLI(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	if err := os.WriteFile(filepath.Join(repoPath, "dirty.txt"), []byte("uncommitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/spotlight/start", nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("spotlight start on dirty root: status = %d, want 409", resp.StatusCode)
	}
}
