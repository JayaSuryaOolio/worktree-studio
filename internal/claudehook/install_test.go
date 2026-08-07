package claudehook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func withFakeClaudeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func writeRealSettings(t *testing.T, home string, content string) {
	t.Helper()
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readSettingsRaw(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v\n%s", err, data)
	}
	return m
}

func TestInstallHookOnMissingSettingsFile(t *testing.T) {
	home := withFakeClaudeHome(t)

	installed, err := IsHookInstalled()
	if err != nil {
		t.Fatalf("IsHookInstalled (no settings file): %v", err)
	}
	if installed {
		t.Fatal("expected not installed before InstallHook runs")
	}

	if err := InstallHook("http://localhost:8787"); err != nil {
		t.Fatalf("InstallHook: %v", err)
	}

	installed, err = IsHookInstalled()
	if err != nil {
		t.Fatalf("IsHookInstalled: %v", err)
	}
	if !installed {
		t.Fatal("expected installed after InstallHook")
	}

	scriptPath, _ := HookScriptPath()
	if _, err := os.Stat(scriptPath); err != nil {
		t.Errorf("hook script not written: %v", err)
	}

	settings := readSettingsRaw(t, home)
	hooks := settings["hooks"].(map[string]any)
	sessionStart := hooks["SessionStart"].([]any)
	if len(sessionStart) != 1 {
		t.Fatalf("SessionStart entries = %d, want 1", len(sessionStart))
	}
}

func TestInstallHookPreservesExistingUnrelatedSettings(t *testing.T) {
	home := withFakeClaudeHome(t)
	writeRealSettings(t, home, `{
		"model": "sonnet",
		"env": {"FOO": "bar"},
		"hooks": {
			"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "some-other-tool"}]}]
		}
	}`)

	if err := InstallHook("http://localhost:8787"); err != nil {
		t.Fatalf("InstallHook: %v", err)
	}

	settings := readSettingsRaw(t, home)
	if settings["model"] != "sonnet" {
		t.Errorf("model field was clobbered: %+v", settings)
	}
	env := settings["env"].(map[string]any)
	if env["FOO"] != "bar" {
		t.Errorf("env field was clobbered: %+v", settings)
	}
	hooks := settings["hooks"].(map[string]any)
	preToolUse := hooks["PreToolUse"].([]any)
	if len(preToolUse) != 1 {
		t.Fatalf("PreToolUse entries = %d, want 1 (must not be touched)", len(preToolUse))
	}
	sessionStart := hooks["SessionStart"].([]any)
	if len(sessionStart) != 1 {
		t.Fatalf("SessionStart entries = %d, want 1", len(sessionStart))
	}
}

func TestInstallHookIsIdempotent(t *testing.T) {
	withFakeClaudeHome(t)
	if err := InstallHook("http://localhost:8787"); err != nil {
		t.Fatalf("InstallHook (1st): %v", err)
	}
	if err := InstallHook("http://localhost:8787"); err != nil {
		t.Fatalf("InstallHook (2nd): %v", err)
	}

	installed, err := IsHookInstalled()
	if err != nil || !installed {
		t.Fatalf("IsHookInstalled after double-install: installed=%v err=%v", installed, err)
	}

	// Re-reading the raw file to count entries directly (not just
	// IsHookInstalled's boolean) is the part that actually proves no
	// duplicate entry was appended on the second call.
	home := os.Getenv("HOME")
	settings := readSettingsRaw(t, home)
	hooks := settings["hooks"].(map[string]any)
	sessionStart := hooks["SessionStart"].([]any)
	if len(sessionStart) != 1 {
		t.Fatalf("SessionStart entries after double-install = %d, want exactly 1 (idempotency broken)", len(sessionStart))
	}
}

func TestInstallHookBacksUpExistingFile(t *testing.T) {
	home := withFakeClaudeHome(t)
	writeRealSettings(t, home, `{"model": "sonnet"}`)

	if err := InstallHook("http://localhost:8787"); err != nil {
		t.Fatalf("InstallHook: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(home, ".worktree-studio", "backups", "claude-settings-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("backup files = %d, want 1", len(matches))
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	var backedUp map[string]any
	if err := json.Unmarshal(data, &backedUp); err != nil {
		t.Fatalf("backup is not valid JSON: %v", err)
	}
	if backedUp["model"] != "sonnet" {
		t.Errorf("backup content = %+v, want the pre-install settings", backedUp)
	}
}

func TestUninstallHookRemovesOnlyOurEntry(t *testing.T) {
	home := withFakeClaudeHome(t)
	writeRealSettings(t, home, `{
		"hooks": {
			"SessionStart": [{"matcher": "", "hooks": [{"type": "command", "command": "someone-elses-hook"}]}]
		}
	}`)

	if err := InstallHook("http://localhost:8787"); err != nil {
		t.Fatalf("InstallHook: %v", err)
	}
	settings := readSettingsRaw(t, home)
	hooks := settings["hooks"].(map[string]any)
	if len(hooks["SessionStart"].([]any)) != 2 {
		t.Fatalf("expected both entries present after install, got %+v", hooks["SessionStart"])
	}

	if err := UninstallHook(); err != nil {
		t.Fatalf("UninstallHook: %v", err)
	}

	settings = readSettingsRaw(t, home)
	hooks = settings["hooks"].(map[string]any)
	sessionStart := hooks["SessionStart"].([]any)
	if len(sessionStart) != 1 {
		t.Fatalf("SessionStart entries after uninstall = %d, want 1 (only ours removed)", len(sessionStart))
	}
	entry := sessionStart[0].(map[string]any)
	cmd := entry["hooks"].([]any)[0].(map[string]any)["command"].(string)
	if cmd != "someone-elses-hook" {
		t.Errorf("remaining entry command = %q, want the other tool's hook preserved", cmd)
	}

	installed, err := IsHookInstalled()
	if err != nil || installed {
		t.Fatalf("IsHookInstalled after uninstall: installed=%v err=%v, want false", installed, err)
	}
}

func TestUninstallHookNoOpWhenNotInstalled(t *testing.T) {
	withFakeClaudeHome(t)
	if err := UninstallHook(); err != nil {
		t.Fatalf("UninstallHook on a machine where it was never installed: %v", err)
	}
}

func TestReadSettingsRejectsInvalidJSONRatherThanClobbering(t *testing.T) {
	home := withFakeClaudeHome(t)
	writeRealSettings(t, home, `this is not json`)

	err := InstallHook("http://localhost:8787")
	if err == nil {
		t.Fatal("expected InstallHook to refuse to touch a settings.json that isn't valid JSON")
	}

	// Confirm the original (invalid, but real) content is untouched.
	data, readErr := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "this is not json" {
		t.Error("settings.json was modified despite the parse failure")
	}
}
