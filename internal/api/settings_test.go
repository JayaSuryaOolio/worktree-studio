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

func TestDependencyStatusReportsClaudeHookAndSkillAsNotInstalled(t *testing.T) {
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

	if status["claude_hook"].Installed {
		t.Error("claude_hook reported installed on a fresh isolated HOME")
	}
	if status["skill"].Installed {
		t.Error("skill reported installed on a fresh isolated HOME")
	}
	if _, ok := status["tmux"]; !ok {
		t.Error("expected a tmux entry regardless of whether it's actually installed on this machine")
	}
}

func TestInstallAndUninstallClaudeHookViaAPI(t *testing.T) {
	ts, _ := newTestServer(t)
	t.Setenv("HOME", t.TempDir())

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/settings/dependencies/claude-hook/install", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("install: status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/settings/dependencies", nil)
	var status map[string]struct {
		Installed bool `json:"installed"`
	}
	decodeInto(t, resp, &status)
	if !status["claude_hook"].Installed {
		t.Fatal("expected claude_hook installed after the install call")
	}

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/settings/dependencies/claude-hook/uninstall", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("uninstall: status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/settings/dependencies", nil)
	decodeInto(t, resp, &status)
	if status["claude_hook"].Installed {
		t.Fatal("expected claude_hook not installed after the uninstall call")
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
