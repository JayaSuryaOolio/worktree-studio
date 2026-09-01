package api

import (
	"net/http"
	"testing"

	"worktree-studio/internal/term"
)

// TestPruneOrphanTmuxSessionsProtectsRecentActivity is the end-to-end
// version of term's own safeguard test: a live orphan tmux session
// (no terminal_sessions row) that was just created must survive a prune
// call whose min_age_hours asks for a 1-hour safety window.
func TestPruneOrphanTmuxSessionsProtectsRecentActivity(t *testing.T) {
	requireTmuxAPI(t)
	ts, _ := newTestServer(t)

	name := "wts-apiorphantest1"
	if err := term.TmuxCmd("new-session", "-d", "-s", name).Run(); err != nil {
		t.Fatalf("tmux new-session: %v", err)
	}
	t.Cleanup(func() { _ = term.TmuxCmd("kill-session", "-t", name).Run() })

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/orphan-tmux-sessions/prune?min_age_hours=1", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("prune: status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Killed    []string `json:"killed"`
		Protected []string `json:"protected"`
	}
	decodeInto(t, resp, &body)

	for _, k := range body.Killed {
		if k == name {
			t.Fatalf("expected a recently-active orphan to be protected, not killed: %+v", body)
		}
	}
	foundProtected := false
	for _, p := range body.Protected {
		if p == name {
			foundProtected = true
		}
	}
	if !foundProtected {
		t.Fatalf("expected %s in the protected list, got %+v", name, body)
	}

	live, err := term.ListLiveTmuxSessionNames()
	if err != nil {
		t.Fatalf("ListLiveTmuxSessionNames: %v", err)
	}
	if !live[name] {
		t.Fatalf("expected the protected session to still be alive after prune")
	}
}

// TestPruneOrphanTmuxSessionsKillsStaleOrphan confirms a min_age_hours of 0
// (protect nothing) actually kills a genuine orphan, and that a known,
// DB-backed terminal session is left completely alone by the same call.
func TestPruneOrphanTmuxSessionsKillsStaleOrphan(t *testing.T) {
	requireGit(t)
	requireTmuxAPI(t)
	ts, srv := newTestServer(t)
	repoPath := newTestGitRepo(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/repos/", map[string]string{"name": "test", "path": repoPath})
	var repo map[string]any
	decodeInto(t, resp, &repo)
	repoID := repo["id"].(string)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repoID+"/worktrees/", map[string]string{"name": "feature"})
	var wt map[string]any
	decodeInto(t, resp, &wt)
	wtID := wt["id"].(string)

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/repos/"+repoID+"/worktrees/"+wtID+"/terminals/", map[string]string{"tab_label": "shell"})
	var knownTerm struct {
		TmuxSessionName string `json:"tmux_session_name"`
	}
	decodeInto(t, resp, &knownTerm)
	t.Cleanup(func() { _ = term.TmuxCmd("kill-session", "-t", knownTerm.TmuxSessionName).Run() })

	orphanName := "wts-apiorphantest2"
	if err := term.TmuxCmd("new-session", "-d", "-s", orphanName).Run(); err != nil {
		t.Fatalf("tmux new-session: %v", err)
	}
	t.Cleanup(func() { _ = term.TmuxCmd("kill-session", "-t", orphanName).Run() })

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/orphan-tmux-sessions/prune?min_age_hours=0", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("prune: status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Killed    []string `json:"killed"`
		Protected []string `json:"protected"`
	}
	decodeInto(t, resp, &body)

	found := false
	for _, k := range body.Killed {
		if k == orphanName {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected %s to be killed with min_age_hours=0, got %+v", orphanName, body)
	}

	live, err := term.ListLiveTmuxSessionNames()
	if err != nil {
		t.Fatalf("ListLiveTmuxSessionNames: %v", err)
	}
	if live[orphanName] {
		t.Fatalf("expected the stale orphan to be gone after prune")
	}
	if !live[knownTerm.TmuxSessionName] {
		t.Fatalf("expected the known, DB-backed terminal session to be left alone by an orphan sweep")
	}

	sessions, err := srv.Term.ListSessions(wtID)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected the known terminal session's DB row to survive an orphan sweep, got %+v", sessions)
	}
}
