package claudehook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// This file owns the one genuinely sensitive operation in this package:
// editing the user's REAL, GLOBAL `~/.claude/settings.json` to add or
// remove a SessionStart hook. That file is shared with every other tool
// that registers a Claude Code hook (see the PreToolUse/UserPromptSubmit/
// Stop/etc. entries already present on a typical machine) — this code
// must never overwrite the file wholesale, only ever merge a single,
// clearly-marked entry into its `hooks.SessionStart` array, and always
// back the file up first. Every entry-point here is only ever reached via
// an explicit user action in the settings UI (see internal/api/settings.go)
// — never automatically on server startup.

const hookEventName = "SessionStart"

// ClaudeSettingsPath returns ~/.claude/settings.json — Claude Code's
// user-level (not project-level) settings file, which is where a hook
// needs to live to fire for every session on the machine regardless of
// which worktree/project it's started in.
func ClaudeSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

// HookScriptPath returns the path this package writes its own hook script
// to. Its uniqueness (nothing else on the machine writes here) is what
// makes it safe to use as the marker identifying "our" entry in
// settings.json's hooks.SessionStart array, for both idempotent install
// and precise uninstall.
func HookScriptPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".worktree-studio", "hooks", "session-start.sh"), nil
}

func hookScriptContent(serverBaseURL string) string {
	// Deliberately fire-and-forget: a short curl timeout, errors and
	// non-2xx responses silently discarded, and the script always exits 0
	// — a claude session must never fail or hang at startup because
	// worktree-studio isn't running or is unreachable. cat pipes stdin
	// (the hook's JSON payload) straight through as the request body; this
	// server's own handler (handleClaudeHook) does the actual parsing.
	return fmt.Sprintf(`#!/bin/sh
# Installed by worktree-studio (internal/claudehook) — safe to delete if
# you uninstall the SessionStart hook from Claude Code's settings.
curl -s -m 2 -X POST -H 'Content-Type: application/json' -d @- '%s/api/claude-hook' >/dev/null 2>&1 || true
exit 0
`, serverBaseURL)
}

// IsHookInstalled reports whether our SessionStart entry is already
// present in ~/.claude/settings.json. A missing settings file, or one
// with no `hooks` or no `hooks.SessionStart` key, is a normal "not
// installed" result, not an error.
func IsHookInstalled() (bool, error) {
	scriptPath, err := HookScriptPath()
	if err != nil {
		return false, err
	}
	settings, _, err := readSettings()
	if err != nil {
		return false, err
	}
	return findOurEntry(settings, scriptPath) != -1, nil
}

// InstallHook writes the hook script (pointed at serverBaseURL, e.g.
// "http://localhost:8787") and merges a SessionStart entry referencing it
// into ~/.claude/settings.json. Idempotent: calling it again when already
// installed is a no-op (does not duplicate the entry, does not rewrite
// the script unnecessarily... actually it does refresh the script content,
// see below). Backs up settings.json before any write.
func InstallHook(serverBaseURL string) error {
	scriptPath, err := HookScriptPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		return fmt.Errorf("create hook script dir: %w", err)
	}
	// Always refresh the script content (e.g. if the server's address
	// changed since a previous install) — cheap, and re-installing is the
	// natural "fix it" action a user takes from the settings UI anyway.
	if err := os.WriteFile(scriptPath, []byte(hookScriptContent(serverBaseURL)), 0o755); err != nil {
		return fmt.Errorf("write hook script: %w", err)
	}

	settings, mode, err := readSettings()
	if err != nil {
		return err
	}

	if findOurEntry(settings, scriptPath) != -1 {
		return nil // already installed, script content refreshed above
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	sessionStart, _ := hooks[hookEventName].([]any)
	sessionStart = append(sessionStart, map[string]any{
		"matcher": "",
		"hooks": []any{
			map[string]any{"type": "command", "command": scriptPath},
		},
	})
	hooks[hookEventName] = sessionStart
	settings["hooks"] = hooks

	return writeSettings(settings, mode)
}

// UninstallHook removes our entry from ~/.claude/settings.json's
// hooks.SessionStart array, leaving every other hook (ours or anyone
// else's) untouched. A no-op, not an error, if it was never installed.
// Does not remove the script file itself — harmless to leave behind, and
// simpler than reasoning about whether anything else might reference it.
func UninstallHook() error {
	scriptPath, err := HookScriptPath()
	if err != nil {
		return err
	}
	settings, mode, err := readSettings()
	if err != nil {
		return err
	}

	idx := findOurEntry(settings, scriptPath)
	if idx == -1 {
		return nil
	}

	hooks := settings["hooks"].(map[string]any)
	sessionStart := hooks[hookEventName].([]any)
	sessionStart = append(sessionStart[:idx], sessionStart[idx+1:]...)
	if len(sessionStart) == 0 {
		delete(hooks, hookEventName)
	} else {
		hooks[hookEventName] = sessionStart
	}
	settings["hooks"] = hooks

	return writeSettings(settings, mode)
}

// readSettings loads ~/.claude/settings.json as a generic map, preserving
// every key this package doesn't know or care about (env, permissions,
// model, enabledPlugins, other tools' hooks, ...) so a write-back never
// drops anything. A missing file reads as an empty settings object, not
// an error — Claude Code itself treats no file as "all defaults."
func readSettings() (map[string]any, os.FileMode, error) {
	path, err := ClaudeSettingsPath()
	if err != nil {
		return nil, 0, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, 0o600, nil
		}
		return nil, 0, fmt.Errorf("read %s: %w", path, err)
	}
	info, err := os.Stat(path)
	mode := os.FileMode(0o600)
	if err == nil {
		mode = info.Mode()
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, 0, fmt.Errorf("parse %s: %w (refusing to touch a file that isn't valid JSON)", path, err)
	}
	return settings, mode, nil
}

// writeSettings backs up the current file (if it exists) to
// ~/.worktree-studio/backups/ before overwriting it — the one piece of
// the broader backup/restore TODO (see PLAN.md) implemented so far,
// scoped narrowly to this one sensitive write.
func writeSettings(settings map[string]any, mode os.FileMode) error {
	path, err := ClaudeSettingsPath()
	if err != nil {
		return err
	}
	if err := backupIfExists(path); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	return os.WriteFile(path, data, mode)
}

func backupIfExists(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s for backup: %w", path, err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	backupDir := filepath.Join(home, ".worktree-studio", "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}
	backupPath := filepath.Join(backupDir, fmt.Sprintf("claude-settings-%d.json", time.Now().UnixNano()))
	return os.WriteFile(backupPath, data, 0o600)
}

// findOurEntry returns the index of our hook entry within
// settings["hooks"]["SessionStart"] (a []any of matcher-groups), or -1 if
// not present. Matches on scriptPath appearing as any hook's "command" —
// exact match, not a substring check, so nothing else could accidentally
// collide with it.
func findOurEntry(settings map[string]any, scriptPath string) int {
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return -1
	}
	sessionStart, ok := hooks[hookEventName].([]any)
	if !ok {
		return -1
	}
	for i, group := range sessionStart {
		groupMap, ok := group.(map[string]any)
		if !ok {
			continue
		}
		entries, ok := groupMap["hooks"].([]any)
		if !ok {
			continue
		}
		for _, e := range entries {
			entryMap, ok := e.(map[string]any)
			if !ok {
				continue
			}
			if cmd, _ := entryMap["command"].(string); cmd == scriptPath {
				return i
			}
		}
	}
	return -1
}
