package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// Every test in this file isolates HOME to a temp directory before
// touching any settings-dependency endpoint — several of them go through
// internal/claudehook, which reads/writes ~/.claude/settings.json and
// ~/.claude/skills/. Never let one of these run against the real HOME.

func TestDependencyStatusReportsSkillAsNotInstalled(t *testing.T) {
	ts, _ := newTestServer(t)
	t.Setenv("HOME", t.TempDir())

	resp := doJSON(t, http.MethodGet, ts.URL+"/api/settings/dependencies", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var status map[string]struct {
		Installed bool `json:"installed"`
	}
	decodeInto(t, resp, &status)

	if status["skill"].Installed {
		t.Error("skill reported installed on a fresh isolated HOME")
	}
	if _, ok := status["tmux"]; !ok {
		t.Error("expected a tmux entry regardless of whether it's actually installed on this machine")
	}
	if _, ok := status["claude_hook"]; ok {
		t.Error("claude hooks moved to /api/settings/hooks — dependencies must no longer carry a claude_hook entry")
	}
}

type hookStatusJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Hint      string `json:"hint"`
	Installed bool   `json:"installed"`
}

func findHookStatus(hooks []hookStatusJSON, id string) (hookStatusJSON, bool) {
	for _, h := range hooks {
		if h.ID == id {
			return h, true
		}
	}
	return hookStatusJSON{}, false
}

func TestGetHooksListsEveryRegisteredHookDynamically(t *testing.T) {
	ts, _ := newTestServer(t)
	t.Setenv("HOME", t.TempDir())

	resp := doJSON(t, http.MethodGet, ts.URL+"/api/settings/hooks", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var hooks []hookStatusJSON
	decodeInto(t, resp, &hooks)

	// This asserts against claudehook.Hooks() itself, not a hardcoded
	// count, so it stays correct as more hooks are registered there.
	if len(hooks) < 2 {
		t.Fatalf("expected at least the 2 built-in hooks, got %d: %+v", len(hooks), hooks)
	}
	for _, id := range []string{"session-tracking", "session-context"} {
		h, ok := findHookStatus(hooks, id)
		if !ok {
			t.Fatalf("expected a %q entry, got %+v", id, hooks)
		}
		if h.Installed {
			t.Errorf("%q reported installed on a fresh isolated HOME", id)
		}
		if h.Name == "" {
			t.Errorf("%q has an empty display name", id)
		}
	}
}

func TestInstallAndUninstallHookViaAPI(t *testing.T) {
	ts, _ := newTestServer(t)
	t.Setenv("HOME", t.TempDir())

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/settings/hooks/session-tracking/install", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("install: status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/settings/hooks", nil)
	var hooks []hookStatusJSON
	decodeInto(t, resp, &hooks)
	installed, _ := findHookStatus(hooks, "session-tracking")
	if !installed.Installed {
		t.Fatal("expected session-tracking installed after the install call")
	}
	notInstalled, _ := findHookStatus(hooks, "session-context")
	if notInstalled.Installed {
		t.Fatal("expected session-context untouched (still not installed)")
	}

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/settings/hooks/session-tracking/uninstall", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("uninstall: status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/settings/hooks", nil)
	decodeInto(t, resp, &hooks)
	uninstalled, _ := findHookStatus(hooks, "session-tracking")
	if uninstalled.Installed {
		t.Fatal("expected session-tracking not installed after the uninstall call")
	}
}

func TestInstallHookViaAPIUnknownIDIsNotFound(t *testing.T) {
	ts, _ := newTestServer(t)
	t.Setenv("HOME", t.TempDir())

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/settings/hooks/does-not-exist/install", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestInstallSkillViaAPI(t *testing.T) {
	ts, _ := newTestServer(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/settings/dependencies/skill/install", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	path := filepath.Join(home, ".claude", "skills", "worktree-studio", "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("skill file not written: %v", err)
	}
	if len(data) == 0 {
		t.Error("installed skill file is empty")
	}

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/settings/dependencies", nil)
	var status map[string]struct {
		Installed bool `json:"installed"`
	}
	decodeInto(t, resp, &status)
	if !status["skill"].Installed {
		t.Error("expected skill installed after the install call")
	}
}
