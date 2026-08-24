package api

import (
	"net/http"
	"net/url"
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

// TestSpotlightStartWithStashRetriesSuccessfully verifies the confirm-and-
// retry flow this endpoint's ?stash=true is for: a plain start refuses on a
// dirty root exactly like the test above, but retrying with ?stash=true
// (what the frontend sends after the user confirms a browser prompt)
// succeeds — the non-interactive equivalent of answering "yes" to the
// spotlight CLI's own interactive stash prompt, which this server-side
// exec call could never actually see or answer.
func TestSpotlightStartWithStashRetriesSuccessfully(t *testing.T) {
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
	t.Cleanup(func() { _ = spotlight.Stop(repoPath) })

	if err := os.WriteFile(filepath.Join(repoPath, "dirty.txt"), []byte("uncommitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/spotlight/start", nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("spotlight start on dirty root (no stash): status = %d, want 409", resp.StatusCode)
	}

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/spotlight/start?stash=true", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("spotlight start with ?stash=true: status = %d, want 200", resp.StatusCode)
	}

	out, err := exec.Command("git", "-C", repoPath, "status", "--porcelain").Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected repo root to be clean (changes stashed) after starting with ?stash=true, got: %q", out)
	}
}

// TestSpotlightCLIStartStatusStop exercises the path-based endpoints backing
// the `worktree-studio spotlight --start|--status|--stop [path]` CLI
// subcommand (cmd/worktree-studio/spotlight.go) — same underlying behavior
// as TestSpotlightStartStatusStop, but resolved from an arbitrary path
// inside the worktree instead of a known worktree id, the way a shell
// command actually calls in.
func TestSpotlightCLIStartStatusStop(t *testing.T) {
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

	// A subdirectory of the worktree, not just its root, should resolve —
	// mirrors store.FindWorktreeByPath's own "cwd inside a subdirectory
	// still counts" contract, which is what makes running this from a
	// terminal pane sitting anywhere inside the worktree work.
	subdir := filepath.Join(wt.Path, "sub")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/spotlight/status?path="+url.QueryEscape(subdir), nil)
	var status map[string]any
	decodeInto(t, resp, &status)
	if status["active"] != false {
		t.Fatalf("expected inactive before start, got %+v", status)
	}

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/spotlight/start", map[string]any{"path": subdir})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("spotlight cli start: status = %d, want 200", resp.StatusCode)
	}
	var startResp map[string]string
	decodeInto(t, resp, &startResp)
	if startResp["root"] == "" {
		t.Fatalf("spotlight cli start returned no root: %+v", startResp)
	}

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/spotlight/status?path="+url.QueryEscape(subdir), nil)
	decodeInto(t, resp, &status)
	if status["active"] != true {
		t.Fatalf("expected active after start, got %+v", status)
	}

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/spotlight/stop", map[string]any{"path": subdir})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("spotlight cli stop: status = %d, want 200", resp.StatusCode)
	}

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/spotlight/status?path="+url.QueryEscape(subdir), nil)
	decodeInto(t, resp, &status)
	if status["active"] != false {
		t.Fatalf("expected inactive after stop, got %+v", status)
	}
}

// TestSpotlightCLIPathOutsideAnyWorktreeIsANoOp mirrors
// TestOpenFileNoMatchingWorktreeIsANoOp's contract for the same reason: this
// subcommand can be run from anywhere on disk, and a path outside every
// tracked worktree is a normal, expected outcome, not an error — reported
// as its own distinct status rather than a failure the CLI would report as
// broken.
func TestSpotlightCLIPathOutsideAnyWorktreeIsANoOp(t *testing.T) {
	ts, _ := newTestServer(t)
	tmp := t.TempDir()

	resp := doJSON(t, http.MethodGet, ts.URL+"/api/spotlight/status?path="+url.QueryEscape(tmp), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status for untracked path: status = %d, want 200", resp.StatusCode)
	}
	var result map[string]string
	decodeInto(t, resp, &result)
	if result["status"] != "no matching worktree" {
		t.Fatalf("status for untracked path = %+v, want no matching worktree", result)
	}

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/spotlight/start", map[string]any{"path": tmp})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("start for untracked path: status = %d, want 200", resp.StatusCode)
	}
	decodeInto(t, resp, &result)
	if result["status"] != "no matching worktree" {
		t.Fatalf("start for untracked path = %+v, want no matching worktree", result)
	}
}
