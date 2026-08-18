package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"worktree-studio/internal/store"
)

func TestClaudeHookLogsSessionForMatchingWorktree(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/claude-hook", map[string]string{
		"session_id": "hook-session-1",
		"cwd":        wt.Path,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST claude-hook: status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/audit-log", nil)
	var entries []map[string]any
	decodeInto(t, resp, &entries)

	var found map[string]any
	for _, e := range entries {
		if e["event"] == "claude.session.create" {
			found = e
		}
	}
	if found == nil {
		t.Fatalf("expected a claude.session.create entry, got %+v", entries)
	}
	if found["claude_session_id"] != "hook-session-1" {
		t.Errorf("claude_session_id = %v, want hook-session-1", found["claude_session_id"])
	}
	if found["source"] != "hook" {
		t.Errorf("source = %v, want hook", found["source"])
	}
}

func TestClaudeHookNotificationMarksWorktreePending(t *testing.T) {
	requireGit(t)
	ts, srv := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/claude-hook", map[string]string{
		"session_id":      "notif-session-1",
		"cwd":             wt.Path,
		"hook_event_name": "Notification",
		"message":         "Claude needs your permission to use Bash",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST claude-hook (Notification): status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	snapshot := srv.Attention.Snapshot()
	if snapshot[wt.ID] != "Claude needs your permission to use Bash" {
		t.Errorf("attention snapshot[%s] = %q, want the notification message", wt.ID, snapshot[wt.ID])
	}

	// A Notification event must never be logged as a claude.session.create
	// entry — that's SessionStart's job only.
	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/audit-log", nil)
	var entries []map[string]any
	decodeInto(t, resp, &entries)
	for _, e := range entries {
		if e["event"] == "claude.session.create" {
			t.Errorf("unexpected claude.session.create entry from a Notification hook: %+v", e)
		}
	}
}

// Regression test for direct feedback: a Notification whose message is
// just background-progress chatter (e.g. still waiting on background
// agents) must not badge the worktree — only a permission prompt,
// waiting-for-input, or finished-result message should.
func TestClaudeHookNotificationSkipsBackgroundAgentMessage(t *testing.T) {
	requireGit(t)
	ts, srv := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/claude-hook", map[string]string{
		"cwd":             wt.Path,
		"hook_event_name": "Notification",
		"message":         "Claude is waiting for background agents to finish before continuing",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST claude-hook (Notification): status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	if _, pending := srv.Attention.Snapshot()[wt.ID]; pending {
		t.Errorf("expected worktree %s to NOT be marked pending for a background-agent status message", wt.ID)
	}
}

func TestClearAttentionEndpointClearsPending(t *testing.T) {
	requireGit(t)
	ts, srv := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	srv.Attention.SetPending(wt.ID, "waiting")

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/attention/clear", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST attention/clear: status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	if _, pending := srv.Attention.Snapshot()[wt.ID]; pending {
		t.Error("expected worktree to no longer be pending after clear")
	}
}

func TestClaudeHookIgnoresUnrelatedCwd(t *testing.T) {
	ts, _ := newTestServer(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/claude-hook", map[string]string{
		"session_id": "some-other-session",
		"cwd":        "/nowhere/tracked",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST claude-hook for unrelated cwd: status = %d, want 200 (silent no-op)", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestClaudeHookIgnoresMalformedBody(t *testing.T) {
	ts, _ := newTestServer(t)

	resp, err := http.Post(ts.URL+"/api/claude-hook", "application/json", nil)
	if err != nil {
		t.Fatalf("POST claude-hook with no body: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (must never fail a claude session's own startup)", resp.StatusCode)
	}
}

func TestClaudeHookContextLogsInjectedTextForMatchingWorktree(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/claude-hook-context", map[string]string{
		"cwd":     wt.Path,
		"context": "Ooga. Claude wake up in cave (folder): " + wt.Path + "\nOoo, worktree-studio cave! Branch-mark say: feature",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST claude-hook-context: status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/audit-log", nil)
	var entries []map[string]any
	decodeInto(t, resp, &entries)

	var found map[string]any
	for _, e := range entries {
		if e["event"] == "claude.session.context" {
			found = e
		}
	}
	if found == nil {
		t.Fatalf("expected a claude.session.context entry, got %+v", entries)
	}
	if context, _ := found["context"].(string); !strings.Contains(context, "Branch-mark say: feature") {
		t.Errorf("context = %q, want it to contain the injected branch line", context)
	}
	if found["source"] != "hook" {
		t.Errorf("source = %v, want hook", found["source"])
	}
}

func TestClaudeHookContextIgnoresUnrelatedCwd(t *testing.T) {
	ts, _ := newTestServer(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/claude-hook-context", map[string]string{
		"cwd":     "/nowhere/tracked",
		"context": "Ooga. Claude wake up in cave (folder): /nowhere/tracked",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST claude-hook-context for unrelated cwd: status = %d, want 200 (silent no-op)", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestClaudeHookContextIgnoresEmptyContext(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/claude-hook-context", map[string]string{"cwd": wt.Path, "context": ""})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (silent no-op)", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/audit-log", nil)
	var entries []map[string]any
	decodeInto(t, resp, &entries)
	for _, e := range entries {
		if e["event"] == "claude.session.context" {
			t.Fatalf("expected no claude.session.context entry for an empty context, got %+v", e)
		}
	}
}

func TestClaudeHookContextIgnoresMalformedBody(t *testing.T) {
	ts, _ := newTestServer(t)

	resp, err := http.Post(ts.URL+"/api/claude-hook-context", "application/json", nil)
	if err != nil {
		t.Fatalf("POST claude-hook-context with no body: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (must never fail a claude session's own startup)", resp.StatusCode)
	}
}

func TestClaudeSessionTitleNotFound(t *testing.T) {
	ts, _ := newTestServer(t)
	t.Setenv("HOME", t.TempDir()) // isolate from the real ~/.claude

	resp := doJSON(t, http.MethodGet, ts.URL+"/api/claude-sessions/does-not-exist/title", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestOpenFilePublishesEventForMatchingWorktree(t *testing.T) {
	requireGit(t)
	ts, srv := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	events, unsubscribe := srv.OpenFile.Subscribe()
	defer unsubscribe()

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/open-file", map[string]string{
		"cwd":  wt.Path,
		"path": "README.md",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST open-file: status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	select {
	case ev := <-events:
		if ev.WorktreeID != wt.ID || ev.Path != "README.md" {
			t.Errorf("got %+v, want worktree_id=%s path=README.md", ev, wt.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for openfile event")
	}
}

func TestOpenFileRelativeToSubdirectoryCwd(t *testing.T) {
	requireGit(t)
	ts, srv := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	subdir := filepath.Join(wt.Path, "sub")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	events, unsubscribe := srv.OpenFile.Subscribe()
	defer unsubscribe()

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/open-file", map[string]string{
		"cwd":  subdir,
		"path": "../README.md",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST open-file: status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	select {
	case ev := <-events:
		if ev.Path != "README.md" {
			t.Errorf("path = %q, want README.md", ev.Path)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for openfile event")
	}
}

func TestOpenFileNoMatchingWorktreeIsANoOp(t *testing.T) {
	ts, srv := newTestServer(t)

	events, unsubscribe := srv.OpenFile.Subscribe()
	defer unsubscribe()

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/open-file", map[string]string{
		"cwd":  "/nowhere/tracked",
		"path": "README.md",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (silent no-op)", resp.StatusCode)
	}
	resp.Body.Close()

	select {
	case ev := <-events:
		t.Fatalf("expected no event to be published, got %+v", ev)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestOpenFileRejectsPathEscapingWorktree(t *testing.T) {
	requireGit(t)
	ts, srv := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	events, unsubscribe := srv.OpenFile.Subscribe()
	defer unsubscribe()

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/open-file", map[string]string{
		"cwd":  wt.Path,
		"path": "../../etc/passwd",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()

	select {
	case ev := <-events:
		t.Fatalf("expected no event to be published for an escaping path, got %+v", ev)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestOpenFileIgnoresMalformedBody(t *testing.T) {
	ts, _ := newTestServer(t)

	resp, err := http.Post(ts.URL+"/api/open-file", "application/json", nil)
	if err != nil {
		t.Fatalf("POST open-file with no body: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestClaudeSessionTitleFound(t *testing.T) {
	ts, _ := newTestServer(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".claude", "projects", "-tmp-x")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"user","message":{"role":"user","content":"do the thing"}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "s1.jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	resp := doJSON(t, http.MethodGet, ts.URL+"/api/claude-sessions/s1/title", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body map[string]string
	decodeInto(t, resp, &body)
	if body["title"] != "do the thing" {
		t.Errorf("title = %q, want %q", body["title"], "do the thing")
	}
}
