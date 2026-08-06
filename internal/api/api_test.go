package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"worktree-studio/internal/audit"
	"worktree-studio/internal/store"
	"worktree-studio/internal/term"
)

// requireGit skips the test if git isn't on PATH.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}
}

// newTestGitRepo creates a throwaway git repo (with one commit) under a
// t.TempDir, which is auto-cleaned.
func newTestGitRepo(t *testing.T) string {
	t.Helper()
	requireGit(t)

	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.name", "test")
	run("config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README.md")
	run("commit", "-q", "-m", "initial commit")
	return dir
}

// newTestServer wires up a Server against fresh temp storage, mounted on a
// real chi router served by httptest, so tests exercise the actual HTTP
// stack (routing, status codes, JSON encoding) rather than calling handler
// methods directly.
func newTestServer(t *testing.T) (*httptest.Server, *Server) {
	t.Helper()

	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	al, err := audit.New(filepath.Join(t.TempDir(), "audit.log.jsonl"))
	if err != nil {
		t.Fatalf("audit.New: %v", err)
	}

	srv := &Server{
		Store:        st,
		Audit:        al,
		Term:         &term.Manager{Store: st, Audit: al},
		WorktreeRoot: filepath.Join(t.TempDir(), "worktrees"),
		Log:          slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1})),
	}

	r := chi.NewRouter()
	srv.Routes(r)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return ts, srv
}

func doJSON(t *testing.T, method, url string, body any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

func decodeInto(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
}

func TestAddRepoRejectsRelativePath(t *testing.T) {
	ts, _ := newTestServer(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{
		"name": "x", "path": "relative/path",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /api/repos/ with relative path: status = %d, want 400", resp.StatusCode)
	}
	var body map[string]string
	decodeInto(t, resp, &body)
	if body["error"] == "" {
		t.Errorf("expected a non-empty error message, got %+v", body)
	}
}

func TestAddRepoRejectsNonGitDir(t *testing.T) {
	ts, _ := newTestServer(t)
	dir := t.TempDir() // absolute, exists, but not a git repo

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{
		"name": "x", "path": dir,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /api/repos/ with non-git dir: status = %d, want 400", resp.StatusCode)
	}
}

func TestAddRepoRejectsDuplicatePath(t *testing.T) {
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "r", "path": repoPath})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("first POST /api/repos/: status = %d, want 201", resp.StatusCode)
	}

	resp2 := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "r2", "path": repoPath})
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate POST /api/repos/: status = %d, want 409", resp2.StatusCode)
	}
}

// TestFullWorktreeLifecycle drives the same repo-add -> list -> name
// suggestion -> create -> list -> delete flow the manual curl verification
// used, as an automated regression test.
func TestFullWorktreeLifecycle(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	// Add repo.
	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "adelaide", "path": repoPath})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/repos/: status = %d, want 201", resp.StatusCode)
	}
	var repo store.Repo
	decodeInto(t, resp, &repo)
	if repo.ID == "" || repo.Path != repoPath {
		t.Fatalf("added repo = %+v, want non-empty ID and Path=%q", repo, repoPath)
	}

	// List worktrees: empty.
	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", nil)
	var wts []store.Worktree
	decodeInto(t, resp, &wts)
	if len(wts) != 0 {
		t.Fatalf("initial worktree list = %+v, want empty", wts)
	}

	// Name suggestion.
	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/new-name-suggestion", nil)
	var suggestion map[string]string
	decodeInto(t, resp, &suggestion)
	name := suggestion["name"]
	if name == "" {
		t.Fatalf("new-name-suggestion returned empty name")
	}

	// Create worktree with the suggested name.
	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": name})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST worktrees: status = %d, want 201", resp.StatusCode)
	}
	var wt store.Worktree
	decodeInto(t, resp, &wt)
	if wt.CreatedAt == "" {
		t.Errorf("created worktree has empty CreatedAt (regression: this was a real bug in v1)")
	}
	if _, err := os.Stat(wt.Path); err != nil {
		t.Errorf("worktree path %q does not exist on disk: %v", wt.Path, err)
	}

	// List worktrees: one entry.
	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", nil)
	decodeInto(t, resp, &wts)
	if len(wts) != 1 || wts[0].ID != wt.ID {
		t.Fatalf("worktree list after create = %+v, want one entry with ID=%q", wts, wt.ID)
	}

	// Delete it.
	resp = doJSON(t, http.MethodDelete, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE worktree: status = %d, want 200", resp.StatusCode)
	}

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", nil)
	decodeInto(t, resp, &wts)
	if len(wts) != 0 {
		t.Fatalf("worktree list after delete = %+v, want empty", wts)
	}
	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Errorf("worktree dir %q still exists after delete: err=%v", wt.Path, err)
	}
}

// TestDeleteWorktreeClosesItsTerminalSessions is a regression test for a
// real bug found by hand while testing step 7.4 (dockview terminal
// arrangement): deleting a worktree never closed its terminal sessions —
// the terminal_sessions DB row disappears via ON DELETE CASCADE when the
// worktree row goes, but nothing ever killed the actual tmux session
// behind it, leaving a permanently orphaned OS process with no DB trace
// pointing back to it at all.
func TestDeleteWorktreeClosesItsTerminalSessions(t *testing.T) {
	requireGit(t)
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not found on PATH")
	}
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/terminals/", map[string]string{"tab_label": "shell"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create terminal: status = %d, want 201", resp.StatusCode)
	}
	var termSession store.TerminalSession
	decodeInto(t, resp, &termSession)

	liveBefore, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		t.Fatalf("tmux list-sessions: %v", err)
	}
	if !strings.Contains(string(liveBefore), termSession.TmuxSessionName) {
		t.Fatalf("expected tmux session %q to be live before delete, got:\n%s", termSession.TmuxSessionName, liveBefore)
	}

	resp = doJSON(t, http.MethodDelete, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete worktree: status = %d, want 200", resp.StatusCode)
	}

	liveAfter, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	// tmux exits non-zero ("no server running") once its last session is
	// gone — that's success here, not a real error.
	if err != nil && !strings.Contains(string(liveAfter), termSession.TmuxSessionName) {
		return
	}
	if strings.Contains(string(liveAfter), termSession.TmuxSessionName) {
		t.Fatalf("expected tmux session %q to be killed by the worktree delete, but it's still live:\n%s", termSession.TmuxSessionName, liveAfter)
	}
}

// TestDeleteDirtyWorktreeRequiresForce verifies the fix for the
// force-remove/no-dirty-check bug: deleting a worktree with uncommitted
// changes should 409 without ?force=true, and only actually remove it (and
// the changes) when force is passed.
func TestDeleteDirtyWorktreeRequiresForce(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "adelaide", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "dirty-one"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	// Dirty the worktree with an untracked file.
	if err := os.WriteFile(filepath.Join(wt.Path, "scratch.txt"), []byte("wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Without force: 409, worktree and record both survive.
	resp = doJSON(t, http.MethodDelete, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("DELETE dirty worktree (no force): status = %d, want 409", resp.StatusCode)
	}
	resp.Body.Close()
	if _, err := os.Stat(wt.Path); err != nil {
		t.Errorf("worktree dir should still exist after a refused delete: %v", err)
	}

	var wts []store.Worktree
	listResp := doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", nil)
	decodeInto(t, listResp, &wts)
	if len(wts) != 1 {
		t.Fatalf("worktree record should still be listed after refused delete, got %+v", wts)
	}

	// With force: succeeds, worktree and record are gone.
	resp = doJSON(t, http.MethodDelete, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"?force=true", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE dirty worktree (force=true): status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()
	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Errorf("worktree dir should be gone after forced delete: err=%v", err)
	}
}

func TestDeleteWorktreeNotFound(t *testing.T) {
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "adelaide", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodDelete, ts.URL+"/api/repos/"+repo.ID+"/worktrees/does-not-exist", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("DELETE missing worktree: status = %d, want 404", resp.StatusCode)
	}
}

// TestCreateWorktreeRollsBackGitOnStoreFailure is a regression test for a
// real bug found in production use: if `git worktree add` succeeds but the
// subsequent store.AddWorktree fails for any reason (disk full, DB locked,
// or — how this was actually first hit — the server's data directory
// having been deleted out from under a still-running process), the
// original code left an orphaned git worktree + branch behind with no DB
// record of it. Every retry with the same name then failed at the git
// layer ("a branch named ... already exists"), with no way to recover
// short of manually running `git worktree remove`/`git branch -D`. Forces
// the store write to fail deterministically via a UNIQUE(path) collision
// (pre-inserting a row at the exact path the handler is about to compute)
// rather than mocking anything, and asserts the git-level rollback
// actually happened.
func TestCreateWorktreeRollsBackGitOnStoreFailure(t *testing.T) {
	requireGit(t)
	ts, srv := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	const name = "feature"
	collisionPath := filepath.Join(srv.WorktreeRoot, repo.ID, name)
	if err := srv.Store.AddWorktree(store.Worktree{
		ID: "pre-existing", RepoID: repo.ID, Name: name, Branch: "unrelated-branch", Path: collisionPath,
	}); err != nil {
		t.Fatalf("seed colliding worktree row: %v", err)
	}

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": name})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("create worktree with a colliding path: status = %d, want 500", resp.StatusCode)
	}
	var body map[string]string
	decodeInto(t, resp, &body)
	if !strings.Contains(body["error"], "rolled back") {
		t.Fatalf("expected the error to mention the rollback, got: %q", body["error"])
	}

	// The real assertion: the git worktree add's side effects must be
	// gone, not left dangling.
	branchOut, err := exec.Command("git", "-C", repoPath, "branch", "--list", name).Output()
	if err != nil {
		t.Fatalf("git branch --list: %v", err)
	}
	if strings.TrimSpace(string(branchOut)) != "" {
		t.Errorf("branch %q should have been rolled back (deleted), but git branch --list found: %q", name, branchOut)
	}
	if _, err := os.Stat(collisionPath); !os.IsNotExist(err) {
		t.Errorf("worktree directory %q should have been rolled back (removed), stat err = %v", collisionPath, err)
	}
	worktreeListOut, err := exec.Command("git", "-C", repoPath, "worktree", "list").Output()
	if err != nil {
		t.Fatalf("git worktree list: %v", err)
	}
	if strings.Contains(string(worktreeListOut), collisionPath) {
		t.Errorf("git worktree list should no longer mention the rolled-back path, got:\n%s", worktreeListOut)
	}
}
