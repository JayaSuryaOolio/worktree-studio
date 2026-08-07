package api

import (
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"worktree-studio/internal/store"
)

func requireTmuxAPI(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not found on PATH")
	}
}

// TestCreateTerminalWithInitialCommand verifies the initial_command field
// (used by the frontend to auto-run `claude` in a freshly created
// worktree's first terminal) actually reaches the real tmux session, end
// to end through the HTTP API — not just that internal/term.CreateSession
// accepts the parameter.
func TestCreateTerminalWithInitialCommand(t *testing.T) {
	requireGit(t)
	requireTmuxAPI(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/terminals/", map[string]string{
		"tab_label":       "claude",
		"initial_command": "echo hello-from-initial-command-api-test",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create terminal with initial_command: status = %d, want 201", resp.StatusCode)
	}
	var termSession store.TerminalSession
	decodeInto(t, resp, &termSession)
	t.Cleanup(func() {
		_ = exec.Command("tmux", "kill-session", "-t", termSession.TmuxSessionName).Run()
	})

	var out []byte
	var err error
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		out, err = exec.Command("tmux", "capture-pane", "-p", "-t", termSession.TmuxSessionName).Output()
		if err != nil {
			t.Fatalf("capture-pane: %v", err)
		}
		if strings.Contains(string(out), "hello-from-initial-command-api-test") {
			return // success
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("expected the initial command's output in the pane within 3s, got:\n%s", out)
}

// TestCreateTerminalLogsClaudeSessionWhenIDProvided verifies the
// claude_session_id/claude_session_title fields land in the audit log as
// their own claude.session.create event — the whole point being that this
// record outlives the terminal/tmux session itself, so a person can later
// `claude --resume <id>` in the worktree even after the tab is closed.
func TestCreateTerminalLogsClaudeSessionWhenIDProvided(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/terminals/", map[string]string{
		"tab_label":            "claude",
		"initial_command":      "claude --session-id abc-123 -n feature",
		"claude_session_id":    "abc-123",
		"claude_session_title": "feature",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create terminal: status = %d, want 201", resp.StatusCode)
	}
	var termSession store.TerminalSession
	decodeInto(t, resp, &termSession)

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/audit-log", nil)
	var entries []map[string]any
	decodeInto(t, resp, &entries)

	var found map[string]any
	for _, e := range entries {
		if e["event"] == "claude.session.create" {
			found = e
			break
		}
	}
	if found == nil {
		t.Fatalf("expected a claude.session.create audit entry, got %+v", entries)
	}
	if found["claude_session_id"] != "abc-123" {
		t.Errorf("claude_session_id = %v, want abc-123", found["claude_session_id"])
	}
	if found["title"] != "feature" {
		t.Errorf("title = %v, want feature", found["title"])
	}
	if found["terminal_id"] != termSession.ID {
		t.Errorf("terminal_id = %v, want %v", found["terminal_id"], termSession.ID)
	}
	if found["worktree_id"] != wt.ID {
		t.Errorf("worktree_id = %v, want %v", found["worktree_id"], wt.ID)
	}
}

// TestCreateTerminalWithoutClaudeSessionIDLogsNoClaudeEvent guards against
// every plain terminal creation (no claude involved at all) spuriously
// logging a claude.session.create event.
func TestCreateTerminalWithoutClaudeSessionIDLogsNoClaudeEvent(t *testing.T) {
	requireGit(t)
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
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/repos/"+repo.ID+"/worktrees/"+wt.ID+"/audit-log", nil)
	var entries []map[string]any
	decodeInto(t, resp, &entries)
	for _, e := range entries {
		if e["event"] == "claude.session.create" {
			t.Fatalf("did not expect a claude.session.create event for a plain terminal, got %+v", entries)
		}
	}
}
