package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"worktree-studio/internal/store"
)

func TestAttentionWSSendsSnapshotOnConnect(t *testing.T) {
	ts, srv := newTestServer(t)
	srv.Attention.SetPending("wt-already-pending", "waiting before anyone connected")

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/attention"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if !strings.Contains(string(msg), `"type":"snapshot"`) {
		t.Fatalf("first message = %s, want a snapshot", msg)
	}
	if !strings.Contains(string(msg), "wt-already-pending") {
		t.Errorf("snapshot = %s, want it to include the already-pending worktree", msg)
	}
}

func TestAttentionWSPushesLiveUpdates(t *testing.T) {
	requireGit(t)
	ts, _ := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo store.Repo
	decodeInto(t, resp, &repo)
	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repo.ID+"/worktrees/", map[string]string{"name": "feature"})
	var wt store.Worktree
	decodeInto(t, resp, &wt)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/attention"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Drain the initial (empty) snapshot before triggering the real event.
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/claude-hook", map[string]string{
		"cwd":             wt.Path,
		"hook_event_name": "Notification",
		"message":         "Claude is waiting for your input",
	})
	resp.Body.Close()

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read update: %v", err)
	}
	body := string(msg)
	if !strings.Contains(body, `"type":"update"`) ||
		!strings.Contains(body, wt.ID) ||
		!strings.Contains(body, `"pending":true`) ||
		!strings.Contains(body, "Claude is waiting for your input") {
		t.Errorf("update message = %s, missing expected fields", body)
	}
}
