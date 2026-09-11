package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"worktree-studio/internal/store"
)

func TestListWorktreeClaudeSessionsEmptyWhenNoneSeen(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	t.Setenv("HOME", t.TempDir()) // isolate from the real ~/.claude
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/claude-sessions", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var sessions []map[string]any
	decodeInto(t, resp, &sessions)
	if len(sessions) != 0 {
		t.Errorf("sessions = %+v, want empty", sessions)
	}
}

func TestListWorktreeClaudeSessionsFindsRealTranscripts(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	// Same transform as claudehook.projectDirName: every non-alphanumeric
	// character in the worktree's own path becomes '-'.
	projectDir := nonAlnumToDash(wt.Path)
	dir := filepath.Join(home, ".claude", "projects", projectDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"user","message":{"role":"user","content":"fix the thing"}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "sess-1.jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/claude-sessions", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var sessions []map[string]any
	decodeInto(t, resp, &sessions)
	if len(sessions) != 1 {
		t.Fatalf("sessions = %+v, want 1", sessions)
	}
	if sessions[0]["session_id"] != "sess-1" {
		t.Errorf("session_id = %v, want sess-1", sessions[0]["session_id"])
	}
	if sessions[0]["preview"] != "fix the thing" {
		t.Errorf("preview = %v, want %q", sessions[0]["preview"], "fix the thing")
	}
	if sizeBytes, ok := sessions[0]["size_bytes"].(float64); !ok || sizeBytes != float64(len(line)) {
		t.Errorf("size_bytes = %v, want %d", sessions[0]["size_bytes"], len(line))
	}
}

func TestListWorktreeClaudeSessionsUnknownWorktree(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/does-not-exist/claude-sessions", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// nonAlnumToDash mirrors claudehook's unexported projectDirName exactly —
// duplicated here (rather than exported from claudehook just for a test)
// since this test's real goal is verifying the HTTP endpoint end-to-end
// against a transcript laid out the way Claude Code actually would, not
// re-testing the transform itself (see claudehook_test.go's own
// TestProjectDirName for that).
func nonAlnumToDash(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			out[i] = c
		} else {
			out[i] = '-'
		}
	}
	return string(out)
}
