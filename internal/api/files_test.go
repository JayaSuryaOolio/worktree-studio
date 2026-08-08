package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"worktree-studio/internal/files"
	"worktree-studio/internal/store"
)

// testFileEnv bundles a running test server with a registered repo and
// worktree, so each test below only needs to build files/tree|content URLs
// against it.
type testFileEnv struct {
	baseURL  string
	repo     store.Repo
	worktree store.Worktree
}

func setupTestWorktree(t *testing.T) (*testFileEnv, string) {
	t.Helper()
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	return &testFileEnv{baseURL: ts.URL, repo: repo, worktree: wt}, wt.Path
}

func (s *testFileEnv) filesURL(sub string, query url.Values) string {
	u := s.baseURL + "/api/repos/" + s.repo.ID + "/worktrees/" + s.worktree.ID + "/files/" + sub
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

func (s *testFileEnv) filesWSURL() string {
	return "ws" + strings.TrimPrefix(s.baseURL, "http") + "/ws/files/" + s.worktree.ID
}

func TestFileTree(t *testing.T) {
	env, wtPath := setupTestWorktree(t)

	if err := os.MkdirAll(filepath.Join(wtPath, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, "src", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resp := doJSON(t, http.MethodGet, env.filesURL("tree", nil), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET files/tree: status = %d, want 200", resp.StatusCode)
	}
	var tree []files.FileNode
	decodeInto(t, resp, &tree)

	var names []string
	for _, n := range tree {
		names = append(names, n.Name)
	}
	found := false
	for _, n := range names {
		if n == "README.md" {
			found = true
		}
	}
	if !found {
		t.Errorf("file tree missing README.md, got %v", names)
	}
}

func TestGetAndPutFileContent(t *testing.T) {
	env, _ := setupTestWorktree(t)

	resp := doJSON(t, http.MethodGet, env.filesURL("content", url.Values{"path": {"README.md"}}), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET files/content: status = %d, want 200", resp.StatusCode)
	}
	var got map[string]string
	decodeInto(t, resp, &got)
	if got["content"] != "hello\n" {
		t.Errorf("content = %q, want %q", got["content"], "hello\n")
	}

	resp = doJSON(t, http.MethodPut, env.filesURL("content", url.Values{"path": {"README.md"}}), map[string]string{
		"content": "updated content\n",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT files/content: status = %d, want 200", resp.StatusCode)
	}

	resp = doJSON(t, http.MethodGet, env.filesURL("content", url.Values{"path": {"README.md"}}), nil)
	decodeInto(t, resp, &got)
	if got["content"] != "updated content\n" {
		t.Errorf("content after write = %q, want %q", got["content"], "updated content\n")
	}
}

func TestGetFileContentRejectsTraversal(t *testing.T) {
	env, _ := setupTestWorktree(t)

	resp := doJSON(t, http.MethodGet, env.filesURL("content", url.Values{"path": {"../../etc/passwd"}}), nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("GET files/content traversal: status = %d, want 400", resp.StatusCode)
	}
}

func TestPutFileContentRejectsTraversal(t *testing.T) {
	env, _ := setupTestWorktree(t)

	resp := doJSON(t, http.MethodPut, env.filesURL("content", url.Values{"path": {"../../tmp/evil.txt"}}), map[string]string{
		"content": "pwned",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("PUT files/content traversal: status = %d, want 400", resp.StatusCode)
	}
}

func TestGetFileContentNotFound(t *testing.T) {
	env, _ := setupTestWorktree(t)

	resp := doJSON(t, http.MethodGet, env.filesURL("content", url.Values{"path": {"does-not-exist.txt"}}), nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET files/content missing file: status = %d, want 404", resp.StatusCode)
	}
}

type changedMsg struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

// TestFilesWSReportsExternalChange verifies the fsnotify-driven push
// actually reaches a real ws client for a file changed outside the API
// (e.g. by a `claude` terminal or `git checkout` in the same worktree) —
// see docs/editor-plan.md pitfall #5.
func TestFilesWSReportsExternalChange(t *testing.T) {
	env, wtPath := setupTestWorktree(t)

	conn, _, err := websocket.DefaultDialer.Dial(env.filesWSURL(), nil)
	if err != nil {
		t.Fatalf("dial files ws: %v", err)
	}
	defer conn.Close()

	time.Sleep(100 * time.Millisecond) // let Subscribe's initial recursive watch-add complete
	if err := os.WriteFile(filepath.Join(wtPath, "README.md"), []byte("changed externally\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("expected a changed event, got error: %v", err)
	}
	var msg changedMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if msg.Type != "changed" || msg.Path != "README.md" {
		t.Errorf("event = %+v, want {changed README.md}", msg)
	}
}

// TestPutFileContentSuppressesOwnWriteEvent is a regression test for a
// real race found during manual verification: MarkOwnWrite must run
// BEFORE the write happens, not after — the OS can emit (and the watcher
// can process) the fsnotify event as soon as the write syscall completes,
// racing whatever runs next in the handler. Marking after the write left
// a window where a save event reached this test's ws client unsuppressed
// nearly every time. See docs/editor-plan.md pitfall #6 and the ordering
// comment in handlePutFileContent.
func TestPutFileContentSuppressesOwnWriteEvent(t *testing.T) {
	env, _ := setupTestWorktree(t)

	conn, _, err := websocket.DefaultDialer.Dial(env.filesWSURL(), nil)
	if err != nil {
		t.Fatalf("dial files ws: %v", err)
	}
	defer conn.Close()

	time.Sleep(100 * time.Millisecond)

	resp := doJSON(t, http.MethodPut, env.filesURL("content", url.Values{"path": {"README.md"}}), map[string]string{
		"content": "saved via api\n",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT files/content: status = %d, want 200", resp.StatusCode)
	}

	// Own-write suppression window is 2s and the debounce window is
	// 300ms — 800ms comfortably covers a real (bad) event without waiting
	// anywhere near the full suppression window.
	conn.SetReadDeadline(time.Now().Add(800 * time.Millisecond))
	_, data, err := conn.ReadMessage()
	if err == nil {
		t.Fatalf("expected no event (own write should be suppressed), got: %s", data)
	}
}
