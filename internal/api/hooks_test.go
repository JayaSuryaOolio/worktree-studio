package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

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

func TestClaudeSessionTitleNotFound(t *testing.T) {
	ts, _ := newTestServer(t)
	t.Setenv("HOME", t.TempDir()) // isolate from the real ~/.claude

	resp := doJSON(t, http.MethodGet, ts.URL+"/api/claude-sessions/does-not-exist/title", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
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
